package review

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
//
// HeadSHA is the commit OID at the time the diff was fetched. It is the
// capture-time anchor used when posting review comments back upstream —
// re-resolving at post time would risk silently re-anchoring to a moved
// HEAD if the PR got pushed mid-review.
//
// BaseSHA and StartSHA are the GitLab diff-refs required to post inline
// review threads (positioned at file:line in the diff view). GitHub uses
// HeadSHA alone; for GitLab we need all three. Empty for GitHub targets.
type ReviewTarget struct {
	Host     Host
	URL      string
	Owner    string
	Repo     string
	Number   int
	HeadRef  string
	HeadSHA  string
	BaseRef  string
	BaseSHA  string
	StartSHA string
}

// ReviewInput is the normalized input the review core consumes. Provider
// adapters produce this shape regardless of which CLI fetched the diff.
type ReviewInput struct {
	Target      ReviewTarget
	Title       string
	Author      string
	Description string // PR/MR body, verbatim (may be ""); populated by Fetch
	// AcceptanceCriteria holds the same-host issues this PR/MR formally
	// closes, resolved via LinkedIssueFetcher. Empty is the normal "no
	// linked issues" state, not an error.
	AcceptanceCriteria []IssueContext
	Files              []*diff.DiffFile
	RawDiff            string
}

// IssueContext is one same-host issue a PR/MR closes. Body may be "" when
// the issue exists but its body could not be fetched.
type IssueContext struct {
	Number int
	Title  string
	Body   string
	URL    string
}

// LinkedIssueFetcher is implemented by providers that can resolve the
// same-host issues a PR/MR formally closes. It is an OPTIONAL capability:
// the app type-asserts a provider to it and skips acceptance-criteria
// enrichment when the assertion fails.
//
// Return contract:
//   - issues: successfully resolved acceptance criteria (may be empty).
//   - notes:  non-fatal, human-readable diagnostics (an issue was dropped,
//     a count cap was hit) — surfaced in the run summary, never swallowed.
//   - err:    TOTAL failure only (the closing-refs query itself failed);
//     the caller surfaces it as one note and proceeds with no criteria.
type LinkedIssueFetcher interface {
	FetchLinkedIssues(ctx context.Context, target ReviewTarget) (issues []IssueContext, notes []string, err error)
}
