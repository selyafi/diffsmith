// Package review hosts the model-agnostic finding validator and the
// runtime Finding type used by the TUI.
//
// Validation enforces docs/review-finding-schema.md against the parsed
// diff produced by internal/diff. Rejected findings are quarantined with
// a reason rather than guessed at, per the v1 schema contract.
package review

import "fmt"

// Severity enumerates the four severity buckets v1 supports.
type Severity int

const (
	SeverityHigh Severity = iota
	SeverityMedium
	SeverityLow
	SeveritySuggestion
)

// String returns the canonical lowercase label, matching what the model
// emits in the JSON output.
func (s Severity) String() string {
	switch s {
	case SeverityHigh:
		return "high"
	case SeverityMedium:
		return "medium"
	case SeverityLow:
		return "low"
	case SeveritySuggestion:
		return "suggestion"
	default:
		return "unknown"
	}
}

// ParseSeverity maps a model-output severity string to the typed enum.
// Unknown strings return an error so the validator quarantines the
// candidate rather than guessing.
func ParseSeverity(s string) (Severity, error) {
	switch s {
	case "high":
		return SeverityHigh, nil
	case "medium":
		return SeverityMedium, nil
	case "low":
		return SeverityLow, nil
	case "suggestion":
		return SeveritySuggestion, nil
	default:
		return 0, fmt.Errorf("unknown severity %q", s)
	}
}

// FindingState is the TUI workflow state for a finding. Initial state on
// every fresh validation is StatePending; the TUI mutates state in M4.
type FindingState int

const (
	StatePending FindingState = iota
	StateApproved
	StateDismissed
)

// Finding is a validated review candidate ready for the TUI. Model and
// State are runtime metadata not present in the model's raw output.
type Finding struct {
	File             string
	Line             int
	Severity         Severity
	Title            string
	Evidence         string
	SuggestedComment string
	FixHint          string
	Confidence       float64
	Model            string
	State            FindingState
}

// Quarantined holds a finding that failed validation, with the reason
// preserved so the TUI's debug surface can explain why.
type Quarantined struct {
	Candidate FindingCandidate
	Reason    string
}
