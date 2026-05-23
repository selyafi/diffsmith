// Package tui implements the interactive TUI for reviewing findings
// using the Bubble Tea framework.
package tui

import (
	"github.com/charmbracelet/bubbles/textarea"

	"github.com/selyafi/diffsmith/internal/review"
)

// Model is the Bubble Tea model representing the TUI state and findings.
//
// markedForPost is a TUI-local intent set kept out of review.Finding so
// the validated finding contract stays unchanged. Approval (State) and
// post-intent are orthogonal: the user can approve without posting (for
// the M5a clipboard workflow), post without approving, or both.
//
// editMode + editor implement F9's "edit the suggested comment" contract
// (diffsmith-axv). When editMode is true the textarea owns key input; the
// normal-mode key bindings resume on exit.
type Model struct {
	findings      []review.Finding
	selected      int
	markedForPost map[int]bool

	editMode bool
	editor   textarea.Model
}

// NewModel constructs a TUI Model with the given findings.
func NewModel(findings []review.Finding) *Model {
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 5000
	ta.SetWidth(58)
	ta.SetHeight(8)
	return &Model{findings: findings, editor: ta}
}

// EditMode reports whether the model is currently in edit mode. Tests
// and the View consult this to gate behavior.
func (m *Model) EditMode() bool { return m.editMode }

// enterEditMode loads the current finding's suggested_comment into the
// textarea and focuses it. No-op if no finding is selected.
func (m *Model) enterEditMode() {
	cur := m.CurrentFinding()
	if cur == nil {
		return
	}
	m.editor.SetValue(cur.SuggestedComment)
	m.editor.Focus()
	m.editMode = true
}

// exitEditMode leaves edit mode. If save is true the textarea's current
// value replaces the finding's SuggestedComment; otherwise the change is
// discarded.
func (m *Model) exitEditMode(save bool) {
	if save {
		if cur := m.CurrentFinding(); cur != nil {
			cur.SuggestedComment = m.editor.Value()
		}
	}
	m.editor.Blur()
	m.editMode = false
}
