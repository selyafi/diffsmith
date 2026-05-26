package post

import (
	"encoding/json"
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

	got := buildAddThreadInput(f, reviewID)

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
	if want := formatBody(f); got.Body != want {
		t.Errorf("Body did not delegate to formatBody\n got: %q\nwant: %q", got.Body, want)
	}
}

func TestBuildAddThreadInput_DoesNotIncludeCommitOID(t *testing.T) {
	// Regression: AddPullRequestReviewThreadInput has no commitOID field
	// (GitHub rejects it as "Field is not defined"). The marshalled JSON
	// must not carry a commitOID key under any name.
	got := buildAddThreadInput(review.Finding{File: "x", Line: 1}, "PRR_x")
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"commitOID", "commit_oid", "CommitOID"} {
		if _, ok := m[key]; ok {
			t.Errorf("addThreadInput JSON must not include %q (not a field on AddPullRequestReviewThreadInput): %s", key, b)
		}
	}
}
