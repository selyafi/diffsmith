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

// DismissCurrent marks the currently selected finding as dismissed and
// drops it from the post-bound set. Without the second step a sequence
// like a→p→d→a would silently resurrect the post mark on re-approve
// (markedForPost[i] was never cleared), bypassing the rule that only
// an explicit 'p' on an approved finding enqueues it for posting.
func (m *Model) DismissCurrent() {
	if current := m.CurrentFinding(); current != nil {
		current.State = review.StateDismissed
		delete(m.markedForPost, m.selected)
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
// posting. Only approved findings are eligible: pressing 'p' on a
// Pending or Dismissed finding leaves the marked set untouched and sets
// a transient status explaining what to do. 'a' remains the only path
// to approval — this keeps the trust boundary explicit so a user can
// never post a finding they have not first approved.
func (m *Model) MarkCurrentForPost() {
	cur := m.CurrentFinding()
	if cur == nil {
		return
	}
	switch cur.State {
	case review.StatePending:
		m.transientStatus = "Approve first (press 'a'); only approved findings can be posted."
		return
	case review.StateDismissed:
		m.transientStatus = "Dismissed findings can't be posted."
		return
	}
	if m.markedForPost == nil {
		m.markedForPost = make(map[int]bool)
	}
	m.markedForPost[m.selected] = true
}

// IsPostBoundRow reports whether the finding at index i is currently
// in the post-bound set. View renderers consult this to draw the
// '[post]' badge; it must match the semantics of GetFindingsMarkedForPost
// so the badge can never claim a finding is queued for posting that
// the filter will silently drop.
func (m *Model) IsPostBoundRow(i int) bool {
	if i < 0 || i >= len(m.findings) {
		return false
	}
	if !m.markedForPost[i] {
		return false
	}
	return m.findings[i].State == review.StateApproved
}

// GetFindingsMarkedForPost returns findings the user explicitly intends
// to post upstream, in the original order they appeared in the model.
// Only currently-approved findings are returned — a finding that was
// approved, marked, then later dismissed is filtered out so the dismiss
// always wins. Callers (the confirmation prompt count, the upstream
// submitter) can rely on this as a single source of truth.
func (m *Model) GetFindingsMarkedForPost() []review.Finding {
	var out []review.Finding
	for i := range m.findings {
		if !m.markedForPost[i] {
			continue
		}
		if m.findings[i].State != review.StateApproved {
			continue
		}
		out = append(out, m.findings[i])
	}
	return out
}
