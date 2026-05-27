package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

func TestMarkCurrentForPost_FlagsOnlyApprovedAndSelected(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1, Title: "First"},
		{File: "b.go", Line: 2, Title: "Second"},
		{File: "c.go", Line: 3, Title: "Third"},
	}
	m := NewModel(findings)

	// Default: no findings are marked.
	if got := m.GetFindingsMarkedForPost(); len(got) != 0 {
		t.Errorf("default marked set should be empty, got %d", len(got))
	}

	// Approve + mark (a.go), advance, approve + mark (b.go), skip c.go.
	m.ApproveCurrent()
	m.MarkCurrentForPost()
	m.MoveDown()
	m.ApproveCurrent()
	m.MarkCurrentForPost()

	marked := m.GetFindingsMarkedForPost()
	if len(marked) != 2 {
		t.Fatalf("got %d marked, want 2", len(marked))
	}
	if marked[0].File != "a.go" || marked[1].File != "b.go" {
		t.Errorf("marked order: got [%s, %s], want [a.go, b.go]", marked[0].File, marked[1].File)
	}
}

// TestMarkCurrentForPost_RejectsPendingFindings asserts the trust
// boundary: pressing 'p' on a Pending finding does NOT add it to the
// post-bound set. The user has not approved the finding yet, so it must
// not be eligible for upstream posting. The user gets a transient status
// telling them what to do.
func TestMarkCurrentForPost_RejectsPendingFindings(t *testing.T) {
	m := NewModel([]review.Finding{{File: "a.go", Line: 1}})

	m.MarkCurrentForPost()

	if got := m.CurrentFinding().State; got != review.StatePending {
		t.Errorf("State must remain Pending; got %v", got)
	}
	if got := len(m.GetFindingsMarkedForPost()); got != 0 {
		t.Errorf("a Pending finding must not be marked for post; got %d marked", got)
	}
	if m.TransientStatus() == "" {
		t.Errorf("user must see a status message explaining why 'p' did nothing")
	}
}

// TestMarkCurrentForPost_RejectsDismissedFindings asserts that pressing
// 'p' on a Dismissed finding never enters the post-bound set. Posting a
// dismissed finding upstream would violate the reviewer's stated intent.
func TestMarkCurrentForPost_RejectsDismissedFindings(t *testing.T) {
	m := NewModel([]review.Finding{{File: "a.go", Line: 1}})
	m.DismissCurrent()

	m.MarkCurrentForPost()

	if got := m.CurrentFinding().State; got != review.StateDismissed {
		t.Errorf("State must remain Dismissed; got %v", got)
	}
	if got := len(m.GetFindingsMarkedForPost()); got != 0 {
		t.Errorf("a Dismissed finding must not be marked for post; got %d marked", got)
	}
	if m.TransientStatus() == "" {
		t.Errorf("user must see a status message explaining why 'p' did nothing")
	}
}

// TestGetFindingsMarkedForPost_ExcludesFindingsDismissedAfterMarking is
// the defence-in-depth check: even if a finding was approved + marked,
// a subsequent dismiss must remove it from the postable set. Without
// this, the sequence 'a, p, d' would leave a dismissed finding silently
// queued for upstream posting.
func TestGetFindingsMarkedForPost_ExcludesFindingsDismissedAfterMarking(t *testing.T) {
	m := NewModel([]review.Finding{{File: "a.go", Line: 1}})
	m.ApproveCurrent()
	m.MarkCurrentForPost()
	if got := len(m.GetFindingsMarkedForPost()); got != 1 {
		t.Fatalf("setup: expected 1 marked after approve+mark, got %d", got)
	}

	m.DismissCurrent()

	if got := len(m.GetFindingsMarkedForPost()); got != 0 {
		t.Errorf("dismissing a previously-marked finding must drop it from the postable set; got %d", got)
	}
}

// TestIsPostBoundRow_MatchesGetFindingsMarkedForPostContract is the
// view-vs-model consistency check: the row-level predicate the view
// renders the '[post]' badge from must return false for findings that
// GetFindingsMarkedForPost would NOT post. Otherwise the badge lies to
// the user (claims a finding is queued for posting when the filter
// will drop it).
func TestIsPostBoundRow_MatchesGetFindingsMarkedForPostContract(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "approved.go"},
		{File: "dismissed-after-mark.go"},
		{File: "pending.go"},
	})

	// Row 0: approved + marked → badge on.
	m.ApproveCurrent()
	m.MarkCurrentForPost()
	if !m.IsPostBoundRow(0) {
		t.Errorf("row 0 (approved+marked) must be post-bound")
	}

	// Row 1: approved, marked, then dismissed. With dvz follow-up
	// fix #1 the mark is cleared; IsPostBoundRow must return false
	// either way (dismissed state alone is enough).
	m.MoveDown()
	m.ApproveCurrent()
	m.MarkCurrentForPost()
	m.DismissCurrent()
	if m.IsPostBoundRow(1) {
		t.Errorf("row 1 (dismissed) must not be post-bound — badge would lie about postability")
	}

	// Row 2: pending, never marked → badge off.
	m.MoveDown()
	if m.IsPostBoundRow(2) {
		t.Errorf("row 2 (pending) must not be post-bound")
	}

	// Out-of-range indices must not panic and must return false.
	if m.IsPostBoundRow(-1) || m.IsPostBoundRow(99) {
		t.Errorf("out-of-range indices must return false, not panic")
	}
}

// TestDismissCurrent_ClearsMarkedForPostSoReApproveDoesNotResurrect
// pins the trust boundary against the a→p→d→a ghost-mark scenario.
// The sequence: approve, mark, dismiss, then re-approve. Without
// clearing markedForPost at dismiss time, the re-approve silently
// re-enables posting because the State filter no longer excludes the
// (still-marked) finding. The user never re-pressed 'p' — that breaks
// the documented 'only an explicit p posts' contract.
func TestDismissCurrent_ClearsMarkedForPostSoReApproveDoesNotResurrect(t *testing.T) {
	m := NewModel([]review.Finding{{File: "a.go", Line: 1}})

	m.ApproveCurrent()
	m.MarkCurrentForPost()
	m.DismissCurrent()
	m.ApproveCurrent()

	if got := len(m.GetFindingsMarkedForPost()); got != 0 {
		t.Errorf("re-approving a previously dismissed finding must NOT silently restore the post mark; got %d marked — user must press 'p' again", got)
	}
}
