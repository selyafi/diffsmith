package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/review"
)

// TestProgramRunRendersAndQuits drives a real tea.Program with a synthetic
// reader/writer instead of a TTY. This is the integration test that actually
// exercises tea.NewProgram.Run(), the program loop, View() rendering, and the
// parsed-keypress path through Update — pieces the unit tests stub out.
func TestProgramRunRendersAndQuits(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "auth.go", Line: 42, Severity: review.SeverityHigh, Title: "Buffer overflow risk", Model: "test", Confidence: 0.8},
	})

	var out bytes.Buffer
	p := tea.NewProgram(m,
		tea.WithInput(strings.NewReader("q")),
		tea.WithOutput(&out),
	)

	done := make(chan error, 1)
	go func() {
		_, err := p.Run()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Program.Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		p.Kill()
		t.Fatal("Program.Run did not terminate within 2s after 'q' input")
	}

	rendered := out.String()
	if !strings.Contains(rendered, "Buffer overflow risk") {
		t.Errorf("View output should include finding title; got:\n%s", rendered)
	}
}

// TestInitReturnsNilCmd verifies Init returns no initial command.
func TestInitReturnsNilCmd(t *testing.T) {
	m := NewModel([]review.Finding{
		{File: "test.go", Line: 1, Severity: review.SeverityHigh, Title: "Test", Model: "test", Confidence: 0.5},
	})
	if cmd := m.Init(); cmd != nil {
		t.Errorf("Init() should return nil, got %v", cmd)
	}
}

// TestGetFindingsForOutput verifies extracting findings ready for output/posting.
func TestGetFindingsForOutput(t *testing.T) {
	findings := []review.Finding{
		{
			File:             "a.go",
			Line:             1,
			Severity:         review.SeverityHigh,
			Title:            "First",
			Model:            "test",
			Confidence:       0.9,
			SuggestedComment: "Comment A",
			State:            review.StatePending,
		},
		{
			File:             "b.go",
			Line:             2,
			Severity:         review.SeverityMedium,
			Title:            "Second",
			Model:            "test",
			Confidence:       0.7,
			SuggestedComment: "Comment B",
			State:            review.StatePending,
		},
		{
			File:             "c.go",
			Line:             3,
			Severity:         review.SeverityLow,
			Title:            "Third",
			Model:            "test",
			Confidence:       0.6,
			SuggestedComment: "Comment C",
			State:            review.StatePending,
		},
	}
	m := NewModel(findings)

	// Approve first and third
	m.ApproveCurrent()
	m.MoveDown()
	m.MoveDown()
	m.ApproveCurrent()

	// Get approved findings
	approved := m.GetApprovedFindings()
	if len(approved) != 2 {
		t.Fatalf("expected 2 approved findings, got %d", len(approved))
	}

	if approved[0].File != "a.go" || approved[1].File != "c.go" {
		t.Error("approved findings should be in original order")
	}

	// Verify SuggestedComment is present (for posting)
	if approved[0].SuggestedComment != "Comment A" {
		t.Error("approved findings should retain their suggested comments")
	}
}
