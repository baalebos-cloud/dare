// Package dare is the Go SDK for the dare LLM debugging CLI. It shells
// out to the core `dare` binary — no provider/routing logic is duplicated
// here, matching the Python and Node SDKs.
package dare

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"time"
)

// Diagnosis is the structured result returned by the core binary.
type Diagnosis struct {
	Summary      string
	SuggestedFix string
	Provider     string
	Raw          string
}

// ErrCoreNotFound is returned when the `dare` binary isn't on PATH.
var ErrCoreNotFound = errors.New(
	"the `dare` core binary was not found on PATH. " +
		"Install it: curl -fsSL https://dare.dev/install.sh | sh",
)

type payload struct {
	Language       string `json:"language"`
	RuntimeVersion string `json:"runtime_version"`
	OS             string `json:"os"`
	Traceback      string `json:"traceback"`
}

type coreResponse struct {
	Summary      string `json:"summary"`
	SuggestedFix string `json:"suggested_fix"`
	Provider     string `json:"provider"`
}

// Diagnose sends an error (with a stack trace, if available) to the core
// binary and returns a structured diagnosis.
func Diagnose(err error, stack []byte, timeout time.Duration) (Diagnosis, error) {
	binary, lookErr := exec.LookPath("dare")
	if lookErr != nil {
		return Diagnosis{}, ErrCoreNotFound
	}

	tb := err.Error()
	if len(stack) > 0 {
		tb = fmt.Sprintf("%s\n%s", err.Error(), string(stack))
	}

	body, _ := json.Marshal(payload{
		Language:       "go",
		RuntimeVersion: runtime.Version(),
		OS:             runtime.GOOS,
		Traceback:      tb,
	})

	cmd := exec.Command(binary, "--json")
	cmd.Stdin = bytes.NewReader(body)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()

	select {
	case runErr := <-done:
		// Non-zero exit isn't necessarily fatal — the core exits 2 when no
		// provider was reachable but may still have written diagnostic text.
		if runErr != nil && stdout.Len() == 0 {
			return Diagnosis{}, fmt.Errorf("dare core exited unexpectedly: %v: %s", runErr, stderr.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return Diagnosis{}, fmt.Errorf("dare core timed out after %s", timeout)
	}

	raw := stdout.String()
	var resp coreResponse
	if jsonErr := json.Unmarshal(stdout.Bytes(), &resp); jsonErr != nil {
		// Core printed human-readable text (no --json support yet in this
		// PoC) — surface it directly rather than erroring.
		return Diagnosis{Summary: raw, Provider: "unknown", Raw: raw}, nil
	}

	return Diagnosis{
		Summary:      resp.Summary,
		SuggestedFix: resp.SuggestedFix,
		Provider:     resp.Provider,
		Raw:          raw,
	}, nil
}

// Recover is meant for `defer dare.Recover()` at the top of main() or a
// goroutine. On panic it diagnoses the recovered value (with the current
// stack trace), prints the diagnosis, then re-panics so normal crash
// behavior (non-zero exit, crash reporters, etc.) is preserved.
func Recover() {
	r := recover()
	if r == nil {
		return
	}

	var err error
	switch v := r.(type) {
	case error:
		err = v
	default:
		err = fmt.Errorf("%v", v)
	}

	stack := debug.Stack()
	diag, diagErr := Diagnose(err, stack, 30*time.Second)
	if diagErr != nil {
		if errors.Is(diagErr, ErrCoreNotFound) {
			fmt.Fprintf(os.Stderr, "\n(dare: %v)\n", diagErr)
		}
		// Don't let the diagnosis path itself swallow the panic.
		panic(r)
	}

	fmt.Fprintf(os.Stderr, "\n✖ dare diagnosis (via %s):\n  %s\n", diag.Provider, diag.Summary)
	if diag.SuggestedFix != "" {
		fmt.Fprintf(os.Stderr, "\n  Suggested fix:\n  %s\n", diag.SuggestedFix)
	}

	panic(r) // preserve original panic/crash behavior
}
