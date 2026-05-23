package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/review"
)

// Tests for the edit-mode behavior added by diffsmith-axv (PRD F9):
//
//   - 'e' enters edit mode and loads the current finding's suggested_comment
//     into the textarea
//   - 'esc' exits edit mode discarding changes
//   - 'ctrl+s' exits edit mode persisting the textarea value back to the finding
//   - normal-mode keys (q, j, k, a, d, c, p) are inert while in edit mode
//     because the textarea owns input
//
// Tests drive Update directly with synthetic tea.KeyMsg values so the
// behavior is exercised without spinning up a Bubble Tea program.

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+s":
		return tea.KeyMsg{Type: tea.KeyCtrlS}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func mkOneFinding(comment string) *Model {
	return NewModel([]review.Finding{
		{
			File:             "auth.go",
			Line:             42,
			Severity:         review.SeverityHigh,
			Title:            "Race in token refresh",
			Evidence:         "Two goroutines touch the same map without locking.",
			SuggestedComment: comment,
			FixHint:          "Use sync.Mutex around the cache.",
			Confidence:       0.81,
			Model:            "codex",
			State:            review.StatePending,
		},
	})
}

func TestEditMode_EnterLoadsSuggestedComment(t *testing.T) {
	m := mkOneFinding("Original suggestion text.")
	if m.EditMode() {
		t.Fatal("EditMode should start false")
	}

	m.Update(keyMsg("e"))

	if !m.EditMode() {
		t.Fatal("EditMode should be true after pressing e")
	}
	if got := m.editor.Value(); got != "Original suggestion text." {
		t.Errorf("textarea should be seeded with the suggested_comment; got %q", got)
	}
}

func TestEditMode_EscDiscardsChanges(t *testing.T) {
	m := mkOneFinding("Original.")
	m.Update(keyMsg("e"))
	m.editor.SetValue("EDITED IN-MEMORY")

	m.Update(keyMsg("esc"))

	if m.EditMode() {
		t.Error("EditMode should be false after esc")
	}
	if got := m.findings[0].SuggestedComment; got != "Original." {
		t.Errorf("esc must discard textarea changes; got %q", got)
	}
}

func TestEditMode_CtrlSSavesChanges(t *testing.T) {
	m := mkOneFinding("Original.")
	m.Update(keyMsg("e"))
	m.editor.SetValue("Rewritten in the reviewer's voice.")

	m.Update(keyMsg("ctrl+s"))

	if m.EditMode() {
		t.Error("EditMode should be false after ctrl+s")
	}
	if got := m.findings[0].SuggestedComment; got != "Rewritten in the reviewer's voice." {
		t.Errorf("ctrl+s must persist textarea value; got %q", got)
	}
}

// In edit mode the normal-mode keys must NOT trigger their normal-mode
// actions — they should be characters typed into the textarea instead.
// Without this gate a reviewer trying to type a word containing 'q' would
// accidentally quit the program.
func TestEditMode_GatesNormalKeys(t *testing.T) {
	m := mkOneFinding("Original.")
	m.Update(keyMsg("e"))
	initialApproved := m.findings[0].State

	// Normal mode 'a' approves; in edit mode it should NOT.
	m.Update(keyMsg("a"))
	if m.findings[0].State != initialApproved {
		t.Error("'a' must not approve in edit mode (textarea should consume it)")
	}

	// Normal mode 'd' dismisses; in edit mode it should NOT.
	m.Update(keyMsg("d"))
	if m.findings[0].State != initialApproved {
		t.Error("'d' must not dismiss in edit mode")
	}

	// EditMode should still be active.
	if !m.EditMode() {
		t.Error("EditMode should still be active; 'a'/'d' did not exit it")
	}
}

func TestEditMode_NoOpWhenNoFinding(t *testing.T) {
	m := NewModel(nil)
	m.Update(keyMsg("e"))
	if m.EditMode() {
		t.Error("pressing e with no findings must not enter edit mode")
	}
}

// View should clearly signal edit mode so the user knows the input
// context has shifted. We check the EDIT MODE badge text rather than
// styling so the test stays robust to color/style tweaks.
func TestEditMode_ViewShowsEditBadge(t *testing.T) {
	m := mkOneFinding("Original.")
	if strings.Contains(m.View(), "EDIT MODE") {
		t.Error("EDIT MODE badge should not appear before entering edit mode")
	}
	m.Update(keyMsg("e"))
	if !strings.Contains(m.View(), "EDIT MODE") {
		t.Error("EDIT MODE badge should appear after entering edit mode")
	}
}
