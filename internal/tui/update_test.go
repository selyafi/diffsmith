package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestUpdateQuitOnQ verifies that pressing 'q' returns tea.Quit so the
// Bubble Tea program loop will terminate cleanly.
func TestUpdateQuitOnQ(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T", Model: "test", Confidence: 0.5},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("pressing 'q' should return a non-nil cmd (tea.Quit)")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("pressing 'q' should return tea.Quit, got %T", cmd())
	}
}

// TestUpdateDownArrowMovesSelection verifies the down arrow advances the cursor.
func TestUpdateDownArrowMovesSelection(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5},
		{File: "b.go", Line: 2, Severity: review.SeverityHigh, Title: "B", Model: "t", Confidence: 0.5},
	})

	m.Update(tea.KeyMsg{Type: tea.KeyDown})

	if got := m.CurrentFinding(); got == nil || got.File != "b.go" {
		t.Errorf("down arrow should select second finding (b.go), got %+v", got)
	}
}

// TestUpdateApproveOnA verifies pressing 'a' approves the current finding.
func TestUpdateApproveOnA(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5, State: review.StatePending},
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})

	if got := m.CurrentFinding(); got == nil || got.State != review.StateApproved {
		t.Errorf("'a' should approve current finding, got state %v", got.State)
	}
}

// TestUpdateDismissOnD verifies pressing 'd' dismisses the current finding.
func TestUpdateDismissOnD(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5, State: review.StatePending},
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})

	if got := m.CurrentFinding(); got == nil || got.State != review.StateDismissed {
		t.Errorf("'d' should dismiss current finding, got state %v", got.State)
	}
}

// TestUpdateQuitOnCtrlC verifies ctrl+c also requests quit (terminal convention).
func TestUpdateQuitOnCtrlC(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5},
	})

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c should return tea.Quit, got nil")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c should return tea.Quit, got %T", cmd())
	}
}

// TestUpdateUpArrowMovesSelection verifies the up arrow retreats the cursor.
func TestUpdateUpArrowMovesSelection(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5},
		{File: "b.go", Line: 2, Severity: review.SeverityHigh, Title: "B", Model: "t", Confidence: 0.5},
	})
	m.MoveDown() // start on b.go

	m.Update(tea.KeyMsg{Type: tea.KeyUp})

	if got := m.CurrentFinding(); got == nil || got.File != "a.go" {
		t.Errorf("up arrow should select first finding (a.go), got %+v", got)
	}
}
