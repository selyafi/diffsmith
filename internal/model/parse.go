package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/selyafi/diffsmith/internal/review"
)

// ParseError describes why a model's stdout couldn't be parsed.
// Kind narrows the cause for actionable error messages; Raw is a
// truncated copy of the offending output for debug surfaces.
type ParseError struct {
	Kind  string // "markdown_fence" | "prose_preamble" | "invalid_json"
	Raw   string
	Cause error
}

func (e *ParseError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("parse model output (%s): %v", e.Kind, e.Cause)
	}
	return fmt.Sprintf("parse model output (%s)", e.Kind)
}

func (e *ParseError) Unwrap() error { return e.Cause }

// ParseFindings turns raw model stdout into review.FindingCandidate values.
//
// The contract from docs/prompt-contract.md is strict: JSON only, no
// markdown fences, no prose preamble. Violations return *ParseError so
// callers can surface a categorized message instead of a stack trace.
func ParseFindings(raw []byte) ([]review.FindingCandidate, error) {
	trimmed := strings.TrimSpace(string(raw))

	if strings.HasPrefix(trimmed, "```") {
		return nil, &ParseError{Kind: "markdown_fence", Raw: truncate(trimmed, 200)}
	}
	if !strings.HasPrefix(trimmed, "{") {
		return nil, &ParseError{Kind: "prose_preamble", Raw: truncate(trimmed, 200)}
	}

	var envelope struct {
		Findings []review.FindingCandidate `json:"findings"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, &ParseError{
			Kind:  "invalid_json",
			Raw:   truncate(trimmed, 200),
			Cause: err,
		}
	}
	return envelope.Findings, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
