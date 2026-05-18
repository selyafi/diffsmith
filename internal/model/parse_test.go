package model

import (
	"errors"
	"testing"
)

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

func TestParseFindingsRejectsMarkdownFence(t *testing.T) {
	raw := []byte("```json\n{\"findings\":[]}\n```")
	_, err := ParseFindings(raw)
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("want *ParseError, got %T: %v", err, err)
	}
	if pe.Kind != "markdown_fence" {
		t.Errorf("Kind: got %q, want markdown_fence", pe.Kind)
	}
}

func TestParseFindingsRejectsProsePreamble(t *testing.T) {
	raw := []byte("Here is the JSON:\n{\"findings\":[]}")
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
