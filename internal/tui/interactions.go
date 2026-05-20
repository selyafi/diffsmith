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

// MarkCurrentForPost flags the currently selected finding for upstream
// posting. Independent of State — does not promote a Pending finding to
// Approved, by design (the 'a' key remains the only path to approval).
func (m *Model) MarkCurrentForPost() {
	if m.CurrentFinding() == nil {
		return
	}
	if m.markedForPost == nil {
		m.markedForPost = make(map[int]bool)
	}
	m.markedForPost[m.selected] = true
}

// GetFindingsMarkedForPost returns findings the user explicitly intends
// to post upstream, in the original order they appeared in the model.
func (m *Model) GetFindingsMarkedForPost() []review.Finding {
	var out []review.Finding
	for i := range m.findings {
		if m.markedForPost[i] {
			out = append(out, m.findings[i])
		}
	}
	return out
}
