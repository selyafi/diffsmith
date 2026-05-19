package review

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
)

func loadIndex(t *testing.T, fixture string) *diff.Index {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "diffs", fixture)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	files, err := diff.Parse(string(raw))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return diff.NewIndex(files)
}

func validCandidate() FindingCandidate {
	return FindingCandidate{
		File:             "auth/session.go",
		Line:             13,
		Severity:         "high",
		Title:            "Token can accept expired session",
		Evidence:         "Clock-skew fallback bypasses expiry check.",
		SuggestedComment: "Should expiry remain mandatory here?",
		FixHint:          "Keep tolerance around comparison, not over expiry.",
		Confidence:       0.78,
	}
}

func TestValidateAcceptsValidCandidate(t *testing.T) {
	idx := loadIndex(t, "modified_simple.diff")
	ok, bad := Validate([]FindingCandidate{validCandidate()}, "codex", idx)

	if len(ok) != 1 {
		t.Fatalf("want 1 valid finding, got %d (bad=%d)", len(ok), len(bad))
	}
	f := ok[0]
	if f.Severity != SeverityHigh {
		t.Errorf("Severity: got %v, want SeverityHigh", f.Severity)
	}
	if f.Model != "codex" {
		t.Errorf("Model: got %q, want %q", f.Model, "codex")
	}
	if f.State != StatePending {
		t.Errorf("State: got %v, want StatePending", f.State)
	}
	if f.Confidence != 0.78 {
		t.Errorf("Confidence: got %v, want 0.78", f.Confidence)
	}
}

func TestValidateQuarantinesEverySchemaViolation(t *testing.T) {
	idx := loadIndex(t, "modified_simple.diff")

	cases := []struct {
		name           string
		mutate         func(*FindingCandidate)
		reasonContains string
	}{
		{"empty file", func(c *FindingCandidate) { c.File = "" }, "file is empty"},
		{"empty suggested_comment", func(c *FindingCandidate) { c.SuggestedComment = "" }, "suggested_comment is empty"},
		{"unknown severity", func(c *FindingCandidate) { c.Severity = "critical" }, "unknown severity"},
		{"confidence above 1", func(c *FindingCandidate) { c.Confidence = 1.5 }, "outside [0.0, 1.0]"},
		{"confidence negative", func(c *FindingCandidate) { c.Confidence = -0.1 }, "outside [0.0, 1.0]"},
		{"file not in diff", func(c *FindingCandidate) { c.File = "nope/missing.go" }, "not in the diff"},
		{"line on context", func(c *FindingCandidate) { c.Line = 10 }, "context line"},
		{"line outside hunk", func(c *FindingCandidate) { c.Line = 999 }, "outside any hunk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := validCandidate()
			tc.mutate(&c)
			ok, bad := Validate([]FindingCandidate{c}, "codex", idx)
			if len(ok) != 0 || len(bad) != 1 {
				t.Fatalf("want 0 ok, 1 bad; got %d ok, %d bad", len(ok), len(bad))
			}
			if !strings.Contains(bad[0].Reason, tc.reasonContains) {
				t.Errorf("reason %q does not contain %q", bad[0].Reason, tc.reasonContains)
			}
			if bad[0].Candidate != c {
				t.Errorf("Quarantined.Candidate must preserve the original candidate")
			}
		})
	}
}

func TestValidateMixesValidAndQuarantined(t *testing.T) {
	idx := loadIndex(t, "modified_simple.diff")
	good := validCandidate()
	bad := validCandidate()
	bad.Line = 10 // context line

	okFindings, quarantined := Validate([]FindingCandidate{good, bad}, "codex", idx)
	if len(okFindings) != 1 {
		t.Errorf("want 1 valid, got %d", len(okFindings))
	}
	if len(quarantined) != 1 {
		t.Errorf("want 1 quarantined, got %d", len(quarantined))
	}
}

func TestValidateBinaryFileIsUnaddressable(t *testing.T) {
	idx := loadIndex(t, "binary_change.diff")
	c := validCandidate()
	c.File = "assets/logo.png"
	c.Line = 1

	ok, bad := Validate([]FindingCandidate{c}, "codex", idx)
	if len(ok) != 0 || len(bad) != 1 {
		t.Fatalf("binary file should be unaddressable; got %d ok, %d bad", len(ok), len(bad))
	}
	if !strings.Contains(bad[0].Reason, "not addressable") && !strings.Contains(bad[0].Reason, "not in the diff") {
		t.Errorf("reason should explain unaddressable: %q", bad[0].Reason)
	}
}

func TestParseSeverityRoundTrip(t *testing.T) {
	for _, s := range []Severity{SeverityHigh, SeverityMedium, SeverityLow, SeveritySuggestion} {
		got, err := ParseSeverity(s.String())
		if err != nil {
			t.Errorf("ParseSeverity(%q): %v", s.String(), err)
		}
		if got != s {
			t.Errorf("round-trip: got %v from %q, want %v", got, s.String(), s)
		}
	}
}
