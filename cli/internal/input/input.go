package input

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ReadPipe returns stdin content when the CLI is invoked as
// `cat app.log | dare`, and false when stdin is a terminal
// (nothing piped in).
func ReadPipe() (string, bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return "", false, err
	}
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		return "", false, nil // interactive terminal, no pipe
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", false, err
	}
	return string(data), true, nil
}

// RunWrapped executes an arbitrary command (`dare run "python main.py"`),
// streams its output live to the user, and also captures combined
// stdout+stderr for analysis if it exits non-zero.
func RunWrapped(shellCmd string) (output string, exitCode int, err error) {
	cmd := exec.Command("sh", "-c", shellCmd)

	var buf bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &buf)
	mwErr := io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = mw
	cmd.Stderr = mwErr

	runErr := cmd.Run()
	output = buf.String()

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return output, exitErr.ExitCode(), nil
		}
		return output, -1, fmt.Errorf("failed to execute command: %w", runErr)
	}
	return output, 0, nil
}
