package gitlabglab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// Adapter fetches GitLab merge request data via the `glab` CLI.
type Adapter struct {
	run       provider.Runner
	preflight *Preflight
}

// New constructs an Adapter. Passing nil uses provider.DefaultRunner and
// a default Preflight that calls exec.LookPath. Tests that need to
// substitute LookPath can use NewWithLookPath.
func New(run provider.Runner) *Adapter {
	return NewWithLookPath(run, nil)
}

// NewWithLookPath constructs an Adapter with explicit run + lookPath
// injection so cross-package tests (e.g. internal/app/) can build a
// fully hermetic Adapter that needs neither real `glab` on PATH nor
// real network. Passing nil for either falls back to defaults
// (provider.DefaultRunner, exec.LookPath).
func NewWithLookPath(run provider.Runner, lookPath func(string) (string, error)) *Adapter {
	if run == nil {
		run = provider.DefaultRunner
	}
	return &Adapter{
		run:       run,
		preflight: NewPreflight(run, lookPath),
	}
}

// Supports reports whether the URL is a GitLab merge request URL.
func (a *Adapter) Supports(rawURL string) bool {
	return Supports(rawURL)
}

// Preflight verifies glab is on PATH and authenticated before any fetch.
func (a *Adapter) Preflight(ctx context.Context) error {
	if a.preflight == nil {
		a.preflight = NewPreflight(a.run, nil)
	}
	return a.preflight.Check(ctx)
}

// Fetch retrieves MR metadata and the unified diff via glab, then returns
// a normalized ReviewInput with Host=review.HostGitLab. The diff is
// parsed via internal/diff to surface classification errors early,
// before the model is invoked.
func (a *Adapter) Fetch(ctx context.Context, rawURL string) (*review.ReviewInput, error) {
	ref, err := ParseURL(rawURL)
	if err != nil {
		return nil, err
	}

	meta, err := a.fetchMetadata(ctx, ref)
	if err != nil {
		return nil, err
	}
	if meta.SHA == "" {
		return nil, fmt.Errorf("glab mr view returned empty sha for MR #%d (cannot anchor review comments without a head commit)", ref.Number)
	}

	rawDiff, err := a.run(ctx, nil, "glab", "mr", "diff", strconv.Itoa(ref.Number),
		"-R", ref.RepoURL, "--raw", "--color", "never")
	if err != nil {
		return nil, fmt.Errorf("glab mr diff: %w", err)
	}
	files, err := diff.Parse(string(rawDiff))
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}

	owner, repo := splitProjectPath(ref.ProjectPath)
	return &review.ReviewInput{
		Target: review.ReviewTarget{
			Host:     review.HostGitLab,
			URL:      ref.URL,
			Owner:    owner,
			Repo:     repo,
			Number:   ref.Number,
			HeadRef:  meta.SourceBranch,
			HeadSHA:  meta.SHA,
			BaseRef:  meta.TargetBranch,
			BaseSHA:  meta.DiffRefs.BaseSHA,
			StartSHA: meta.DiffRefs.StartSHA,
		},
		Title:   meta.Title,
		Author:  meta.Author.Username,
		Files:   files,
		RawDiff: string(rawDiff),
	}, nil
}

// mrMetadata mirrors the snake_case JSON shape returned by
// `glab mr view --output json`. Only the fields needed to populate
// ReviewTarget + Title/Author are decoded; everything else (id, iid,
// description, project_id, references, milestone, etc.) is ignored. SHA
// is the head commit at fetch time and serves as the capture-time anchor
// for upstream review comments — re-resolving at post time would risk
// silently re-anchoring to a moved head if the MR got pushed mid-review.
type mrMetadata struct {
	Title  string `json:"title"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
	WebURL       string `json:"web_url"`
	// DiffRefs carries the SHAs needed to position inline review
	// threads via GitLab's discussions API. All three are required
	// when posting a thread at a specific file:line.
	DiffRefs struct {
		BaseSHA  string `json:"base_sha"`
		HeadSHA  string `json:"head_sha"`
		StartSHA string `json:"start_sha"`
	} `json:"diff_refs"`
}

func (a *Adapter) fetchMetadata(ctx context.Context, ref *MergeRequestRef) (*mrMetadata, error) {
	out, err := a.run(ctx, nil, "glab", "mr", "view", strconv.Itoa(ref.Number),
		"-R", ref.RepoURL, "--output", "json")
	if err != nil {
		return nil, fmt.Errorf("glab mr view: %w", err)
	}
	var m mrMetadata
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("decode glab mr view JSON: %w", err)
	}
	return &m, nil
}

// splitProjectPath splits "group/project" or "group/sub/project" into
// (owner, repo) at the LAST slash. The owner half preserves namespace
// depth for nested groups; the repo half is the leaf project. ParseURL
// guarantees the path contains at least one slash, so LastIndex is safe.
func splitProjectPath(p string) (owner, repo string) {
	i := strings.LastIndex(p, "/")
	return p[:i], p[i+1:]
}

// PreflightList verifies glab is authenticated before listing MRs.
func (a *Adapter) PreflightList(ctx context.Context) error {
	if _, err := a.run(ctx, nil, "glab", "auth", "status"); err != nil {
		return fmt.Errorf("glab is installed but not authenticated; run 'glab auth login': %w", err)
	}
	return nil
}

// glabMR mirrors the per-item JSON shape returned by `glab mr list --output json`.
type glabMR struct {
	IID    int    `json:"iid"`
	Title  string `json:"title"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	WebURL    string    `json:"web_url"`
	Draft     bool      `json:"draft"`
}

// List enumerates open MRs for the repo via `glab mr list`. Omitting
// `--opened` (deprecated as of glab v1.x) inherits the default "open"
// behavior without triggering the deprecation warning that glab writes
// to stdout, mixed with the JSON.
func (a *Adapter) List(ctx context.Context, repo provider.RepoCoord) ([]provider.PRSummary, error) {
	args := []string{
		"mr", "list",
		"--repo", repo.Owner + "/" + repo.Name,
		"--output", "json",
		"--per-page", "30",
	}
	out, err := a.run(ctx, nil, "glab", args...)
	if err != nil {
		return nil, fmt.Errorf("glab mr list: %w", err)
	}
	// glab writes warnings (deprecations, update-available notices, etc.)
	// to stdout, prefixed before the JSON payload. Skip everything up to
	// the first `[` so a stray preamble line doesn't break unmarshal.
	jsonStart := bytes.IndexByte(out, '[')
	if jsonStart > 0 {
		out = out[jsonStart:]
	}
	var raw []glabMR
	if err := json.Unmarshal(out, &raw); err != nil {
		preview := string(out)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		return nil, fmt.Errorf("failed to parse glab output: %w (raw: %s)", err, preview)
	}
	result := make([]provider.PRSummary, 0, len(raw))
	for _, r := range raw {
		result = append(result, provider.PRSummary{
			Number:    r.IID,
			Title:     r.Title,
			Author:    r.Author.Username,
			URL:       r.WebURL,
			UpdatedAt: r.UpdatedAt,
			Draft:     r.Draft,
		})
	}
	return result, nil
}
