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
	Kind  string // "prose_preamble" | "invalid_json" | "wrong_shape"
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
// The contract from docs/prompt-contract.md is strict JSON, but reality
// is that some CLIs (notably gemini) wrap their output in markdown code
// fences despite the prompt forbidding them. Rather than rejecting
// fenced output and failing the whole review, stripMarkdownFences
// peels the wrapper before parsing. Any other deviation (prose
// preamble, malformed JSON, missing findings key) still returns a
// categorized *ParseError so callers can surface a useful message.
func ParseFindings(raw []byte) ([]review.FindingCandidate, error) {
	trimmed := stripMarkdownFences(string(raw))

	if !strings.HasPrefix(trimmed, "{") {
		return nil, &ParseError{Kind: "prose_preamble", Raw: truncate(trimmed, 200)}
	}

	// Findings is a pointer so we can distinguish "key missing" (nil
	// pointer) from "key present, value []" (non-nil pointer to empty
	// slice). Without this, well-formed JSON like {"foo":"bar"} or
	// {"type":"result",...} would silently parse to an empty findings
	// slice and the adapter would report a successful zero-finding
	// review — the worst class of bug for a review tool.
	var envelope struct {
		Findings *[]review.FindingCandidate `json:"findings"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return nil, &ParseError{
			Kind:  "invalid_json",
			Raw:   truncate(trimmed, 200),
			Cause: err,
		}
	}
	if envelope.Findings == nil {
		return nil, &ParseError{
			Kind: "wrong_shape",
			Raw:  truncate(trimmed, 200),
		}
	}
	return *envelope.Findings, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// stripMarkdownFences removes a leading ```lang\n (or ```\n) and a
// trailing \n``` from s, if both are present. Returns the input
// whitespace-trimmed when no fences are detected, so well-behaved
// outputs pass through unchanged. Only the outermost fence pair is
// stripped — nested fences inside the JSON body are preserved.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Drop the opening fence line up to and including the first
	// newline. This handles both ```json\n and ```\n forms.
	if newline := strings.IndexByte(s, '\n'); newline >= 0 {
		s = s[newline+1:]
	} else {
		// Single-line fenced output (no newline) — just drop the
		// opening backticks and let later parsing decide.
		s = strings.TrimPrefix(s, "```")
	}
	s = strings.TrimRightFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' })
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
