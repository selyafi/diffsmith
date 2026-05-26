package post

import "github.com/selyafi/diffsmith/internal/review"

// diffSide is GitHub's GraphQL DiffSide enum. Comments on added lines use
// RIGHT; LEFT-side commentary on deleted lines stays out of scope until a
// Finding gains a Side field.
type diffSide string

const diffSideRight diffSide = "RIGHT"

// addThreadInput is the variables shape for GitHub's
// addPullRequestReviewThread mutation. JSON tags match the GraphQL input
// field names exactly so json.Marshal produces a valid payload directly.
//
// Per the live AddPullRequestReviewThreadInput schema, only `body` is
// required; the thread anchors to (path, line) on the PR's current HEAD
// implicitly. There is no commitOID field — sending one makes the call
// fail with "Field is not defined on AddPullRequestReviewThreadInput".
type addThreadInput struct {
	PullRequestReviewID string   `json:"pullRequestReviewId"`
	Path                string   `json:"path"`
	Line                int      `json:"line"`
	Side                diffSide `json:"side"`
	Body                string   `json:"body"`
}

// buildAddThreadInput assembles the GraphQL input for a single inline
// review thread on (file, line). The body is delegated to formatBody so
// rendering changes flow through one seam.
func buildAddThreadInput(f review.Finding, reviewID string) addThreadInput {
	return addThreadInput{
		PullRequestReviewID: reviewID,
		Path:                f.File,
		Line:                f.Line,
		Side:                diffSideRight,
		Body:                formatBody(f),
	}
}
