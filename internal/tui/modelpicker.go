package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ModelPickerItem represents one row in the picker.
type ModelPickerItem struct {
	Name        string
	Available   bool
	Unavailable string // reason shown when !Available
	checked     bool
}

// ModelPickerModel is the multi-select Bubble Tea screen shown at
// startup. After Update sees a terminal key, Confirmed() or
// Cancelled() returns true; the app layer reads SelectedNames() to
// build the SelectedModels for the session.
type ModelPickerModel struct {
	items     []ModelPickerItem
	cursor    int
	confirmed bool
	cancelled bool
}

// priority reflects the synthesis lead priority: codex > claude >
// gemini > antigravity. Used only for default checking and lead-name
// display. Antigravity sits last because it's an experimental stub
// (interactive OAuth, S8b) — Preflight will reject it, so it always
// renders unavailable.
var pickerPriority = map[string]int{
	"codex":       0,
	"claude":      1,
	"gemini":      2,
	"antigravity": 3,
}

// NewModelPickerModel constructs a picker with default checks applied:
// codex, claude, and gemini pre-checked if available; antigravity
// unchecked. If codex is unavailable, the highest-priority available
// model is pre-checked alone as a fallback.
func NewModelPickerModel(items []ModelPickerItem) *ModelPickerModel {
	// Copy so default-check mutations don't leak into the caller's slice.
	items = append([]ModelPickerItem(nil), items...)
	codexOK := false
	for i, it := range items {
		if it.Name == "codex" && it.Available {
			items[i].checked = true
			codexOK = true
		}
		if it.Name == "claude" && it.Available {
			items[i].checked = true
		}
		if it.Name == "gemini" && it.Available {
			items[i].checked = true
		}
	}
	if !codexOK {
		// Fallback: ensure the highest-priority available model is checked.
		best := -1
		bestPri := len(pickerPriority) + 1
		for i, it := range items {
			if !it.Available {
				continue
			}
			pri := pickerPriority[it.Name]
			if _, known := pickerPriority[it.Name]; !known {
				pri = len(pickerPriority)
			}
			if pri < bestPri {
				bestPri = pri
				best = i
			}
		}
		// Reset everything, then check the best one.
		for i := range items {
			items[i].checked = false
		}
		if best >= 0 {
			items[best].checked = true
		}
	}
	return &ModelPickerModel{items: items}
}

func (m *ModelPickerModel) Init() tea.Cmd { return nil }

func (m *ModelPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case " ", "space":
			if m.cursor < len(m.items) && m.items[m.cursor].Available {
				m.items[m.cursor].checked = !m.items[m.cursor].checked
			}
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		case "q", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *ModelPickerModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Select models for this session") + "\n\n")
	for i, it := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		box := "[ ]"
		if it.checked {
			box = "[✓]"
		} else if !it.Available {
			box = "[✗]"
		}
		status := "available"
		if !it.Available {
			status = "unavailable"
			if it.Unavailable != "" {
				status = "unavailable: " + it.Unavailable
			}
		}
		line := fmt.Sprintf("%s%s %s  (%s)", cursor, box, it.Name, status)
		if i == m.cursor {
			b.WriteString(rowSelectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n")
	if lead := m.leadName(); lead != "" {
		b.WriteString(labelStyle.Render("Synthesis lead: ") + lead + "\n\n")
	}
	b.WriteString(footerStyle.Render("space toggle  ·  enter confirm  ·  q cancel"))
	return b.String()
}

// leadName returns the highest-priority CHECKED model name, or "" if
// nothing is checked.
func (m *ModelPickerModel) leadName() string {
	best := ""
	bestPri := len(pickerPriority) + 1
	for _, it := range m.items {
		if !it.checked {
			continue
		}
		pri, known := pickerPriority[it.Name]
		if !known {
			pri = len(pickerPriority)
		}
		if pri < bestPri {
			bestPri = pri
			best = it.Name
		}
	}
	return best
}

// IsChecked reports whether the given name is currently checked.
func (m *ModelPickerModel) IsChecked(name string) bool {
	for _, it := range m.items {
		if it.Name == name {
			return it.checked
		}
	}
	return false
}

// SelectedNames returns the names of currently-checked items, in
// priority order (codex > claude > gemini > antigravity > others).
func (m *ModelPickerModel) SelectedNames() []string {
	names := []string{}
	for _, pname := range []string{"codex", "claude", "gemini", "antigravity"} {
		for _, it := range m.items {
			if it.Name == pname && it.checked {
				names = append(names, it.Name)
			}
		}
	}
	known := map[string]bool{"codex": true, "claude": true, "gemini": true, "antigravity": true}
	for _, it := range m.items {
		if it.checked && !known[it.Name] {
			names = append(names, it.Name)
		}
	}
	return names
}

// Confirmed returns true if the user pressed enter.
func (m *ModelPickerModel) Confirmed() bool { return m.confirmed }

// Cancelled returns true if the user pressed q / ctrl+c.
func (m *ModelPickerModel) Cancelled() bool { return m.cancelled }

