package antigravitycli

import (
	"context"
	"errors"
	"os/exec"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// Adapter implements model.Model against the Antigravity CLI (`agy`).
//
// Per spike S8b (see doc.go) the adapter is a Preflight stub in v1: the
// `agy` CLI cannot authenticate non-interactively, so Review never reaches
// the runner. The struct still exposes the constructor signature shared
// by the other adapters so it can sit in defaultModels() and surface an
// actionable error when a user picks `--model antigravity`.
type Adapter struct {
	lookPath func(name string) (string, error)
}

// New constructs an Adapter. The provider.Runner argument is accepted
// for uniformity with the codex and claude adapters but unused in v1:
// the adapter is gated behind a Preflight error per S8b.
func New(_ provider.Runner) *Adapter {
	return &Adapter{lookPath: exec.LookPath}
}

// Name returns the model identifier surfaced to users via --model.
func (a *Adapter) Name() string { return "antigravity" }

// Preflight always returns an error in v1. If `agy` is missing from PATH
// the error explains how to install it; if it is present the error
// explains the experimental gate (interactive-only OAuth per S8b).
func (a *Adapter) Preflight(_ context.Context) error {
	if _, err := a.lookPath("agy"); err != nil {
		return errors.New("agy (Antigravity CLI) not found on PATH. The antigravity adapter is experimental in v1; install agy or select --model codex or --model claude")
	}
	return errors.New("antigravity adapter is experimental in v1: agy requires interactive browser OAuth on every invocation with no persistent-token path, so it cannot run as a non-interactive review backend. Select --model codex or --model claude")
}

// Review delegates to Preflight in v1. The runner is never invoked.
func (a *Adapter) Review(ctx context.Context, _ *review.ReviewInput) (*review.ModelReviewResult, error) {
	if err := a.Preflight(ctx); err != nil {
		return nil, err
	}
	return nil, errors.New("antigravity adapter is not implemented; preflight should have rejected this call")
}
