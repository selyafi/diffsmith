package review

import (
	"fmt"
	"unicode/utf8"
)

// Budget caps on the context contribution to a reviewer prompt. The
// description and each linked-issue body count toward an adapter's input
// budget (DefaultInputBudgetBytes, 1 MiB); bounding them here guarantees
// enrichment can never be the reason a review busts the budget and fails.
const (
	MaxDescriptionBytes = 8 * 1024
	MaxIssueBodyBytes   = 8 * 1024
	MaxLinkedIssues     = 10
)

// CapContext bounds Description and AcceptanceCriteria in place and returns
// a human-readable note for every truncation or drop it performed (nil
// when nothing was capped). Context is bounded, never silently shrunk:
// callers surface the notes so the operator knows the model saw less than
// the source.
func (in *ReviewInput) CapContext() []string {
	var notes []string

	if n := len(in.Description); n > MaxDescriptionBytes {
		in.Description = truncateUTF8(in.Description, MaxDescriptionBytes)
		notes = append(notes, fmt.Sprintf("description truncated from %d to %d bytes", n, len(in.Description)))
	}

	if n := len(in.AcceptanceCriteria); n > MaxLinkedIssues {
		in.AcceptanceCriteria = in.AcceptanceCriteria[:MaxLinkedIssues]
		notes = append(notes, fmt.Sprintf("%d linked issue(s) beyond the first %d dropped", n-MaxLinkedIssues, MaxLinkedIssues))
	}

	for i := range in.AcceptanceCriteria {
		if n := len(in.AcceptanceCriteria[i].Body); n > MaxIssueBodyBytes {
			in.AcceptanceCriteria[i].Body = truncateUTF8(in.AcceptanceCriteria[i].Body, MaxIssueBodyBytes)
			notes = append(notes, fmt.Sprintf("issue #%d body truncated from %d to %d bytes", in.AcceptanceCriteria[i].Number, n, len(in.AcceptanceCriteria[i].Body)))
		}
	}

	return notes
}

// truncateUTF8 returns s capped to at most max bytes without splitting a
// multi-byte rune at the boundary.
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	t := s[:max]
	// Back off while the byte just past the cut is a UTF-8 continuation
	// byte, i.e. the cut landed inside a rune.
	for len(t) > 0 && !utf8.RuneStart(s[len(t)]) {
		t = t[:len(t)-1]
	}
	return t
}
