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

// Model adapters produce normalized review findings for one CLI family.
//
// Callers must invoke Preflight before Review so the user sees an
// actionable error if the CLI is missing, rather than a stack trace
// from os/exec.
type Model interface {
	Name() string
	Preflight(ctx context.Context) error
	Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error)

	// Synthesize takes the diff input plus per-model review results
	// from other (or all) selected models and re-emits a unified
	// []Finding set in this model's voice. Used by the multi-model
	// flow when N≥2 selected models successfully produced findings.
	Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error)
}
