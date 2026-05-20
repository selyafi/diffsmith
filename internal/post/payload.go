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
type addThreadInput struct {
	PullRequestReviewID string   `json:"pullRequestReviewId"`
	Path                string   `json:"path"`
	Line                int      `json:"line"`
	Side                diffSide `json:"side"`
	CommitOID           string   `json:"commitOID"`
	Body                string   `json:"body"`
}

// buildAddThreadInput assembles the GraphQL input for a single inline
// review thread on (file, line). The body is delegated to formatBody so
// rendering changes flow through one seam.
func buildAddThreadInput(f review.Finding, reviewID, commitOID string) addThreadInput {
	return addThreadInput{
		PullRequestReviewID: reviewID,
		Path:                f.File,
		Line:                f.Line,
		Side:                diffSideRight,
		CommitOID:           commitOID,
		Body:                formatBody(f),
	}
}
