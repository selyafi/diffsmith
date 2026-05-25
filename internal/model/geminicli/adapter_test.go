package geminicli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/review"
)

func TestNew(t *testing.T) {
	a := New(nil)
	if a == nil {
		t.Fatal("New(nil) returned nil")
	}
}

func TestName(t *testing.T) {
	a := New(nil)
	if got := a.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}

func TestPreflightSuccess(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		if name != "gemini" {
			t.Errorf("lookPath called with %q, want %q", name, "gemini")
		}
		return "/usr/bin/gemini", nil
	}

	if err := a.Preflight(context.Background()); err != nil {
		t.Errorf("Preflight() = %v, want nil", err)
	}
}

func TestPreflightMissing(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		return "", errors.New("not found")
	}

	err := a.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight() = nil, want error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("gemini")) {
		t.Errorf("error doesn't mention gemini: %v", err)
	}
}

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
		if name != "gemini" {
			t.Errorf("called %q, want gemini", name)
		}
		expectedArgs := []string{"-o", "text"}
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
	if result.Model != "gemini" {
		t.Errorf("Model = %q, want gemini", result.Model)
	}
	if len(result.Findings) != 1 {
		t.Errorf("Findings count = %d, want 1", len(result.Findings))
	}
	if callCount != 1 {
		t.Errorf("runner called %d times, want 1", callCount)
	}
}

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

type recordedCall struct {
	name  string
	args  []string
	stdin string
}

func scriptedRunner(t *testing.T, responses [][]byte) (func(context.Context, io.Reader, string, ...string) ([]byte, error), *[]recordedCall) {
	t.Helper()
	idx := 0
	calls := &[]recordedCall{}
	run := func(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		var buf bytes.Buffer
		if stdin != nil {
			_, _ = io.Copy(&buf, stdin)
		}
		*calls = append(*calls, recordedCall{
			name:  name,
			args:  append([]string(nil), args...),
			stdin: buf.String(),
		})
		if idx >= len(responses) {
			t.Fatalf("scriptedRunner: unexpected call #%d (only %d canned)", idx+1, len(responses))
		}
		resp := responses[idx]
		idx++
		return resp, nil
	}
	return run, calls
}

func TestAdapter_Synthesize_Success(t *testing.T) {
	canned := []byte(`{"findings":[{"file":"x.go","line":3,"severity":"low","title":"unified","evidence":"e","suggested_comment":"c","fix_hint":"f","confidence":0.7}]}`)
	run, calls := scriptedRunner(t, [][]byte{canned})
	a := New(run)

	input := &review.ReviewInput{RawDiff: "diff --git a/x.go b/x.go\n+y"}
	results := []*review.ModelReviewResult{
		{Model: "codex", RawOutput: "{}"},
		{Model: "claude", RawOutput: "{}"},
	}

	got, err := a.Synthesize(context.Background(), input, results)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Title != "unified" {
		t.Errorf("unexpected synthesized findings: %+v", got)
	}
	if got.Model != "gemini" {
		t.Errorf("Model field should be gemini; got %s", got.Model)
	}
	if len(*calls) != 1 {
		t.Errorf("expected one gemini invocation; got %d", len(*calls))
	}
}

func TestAdapter_Synthesize_RunnerError(t *testing.T) {
	failingRun := func(ctx context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		return nil, errors.New("simulated gemini failure")
	}
	a := New(failingRun)
	_, err := a.Synthesize(context.Background(),
		&review.ReviewInput{RawDiff: "d"},
		[]*review.ModelReviewResult{{Model: "codex", RawOutput: "{}"}})
	if err == nil {
		t.Fatal("expected error when gemini exec fails")
	}
}
