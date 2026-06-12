package review

import (
	"context"
	"net/url"

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
// HeadSHA is the commit OID at the time the diff was fetched. GitLab
// uses it directly as the posting anchor (position.head_sha). GitHub's
// thread mutation cannot carry a commit OID — threads implicitly anchor
// to the PR's current head — so there HeadSHA serves as a drift guard:
// posting refuses when the head has moved since fetch, instead of
// silently re-anchoring comments to lines the review never saw
// (diffsmith-cgq).
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

// Hostname returns the host of the target URL (e.g. "gitlab.com",
// "gitlab.example.com"). CLI calls that operate on the target must pin
// themselves to this host (gh --hostname / glab --hostname): glab in
// particular resolves bare API calls against its configured default
// host, which can be a different instance than the MR's
// (diffsmith-1bk). Empty when the URL is unparseable, in which case
// callers fall back to the CLI's default resolution.
func (t ReviewTarget) Hostname() string {
	u, err := url.Parse(t.URL)
	if err != nil {
		return ""
	}
	return u.Host
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
