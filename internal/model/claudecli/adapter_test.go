package claudecli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/review"
)

// TestNew verifies the constructor creates an adapter with defaults.
func TestNew(t *testing.T) {
	a := New(nil)
	if a == nil {
		t.Fatal("New(nil) returned nil")
	}
}

// TestName verifies the adapter identifies as "claude".
func TestName(t *testing.T) {
	a := New(nil)
	if got := a.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

// TestPreflightSuccess verifies Preflight succeeds when claude binary exists.
func TestPreflightSuccess(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		if name != "claude" {
			t.Errorf("lookPath called with %q, want %q", name, "claude")
		}
		return "/usr/bin/claude", nil
	}

	err := a.Preflight(context.Background())
	if err != nil {
		t.Errorf("Preflight() = %v, want nil", err)
	}
}

// TestPreflightMissing verifies Preflight fails with actionable error when binary is missing.
func TestPreflightMissing(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		return "", errors.New("not found")
	}

	err := a.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight() = nil, want error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("claude")) {
		t.Errorf("error doesn't mention claude: %v", err)
	}
}

// TestReviewSuccess verifies Review invokes the CLI and parses JSON output.
func TestReviewSuccess(t *testing.T) {
	input := &review.ReviewInput{
		Target: review.ReviewTarget{URL: "https://github.com/test/repo/pull/1"},
		Title:  "Test PR",
		Author: "test-user",
		Files: []*diff.DiffFile{
			{Path: "main.go", Kind: diff.FileText},
		},
		RawDiff: "diff --git a/main.go b/main.go\n",
	}

	successJSON := []byte(`{"findings":[{"file":"main.go","line":1,"severity":"suggestion","title":"Test","evidence":"Evidence","suggested_comment":"Comment","fix_hint":"Hint","confidence":0.8}]}`)

	callCount := 0
	runner := func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		callCount++
		if name != "claude" {
			t.Errorf("called %q, want claude", name)
		}
		expectedArgs := []string{"--print", "--output-format=json"}
		if len(args) != len(expectedArgs) {
			t.Errorf("got %d args, want %d", len(args), len(expectedArgs))
		}
		for i, arg := range args {
			if arg != expectedArgs[i] {
				t.Errorf("arg[%d] = %q, want %q", i, arg, expectedArgs[i])
			}
		}
		if stdin == nil {
			t.Fatal("stdin is nil")
		}
		return successJSON, nil
	}

	a := New(runner)
	result, err := a.Review(context.Background(), input)
	if err != nil {
		t.Fatalf("Review() = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("Review() returned nil result")
	}
	if result.Model != "claude" {
		t.Errorf("Model = %q, want claude", result.Model)
	}
	if len(result.Findings) != 1 {
		t.Errorf("Findings count = %d, want 1", len(result.Findings))
	}
	if callCount != 1 {
		t.Errorf("runner called %d times, want 1", callCount)
	}
}

// TestReviewLargeInput verifies Review rejects oversized prompts.
func TestReviewLargeInput(t *testing.T) {
	input := &review.ReviewInput{
		Target:  review.ReviewTarget{URL: "https://github.com/test/repo/pull/1"},
		Files:   []*diff.DiffFile{},
		RawDiff: string(make([]byte, DefaultInputBudgetBytes+1)),
	}

	a := New(nil)
	_, err := a.Review(context.Background(), input)
	if err == nil {
		t.Fatal("Review() = nil, want error for oversized input")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceeds input budget")) {
		t.Errorf("error doesn't mention budget: %v", err)
	}
}

// TestReviewInvalidJSON verifies Review handles invalid JSON output.
func TestReviewInvalidJSON(t *testing.T) {
	input := &review.ReviewInput{
		Target:  review.ReviewTarget{URL: "https://github.com/test/repo/pull/1"},
		Files:   []*diff.DiffFile{},
		RawDiff: "diff\n",
	}

	runner := func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	a := New(runner)
	_, err := a.Review(context.Background(), input)
	if err == nil {
		t.Fatal("Review() = nil, want error for invalid JSON")
	}
}

// TestReviewEmptyFindings verifies Review handles empty findings gracefully.
func TestReviewEmptyFindings(t *testing.T) {
	input := &review.ReviewInput{
		Target:  review.ReviewTarget{URL: "https://github.com/test/repo/pull/1"},
		Files:   []*diff.DiffFile{},
		RawDiff: "diff\n",
	}

	emptyJSON := []byte(`{"findings":[]}`)

	runner := func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		return emptyJSON, nil
	}

	a := New(runner)
	result, err := a.Review(context.Background(), input)
	if err != nil {
		t.Fatalf("Review() = %v, want nil", err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("Findings count = %d, want 0", len(result.Findings))
	}
}
