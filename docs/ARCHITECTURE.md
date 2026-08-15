# `dare` — Multi-Language LLM Debugging CLI & SDK

## 1. High-Level Architecture & Data Flow

```
                         ┌─────────────────────────────┐
                         │        User Terminal        │
                         │  cat app.log | dare │
                         │  dare run "python…" │
                         └──────────────┬───────────────┘
                                        │ stdin / subprocess wrap
                                        ▼
                         ┌─────────────────────────────┐
                         │      Core CLI (Go binary)    │
                         │  dare-core            │
                         │                              │
                         │  1. Input Parser             │
                         │  2. Context Enricher          │
                         │  3. Provider Router (below)   │
                         │  4. Renderer (TUI/plain)      │
                         └──────────────┬───────────────┘
                                        │
                         ┌──────────────┴───────────────┐
                         │      Provider Router          │
                         │  (ordered fallback chain)     │
                         └──┬───────┬──────────┬─────────┘
                            │       │          │
                 ┌──────────▼─┐ ┌───▼──────┐ ┌─▼─────────────────┐
                 │ Local Mode │ │ Gateway  │ │ Explicit Provider   │
                 │ (default)  │ │ Proxy    │ │ (user-configured)   │
                 │            │ │ Mode     │ │ OpenRouter/Groq/etc │
                 │ Ollama     │ │          │ │ (BYO key, optional) │
                 │ llama.cpp  │ │ Keyless, │ │                     │
                 │ LocalAI    │ │ server-  │ │                     │
                 │            │ │ side key │ │                     │
                 └────────────┘ └──────────┘ └─────────────────────┘
```

### Routing decision (per request)
1. **Local Execution Mode (default).** On startup the core probes fixed local
   ports/sockets in parallel with a short timeout (300–500ms each):
   - `http://127.0.0.1:11434/api/tags` (Ollama)
   - `http://127.0.0.1:8080/health` (llama.cpp server)
   - `http://127.0.0.1:8081/readyz` (LocalAI)
   Whichever responds first is cached for the session (`~/.cache/dare/providers.json`,
   TTL 10 min) so repeat calls skip re-probing.
2. **Gateway Proxy Mode (fallback).** If no local backend is reachable, the
   CLI calls a fixed public endpoint (e.g. `https://gw.dare.dev/v1/chat`)
   that Anthropic-style-wraps a free-tier model. The gateway holds the real
   provider key server-side — the client binary never embeds or requests one.
   This is what makes the tool "keyless by default."
3. **Explicit Provider (opt-in).** If the user sets `DARE_PROVIDER`
   and a key (`OPENROUTER_API_KEY`, `GROQ_API_KEY`, …) in env or
   `~/.config/dare/config.toml`, that provider is tried first and
   local/gateway become the fallback chain if it errors or rate-limits.

### Failover chain
Each provider implements a common `Provider` interface (`Complete(ctx, Request) (Response, error)`).
The router walks an ordered list and moves to the next entry on: network
error, timeout (configurable, default 12s), non-2xx, or empty completion.
Every attempt is logged at `--verbose` level so failures are visible, not
silent.

### Why a compiled core + thin SDKs
The heavy lifting (probing, HTTP, context capture, prompt templating,
fallback logic) lives once in the Go binary. Every language SDK is a thin
shim that either (a) shells out to the installed `dare` binary, or
(b) talks to it over a local Unix socket/named pipe if it's already running
as a daemon (`dare daemon`). This avoids re-implementing provider
logic in five languages and keeps per-language packages tiny (a few hundred
lines).

### Keyless privacy note
Gateway Proxy Mode necessarily sends log/error content to a third-party
server. This is disclosed on first run (`dare` prints a one-time
consent notice) and is fully bypassed once a local model is detected or a
BYO key is configured. `--offline` forces local-only and errors instead of
falling back to the gateway.

---

## 2. Monorepo Directory Structure

