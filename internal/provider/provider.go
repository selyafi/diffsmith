package provider

import (
	"context"

	"github.com/selyafi/diffsmith/internal/diff"
)

// Host names the review target's hosting service.
type Host string

const (
	HostGitHub Host = "github.com"
	HostGitLab Host = "gitlab.com"
)

// ReviewTarget identifies the PR/MR being reviewed.
type ReviewTarget struct {
	Host    Host
	URL     string
	Owner   string
	Repo    string
	Number  int
	HeadRef string
	BaseRef string
}

// ReviewInput is the normalized input the review core consumes. Provider
// adapters produce this shape regardless of which CLI fetched the diff.
type ReviewInput struct {
	Target  ReviewTarget
	Title   string
	Author  string
	Files   []*diff.DiffFile
	RawDiff string
}

// Provider fetches review target data for one host family.
//
// Callers must invoke Preflight before Fetch. Preflight verifies the
// runtime environment (required CLI present, authenticated) so the model
// is never invoked when the fetch path is doomed to fail.
type Provider interface {
	Supports(rawURL string) bool
	Preflight(ctx context.Context) error
	Fetch(ctx context.Context, rawURL string) (*ReviewInput, error)
}
