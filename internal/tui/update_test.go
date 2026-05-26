package tui

import (
	"errors"
	"strings"
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

// TestUpdateCopyOnC verifies pressing 'c' copies the current finding's
// SuggestedComment via the injectable copyToClipboard seam.
func TestUpdateCopyOnC(t *testing.T) {
	var captured string
	prev := copyToClipboard
	copyToClipboard = func(s string) error {
		captured = s
		return nil
	}
	t.Cleanup(func() { copyToClipboard = prev })

	m := NewModel([]review.Finding{
		{
			File: "a.go", Line: 1, Severity: review.SeverityHigh,
			Title: "T", SuggestedComment: "Fix this leak", Model: "test", Confidence: 0.5,
		},
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	if captured != "Fix this leak" {
		t.Errorf("'c' should copy current finding's SuggestedComment; got %q", captured)
	}
}

// TestUpdateCopyOnC_SurfacesError verifies that when copyToClipboard
// fails (e.g. Linux user with no xclip/wl-copy installed), the error
// reaches a user-visible status on the model so the View can show it
// in the footer. Without this, pressing 'c' silently does nothing and
// the user pastes stale clipboard content into a real review thread.
func TestUpdateCopyOnC_SurfacesError(t *testing.T) {
	prev := copyToClipboard
	copyToClipboard = func(_ string) error {
		return errors.New("xclip: command not found")
	}
	t.Cleanup(func() { copyToClipboard = prev })

	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T", SuggestedComment: "x", Model: "test", Confidence: 0.5},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})

	status := m.TransientStatus()
	if status == "" {
		t.Fatal("'c' on a copy failure must set a transient status; got empty")
	}
	if !strings.Contains(status, "Copy failed") {
		t.Errorf("status should indicate copy failure; got %q", status)
	}
	if !strings.Contains(status, "xclip") {
		t.Errorf("status should include the underlying error; got %q", status)
	}
}

// TestUpdateCopyOnC_ClearsOnNextKey verifies that the transient
// status is cleared by the next keypress, so successive operations
// don't leave stale messages in the footer.
func TestUpdateCopyOnC_ClearsOnNextKey(t *testing.T) {
	prev := copyToClipboard
	copyToClipboard = func(_ string) error { return errors.New("nope") }
	t.Cleanup(func() { copyToClipboard = prev })

	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T", SuggestedComment: "x", Model: "test", Confidence: 0.5},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if m.TransientStatus() == "" {
		t.Fatal("setup: failure status should be set after first 'c'")
	}
	// Any subsequent key (down here) should clear the prior status.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.TransientStatus() != "" {
		t.Errorf("transient status must clear on next keypress; still got %q", m.TransientStatus())
	}
}

// TestUpdateMarkForPostOnP verifies pressing 'p' marks the current
// finding for upstream posting via the M5b runner.
func TestUpdateMarkForPostOnP(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "A", Model: "t", Confidence: 0.5},
		{File: "b.go", Line: 2, Severity: review.SeverityHigh, Title: "B", Model: "t", Confidence: 0.5},
	})

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})

	marked := m.GetFindingsMarkedForPost()
	if len(marked) != 1 || marked[0].File != "a.go" {
		t.Errorf("'p' should mark current (a.go) for post; got %+v", marked)
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
