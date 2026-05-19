// Package model defines the Model interface and prompt/finding transport
// types shared by all model-CLI adapters.
//
// The model layer never validates findings against the diff — that's the
// review package's job. The model layer's contract is "produce a parsed,
// well-typed transport object from a CLI invocation."
package model

import (
	"context"

	"github.com/selyafi/diffsmith/internal/provider"
)

// FindingCandidate is the transport-layer shape of a finding returned by
// a model CLI. Severity is kept as a string here; the review package
// validates and converts to a typed Severity.
//
// Field ordering and JSON tags mirror docs/review-finding-schema.md
// exactly so the same struct can be used for both encoding the schema
// example in the prompt and decoding the model's reply.
type FindingCandidate struct {
	File             string  `json:"file"`
	Line             int     `json:"line"`
	Severity         string  `json:"severity"`
	Title            string  `json:"title"`
	Evidence         string  `json:"evidence"`
	SuggestedComment string  `json:"suggested_comment"`
	FixHint          string  `json:"fix_hint"`
	Confidence       float64 `json:"confidence"`
}

// ModelReviewResult is what a model adapter returns after invocation.
// RawOutput preserves the model's stdout so the TUI's debug surface can
// show it when validation rejects everything.
type ModelReviewResult struct {
	Model     string
	Findings  []FindingCandidate
	RawOutput string
}

// Model adapters produce normalized review findings for one CLI family.
//
// Callers must invoke Preflight before Review so the user sees an
// actionable error if the CLI is missing, rather than a stack trace
// from os/exec.
type Model interface {
	Name() string
	Preflight(ctx context.Context) error
	Review(ctx context.Context, input *provider.ReviewInput) (*ModelReviewResult, error)
}
