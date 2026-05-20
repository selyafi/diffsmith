package post

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

func TestBuildAddThreadInput_PopulatesAllRequiredGitHubInputFields(t *testing.T) {
	f := review.Finding{
		File:             "internal/store/buffer.go",
		Line:             42,
		Severity:         review.SeverityHigh,
		Title:            "Race condition on shared buffer",
		SuggestedComment: "Hold the mutex while appending.",
	}
	const reviewID = "PRR_kwDOEXAMPLE"
	const commitOID = "a1b2c3d4e5f6"

	got := buildAddThreadInput(f, reviewID, commitOID)

	if got.PullRequestReviewID != reviewID {
		t.Errorf("PullRequestReviewID = %q, want %q", got.PullRequestReviewID, reviewID)
	}
	if got.Path != f.File {
		t.Errorf("Path = %q, want %q", got.Path, f.File)
	}
	if got.Line != f.Line {
		t.Errorf("Line = %d, want %d", got.Line, f.Line)
	}
	if got.Side != diffSideRight {
		t.Errorf("Side = %q, want %q", got.Side, diffSideRight)
	}
	if got.CommitOID != commitOID {
		t.Errorf("CommitOID = %q, want %q", got.CommitOID, commitOID)
	}
	if want := formatBody(f); got.Body != want {
		t.Errorf("Body did not delegate to formatBody\n got: %q\nwant: %q", got.Body, want)
	}
}
