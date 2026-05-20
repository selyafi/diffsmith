package review

import "github.com/selyafi/diffsmith/internal/diff"

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
type ReviewTarget struct {
	Host    Host
	URL     string
	Owner   string
	Repo    string
	Number  int
	HeadRef string
	HeadSHA string
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
