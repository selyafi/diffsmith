package provider

import (
	"context"
	"time"

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

	// PreflightList is a stricter preflight required for List(). The
	// public-URL Fetch flow doesn't need auth, but listing typically
	// does. Implementations should verify `gh auth status` / `glab auth
	// status` (or equivalent) succeeds and return an actionable error
	// otherwise.
	PreflightList(ctx context.Context) error

	// List enumerates open PRs/MRs for the given repo. Returns at most
	// 30 results in v1 (no pagination).
	List(ctx context.Context, repo RepoCoord) ([]PRSummary, error)
}

// RepoCoord identifies a repository hosted on a Git forge. Used by the
// inbox flow to enumerate PRs/MRs for a specific repo without first
// constructing a URL.
type RepoCoord struct {
	Host  string // "github.com", "gitlab.com", "gitlab.example.com"
	Owner string // "selyafi" or "my-group/sub-group" (GitLab nested)
	Name  string // "diffsmith"
}

// PRSummary is one row in the inbox list. URL is the only field the
// review pipeline strictly needs; the rest are display-only.
type PRSummary struct {
	Number    int
	Title     string
	Author    string
	URL       string // canonical, ready to hand to runReview
	UpdatedAt time.Time
	Draft     bool

	// Enrichment (display-only; populated by List, zero when not enriched).
	CommentCount      int      // conversation + inline comments, incl. bots
	ResolvedThreads   int      // ✔ review threads / resolvable discussions
	UnresolvedThreads int      // ✖
	HumanCommenters   []string // bot-filtered, deduped, excludes the PR author
	Enriched          bool     // false if enrichment failed for this row
}
