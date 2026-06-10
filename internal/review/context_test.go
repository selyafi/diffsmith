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
	cases := []struct {
		name string
		s    string
		max  int
	}{
		// "世" is 3 bytes; cutting at 4 must back off to a rune boundary.
		{"3-byte rune", strings.Repeat("世", 3), 4},
		// "🙂" is 4 bytes; cutting at 5 must back off to a rune boundary.
		{"4-byte rune (emoji)", strings.Repeat("🙂", 3), 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateUTF8(tc.s, tc.max)
			if !utf8.ValidString(got) {
				t.Errorf("truncateUTF8 split a rune: %q is not valid UTF-8", got)
			}
			if len(got) > tc.max {
				t.Errorf("truncateUTF8 exceeded max: got %d bytes, want <= %d", len(got), tc.max)
			}
		})
	}
}

func TestTruncateUTF8MalformedInput(t *testing.T) {
	// All continuation bytes: the rune-boundary back-off trims to "".
	// Documents that truncateUTF8 prefers a valid (empty) result over
	// emitting a partial/invalid rune for malformed input.
	if got := truncateUTF8("\x80\x81\x82\x83", 3); got != "" {
		t.Errorf("malformed input: got %q, want empty string", got)
	}
}

func TestCapContextKeepsExactlyMaxIssues(t *testing.T) {
	in := &ReviewInput{}
	for i := 0; i < MaxLinkedIssues; i++ {
		in.AcceptanceCriteria = append(in.AcceptanceCriteria, IssueContext{Number: i})
	}
	notes := in.CapContext()
	if len(in.AcceptanceCriteria) != MaxLinkedIssues {
		t.Errorf("exactly MaxLinkedIssues must be kept, got %d", len(in.AcceptanceCriteria))
	}
	if len(notes) != 0 {
		t.Errorf("no drop note expected at exactly the cap, got %v", notes)
	}
}
