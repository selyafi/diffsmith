package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestEditCurrentFindingTitle verifies editing the current finding's title.
func TestEditCurrentFindingTitle(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "main.go",
			Line:             42,
			Severity:         review.SeverityHigh,
			Title:            "Original title",
			Model:            "codex",
			Confidence:       0.85,
			SuggestedComment: "Original comment",
		},
	}

	m := NewModel(findings)
	current := m.CurrentFinding()
	if current == nil {
		t.Fatal("CurrentFinding returned nil")
	}

	m.EditCurrentTitle("Updated title")
	current = m.CurrentFinding()
	if current.Title != "Updated title" {
		t.Errorf("Title not updated: got %q, want %q", current.Title, "Updated title")
	}
}

// TestEditCurrentFindingComment verifies editing the current finding's comment.
func TestEditCurrentFindingComment(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "util.go",
			Line:             15,
			Severity:         review.SeverityMedium,
			Title:            "Inefficiency",
			Model:            "codex",
			Confidence:       0.7,
			SuggestedComment: "Original suggestion",
		},
	}

	m := NewModel(findings)
	m.EditCurrentComment("Better suggestion")
	current := m.CurrentFinding()
	if current.SuggestedComment != "Better suggestion" {
		t.Errorf("Comment not updated: got %q, want %q", current.SuggestedComment, "Better suggestion")
	}
}

// TestEditCurrentFindingEvidence verifies editing evidence.
func TestEditCurrentFindingEvidence(t *testing.T) {
	findings := []review.Finding{
		{
			File:       "test.go",
			Line:       5,
			Severity:   review.SeveritySuggestion,
			Title:      "Minor issue",
			Model:      "codex",
			Confidence: 0.6,
			Evidence:   "Original evidence",
		},
	}

	m := NewModel(findings)
	m.EditCurrentEvidence("Updated evidence")
	current := m.CurrentFinding()
	if current.Evidence != "Updated evidence" {
		t.Errorf("Evidence not updated: got %q, want %q", current.Evidence, "Updated evidence")
	}
}

// TestEditCurrentFindingFixHint verifies editing fix hint.
func TestEditCurrentFindingFixHint(t *testing.T) {
	findings := []review.Finding{
		{
			File:       "fix.go",
			Line:       20,
			Severity:   review.SeverityHigh,
			Title:      "Critical bug",
			Model:      "codex",
			Confidence: 0.95,
			FixHint:    "Old fix hint",
		},
	}

	m := NewModel(findings)
	m.EditCurrentFixHint("New fix hint")
	current := m.CurrentFinding()
	if current.FixHint != "New fix hint" {
		t.Errorf("FixHint not updated: got %q, want %q", current.FixHint, "New fix hint")
	}
}

// TestEditWithNilCurrentFinding verifies no panic when editing with no findings.
func TestEditWithNilCurrentFinding(t *testing.T) {
	m := NewModel(nil)

	// These should not panic
	m.EditCurrentTitle("Title")
	m.EditCurrentComment("Comment")
	m.EditCurrentEvidence("Evidence")
	m.EditCurrentFixHint("Fix")
}
