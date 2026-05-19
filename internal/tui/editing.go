package tui

// EditCurrentTitle updates the title of the currently selected finding.
func (m *Model) EditCurrentTitle(newTitle string) {
	if current := m.CurrentFinding(); current != nil {
		current.Title = newTitle
	}
}

// EditCurrentComment updates the suggested comment of the currently selected finding.
func (m *Model) EditCurrentComment(newComment string) {
	if current := m.CurrentFinding(); current != nil {
		current.SuggestedComment = newComment
	}
}

// EditCurrentEvidence updates the evidence of the currently selected finding.
func (m *Model) EditCurrentEvidence(newEvidence string) {
	if current := m.CurrentFinding(); current != nil {
		current.Evidence = newEvidence
	}
}

// EditCurrentFixHint updates the fix hint of the currently selected finding.
func (m *Model) EditCurrentFixHint(newFixHint string) {
	if current := m.CurrentFinding(); current != nil {
		current.FixHint = newFixHint
	}
}
