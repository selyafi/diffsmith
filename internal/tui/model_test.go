package tui

import (
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestNewModelWithNoFindings verifies the TUI model initializes correctly
// when given zero findings (RED test: expects NewModel and View to exist).
func TestNewModelWithNoFindings(t *testing.T) {
	m := NewModel(nil)
	if m == nil {
		t.Fatal("NewModel returned nil")
	}

	view := m.View()
	if view == "" {
		t.Error("View should render some output even with zero findings")
	}
}

// TestNewModelWithValidFindings verifies the TUI model displays findings.
func TestNewModelWithValidFindings(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "main.go",
			Line:             42,
			Severity:         review.SeverityHigh,
			Title:            "Buffer overflow risk",
			Model:            "codex",
			Confidence:       0.85,
			SuggestedComment: "Use bounds checking",
		},
	}

	m := NewModel(findings)
	if m == nil {
		t.Fatal("NewModel returned nil")
	}

	view := m.View()
	if view == "" {
		t.Error("View should render findings")
	}
}
