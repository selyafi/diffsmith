package model

import (
	"errors"
	"strings"
	"testing"
)

// TestParseErrorPreservesFullRawOutput is diffsmith-2xy: when a model
// emits unparseable output, the FULL raw payload must be retained (not
// truncated) so it can be surfaced for diagnosis. Previously Raw was
// clipped to 200 chars, discarding the rest of the evidence.
func TestParseErrorPreservesFullRawOutput(t *testing.T) {
	raw := []byte(`{"findings":[` + strings.Repeat("x", 400) + `TAIL_MARKER`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if !strings.Contains(pe.Raw, "TAIL_MARKER") {
		t.Errorf("Raw must preserve the full output (len %d); the tail past 200 chars was dropped", len(pe.Raw))
	}
}

// TestParseErrorMessageIncludesRawSnippet ensures the error STRING carries
// a snippet of the offending output. Adapters wrap this error with %w and
// the dropped-model run summary prints it with %v, so without a snippet a
// user whose model returned garbage sees no bytes of that garbage.
func TestParseErrorMessageIncludesRawSnippet(t *testing.T) {
	raw := []byte(`{"oops": SNIPPET_MARKER not valid json`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if !strings.Contains(pe.Error(), "SNIPPET_MARKER") {
		t.Errorf("Error() should include a raw snippet for diagnosis; got %q", pe.Error())
	}
}

func TestParseFindingsHappy(t *testing.T) {
	raw := []byte(`{
		"findings": [
			{
				"file": "auth/session.go",
				"line": 13,
				"severity": "high",
				"title": "Token can accept expired session",
				"evidence": "Clock-skew fallback bypasses expiry check.",
				"suggested_comment": "Should expiry remain mandatory here?",
				"fix_hint": "Keep tolerance around comparison, not over expiry.",
				"confidence": 0.78
			}
		]
	}`)
	fs, err := ParseFindings(raw)
	if err != nil {
		t.Fatalf("ParseFindings: %v", err)
	}
	if len(fs) != 1 {
		t.Fatalf("want 1 finding, got %d", len(fs))
	}
	got := fs[0]
	if got.File != "auth/session.go" || got.Line != 13 || got.Severity != "high" {
		t.Errorf("decoded wrong: %+v", got)
	}
	if got.Confidence != 0.78 {
		t.Errorf("Confidence: got %v, want 0.78", got.Confidence)
	}
}

func TestParseFindingsEmpty(t *testing.T) {
	fs, err := ParseFindings([]byte(`{"findings":[]}`))
	if err != nil {
		t.Errorf("empty findings should not error: %v", err)
	}
	if len(fs) != 0 {
		t.Errorf("want 0 findings, got %d", len(fs))
	}
}

// TestParseFindingsStripsJSONFence verifies that the most common gemini
// drift — wrapping the JSON envelope in a ```json ... ``` block —
// is silently stripped and parsed. Without this, gemini reviews fail
// even when the underlying JSON is structurally valid.
func TestParseFindingsStripsJSONFence(t *testing.T) {
	raw := []byte("```json\n{\"findings\":[]}\n```")
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatalf("ParseFindings should strip ```json fence; got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d findings, want 0 (empty envelope)", len(got))
	}
}

// TestParseFindingsStripsBareFence verifies that a fence without a
// language tag — ```\n...\n``` — is also stripped. Some models emit
// this form when the prompt forbids any language tag.
func TestParseFindingsStripsBareFence(t *testing.T) {
	raw := []byte("```\n{\"findings\":[{\"file\":\"a.go\",\"line\":1,\"severity\":\"low\",\"title\":\"t\",\"evidence\":\"e\",\"suggested_comment\":\"c\",\"fix_hint\":\"f\",\"confidence\":0.5}]}\n```")
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatalf("ParseFindings should strip bare ``` fence; got: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1", len(got))
	}
}

// TestParseFindingsStripsFenceWithTrailingWhitespace covers the case
// where the model adds whitespace after the closing fence.
func TestParseFindingsStripsFenceWithTrailingWhitespace(t *testing.T) {
	raw := []byte("```json\n{\"findings\":[]}\n```\n\n  ")
	if _, err := ParseFindings(raw); err != nil {
		t.Fatalf("ParseFindings should tolerate trailing whitespace after fence; got: %v", err)
	}
}

