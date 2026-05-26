package githubgh

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// Adapter fetches GitHub pull request data via the `gh` CLI.
type Adapter struct {
	run       provider.Runner
	preflight *Preflight
}

// New constructs an Adapter. Passing nil uses provider.DefaultRunner and a
// default Preflight that calls exec.LookPath. Tests that need to substitute
// LookPath can build an Adapter literal directly.
func New(run provider.Runner) *Adapter {
	if run == nil {
		run = provider.DefaultRunner
	}
	return &Adapter{
		run:       run,
		preflight: NewPreflight(run, nil),
	}
}

// Supports reports whether the URL is a GitHub pull request URL.
func (a *Adapter) Supports(rawURL string) bool {
	return Supports(rawURL)
}

// Preflight verifies gh is on PATH and authenticated before any fetch.
// New always wires a.preflight, and Adapter's fields are unexported so
// no zero-value literal can escape the package — a defensive nil-init
// here would be dead code.
func (a *Adapter) Preflight(ctx context.Context) error {
	return a.preflight.Check(ctx)
}

// Fetch retrieves PR metadata and the unified diff, then returns a
// normalized ReviewInput. The diff is parsed via internal/diff to surface
// classification errors early, before the model is invoked.
func (a *Adapter) Fetch(ctx context.Context, rawURL string) (*review.ReviewInput, error) {
	ref, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}

	meta, err := a.fetchMetadata(ctx, ref.URL)
	if err != nil {
		return nil, err
	}

	rawDiff, err := a.run(ctx, nil, "gh", "pr", "diff", ref.URL, "--patch", "--color", "never")
	if err != nil {
		return nil, fmt.Errorf("gh pr diff: %w", err)
	}
	files, err := diff.Parse(string(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	return &review.ReviewInput{
		Target: review.ReviewTarget{
			Host:    review.HostGitHub,
			URL:     firstNonEmpty(meta.URL, ref.URL),
			Owner:   ref.Owner,
			Repo:    ref.Repo,
			Number:  ref.Number,
			HeadRef: meta.HeadRefName,
			HeadSHA: meta.HeadRefOid,
			BaseRef: meta.BaseRefName,
		},
		Title:   meta.Title,
		Author:  meta.Author.Login,
		Files:   files,
		RawDiff: string(rawDiff),
	}, nil
}

// ghMetadata mirrors the JSON shape returned by `gh pr view --json …`.
// HeadRefOid is the head commit SHA captured at diff-fetch time so the
// poster can anchor inline comments without re-resolving HEAD later.
type ghMetadata struct {
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
}

func (a *Adapter) fetchMetadata(ctx context.Context, prURL string) (*ghMetadata, error) {
	out, err := a.run(ctx, nil, "gh", "pr", "view", prURL, "--json", "title,author,headRefName,headRefOid,baseRefName,url")
	if err != nil {
		return nil, fmt.Errorf("gh pr view: %w", err)
	}
	var m ghMetadata
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("decode gh pr view JSON: %w", err)
	}
	return &m, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// PreflightList verifies gh is authenticated before listing PRs.
func (a *Adapter) PreflightList(ctx context.Context) error {
	if _, err := a.run(ctx, nil, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is installed but not authenticated; run 'gh auth login': %w", err)
	}
	return nil
}

type ghPR struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
	IsDraft   bool      `json:"isDraft"`
}

// List enumerates open PRs for the repo.
func (a *Adapter) List(ctx context.Context, repo provider.RepoCoord) ([]provider.PRSummary, error) {
	args := []string{
		"pr", "list",
		"--repo", repo.Owner + "/" + repo.Name,
		"--state=open",
		"--json", "number,title,author,updatedAt,url,isDraft",
		"--limit", "30",
	}
	out, err := a.run(ctx, nil, "gh", args...)
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var raw []ghPR
	if err := json.Unmarshal(out, &raw); err != nil {
		preview := string(out)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return nil, fmt.Errorf("failed to parse gh output: %w (raw: %s)", err, preview)
	}
	result := make([]provider.PRSummary, 0, len(raw))
	for _, r := range raw {
		result = append(result, provider.PRSummary{
			Number:    r.Number,
			Title:     r.Title,
			Author:    r.Author.Login,
			URL:       r.URL,
			UpdatedAt: r.UpdatedAt,
			Draft:     r.IsDraft,
		})
	}
	return result, nil
}
