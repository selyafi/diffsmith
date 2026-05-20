// Package tui implements the interactive TUI for reviewing findings
// using the Bubble Tea framework.
package tui

import (
	"fmt"

	"github.com/selyafi/diffsmith/internal/review"
)

// Model is the Bubble Tea model representing the TUI state and findings.
//
// markedForPost is a TUI-local intent set kept out of review.Finding so
// the validated finding contract stays unchanged. Approval (State) and
// post-intent are orthogonal: the user can approve without posting (for
// the M5a clipboard workflow), post without approving, or both.
type Model struct {
	findings      []review.Finding
	selected      int
	markedForPost map[int]bool
}

// NewModel constructs a TUI Model with the given findings.
func NewModel(findings []review.Finding) *Model {
	return &Model{findings: findings}
}

// View renders the TUI as a string.
func (m *Model) View() string {
	if len(m.findings) == 0 {
		return "No findings to review.\n"
	}

	var out string
	out += fmt.Sprintf("Diffsmith Review (%d findings)\n", len(m.findings))
	out += "──────────────────────────────\n\n"

	for i, f := range m.findings {
		marker := " "
		if i == m.selected {
			marker = ">"
		}
		out += fmt.Sprintf("%s [%d] %s:%d\n", marker, i+1, f.File, f.Line)
		out += fmt.Sprintf("  %s (%s, %.0f%%)\n", f.Title, f.Severity, f.Confidence*100)
		out += "\n"
	}

	out += "──────────────────────────────\n"
	out += "Navigate: ↑↓  |  Select: Enter  |  Quit: q\n"

	return out
}