// TestParseFindingsStripsProsePreamble verifies that prose before the
// JSON envelope is stripped (covers claude's occasional "Here is my
// analysis: {...}" drift).
func TestParseFindingsStripsProsePreamble(t *testing.T) {
	raw := []byte("Here is the JSON:\n{\"findings\":[]}")
	got, err := ParseFindings(raw)
	if err != nil {
		t.Fatalf("ParseFindings should strip prose preamble; got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d findings, want 0 (empty envelope)", len(got))
	}
}

// TestParseFindingsStripsTrailingProse verifies that prose after the
// JSON envelope (e.g. "Hope this helps!") is also stripped.
func TestParseFindingsStripsTrailingProse(t *testing.T) {
	raw := []byte("{\"findings\":[]}\n\nLet me know if you'd like more.")
	if _, err := ParseFindings(raw); err != nil {
		t.Fatalf("ParseFindings should strip trailing prose; got: %v", err)
	}
}

// TestParseFindingsStripsCombinedFenceAndProse verifies that a model
// can wrap the envelope in BOTH prose and a code fence and still parse.
func TestParseFindingsStripsCombinedFenceAndProse(t *testing.T) {
	raw := []byte("Sure! Here is the review:\n```json\n{\"findings\":[]}\n```\nLet me know.")
	if _, err := ParseFindings(raw); err != nil {
		t.Fatalf("ParseFindings should strip fence + prose combination; got: %v", err)
	}
}

// TestParseFindingsRejectsOutputWithoutJSON verifies that prose_preamble
// still fires for output that contains no JSON envelope at all (the
// strip is bounded — it can only peel JSON when JSON is present).
func TestParseFindingsRejectsOutputWithoutJSON(t *testing.T) {
	raw := []byte("I refuse to review this code.")
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "prose_preamble" {
		t.Errorf("Kind: got %q, want prose_preamble", pe.Kind)
	}
}

func TestParseFindingsRejectsMalformedJSON(t *testing.T) {
	raw := []byte(`{"findings": [{`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "invalid_json" {
		t.Errorf("Kind: got %q, want invalid_json", pe.Kind)
	}
	if pe.Cause == nil {
		t.Error("invalid_json should carry the json.Unmarshal cause")
	}
}

func TestParseFindingsToleratesLeadingWhitespace(t *testing.T) {
	raw := []byte("\n\n  {\"findings\":[]}\n")
	if _, err := ParseFindings(raw); err != nil {
		t.Errorf("leading whitespace should be tolerated: %v", err)
	}
}

// TestParseFindingsRejectsMissingFindingsKey guards against silent failure
// when the model emits a structurally different JSON object. Without this
// check, json.Unmarshal happily produces a zero-value Findings slice and
// the adapter reports zero findings as if the review succeeded.
func TestParseFindingsRejectsMissingFindingsKey(t *testing.T) {
	raw := []byte(`{"foo":"bar"}`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "wrong_shape" {
		t.Errorf("Kind: got %q, want wrong_shape", pe.Kind)
	}
}

// TestParseFindingsRejectsClaudeEnvelope guards against the specific
// real-world shape that motivated this hardening (diffsmith-e2w):
// `claude --output-format=json` emits result-event records. The flag
// fix in diffsmith-e2w prevents this output from reaching the parser
// today, but if a future adapter regresses on the flag the parser must
// not silently swallow the failure.
func TestParseFindingsRejectsClaudeEnvelope(t *testing.T) {
	raw := []byte(`{"type":"result","subtype":"success","result":"{\"findings\":[]}","session_id":"abc"}`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "wrong_shape" {
		t.Errorf("Kind: got %q, want wrong_shape", pe.Kind)
	}
}

// TestParseFindingsRejectsAlternativeKey guards against prompt drift or
// model behavior change in which the model returns the right structure
// under a different top-level key (e.g. "comments" instead of "findings").
func TestParseFindingsRejectsAlternativeKey(t *testing.T) {
	raw := []byte(`{"comments":[{"file":"x.go","line":1,"severity":"low"}]}`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "wrong_shape" {
		t.Errorf("Kind: got %q, want wrong_shape", pe.Kind)
	}
}

// TestParseFindingsRejectsNullFindings guards against an explicit JSON
// null where the array is expected. null is not the same as [] — it
// signals a model that built the object incorrectly, not an empty review.
func TestParseFindingsRejectsNullFindings(t *testing.T) {
	raw := []byte(`{"findings":null}`)
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "wrong_shape" {
		t.Errorf("Kind: got %q, want wrong_shape", pe.Kind)
	}
}