```
dare/
├── cli/                          # Core compiled binary (Go)
│   ├── cmd/
│   │   └── dare/
│   │       └── main.go
│   ├── internal/
│   │   ├── input/                # stdin/pipe + `run` wrapper parsing
│   │   │   ├── pipe.go
│   │   │   └── wrap.go
│   │   ├── context/               # OS, runtime, code-snippet enrichment
│   │   │   └── capture.go
│   │   ├── provider/               # Pluggable backend implementations
│   │   │   ├── provider.go        # interface + registry
│   │   │   ├── ollama.go
│   │   │   ├── llamacpp.go
│   │   │   ├── localai.go
│   │   │   ├── gateway.go
│   │   │   ├── openrouter.go
│   │   │   └── groq.go
│   │   ├── router/
│   │   │   └── router.go          # discovery + fallback chain
│   │   ├── config/
│   │   │   └── config.go          # ~/.config/dare/config.toml
│   │   └── render/
│   │       └── render.go          # terminal output formatting
│   ├── go.mod
│   └── go.sum
│
├── daemon/                       # optional background process
│   └── socket_server.go          # unix socket for SDK IPC, avoids re-exec cost
│
├── gateway/                       # Server-side keyless proxy (deployed separately)
│   ├── main.py / main.go          # thin reverse-proxy w/ rate limiting per IP
│   ├── providers.yaml             # server-held keys, free-tier model routing
│   └── Dockerfile
│
├── sdks/
│   ├── python/
│   │   ├── dare_sdk/
│   │   │   ├── __init__.py
│   │   │   ├── hook.py            # sys.excepthook integration
│   │   │   └── client.py          # talks to core binary/daemon
│   │   ├── pyproject.toml
│   │   └── README.md
│   ├── node/
│   │   ├── src/
│   │   │   ├── index.ts
│   │   │   └── hook.ts            # process.on('uncaughtException')
│   │   ├── package.json
│   │   └── README.md
│   └── go/
│       ├── dare.go        # recover() middleware helper
│       └── go.mod
│
├── docs/
│   ├── ARCHITECTURE.md
│   └── PROVIDERS.md
│
├── scripts/
│   └── install.sh                 # curl | sh installer, downloads binary per-OS
│
└── README.md
```

---

## 3. CLI & SDK Interface Specs

### CLI commands

```bash
# Pipe mode — analyze arbitrary log/stderr content
cat app.log | dare

# Command-wrap mode — run a process, capture its stderr/exit code, analyze on failure
dare run "python main.py"
dare run -- node server.js

# Direct file analysis
dare analyze --file ./crash.log --lang python

# Git-diff aware analysis ("why did this change break the tests")
dare diff HEAD~1 --test-cmd "pytest -x"

# Force a specific provider / go fully offline
dare run "go test ./..." --provider ollama
dare run "go test ./..." --offline

# Background daemon (keeps model warm, serves SDKs over unix socket)
dare daemon start
```

Sample plain-text output:

```
✖ Exception detected: ZeroDivisionError (main.py:42)

  Context: division by zero in calculate_average()
  Likely cause: `total / len(items)` when items is empty

  Suggested fix:
    if not items:
        return 0
    return total / len(items)

  Provider: ollama/qwen2.5-coder:7b (local, 340ms)
```

### Python SDK

```python
import dare_sdk

# Option A: auto-hook uncaught exceptions for the whole process
dare_sdk.install()

# Option B: explicit capture
try:
    risky_operation()
except Exception as e:
    diagnosis = dare_sdk.diagnose(e)
    print(diagnosis.summary)
    print(diagnosis.suggested_fix)
```

### Node.js / TypeScript SDK

```typescript
import { install, diagnose } from "@dare/sdk";

// auto-hook
install();

// explicit
try {
  riskyOperation();
} catch (err) {
  const diagnosis = await diagnose(err);
  console.log(diagnosis.summary, diagnosis.suggestedFix);
}
```

### Go SDK

```go
import "github.com/baalebos-cloud/dare/sdks/go"

defer dare.Recover() // wraps recover(), diagnoses panics before re-panicking
```
