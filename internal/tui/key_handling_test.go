package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestApproveCurrentFinding verifies marking a finding as approved.
func TestApproveFinding(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "main.go",
			Line:             10,
			Severity:         review.SeverityHigh,
			Title:            "Critical bug",
			Model:            "test",
			Confidence:       0.95,
			SuggestedComment: "Fix immediately",
			State:            review.StatePending,
		},
	}

	m := NewModel(findings)
	current := m.CurrentFinding()
	if current.State != review.StatePending {
		t.Error("initial state should be pending")
	}

	m.ApproveCurrent()
	current = m.CurrentFinding()
	if current.State != review.StateApproved {
		t.Error("after ApproveCurrent(), state should be approved")
	}
}

// TestDismissFinding verifies marking a finding as dismissed.
func TestDismissFinding(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "util.go",
			Line:             25,
			Severity:         review.SeverityLow,
			Title:            "Minor style issue",
			Model:            "test",
			Confidence:       0.5,
			SuggestedComment: "Can improve",
			State:            review.StatePending,
		},
	}

	m := NewModel(findings)
	m.DismissCurrent()
	current := m.CurrentFinding()
	if current.State != review.StateDismissed {
		t.Error("after DismissCurrent(), state should be dismissed")
	}
}

// TestToggleApprovalState verifies changing state from approved to dismissed.
func TestToggleApprovalState(t *testing.T) {
	findings := []review.Finding{
		{
			File:       "test.go",
			Line:       15,
			Severity:   review.SeverityMedium,
			Title:      "Test issue",
			Model:      "test",
			Confidence: 0.7,
			State:      review.StatePending,
		},
	}

	m := NewModel(findings)

	m.ApproveCurrent()
	if m.CurrentFinding().State != review.StateApproved {
		t.Error("should be approved after first approve")
	}

	m.DismissCurrent()
	if m.CurrentFinding().State != review.StateDismissed {
		t.Error("should be dismissed after dismissing an approved finding")
	}
}

// TestSwitchBetweenFindings verifies navigating and approving different findings.
func TestSwitchBetweenFindings(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "First", Model: "test", Confidence: 0.9, State: review.StatePending},
		{File: "b.go", Line: 2, Severity: review.SeverityMedium, Title: "Second", Model: "test", Confidence: 0.7, State: review.StatePending},
		{File: "c.go", Line: 3, Severity: review.SeverityLow, Title: "Third", Model: "test", Confidence: 0.6, State: review.StatePending},
	}

	m := NewModel(findings)

	// Approve first finding
	m.ApproveCurrent()
	if m.CurrentFinding().State != review.StateApproved {
		t.Error("first finding should be approved")
	}

	// Move to second and dismiss
	m.MoveDown()
	m.DismissCurrent()
	if m.CurrentFinding().State != review.StateDismissed {
		t.Error("second finding should be dismissed")
	}

	// Move to third and approve
	m.MoveDown()
	m.ApproveCurrent()
	if m.CurrentFinding().State != review.StateApproved {
		t.Error("third finding should be approved")
	}

	// Verify counts
	approved := m.ApprovedCount()
	dismissed := m.DismissedCount()
	if approved != 2 || dismissed != 1 {
		t.Errorf("expected 2 approved and 1 dismissed, got %d approved and %d dismissed", approved, dismissed)
	}
}
