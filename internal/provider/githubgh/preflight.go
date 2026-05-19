package githubgh

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/selyafi/diffsmith/internal/provider"
)

// Preflight verifies the runtime environment is ready to fetch GitHub PRs.
//
// Per docs/architecture.md § Pre-Flight Checks, the model is never invoked
// when these fail; the user sees an actionable message instead of a stack
// trace from `os/exec`.
type Preflight struct {
	Run      provider.Runner
	LookPath func(name string) (string, error)
}

// NewPreflight builds a Preflight with sensible defaults. Tests inject
// fakes; production code calls NewPreflight(nil, nil).
func NewPreflight(run provider.Runner, lookPath func(string) (string, error)) *Preflight {
	if run == nil {
		run = provider.DefaultRunner
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &Preflight{Run: run, LookPath: lookPath}
}

// Check runs `gh` binary presence and `gh auth status` in order. The first
// failure is surfaced with an actionable message; later checks are skipped.
func (p *Preflight) Check(ctx context.Context) error {
	if _, err := p.LookPath("gh"); err != nil {
		return errors.New("gh CLI not found on PATH. Install it from https://cli.github.com/")
	}
	if _, err := p.Run(ctx, nil, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not authenticated: %w. Run `gh auth login` to authenticate", err)
	}
	return nil
}
