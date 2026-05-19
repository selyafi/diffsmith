package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestNavigateNextFinding verifies moving to the next finding.
func TestNavigateNextFinding(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "First", Model: "test", Confidence: 0.5},
		{File: "b.go", Line: 2, Severity: review.SeverityMedium, Title: "Second", Model: "test", Confidence: 0.5},
		{File: "c.go", Line: 3, Severity: review.SeverityLow, Title: "Third", Model: "test", Confidence: 0.5},
	}
	m := NewModel(findings)

	if m.CurrentFinding().File != "a.go" {
		t.Error("initial selection should be first finding")
	}

	m.MoveDown()
	if m.CurrentFinding().File != "b.go" {
		t.Error("after MoveDown(), should select second finding")
	}

	m.MoveDown()
	if m.CurrentFinding().File != "c.go" {
		t.Error("after another MoveDown(), should select third finding")
	}

	// Should stay on last finding when at boundary
	m.MoveDown()
	if m.CurrentFinding().File != "c.go" {
		t.Error("at boundary, MoveDown() should not move past last finding")
	}
}

// TestNavigatePreviousFinding verifies moving to the previous finding.
func TestNavigatePreviousFinding(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "First", Model: "test", Confidence: 0.5},
		{File: "b.go", Line: 2, Severity: review.SeverityMedium, Title: "Second", Model: "test", Confidence: 0.5},
		{File: "c.go", Line: 3, Severity: review.SeverityLow, Title: "Third", Model: "test", Confidence: 0.5},
	}
	m := NewModel(findings)
	m.MoveDown()
	m.MoveDown() // now at c.go

	m.MoveUp()
	if m.CurrentFinding().File != "b.go" {
		t.Error("after MoveUp(), should select second finding")
	}

	m.MoveUp()
	if m.CurrentFinding().File != "a.go" {
		t.Error("after another MoveUp(), should select first finding")
	}

	// Should stay on first finding when at boundary
	m.MoveUp()
	if m.CurrentFinding().File != "a.go" {
		t.Error("at boundary, MoveUp() should not move before first finding")
	}
}

// TestCurrentFindingWithNoFindings verifies behavior when no findings exist.
func TestCurrentFindingWithNoFindings(t *testing.T) {
	m := NewModel(nil)
	current := m.CurrentFinding()
	if current != nil {
		t.Error("CurrentFinding() should return nil when no findings exist")
	}
}

// TestApproveAndDismissFindings verifies approval/dismissal tracking.
func TestApproveAndDismissFindings(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "First", Model: "test", Confidence: 0.9},
		{File: "b.go", Line: 2, Severity: review.SeverityMedium, Title: "Second", Model: "test", Confidence: 0.7},
	}
	m := NewModel(findings)

	// Initially all findings are pending
	if m.ApprovedCount() != 0 {
		t.Error("initially, no findings should be approved")
	}

	m.ApproveCurrent()
	if m.ApprovedCount() != 1 {
		t.Error("after ApproveCurrent(), should have 1 approved")
	}

	m.MoveDown()
	m.DismissCurrent()
	if m.DismissedCount() != 1 {
		t.Error("after DismissCurrent(), should have 1 dismissed")
	}
}

// TestGetApprovedFindings verifies extracting approved findings.
func TestGetApprovedFindings(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "First", Model: "test", Confidence: 0.9, State: review.StatePending},
		{File: "b.go", Line: 2, Severity: review.SeverityMedium, Title: "Second", Model: "test", Confidence: 0.7, State: review.StatePending},
		{File: "c.go", Line: 3, Severity: review.SeverityLow, Title: "Third", Model: "test", Confidence: 0.6, State: review.StatePending},
	}
	m := NewModel(findings)

	// Approve first and third, dismiss second
	m.ApproveCurrent()
	m.MoveDown()
	m.DismissCurrent()
	m.MoveDown()
	m.ApproveCurrent()

	approved := m.GetApprovedFindings()
	if len(approved) != 2 {
		t.Fatalf("should have 2 approved findings, got %d", len(approved))
	}

	if approved[0].File != "a.go" || approved[1].File != "c.go" {
		t.Error("approved findings should be in order (a.go, c.go)")
	}
}
