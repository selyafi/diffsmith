package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/provider"
)

func mkInbox(summaries []provider.PRSummary) *InboxModel {
	return NewInboxModel(summaries, "cli", "cli")
}

func TestInbox_InitialSelectionIsFirst(t *testing.T) {
	m := mkInbox([]provider.PRSummary{
		{Number: 1, Title: "a", Author: "alice"},
		{Number: 2, Title: "b", Author: "bob"},
	})
	if m.Selected() != 0 {
		t.Errorf("expected initial selection 0, got %d", m.Selected())
	}
}

func TestInbox_DownArrowMovesSelection(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}, {Number: 2}, {Number: 3}})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Selected() != 1 {
		t.Errorf("after down, expected 1, got %d", m.Selected())
	}
}

func TestInbox_JMovesDownLikeArrow(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}, {Number: 2}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.Selected() != 1 {
		t.Errorf("after j, expected 1, got %d", m.Selected())
	}
}

func TestInbox_DownAtEndDoesNotWrap(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}, {Number: 2}})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Selected() != 1 {
		t.Errorf("expected clamp at 1, got %d", m.Selected())
	}
}

func TestInbox_UpAtTopDoesNotWrap(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}, {Number: 2}})
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.Selected() != 0 {
		t.Errorf("expected clamp at 0, got %d", m.Selected())
	}
}

func TestInbox_EnterReturnsActionOpen(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 42, URL: "https://example/42"}})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Action() != InboxActionOpen {
		t.Errorf("expected ActionOpen, got %v", m.Action())
	}
	if m.Pick() == nil || m.Pick().Number != 42 {
		t.Errorf("expected pick=#42, got %+v", m.Pick())
	}
}

func TestInbox_RReturnsActionRefresh(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	if m.Action() != InboxActionRefresh {
		t.Errorf("expected ActionRefresh, got %v", m.Action())
	}
}

func TestInbox_QReturnsActionQuit(t *testing.T) {
	m := mkInbox([]provider.PRSummary{{Number: 1}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.Action() != InboxActionQuit {
		t.Errorf("expected ActionQuit, got %v", m.Action())
	}
}

func TestInbox_EmptyStateShowsQuitHint(t *testing.T) {
	m := mkInbox(nil)
	view := m.View()
	if !strings.Contains(view, "No open PRs") {
		t.Errorf("empty-state view missing 'No open PRs': %s", view)
	}
	if !strings.Contains(view, "q quit") {
		t.Errorf("empty-state view missing 'q quit' hint: %s", view)
	}
}

// TestInboxModel_ResetSessionClearsExitState is the diffsmith-qe5
// regression: the app reuses one InboxModel across tea sessions to keep
// the cached PR list, so after the first open the stale action/pick
// made the clean-quit (ActionNone) path unreachable — a
// teardown-without-keypress replayed the previous open and re-launched
// a full review. ResetSession must clear exit state, keep the list.
func TestInboxModel_ResetSessionClearsExitState(t *testing.T) {
	m := NewInboxModel([]provider.PRSummary{{Number: 5, Title: "x", URL: "https://e/pr/5"}}, "o", "r")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // user opens #5
	if m.Action() != InboxActionOpen || m.Pick() == nil {
		t.Fatalf("precondition: enter should set Open+pick; got %v %v", m.Action(), m.Pick())
	}
	m.ResetSession()
	if m.Action() != InboxActionNone {
		t.Errorf("ResetSession must clear action; got %v", m.Action())
	}
	if m.Pick() != nil {
		t.Errorf("ResetSession must clear pick; got %+v", m.Pick())
	}
}
