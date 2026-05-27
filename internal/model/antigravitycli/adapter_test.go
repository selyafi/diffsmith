package antigravitycli

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// TestNew verifies the constructor creates an adapter with defaults.
func TestNew(t *testing.T) {
	a := New(nil)
	if a == nil {
		t.Fatal("New(nil) returned nil")
	}
}

// TestName verifies the adapter identifies as "antigravity".
func TestName(t *testing.T) {
	a := New(nil)
	if got := a.Name(); got != "antigravity" {
		t.Errorf("Name() = %q, want %q", got, "antigravity")
	}
}

// TestPreflightBinaryMissing verifies Preflight produces an actionable
// error mentioning the binary name when agy is not on PATH.
func TestPreflightBinaryMissing(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		if name != "agy" {
			t.Errorf("lookPath called with %q, want %q", name, "agy")
		}
		return "", errors.New("not found")
	}

	err := a.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight() = nil, want error")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("agy")) {
		t.Errorf("error doesn't mention agy: %v", err)
	}
}

// TestPreflightBinaryPresentStillFails verifies that even when agy is on
// PATH, Preflight returns an experimental-gate error explaining that the
// adapter cannot run non-interactively in v1 per S8b findings. This is
// the key behavioral difference from the codex/claude adapters: agy on
// PATH is necessary but not sufficient.
func TestPreflightBinaryPresentStillFails(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		return "/usr/local/bin/agy", nil
	}

	err := a.Preflight(context.Background())
	if err == nil {
		t.Fatal("Preflight() = nil, want experimental-gate error")
	}
	msg := err.Error()
	if !bytes.Contains([]byte(msg), []byte("experimental")) {
		t.Errorf("error doesn't mention experimental status: %v", err)
	}
	if !bytes.Contains([]byte(msg), []byte("antigravity")) {
		t.Errorf("error doesn't mention antigravity: %v", err)
	}
}

// TestReviewPropagatesPreflightError verifies Review returns the
// Preflight error rather than attempting to invoke agy. The adapter
// must never reach the runner in v1.
func TestReviewPropagatesPreflightError(t *testing.T) {
	a := New(nil)
	a.lookPath = func(name string) (string, error) {
		return "/usr/local/bin/agy", nil
	}

	input := &review.ReviewInput{
		Target: review.ReviewTarget{URL: "https://github.com/test/repo/pull/1"},
	}

	result, err := a.Review(context.Background(), input)
	if err == nil {
		t.Fatal("Review() = nil error, want preflight error")
	}
	if result != nil {
		t.Errorf("Review() returned non-nil result, want nil: %+v", result)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("experimental")) {
		t.Errorf("error doesn't surface the experimental gate: %v", err)
	}
}

// TestAdapter_DoesNotImplementSynthesizer is the post-dvz.7 contract:
// the antigravity adapter intentionally does NOT carry a Synthesize
// method, so a runtime type-assertion for model.Synthesizer must fail.
// This frees experimental/review-only adapters from carrying a fake
// Synthesize that exists only to satisfy the old composite interface.
//
// The synthesis call site in app/review.go consults this via
// `leadModel.(model.Synthesizer)` and skips models that don't satisfy
// the capability.
func TestAdapter_DoesNotImplementSynthesizer(t *testing.T) {
	a := New(nil)

	// Must satisfy the base Reviewer interface (Name, Preflight, Review).
	var _ model.Reviewer = a

	// Must NOT satisfy the Synthesizer capability.
	if _, ok := any(a).(model.Synthesizer); ok {
		t.Errorf("antigravity Adapter must not implement model.Synthesizer in v1 (agy has no non-interactive auth path; a real Synthesize would be a stub)")
	}
}
