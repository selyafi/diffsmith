package post

import (
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

func TestFormatBody_OmitsEmptyOptionalSections(t *testing.T) {
	f := review.Finding{
		Severity:         review.SeverityLow,
		Title:            "Minor naming nit",
		SuggestedComment: "Consider renaming `tmp` to something self-describing.",
		// Evidence and FixHint deliberately empty — model couldn't or
		// chose not to supply them.
	}

	got := formatBody(f)

	if strings.Contains(got, "Evidence:") {
		t.Errorf("formatBody emitted Evidence section with no evidence\nGOT:\n%s", got)
	}
	if strings.Contains(got, "Suggested fix:") {
		t.Errorf("formatBody emitted Suggested fix section with no fix hint\nGOT:\n%s", got)
	}
	// Verify we didn't accidentally drop the comment along with the sections.
	if !strings.Contains(got, "Consider renaming") {
		t.Errorf("formatBody dropped suggested comment\nGOT:\n%s", got)
	}
}

func TestFormatBody_ComposesAllFindingFields(t *testing.T) {
	f := review.Finding{
		File:             "internal/store/buffer.go",
		Line:             42,
		Severity:         review.SeverityMedium,
		Title:            "Unbounded slice growth in append loop",
		Evidence:         "for _, x := range items { buf = append(buf, x...) }",
		SuggestedComment: "Pre-allocate buf with capacity equal to the expected total to avoid quadratic copy cost.",
		FixHint:          "buf := make([]byte, 0, expectedSize)",
	}

	got := formatBody(f)

	wantSubstrings := []string{
		"Unbounded slice growth in append loop",
		"medium",
		"Pre-allocate buf",
		"for _, x := range items",
		"make([]byte, 0, expectedSize)",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(got, want) {
			t.Errorf("formatBody output missing %q\nGOT:\n%s", want, got)
		}
	}
}
