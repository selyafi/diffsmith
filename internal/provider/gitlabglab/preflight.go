package gitlabglab

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/selyafi/diffsmith/internal/provider"
)

// Preflight verifies the runtime environment is ready to fetch GitLab MRs.
//
// Per docs/architecture.md § Pre-Flight Checks, the model is never invoked
// when these fail; the user sees an actionable message instead of a stack
// trace from `os/exec`. Mirrors internal/provider/githubgh.Preflight; the
// only intentional divergence is the auth-failure error string ordering
// (actionable text first, %w wrap at the tail) so the contiguous
// acceptance-required substring is preserved without sacrificing
// errors.Is/Unwrap on the underlying transport error.
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

// Check runs `glab` binary presence and `glab auth status` in order. The
// first failure is surfaced with an actionable message; later checks are
// skipped.
func (p *Preflight) Check(ctx context.Context) error {
	if _, err := p.LookPath("glab"); err != nil {
		return errors.New("glab CLI not found on PATH. Install it from https://gitlab.com/gitlab-org/cli")
	}
	if _, err := p.Run(ctx, nil, "glab", "auth", "status"); err != nil {
		return fmt.Errorf("glab is not authenticated. Run `glab auth login` to authenticate: %w", err)
	}
	return nil
}
