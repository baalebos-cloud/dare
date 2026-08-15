package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tctx "github.com/baalebos-cloud/dare/cli/internal/context"
	"github.com/baalebos-cloud/dare/cli/internal/config"
	"github.com/baalebos-cloud/dare/cli/internal/input"
	"github.com/baalebos-cloud/dare/cli/internal/provider"
	"github.com/baalebos-cloud/dare/cli/internal/router"
)

// systemPrompt is deliberately language-agnostic: it never names a fixed
// set of languages. The model is asked to infer the language itself from
// the log/traceback/snippet — this is what lets the tool handle any
// programming language, including ones not in extToLanguage's hint table,
// without a code change.
const systemPrompt = `You are an expert debugging assistant embedded in a CLI tool.
You support any programming language — infer the language from the log,
traceback, file extension, or code snippet provided; do not assume Python
or any specific language unless the evidence says so.
Given a raw log/traceback, OS/runtime context, and an optional code snippet,
identify the language (state it), the root cause, and propose a minimal,
concrete fix. Be terse. Format: language, one-line cause, then a short fix
(code if applicable).`

func main() {
	rawArgs := os.Args[1:]

	// Recognized flags can appear anywhere (before or after the
	// subcommand) — e.g. both `dare --offline run "cmd"` and
	// `dare run "cmd" --offline` work. Positional args (the subcommand and
	// its own arguments) keep their relative order.
	flags, args := extractFlags(rawArgs)

	cfg := config.Load()
	if flags["--offline"] {
		cfg.Offline = true
	}

	rt := router.New(cfg, flags["--verbose"])

	// `dare run "<cmd>"` — wrap a command, capture failure, analyze
	if len(args) >= 2 && args[0] == "run" {
		shellCmd := strings.Join(args[1:], " ")
		output, exitCode, err := input.RunWrapped(shellCmd)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dare: failed to run command:", err)
			os.Exit(1)
		}
		if exitCode == 0 {
			os.Exit(0) // command succeeded — nothing to diagnose
		}
		analyze(rt, output)
		os.Exit(exitCode)
	}

	// `cat app.log | dare` — pipe mode
	piped, ok, err := input.ReadPipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "dare: failed to read stdin:", err)
		os.Exit(1)
	}
	if ok {
		analyze(rt, piped)
		return
	}

	fmt.Println("Usage:")
	fmt.Println(`  cat app.log | dare`)
	fmt.Println(`  dare run "python main.py"`)
	fmt.Println(`  Flags: --offline (skip the gateway) --verbose (log provider selection)`)
}

func analyze(rt *router.Router, rawLog string) {
	snap := tctx.Capture(rawLog)

	langHint := snap.Language
	if langHint == "" {
		langHint = "unknown — infer from the log/snippet"
	}

	userPrompt := fmt.Sprintf(
		"OS: %s/%s\nCWD: %s\nLanguage hint: %s\n\nLog/stderr:\n%s\n\nCode context:\n%s",
		snap.OS, snap.Arch, snap.CWD, langHint, snap.RawLog, snap.CodeSnippet,
	)

	ctx := context.Background()
	resp, err := rt.Complete(ctx, provider.Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "✖ dare: no provider could complete the request:", err)
		fmt.Fprintln(os.Stderr, "  Try: `ollama serve` locally, or check your network for gateway access.")
		os.Exit(2)
	}

	fmt.Printf("\n✖ Diagnosis (via %s/%s, %dms)\n\n%s\n", resp.ProviderID, resp.Model, resp.LatencyMS, resp.Text)
}

// recognizedFlags are boolean flags that can appear anywhere in argv.
var recognizedFlags = map[string]bool{
	"--offline": true,
	"--verbose": true,
	"--json":    true, // parsed but not yet implemented — see README
}

// extractFlags splits argv into recognized boolean flags (order-independent)
// and the remaining positional args (order-preserved). This is what lets
// `--offline` work whether it's typed before or after `run "<cmd>"`.
func extractFlags(args []string) (map[string]bool, []string) {
	found := map[string]bool{}
	positional := make([]string, 0, len(args))
	for _, a := range args {
		if recognizedFlags[a] {
			found[a] = true
			continue
		}
		positional = append(positional, a)
	}
	return found, positional
}
