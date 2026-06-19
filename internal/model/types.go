// Package model defines the Model interface and prompt/parse helpers
// shared by all model-CLI adapters. Transport types (FindingCandidate,
// ModelReviewResult, ReviewInput) live in internal/review so that
// review (the domain leaf) does not depend on this package.
//
// The model layer never validates findings against the diff — that's the
// review package's job. The model layer's contract is "produce a parsed,
// well-typed transport object from a CLI invocation."
package model

import (
	"context"

	"github.com/selyafi/diffsmith/internal/review"
)

// Reviewer is the base capability every model-CLI adapter implements:
// preflight checks, then a single Review call against one diff. Each
// adapter pairs with exactly one CLI family (codex, claude,
// antigravity).
//
// Callers must invoke Preflight before Review so the user sees an
// actionable error if the CLI is missing, rather than a stack trace
// from os/exec.
//
// Multi-model synthesis is an OPTIONAL capability layered on top of
// Reviewer; see [Synthesizer]. Splitting the two means a future
// review-only adapter would not need to carry a fake Synthesize method
// that only exists to satisfy a composite interface. (All current
// adapters — codex, claude, antigravity — are full peers.)
type Reviewer interface {
	Name() string
	Preflight(ctx context.Context) error
	Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error)
}

// Synthesizer is the optional capability: take the diff input plus
// per-model review results from N≥2 selected reviewers and re-emit a
// unified findings set in this model's voice. Used by the multi-model
// flow when at least two selected models successfully produced
// findings.
//
// The synthesis call site (app/review.go) type-asserts the lead
// candidate against this interface and skips lead candidates that
// don't satisfy it — there is no fallback to a stub. A model that
// cannot synthesize is simply not eligible to lead synthesis.
type Synthesizer interface {
	Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error)
}

// Model is an alias for Reviewer kept so existing call sites that
// reference model.Model continue to compile. New code should prefer
// the explicit capability names (Reviewer for the base, Synthesizer
// for the optional synthesis path).
type Model = Reviewer

// InputBudgetSetter is an optional capability for adapters that cap the
// prompt size sent to their backing CLI. The app layer type-asserts to
// this interface and applies a user-supplied --input-budget value
// before Review runs. An adapter that didn't enforce a budget simply
// wouldn't implement it and would be skipped silently by the override
// loop. (All current adapters — codex, claude, antigravity — implement
// it.)
//
// Implementations must treat n <= 0 as a no-op so a missing/zeroed flag
// can't accidentally turn the budget off and let an arbitrarily large
// prompt through.
type InputBudgetSetter interface {
	SetInputBudget(bytes int)
}
