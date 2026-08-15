# dare

Open-source, multi-language LLM-powered debugging CLI + SDKs.

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full design:
keyless routing, local-first fallback chain, monorepo layout, and API specs.

## Install

**Today (from source — no release published yet):**

```bash
git clone https://github.com/baalebos-cloud/dare.git
cd dare/cli
go build -o dare ./cmd/dare
sudo mv dare /usr/local/bin/    # or any dir on your PATH

dare --help
```

Or without cloning, straight from a Go toolchain (once the module path is a
real public repo):

```bash
go install github.com/baalebos-cloud/dare/cli/cmd/dare@latest
```

**Once you publish binary releases** (see `.github/workflows/release.yml` —
push a `v*` tag and it cross-compiles for linux/darwin/windows × amd64/arm64
and attaches the archives to a GitHub Release), anyone can install with:

```bash
curl -fsSL https://dare.dev/install.sh | sh
```

`scripts/install.sh` implements this: detects OS/arch, pulls the matching
asset from the latest GitHub Release, installs to `~/.local/bin`, and warns
if that directory isn't on `PATH`. No API key prompt — it's keyless by
design (see Architecture, §1).

**Python SDK**, once the core binary is on PATH:

```bash
pip install dare-sdk    # once published to PyPI
# or locally: pip install -e sdks/python
```

```python
import dare_sdk
dare_sdk.install()  # auto-diagnoses uncaught exceptions
```

**Node.js SDK**:

```bash
npm install @dare/sdk    # once published to npm
# or locally: cd sdks/node && npm install && npm run build
```

```typescript
import { install } from "@dare/sdk";
install(); // auto-diagnoses uncaughtException / unhandledRejection
```

**Go SDK**:

```bash
go get github.com/baalebos-cloud/dare/sdks/go
```

```go
import "github.com/baalebos-cloud/dare/sdks/go"

func main() {
    defer dare.Recover() // diagnoses a panic, then re-panics
    // ...
}
```

## Self-hosted gateway (n8n via Docker)

The gateway is meant to be swappable — a self-hosted n8n instance on Docker
is a solid fit for a long-running project since it has no trial expiry and
you own the webhook.

```bash
cd gateway
cp .env.example .env        # set a strong N8N_BASIC_AUTH_PASSWORD
docker compose up -d
```

Open `http://localhost:5678`, log in with the credentials from `.env`, and
import `gateway/n8n-workflow.json` (Menu → Import from File). It wires:
**Webhook → HTTP Request (Groq, free tier) → Respond to Webhook**. Add your
Groq API key as an n8n credential on the "Call Groq" node — the key lives
inside n8n, never in the CLI binary, which is what keeps the client keyless.
Swap the HTTP Request node for any other provider (OpenRouter, your own
model server, etc.) without touching the CLI at all.

> Not run/tested against a live Docker daemon in this environment — the
> compose file is valid YAML and the workflow is valid JSON, but review
> both before deploying, especially the n8n node parameter names (they can
> shift between n8n versions).

Point the CLI at your webhook instead of the (placeholder) default gateway:

```bash
export DARE_GATEWAY_ENDPOINT="http://your-host:5678/webhook/dare"
dare run "python main.py"
```

or persist it in `~/.config/dare/config.toml`:

```toml
gateway_endpoint = "http://your-host:5678/webhook/dare"
```

For production, put a reverse proxy (Caddy/nginx/Traefik) in front of n8n
for HTTPS and a stable domain — running the bare webhook on an open port is
fine for testing, not for anything public-facing long-term.

## Language support

The tool doesn't hardcode a language list. `context/capture.go` matches
`path.ext:line` patterns generically (Python's `File "x.py", line N` style,
plus the generic `file.ext:N` / `file.ext(N)` style most other toolchains
use) and only uses the extension as a *hint* passed to the model — the
system prompt explicitly tells the LLM to infer the language rather than
assume one. An unrecognized extension or log format still gets sent for
diagnosis; it just won't have a named-language hint attached.

## What's implemented in this PoC

- `cli/` — Go core: provider interface, Ollama auto-discovery, keyless
  gateway fallback, ordered router with failover, pipe input, `run` command
  wrapping (flags work in any position — `dare --offline run "..."` and
  `dare run "..." --offline` are equivalent), language-agnostic traceback
  capture, and a dependency-free config loader
  (`~/.config/dare/config.toml` + env vars). Compiled and smoke-tested with
  a real Go 1.22 toolchain — pipe mode, `run` wrapping (success and failure
  paths), and the no-provider-available path all verified end-to-end.
- `sdks/python/` — working Python SDK: `diagnose(exc)` and `install()`,
  verified against a mocked core binary.
- `sdks/node/` — working TypeScript SDK: `diagnose(err)` and `install()`,
  compiled with `tsc` (zero errors) and run end-to-end against a fake
  `dare` binary on `PATH`, including the missing-binary error path.
- `sdks/go/` — working Go SDK: `dare.Recover()` for `defer`-based panic
  diagnosis. Compiled and vetted with `go build`/`go vet`, not yet run
  against a live core binary (same `--json` gap as the other SDKs — see
  below).
- `gateway/` — self-hosted n8n via Docker Compose + an importable workflow
  (Webhook → LLM call → Respond) so the gateway has no trial expiry.

## What's stubbed / next steps

- `cli` **parses `--json` as a recognized flag but doesn't implement JSON
  output yet** — all three SDKs send `--json` and fall back to treating
  stdout as plain text when it doesn't parse, so they degrade gracefully,
  but none get structured output today. This is the top priority next
  step: add a JSON-mode renderer and wire it into `main.go`'s `analyze()`.
- `llama.cpp` / `LocalAI` providers follow the exact same pattern as
  `ollama.go` but aren't implemented yet — same `Provider` interface.
- `gateway/` has a self-hosted n8n Docker setup + starter workflow — not
  yet deployed/tested against a live instance, review the n8n node
  parameters before importing (they can drift between n8n versions).
- `dare diff` (git-diff-aware analysis) is spec'd in the CLI interface but
  not implemented.
- No automated test suite yet (verification so far has been manual
  build/run/vet passes) — worth adding `go test` coverage for the router's
  fallback logic and `context.Capture`'s file:line parsing before this
  grows further.

## Layout

```
dare/
├── cli/            # Go core binary — compiled & smoke-tested
├── gateway/         # self-hosted n8n (Docker) — compose + starter workflow
├── sdks/
│   ├── python/      # working, verified
│   ├── node/         # working, verified (tsc + runtime test)
│   └── go/           # working, compiled + vetted
└── docs/
```
