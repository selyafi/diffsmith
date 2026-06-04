package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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

// DefaultRunner runs the command via os/exec and returns stdout. The
// child inherits diffsmith's working directory. Use this for provider
// CLIs (gh/glab) that operate on explicit URLs and for any caller that
// genuinely needs the real cwd.
func DefaultRunner(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	return runCmd(ctx, "", stdin, name, args...)
}

// IsolatedRunner returns a Runner that executes each command in a fresh,
// empty temp directory (removed once the command returns). It exists for
// the model adapters (diffsmith-4tz): reviewer CLIs like codex/gemini/
// claude autoload project context from their working directory —
// codex discovers .agents/skills/*/SKILL.md and may *activate* a project
// skill (e.g. one whose workflow posts review comments), gemini/claude
// onboard from AGENTS.md / CLAUDE.md. Running them in a neutral temp dir
// neutralizes that autoload, which protects diffsmith's no-auto-post
// guarantee and keeps reviews deterministic. The whole diff is piped via
// stdin, so reviewers need no access to the caller's cwd.
//
// Only the working directory is isolated; user-level config and auth
// (~/.codex, ~/.gemini, ~/.claude) live under $HOME and are found via the
// environment, not cwd, so they keep working.
func IsolatedRunner() Runner {
	return func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		dir, err := os.MkdirTemp("", "diffsmith-iso-*")
		if err != nil {
			return nil, fmt.Errorf("create isolation dir: %w", err)
		}
		defer func() { _ = os.RemoveAll(dir) }()
		return runCmd(ctx, dir, stdin, name, args...)
	}
}

// runCmd is the shared exec body. When workDir is non-empty the child runs
// there; otherwise it inherits the caller's cwd. On a non-zero exit it
// returns an error including the exit code and a trimmed copy of stderr.
func runCmd(ctx context.Context, workDir string, stdin io.Reader, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = workDir
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
