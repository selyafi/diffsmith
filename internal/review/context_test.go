package review

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCapContextTruncatesLongDescription(t *testing.T) {
	in := &ReviewInput{Description: strings.Repeat("a", MaxDescriptionBytes+500)}
	notes := in.CapContext()
	if len(in.Description) > MaxDescriptionBytes {
		t.Errorf("description not capped: got %d bytes, want <= %d", len(in.Description), MaxDescriptionBytes)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "description truncated") {
		t.Errorf("want one description-truncation note, got %v", notes)
	}
}

func TestCapContextDropsExcessIssues(t *testing.T) {
	in := &ReviewInput{}
	for i := 0; i < MaxLinkedIssues+2; i++ {
		in.AcceptanceCriteria = append(in.AcceptanceCriteria, IssueContext{Number: i})
	}
	notes := in.CapContext()
	if len(in.AcceptanceCriteria) != MaxLinkedIssues {
		t.Errorf("issue count not capped: got %d, want %d", len(in.AcceptanceCriteria), MaxLinkedIssues)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "linked issue") {
		t.Errorf("want one issue-drop note, got %v", notes)
	}
}

func TestCapContextTruncatesIssueBody(t *testing.T) {
	in := &ReviewInput{AcceptanceCriteria: []IssueContext{
		{Number: 7, Body: strings.Repeat("b", MaxIssueBodyBytes+10)},
	}}
	notes := in.CapContext()
	if len(in.AcceptanceCriteria[0].Body) > MaxIssueBodyBytes {
		t.Errorf("issue body not capped: got %d, want <= %d", len(in.AcceptanceCriteria[0].Body), MaxIssueBodyBytes)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "#7 body truncated") {
		t.Errorf("want one issue-body-truncation note, got %v", notes)
	}
}

func TestCapContextNoNotesWhenWithinLimits(t *testing.T) {
	in := &ReviewInput{
		Description:        "short",
		AcceptanceCriteria: []IssueContext{{Number: 1, Body: "tiny"}},
	}
	if notes := in.CapContext(); len(notes) != 0 {
		t.Errorf("want no notes for in-budget context, got %v", notes)
	}
}

func TestTruncateUTF8DoesNotSplitRune(t *testing.T) {
	// "世" is 3 bytes; cutting at 4 bytes must back off to a rune boundary.
	s := strings.Repeat("世", 3) // 9 bytes
	got := truncateUTF8(s, 4)
	if !utf8.ValidString(got) {
		t.Errorf("truncateUTF8 split a rune: %q is not valid UTF-8", got)
	}
	if len(got) > 4 {
		t.Errorf("truncateUTF8 exceeded max: got %d bytes", len(got))
	}
}
