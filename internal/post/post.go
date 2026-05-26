package post

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// runGH is the test seam for shelling out to gh. Tests reassign this to
// avoid touching the real CLI; production code uses provider.DefaultRunner.
// Mirrors the swappable-package-var pattern used by internal/clipboard
// and internal/app rather than threading a Runner through every call.
var runGH provider.Runner = provider.DefaultRunner

// runGlab is the GitLab equivalent of runGH; used by submitGitLab when
// the review target is a GitLab MR.
var runGlab provider.Runner = provider.DefaultRunner

// Poster submits approved review findings to GitHub as inline PR review
// comments grouped under a single Review.
type Poster struct {
	// Out receives dry-run payloads and the resulting Review URL on submit.
	Out io.Writer
	// Repost, when true, bypasses the pre-post dedup step that skips
	// findings whose (file, line) already has a diffsmith thread on the
	// MR/PR. Use this for the explicit "I know there are duplicates and
	// I want them anyway" path; the default behavior is to skip.
	Repost bool
}

const queryResolvePRID = `query($owner:String!,$repo:String!,$number:Int!){
  repository(owner:$owner,name:$repo){pullRequest(number:$number){id}}
}`

const mutationBeginReview = `mutation($input:AddPullRequestReviewInput!){
  addPullRequestReview(input:$input){pullRequestReview{id}}
}`

const mutationAddThread = `mutation($input:AddPullRequestReviewThreadInput!){
  addPullRequestReviewThread(input:$input){thread{id}}
}`

const mutationSubmitReview = `mutation($input:SubmitPullRequestReviewInput!){
  submitPullRequestReview(input:$input){pullRequestReview{url}}
}`

const mutationDeleteReview = `mutation($input:DeletePullRequestReviewInput!){
  deletePullRequestReview(input:$input){pullRequestReview{id}}
}`

// threadFailure records one per-finding addThread failure so Submit can
// finalize the batch (submit vs. delete the pending review) once every
// finding has been attempted.
type threadFailure struct {
	finding review.Finding
	err     error
}

// Submit runs the four-phase GraphQL flow that turns approved findings
// into a single grouped GitHub Review:
//
//  1. Resolve the PR's GraphQL node ID from (owner, repo, number).
//  2. Begin a pending pull request review.
//  3. Add one inline thread per finding, anchored to (path, line, HeadSHA).
//  4. Finalize: submit if ≥1 thread succeeded; otherwise delete the
//     pending review so it doesn't strand on GitHub.
//
// Per-finding addThread failures do NOT abort the batch — every finding
// is attempted, then a summary is printed and the appropriate finalize
// call runs. This keeps GitHub state consistent: callers either see a
// real review URL (with a summary that names any failed findings) or
// a clean error after the pending review has been swept away.
func (p *Poster) Submit(ctx context.Context, target review.ReviewTarget, findings []review.Finding) error {
	if len(findings) == 0 {
		return nil
	}

	// Route by host. GitHub gets the four-phase GraphQL flow; GitLab
	// gets inline review threads via the discussions API.
	if target.Host == review.HostGitLab {
		return p.submitGitLab(ctx, target, findings)
	}

	// Dedup: skip findings whose (file, line) already has a diffsmith
	// thread on the PR. Best-effort — a fetch failure just means we
	// post everything (better than aborting the whole flow on a
	// transient GitHub API hiccup). Bypassed entirely when Repost=true.
	findings = p.applyDedup(ctx, target, findings, fetchExistingGitHubKeys, runGH)

	prID, err := resolvePRID(ctx, target)
	if err != nil {
		return fmt.Errorf("resolve PR ID: %w", err)
	}
	if len(findings) == 0 {
		// Every finding was a duplicate. applyDedup already printed the
		// summary; return cleanly without creating an empty review.
		return nil
	}

	reviewID, err := beginReview(ctx, prID)
	if err != nil {
		return fmt.Errorf("begin pending review: %w", err)
	}

	var failures []threadFailure
	for _, f := range findings {
		if err := addThread(ctx, reviewID, target.HeadSHA, f); err != nil {
			failures = append(failures, threadFailure{finding: f, err: err})
		}
	}

	posted := len(findings) - len(failures)
	p.writeSummary(posted, len(findings), failures)

	if posted == 0 {
		if delErr := deleteReview(ctx, reviewID); delErr != nil {
			return fmt.Errorf("all %d findings failed and cleanup of pending review failed: %w",
				len(findings), errors.Join(delErr, joinFailureErrors(failures)))
		}
		return fmt.Errorf("post review: all %d findings failed; pending review deleted: %w",
			len(findings), joinFailureErrors(failures))
	}

	url, err := submitReview(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("submit review: %w", err)
	}

	fmt.Fprintf(p.Out, "Posted review: %s\n", url)
	return nil
}

