package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/clipboard"
)

// copyToClipboard is the seam between Update and the OS clipboard. Tests
// swap it to capture the copied text without touching the real clipboard.
var copyToClipboard = clipboard.Copy

// Run launches the interactive Bubble Tea program for the given model and
// blocks until the user quits. The model is mutated in place; callers read
// approved findings via Model.GetApprovedFindings after Run returns.
func Run(m *Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	// Clear any transient status from the prior keypress so each
	// message lives for exactly one interaction. Handlers below can
	// set m.transientStatus again before this Update returns.
	m.transientStatus = ""

	// Edit mode: the textarea owns most key input. Only esc (cancel) and
	// ctrl+s (save) toggle back to normal mode.
	if m.editMode {
		switch key.String() {
		case "esc":
			m.exitEditMode(false)
			return m, nil
		case "ctrl+s":
			m.exitEditMode(true)
			return m, nil
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd
	}

	// Normal mode.
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "down", "j":
		m.MoveDown()
	case "up", "k":
		m.MoveUp()
	case "a":
		m.ApproveCurrent()
	case "d":
		m.DismissCurrent()
	case "c":
		if cur := m.CurrentFinding(); cur != nil {
			if err := copyToClipboard(cur.SuggestedComment); err != nil {
				// Surface the failure in the footer so the user
				// doesn't silently paste stale clipboard content
				// into a real review thread. Common cause on
				// Linux: neither xclip nor wl-copy installed.
				m.transientStatus = "Copy failed: " + err.Error()
			} else {
				m.transientStatus = "Copied to clipboard"
			}
		}
	case "p":
		m.MarkCurrentForPost()
	case "e":
		m.enterEditMode()
	}
	return m, nil
}
