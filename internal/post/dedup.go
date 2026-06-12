package post

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// diffsmithMarker is the invisible HTML-comment sentinel every posted
// thread body carries so dedup can recognise diffsmith-authored
// threads without polluting the visible body. Both GitHub and GitLab
// strip HTML comments at render time, so this is invisible to readers
// (matches the convention used by Dependabot, renovate, semantic-
// release, etc.).
//
// The matcher uses strings.Contains (not HasPrefix) so the marker can
// sit anywhere in the body — leading is conventional but not required
// by the contract. Versioning could later be encoded as
// `<!-- diffsmith:v2 -->` while keeping the prefix as the match key.
const diffsmithMarker = "<!-- diffsmith"

// dedupKey returns the position-based key used to match a new finding
// against an existing diffsmith thread on the same line. The key is
// intentionally just (file, line) — including title or body hash would
// miss duplicates because LLM variance produces different wording for
// the same conceptual finding across runs. False-positive cost is low
// (rare case: two genuinely distinct issues on the same line); false-
// negative cost is high (every re-run piles up new comments).
func dedupKey(file string, line int) string {
	return fmt.Sprintf("%s:%d", file, line)
}

// glabDiscussion mirrors the JSON shape returned by
// `glab api projects/.../merge_requests/N/discussions`. Only the fields
// we use for dedup are decoded.
type glabDiscussion struct {
	Notes []struct {
		Body     string `json:"body"`
		Position *struct {
			NewPath string `json:"new_path"`
			NewLine int    `json:"new_line"`
		} `json:"position"`
	} `json:"notes"`
}

// fetchExistingGitLabKeys fetches all discussions on the MR via glab,
// filters to diffsmith-authored notes, and returns a set of
// (file, line) keys that identify already-posted threads.
//
// Best-effort: a fetch failure returns an empty set + error so the
// caller can decide whether to abort or proceed without dedup.
func fetchExistingGitLabKeys(ctx context.Context, run provider.Runner, target review.ReviewTarget) (map[string]bool, error) {
	repo := target.Owner + "/" + target.Repo
	projectID := url.PathEscape(repo)
	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/discussions", projectID, target.Number)

	args := []string{"api", endpoint, "--paginate"}
	// Pin the MR's instance: bare glab api resolves against the
	// configured default host, which can differ. diffsmith-1bk.
	if h := target.Hostname(); h != "" {
		args = append(args, "--hostname", h)
	}
	out, err := run(ctx, nil, "glab", args...)
	if err != nil {
		return nil, fmt.Errorf("fetch existing gitlab discussions: %w", err)
	}
	// --paginate emits one JSON array per page; DecodePages handles
	// single- and multi-page output alike. diffsmith-kjk.
	discussions, err := provider.DecodePages[glabDiscussion](out)
	if err != nil {
		return nil, fmt.Errorf("parse gitlab discussions JSON: %w", err)
	}
	keys := make(map[string]bool)
	for _, d := range discussions {
		for _, n := range d.Notes {
			if !strings.Contains(n.Body, diffsmithMarker) {
				continue
			}
			if n.Position == nil || n.Position.NewPath == "" || n.Position.NewLine == 0 {
				continue
			}
			keys[dedupKey(n.Position.NewPath, n.Position.NewLine)] = true
		}
	}
	return keys, nil
}

// ghReviewComment mirrors the JSON shape returned by
// `gh api repos/owner/repo/pulls/N/comments`.
type ghReviewComment struct {
	Body string `json:"body"`
	Path string `json:"path"`
	Line int    `json:"line"`
}

// fetchExistingGitHubKeys fetches all review comments on the PR via gh,
// filters to diffsmith-authored ones, and returns a set of
// (file, line) keys.
func fetchExistingGitHubKeys(ctx context.Context, run provider.Runner, target review.ReviewTarget) (map[string]bool, error) {
	endpoint := fmt.Sprintf("repos/%s/%s/pulls/%d/comments", target.Owner, target.Repo, target.Number)
	out, err := run(ctx, nil, "gh", "api", endpoint, "--paginate")
	if err != nil {
		return nil, fmt.Errorf("fetch existing github review comments: %w", err)
	}
	// --paginate emits one JSON array per page; DecodePages handles
	// single- and multi-page output alike. diffsmith-kjk.
	comments, err := provider.DecodePages[ghReviewComment](out)
	if err != nil {
		return nil, fmt.Errorf("parse github comments JSON: %w", err)
	}
	keys := make(map[string]bool)
	for _, c := range comments {
		if !strings.Contains(c.Body, diffsmithMarker) {
			continue
		}
		if c.Path == "" || c.Line == 0 {
			continue
		}
		keys[dedupKey(c.Path, c.Line)] = true
	}
	return keys, nil
}

// filterDuplicates partitions findings into (toPost, skipped) based on
// the existing-keys set. A finding is skipped iff its (file, line) is
// already in the set. Order within each slice mirrors the input order.
func filterDuplicates(findings []review.Finding, existing map[string]bool) (toPost, skipped []review.Finding) {
	for _, f := range findings {
		if existing[dedupKey(f.File, f.Line)] {
			skipped = append(skipped, f)
		} else {
			toPost = append(toPost, f)
		}
	}
	return toPost, skipped
}