// writeSummary emits the per-finding outcome to p.Out before any URL or
// error so the user sees what was attempted regardless of the finalize
// path. Format: a counts line, then one indented "file:line — err" line
// per failure so multi-failure summaries stay readable.
func (p *Poster) writeSummary(posted, total int, failures []threadFailure) {
	if len(failures) == 0 {
		fmt.Fprintf(p.Out, "Posted: %d/%d\n", posted, total)
		return
	}
	fmt.Fprintf(p.Out, "Posted: %d/%d (%d failed)\n", posted, total, len(failures))
	for _, f := range failures {
		fmt.Fprintf(p.Out, "  %s:%d — %s\n", f.finding.File, f.finding.Line, f.err)
	}
}

func joinFailureErrors(failures []threadFailure) error {
	errs := make([]error, 0, len(failures))
	for _, f := range failures {
		errs = append(errs, fmt.Errorf("%s:%d: %w", f.finding.File, f.finding.Line, f.err))
	}
	return errors.Join(errs...)
}

// graphqlCall sends a single GraphQL operation to gh via stdin. The body
// is the standard {query, variables} envelope so any operation shape
// (query or mutation, top-level or input-wrapped variables) uses one path.
//
// GitHub's GraphQL endpoint returns HTTP 200 with a top-level errors[]
// array for permission denials, invalid input, etc. graphqlCall surfaces
// such responses as a 'graphql: <msgs>' error so callers don't have to
// each parse the body shape themselves — and so a partial response
// (errors[] alongside data) is never silently treated as success.
func graphqlCall(ctx context.Context, query string, variables any) ([]byte, error) {
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal graphql body: %w", err)
	}
	out, err := runGH(ctx, bytes.NewReader(body), "gh", "api", "graphql", "--input", "-")
	if err != nil {
		return out, err
	}
	if gErr := detectGraphQLErrors(out); gErr != nil {
		return nil, gErr
	}
	return out, nil
}

// detectGraphQLErrors returns a 'graphql: msg1; msg2' error when the
// response body has a non-empty top-level errors[] array. Unparseable
// JSON or absent/empty errors[] returns nil so the per-step decoder can
// still produce its own specific error for the body it sees.
func detectGraphQLErrors(body []byte) error {
	var resp struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil
	}
	if len(resp.Errors) == 0 {
		return nil
	}
	msgs := make([]string, len(resp.Errors))
	for i, e := range resp.Errors {
		msgs[i] = e.Message
	}
	return fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
}

func resolvePRID(ctx context.Context, target review.ReviewTarget) (string, error) {
	out, err := graphqlCall(ctx, queryResolvePRID, map[string]any{
		"owner":  target.Owner,
		"repo":   target.Repo,
		"number": target.Number,
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ID string `json:"id"`
				} `json:"pullRequest"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode resolve PR ID response: %w", err)
	}
	if resp.Data.Repository.PullRequest.ID == "" {
		return "", fmt.Errorf("empty PR ID in response: %s", string(out))
	}
	return resp.Data.Repository.PullRequest.ID, nil
}

func beginReview(ctx context.Context, prID string) (string, error) {
	// Omit "event" from the input: GitHub's PullRequestReviewEvent enum
	// is {COMMENT, APPROVE, REQUEST_CHANGES, DISMISS} — there is no
	// PENDING. A missing event creates a draft review, which is exactly
	// what we want; the real event is supplied later at submit time.
	out, err := graphqlCall(ctx, mutationBeginReview, map[string]any{
		"input": map[string]any{
			"pullRequestId": prID,
		},
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			AddPullRequestReview struct {
				PullRequestReview struct {
					ID string `json:"id"`
				} `json:"pullRequestReview"`
			} `json:"addPullRequestReview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode begin review response: %w", err)
	}
	if resp.Data.AddPullRequestReview.PullRequestReview.ID == "" {
		return "", fmt.Errorf("empty review ID in response: %s", string(out))
	}
	return resp.Data.AddPullRequestReview.PullRequestReview.ID, nil
}

