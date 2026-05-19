package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Runner executes an external command and returns its stdout.
//
// Modeled as a function type so tests inject canned responses without
// implementing an interface. Real implementations should use argv form
// (variadic args) — passing user-controlled values through `sh -c` is
// forbidden by docs/architecture.md § Process Execution.
//
// Pass stdin as nil when the command takes no input. Adapters that pipe
// data to the child (e.g. codex via ADR 0007) provide a Reader here.
type Runner func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

// DefaultRunner runs the command via os/exec and returns stdout. On a
// non-zero exit it returns an error including the exit code and a trimmed
// copy of stderr.
func DefaultRunner(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				return nil, fmt.Errorf("%s: exit %d", name, exitErr.ExitCode())
			}
			return nil, fmt.Errorf("%s: exit %d: %s", name, exitErr.ExitCode(), msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return out, nil
}
