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
