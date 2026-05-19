package tui

import "github.com/selyafi/diffsmith/internal/review"

// CurrentFinding returns the currently selected finding, or nil if none.
func (m *Model) CurrentFinding() *review.Finding {
	if m == nil || len(m.findings) == 0 {
		return nil
	}
	if m.selected < 0 || m.selected >= len(m.findings) {
		return nil
	}
	return &m.findings[m.selected]
}

// MoveUp moves the selection up (towards the first finding).
func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
	}
}

// MoveDown moves the selection down (towards the last finding).
func (m *Model) MoveDown() {
	if m.selected < len(m.findings)-1 {
		m.selected++
	}
}

// ApproveCurrent marks the currently selected finding as approved.
func (m *Model) ApproveCurrent() {
	if current := m.CurrentFinding(); current != nil {
		current.State = review.StateApproved
	}
}

// DismissCurrent marks the currently selected finding as dismissed.
func (m *Model) DismissCurrent() {
	if current := m.CurrentFinding(); current != nil {
		current.State = review.StateDismissed
	}
}

// ApprovedCount returns the number of approved findings.
func (m *Model) ApprovedCount() int {
	count := 0
	for i := range m.findings {
		if m.findings[i].State == review.StateApproved {
			count++
		}
	}
	return count
}

// DismissedCount returns the number of dismissed findings.
func (m *Model) DismissedCount() int {
	count := 0
	for i := range m.findings {
		if m.findings[i].State == review.StateDismissed {
			count++
		}
	}
	return count
}

// GetApprovedFindings returns a slice of all approved findings in order.
func (m *Model) GetApprovedFindings() []review.Finding {
	var approved []review.Finding
	for i := range m.findings {
		if m.findings[i].State == review.StateApproved {
			approved = append(approved, m.findings[i])
		}
	}
	return approved
}
