package post

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// runGH is the test seam for shelling out to gh. Tests reassign this to
// avoid touching the real CLI; production code uses provider.DefaultRunner.
// Mirrors the swappable-package-var pattern used by internal/clipboard
// and internal/app rather than threading a Runner through every call.
var runGH provider.Runner = provider.DefaultRunner

// Poster submits approved review findings to GitHub as inline PR review
// comments grouped under a single Review.
type Poster struct {
	// Out receives dry-run payloads and the resulting Review URL on submit.
	Out io.Writer
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

// Submit runs the four-phase GraphQL flow that turns approved findings
// into a single grouped GitHub Review:
//
//  1. Resolve the PR's GraphQL node ID from (owner, repo, number).
//  2. Begin a pending pull request review.
//  3. Add one inline thread per finding, anchored to (path, line, HeadSHA).
//  4. Submit the review with event=COMMENT.
//
// On success the resulting Review URL is written to p.Out so the user
// sees where their comments landed.
func (p *Poster) Submit(ctx context.Context, target review.ReviewTarget, findings []review.Finding) error {
	if len(findings) == 0 {
		return nil
	}

	prID, err := resolvePRID(ctx, target)
	if err != nil {
		return fmt.Errorf("resolve PR ID: %w", err)
	}

	reviewID, err := beginReview(ctx, prID)
	if err != nil {
		return fmt.Errorf("begin pending review: %w", err)
	}

	for _, f := range findings {
		if err := addThread(ctx, reviewID, target.HeadSHA, f); err != nil {
			return fmt.Errorf("add thread for %s:%d: %w", f.File, f.Line, err)
		}
	}

	url, err := submitReview(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("submit review: %w", err)
	}

	fmt.Fprintf(p.Out, "Posted review: %s\n", url)
	return nil
}

// graphqlCall sends a single GraphQL operation to gh via stdin. The body
// is the standard {query, variables} envelope so any operation shape
// (query or mutation, top-level or input-wrapped variables) uses one path.
func graphqlCall(ctx context.Context, query string, variables any) ([]byte, error) {
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal graphql body: %w", err)
	}
	return runGH(ctx, bytes.NewReader(body), "gh", "api", "graphql", "--input", "-")
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
	out, err := graphqlCall(ctx, mutationBeginReview, map[string]any{
		"input": map[string]any{
			"pullRequestId": prID,
			"event":         "PENDING",
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
	return resp.Data.SubmitPullRequestReview.PullRequestReview.URL, nil
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