func addThread(ctx context.Context, reviewID, commitOID string, f review.Finding) error {
	_, err := graphqlCall(ctx, mutationAddThread, map[string]any{
		"input": buildAddThreadInput(f, reviewID, commitOID),
	})
	return err
}

// deleteReview tears down a pending review that never made it to submit.
// Used when every addThread failed so the user doesn't have to clean up
// a stranded draft review by hand. Best-effort: callers receive the
// underlying gh error if cleanup itself fails so they know the review
// is still alive on GitHub.
func deleteReview(ctx context.Context, reviewID string) error {
	_, err := graphqlCall(ctx, mutationDeleteReview, map[string]any{
		"input": map[string]any{
			"pullRequestReviewId": reviewID,
		},
	})
	return err
}

func submitReview(ctx context.Context, reviewID string) (string, error) {
	out, err := graphqlCall(ctx, mutationSubmitReview, map[string]any{
		"input": map[string]any{
			"pullRequestReviewId": reviewID,
			"event":               "COMMENT",
		},
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			SubmitPullRequestReview struct {
				PullRequestReview struct {
					URL string `json:"url"`
				} `json:"pullRequestReview"`
			} `json:"submitPullRequestReview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	// Mirror the empty-field guards on resolvePRID + beginReview. Without
	// this, a malformed-shape 200 response would fall through and the
	// caller would print "Posted review: " with no URL — looks successful,
	// isn't recoverable, user can't find the review on GitHub.
	if resp.Data.SubmitPullRequestReview.PullRequestReview.URL == "" {
		return "", fmt.Errorf("empty review URL in response: %s", string(out))
	}
	return resp.Data.SubmitPullRequestReview.PullRequestReview.URL, nil
}

// submitGitLab posts each finding as an inline review thread via
// GitLab's discussions API (positioned at file:new_line in the diff
// view).
//
// The body is sent as a JSON document via stdin because glab's `-F`
// flag does NOT nest bracket syntax (unlike gh) — passing
// `-F "position[base_sha]=X"` produced a literal JSON key
// `position[base_sha]` which GitLab silently ignored, creating an
// unpositioned (top-level) discussion that looked anchored but wasn't.
// Sending {body, position: {...}} as proper JSON via --input - makes
// the nesting explicit and unambiguous.
//
// Per-finding failures don't abort the batch (best-effort posting), but
// they are NEVER silently fallen back to top-level notes — a failed
// inline post is reported in the summary so the user can see what went
// wrong instead of seeing fake success.
func (p *Poster) submitGitLab(ctx context.Context, target review.ReviewTarget, findings []review.Finding) error {
	if target.BaseSHA == "" || target.HeadSHA == "" || target.StartSHA == "" {
		return fmt.Errorf("gitlab: missing diff-refs (base=%q head=%q start=%q); cannot anchor inline threads",
			target.BaseSHA, target.HeadSHA, target.StartSHA)
	}

	// Dedup: skip findings whose (file, line) already has a diffsmith
	// thread on the MR. Best-effort — fetch failure means we proceed
	// with all findings (the user sees the fetch error printed, but
	// the post still happens). Bypassed entirely when Repost=true.
	findings = p.applyDedup(ctx, target, findings, fetchExistingGitLabKeys, runGlab)
	if len(findings) == 0 {
		// Every finding was a duplicate. applyDedup printed the summary.
		return nil
	}

	repo := target.Owner + "/" + target.Repo
	projectID := url.PathEscape(repo)
	endpoint := fmt.Sprintf("projects/%s/merge_requests/%d/discussions", projectID, target.Number)

	var failures []threadFailure
	for _, f := range findings {
		reqBody, err := json.Marshal(map[string]any{
			"body": formatGitLabNote(f),
			"position": map[string]any{
				"base_sha":      target.BaseSHA,
				"head_sha":      target.HeadSHA,
				"start_sha":     target.StartSHA,
				"position_type": "text",
				"new_path":      f.File,
				"old_path":      f.File,
				"new_line":      f.Line,
			},
		})
		if err != nil {
			failures = append(failures, threadFailure{finding: f, err: fmt.Errorf("marshal discussion body: %w", err)})
			continue
		}
		if _, err := runGlab(ctx, bytes.NewReader(reqBody), "glab", "api", endpoint,
			"--method", "POST",
			"--input", "-",
			"-H", "Content-Type: application/json"); err != nil {
			failures = append(failures, threadFailure{finding: f, err: err})
		}
	}

	posted := len(findings) - len(failures)
	p.writeSummary(posted, len(findings), failures)

	if posted == 0 {
		return fmt.Errorf("post review: all %d findings failed: %w",
			len(findings), joinFailureErrors(failures))
	}

	fmt.Fprintf(p.Out, "Posted %d inline thread(s) to %s MR !%d\n", posted, repo, target.Number)
	return nil
}

// applyDedup is the shared dedup gate used by both submit paths.
// Fetches existing diffsmith threads via the host-specific fetcher,
// filters out duplicates, and prints a summary to p.Out. Returns the
// filtered findings slice (or the original slice unchanged when
// Repost=true or the fetch failed). The fetch error, if any, is
// printed to p.Out so the user knows dedup was skipped — but never
// returned, since a dedup failure should not block posting.
func (p *Poster) applyDedup(
	ctx context.Context,
	target review.ReviewTarget,
	findings []review.Finding,
	fetcher func(context.Context, provider.Runner, review.ReviewTarget) (map[string]bool, error),
	run provider.Runner,
) []review.Finding {
	if p.Repost {
		fmt.Fprintln(p.Out, "Dedup disabled (--repost); posting all findings.")
		return findings
	}
	existing, err := fetcher(ctx, run, target)
	if err != nil {
		fmt.Fprintf(p.Out, "Warning: could not fetch existing threads for dedup (%v); posting all findings.\n", err)
		return findings
	}
	toPost, skipped := filterDuplicates(findings, existing)
	if len(skipped) == 0 {
		return toPost
	}
	fmt.Fprintf(p.Out, "Skipping %d finding(s) already posted (use --repost to override):\n", len(skipped))
	for _, f := range skipped {
		fmt.Fprintf(p.Out, "  %s:%d\n", f.File, f.Line)
	}
	return toPost
}

// formatGitLabNote renders a finding into a GitLab Markdown body.
// The inline thread is already anchored at file:line via the position
// fields, so the body leads with severity/model/confidence metadata,
// then the suggested comment, then the fix hint.
func formatGitLabNote(f review.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**diffsmith review** — %s, model: %s, confidence: %.0f%%\n\n",
		f.Severity, f.Model, f.Confidence*100)
	b.WriteString(f.SuggestedComment)
	if f.FixHint != "" {
		fmt.Fprintf(&b, "\n\n*Fix hint:* %s", f.FixHint)
	}
	return b.String()
}

// PrintPayload writes the addPullRequestReviewThread input as one JSON
// document per finding to p.Out — a hermetic preview of what Submit would
// send. The review ID is a placeholder ("<REVIEW_ID>") because the pending
// review only exists after a real submit, but the commit OID is the real
// capture-time HeadSHA so users can verify the anchor.
func (p *Poster) PrintPayload(target review.ReviewTarget, findings []review.Finding) error {
	for _, f := range findings {
		input := buildAddThreadInput(f, "<REVIEW_ID>", target.HeadSHA)
		data, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("marshal thread input for %s:%d: %w", f.File, f.Line, err)
		}
		if _, err := fmt.Fprintln(p.Out, string(data)); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}
