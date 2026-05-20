package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

func TestMarkCurrentForPost_FlagsOnlySelected(t *testing.T) {
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

	// Mark current (a.go), advance, mark current (b.go), skip c.go.
	m.MarkCurrentForPost()
	m.MoveDown()
	m.MarkCurrentForPost()

	marked := m.GetFindingsMarkedForPost()
	if len(marked) != 2 {
		t.Fatalf("got %d marked, want 2", len(marked))
	}
	if marked[0].File != "a.go" || marked[1].File != "b.go" {
		t.Errorf("marked order: got [%s, %s], want [a.go, b.go]", marked[0].File, marked[1].File)
	}
}

func TestMarkCurrentForPost_IsOrthogonalToApproveDismiss(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1},
	}
	m := NewModel(findings)

	// 'p' alone must not flip State — preserves the M5a clipboard
	// workflow where 'a' is the only path to StateApproved.
	m.MarkCurrentForPost()
	if got := m.CurrentFinding().State; got != review.StatePending {
		t.Errorf("MarkCurrentForPost should not change State; got %v, want StatePending", got)
	}
	if got := m.ApprovedCount(); got != 0 {
		t.Errorf("MarkCurrentForPost should not increment ApprovedCount; got %d", got)
	}
	if got := len(m.GetFindingsMarkedForPost()); got != 1 {
		t.Errorf("MarkCurrentForPost should still flag the finding; got %d marked", got)
	}
}
