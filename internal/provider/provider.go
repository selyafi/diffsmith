package provider

import (
	"context"

	"github.com/selyafi/diffsmith/internal/review"
)

// Provider fetches review target data for one host family.
//
// Callers must invoke Preflight before Fetch. Preflight verifies the
// runtime environment (required CLI present, authenticated) so the model
// is never invoked when the fetch path is doomed to fail.
type Provider interface {
	Supports(rawURL string) bool
	Preflight(ctx context.Context) error
	Fetch(ctx context.Context, rawURL string) (*review.ReviewInput, error)
}
