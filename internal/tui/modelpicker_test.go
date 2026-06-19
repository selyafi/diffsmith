package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func mkPicker(items []ModelPickerItem) *ModelPickerModel {
	return NewModelPickerModel(items)
}

func TestPicker_DefaultSelectsCodexAndClaude(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
		{Name: "antigravity", Available: false, Unavailable: "agy not on PATH"},
	})
	if !m.IsChecked("codex") {
		t.Error("codex should be checked by default")
	}
	if !m.IsChecked("claude") {
		t.Error("claude should be checked by default")
	}
	if m.IsChecked("antigravity") {
		t.Error("antigravity should NOT be checked by default")
	}
}

// TestPicker_DefaultPreChecksAntigravityWhenAvailable pins the post-pivot
// behavior: antigravity is now a working model and is pre-checked when
// available, taking the third slot gemini vacated. (Before the pivot the
// picker pre-checked gemini and deliberately excluded antigravity.)
func TestPicker_DefaultPreChecksAntigravityWhenAvailable(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
		{Name: "antigravity", Available: true},
	})
	if !m.IsChecked("antigravity") {
		t.Error("antigravity should be checked by default when available")
	}
	got := m.SelectedNames()
	want := []string{"codex", "claude", "antigravity"}
	if len(got) != len(want) {
		t.Fatalf("SelectedNames length = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("SelectedNames[%d] = %q, want %q (priority order: codex > claude > antigravity)", i, got[i], name)
		}
	}
}

func TestPicker_AntigravityUncheckedWhenUnavailable(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
		{Name: "antigravity", Available: false, Unavailable: "agy not on PATH"},
	})
	if m.IsChecked("antigravity") {
		t.Error("antigravity unavailable should NOT be pre-checked")
	}
}

func TestPicker_DefaultFallbackWhenCodexUnavailable(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: false, Unavailable: "no binary"},
		{Name: "claude", Available: true},
		{Name: "antigravity", Available: false},
	})
	if m.IsChecked("codex") {
		t.Error("codex unavailable should not be checked")
	}
	if !m.IsChecked("claude") {
		t.Error("claude should be checked as fallback when codex unavailable")
	}
}

func TestPicker_SpaceTogglesAvailableItem(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
	})
	// cursor on row 0; uncheck codex
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.IsChecked("codex") {
		t.Error("space should have unchecked codex")
	}
}

func TestPicker_SpaceDoesNotToggleUnavailable(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "agy", Available: false},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.IsChecked("agy") {
		t.Error("unavailable items must never be checked")
	}
}

func TestPicker_DownArrowMovesCursor(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Toggle the second row.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	if m.IsChecked("claude") {
		t.Error("space on row 1 should have unchecked claude")
	}
}

func TestPicker_EnterConfirms(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.Confirmed() {
		t.Error("enter should set Confirmed=true")
	}
	if m.Cancelled() {
		t.Error("enter should not cancel")
	}
}

func TestPicker_QCancels(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
	})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if !m.Cancelled() {
		t.Error("q should set Cancelled=true")
	}
	if m.Confirmed() {
		t.Error("q should not confirm")
	}
}

func TestPicker_SelectedNamesReflectsChecks(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
	})
	// codex and claude both checked by default
	got := m.SelectedNames()
	if len(got) != 2 || got[0] != "codex" || got[1] != "claude" {
		t.Errorf("expected [codex claude]; got %v", got)
	}
	// Toggle codex off
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	got = m.SelectedNames()
	if len(got) != 1 || got[0] != "claude" {
		t.Errorf("expected [claude]; got %v", got)
	}
}

func TestPicker_ViewShowsLeadName(t *testing.T) {
	m := mkPicker([]ModelPickerItem{
		{Name: "codex", Available: true},
		{Name: "claude", Available: true},
	})
	v := m.View()
	if !strings.Contains(v, "Synthesis lead:") || !strings.Contains(v, "codex") {
		t.Errorf("View should announce codex as lead; got: %s", v)
	}
}
