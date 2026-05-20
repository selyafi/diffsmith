package tui

import (
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
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
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
				_ = copyToClipboard(cur.SuggestedComment)
			}
		case "p":
			m.MarkCurrentForPost()
		}
	}
	return m, nil
}
