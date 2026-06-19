package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/selyafi/diffsmith/internal/review"
)

// ParseError describes why a model's stdout couldn't be parsed.
// Kind narrows the cause for actionable error messages; Raw is the FULL
// offending output, retained untruncated so it can be surfaced for
// diagnosis (diffsmith-2xy). Error() prints only a bounded snippet of it;
// callers wanting the whole payload read Raw directly.
type ParseError struct {
	Kind  string // "prose_preamble" | "invalid_json" | "wrong_shape"
	Raw   string
	Cause error
}

func (e *ParseError) Error() string {
	msg := fmt.Sprintf("parse model output (%s)", e.Kind)
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	// Surface a bounded snippet of the raw output. Adapters wrap this
	// error with %w and the dropped-model run summary prints it with %v,
	// so without this a model that returned garbage leaves no trace of
	// what it actually said. The full payload stays on Raw.
	if e.Raw != "" {
		msg += fmt.Sprintf(" [raw: %s]", truncate(e.Raw, 200))
	}
	return msg
}

func (e *ParseError) Unwrap() error { return e.Cause }

// ParseFindings turns raw model stdout into review.FindingCandidate values.
//
// The contract from docs/prompt-contract.md is strict JSON, but reality
// is that models occasionally wrap the envelope in markdown code fences
// (agy) or chatty prose (claude's "Here is the review: ..."). Rather
// than rejecting these and failing the whole review, stripWrapper peels
// any outer wrapper before parsing — it slices to the outermost JSON
// object delimited by the first `{` and the last `}`. Any other
// deviation (no JSON at all, malformed JSON, missing findings key)
// still returns a categorized *ParseError so callers can surface a
// useful message.
func ParseFindings(raw []byte) ([]review.FindingCandidate, error) {
	trimmed := stripWrapper(string(raw))

	if !strings.HasPrefix(trimmed, "{") {
		return nil, &ParseError{Kind: "prose_preamble", Raw: trimmed}
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
			Raw:   trimmed,
			Cause: err,
		}
	}
	if envelope.Findings == nil {
		return nil, &ParseError{
			Kind: "wrong_shape",
			Raw:  trimmed,
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

// stripWrapper peels any outer wrapper from s and returns the inner
// JSON-looking slice. It handles three common forms of model drift:
//
//   - markdown fences:  ```json\n{...}\n```
//   - prose preamble:   Here is the review: {...}
//   - trailing prose:   {...}\n\nHope this helps!
//   - combinations:     Sure! ```json\n{...}\n``` Let me know.
//
// The strategy is intentionally aggressive: find the outermost {...}
// span by locating the first '{' and the last '}', then slice between
// them (inclusive). Well-behaved outputs that are already a clean JSON
// object pass through unchanged. Outputs with no '{' at all (e.g.
// model refused to respond) return whitespace-trimmed input so the
// caller's existing prose_preamble error still fires.
//
// Tradeoff: a malformed JSON body that contains stray '}' characters
// after the true closing brace will be sliced wrong, producing an
// invalid_json error instead of a more precise one. That's acceptable
// — the result is still a categorized failure with a truncated raw
// payload for debugging.
func stripWrapper(s string) string {
	s = strings.TrimSpace(s)
	first := strings.IndexByte(s, '{')
	last := strings.LastIndexByte(s, '}')
	if first < 0 || last <= first {
		return s
	}
	return s[first : last+1]
}
