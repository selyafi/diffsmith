package githubgh

import (
	"context"
	"encoding/json"
	"fmt"

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
func (a *Adapter) Preflight(ctx context.Context) error {
	if a.preflight == nil {
		a.preflight = NewPreflight(a.run, nil)
	}
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
			BaseRef: meta.BaseRefName,
		},
		Title:   meta.Title,
		Author:  meta.Author.Login,
		Files:   files,
		RawDiff: string(rawDiff),
	}, nil
}

// ghMetadata mirrors the JSON shape returned by
// `gh pr view --json title,author,headRefName,baseRefName,url`.
type ghMetadata struct {
	Title  string `json:"title"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	URL         string `json:"url"`
}

func (a *Adapter) fetchMetadata(ctx context.Context, prURL string) (*ghMetadata, error) {
	out, err := a.run(ctx, nil, "gh", "pr", "view", prURL, "--json", "title,author,headRefName,baseRefName,url")
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
