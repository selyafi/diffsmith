package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// reviewerOnlyFake implements model.Reviewer but NOT model.Synthesizer.
// Used to drive the "lead doesn't satisfy Synthesizer" branch.
type reviewerOnlyFake struct{ name string }

func (r reviewerOnlyFake) Name() string                                    { return r.name }
func (r reviewerOnlyFake) Preflight(context.Context) error                 { return nil }
func (r reviewerOnlyFake) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	return nil, nil
}

// synthFake implements both Reviewer and Synthesizer with scripted
// outcomes.
type synthFake struct {
	name string
	out  *review.ModelReviewResult
	err  error
}

func (s synthFake) Name() string                                    { return s.name }
func (s synthFake) Preflight(context.Context) error                 { return nil }
func (s synthFake) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	return nil, nil
}
func (s synthFake) Synthesize(context.Context, *review.ReviewInput, []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return s.out, s.err
}

// TestAttemptSynthesis_SuccessReturnsResultNoSkip pins the happy path:
// when the lead returns (result, nil), attemptSynthesis must return
// that result and an empty skip reason.
func TestAttemptSynthesis_SuccessReturnsResultNoSkip(t *testing.T) {
	want := &review.ModelReviewResult{Model: "codex"}
	got, skip := attemptSynthesis(context.Background(), synthFake{name: "codex", out: want}, &review.ReviewInput{}, nil)
	if skip != "" {
		t.Errorf("happy path must return empty skip; got %q", skip)
	}
	if got != want {
		t.Errorf("result mismatch: got %v want %v", got, want)
	}
}

// TestAttemptSynthesis_NilNilTreatedAsSkip is the diffsmith-4f8
// regression: an adapter that returns (nil, nil) must NOT silently
// advance the loop. attemptSynthesis surfaces a clear skip reason so
// the caller can log it instead of falling back unannounced.
func TestAttemptSynthesis_NilNilTreatedAsSkip(t *testing.T) {
	got, skip := attemptSynthesis(context.Background(), synthFake{name: "codex"}, &review.ReviewInput{}, nil)
	if got != nil {
		t.Errorf("got non-nil result %v from (nil, nil); want nil", got)
	}
	if skip == "" {
		t.Fatal("(nil, nil) must produce a non-empty skip reason; got empty (silent-fallback regression)")
	}
	// Reason must mention the (nil, nil) shape so the user can diagnose
	// the adapter that misbehaved.
	if !strings.Contains(skip, "nil") {
		t.Errorf("skip reason should name the (nil, nil) shape so the failing adapter is identifiable; got %q", skip)
	}
}

// TestAttemptSynthesis_ErrorPropagatesAsSkip confirms a Synthesize
// error becomes a skip with the error text embedded.
func TestAttemptSynthesis_ErrorPropagatesAsSkip(t *testing.T) {
	_, skip := attemptSynthesis(context.Background(), synthFake{name: "codex", err: errors.New("budget exceeded")}, &review.ReviewInput{}, nil)
	if !strings.Contains(skip, "budget exceeded") {
		t.Errorf("skip reason should include the underlying error; got %q", skip)
	}
}

// TestAttemptSynthesis_ReviewerOnlyLeadSkipsWithCapabilityReason
// confirms a lead that doesn't satisfy model.Synthesizer is skipped
// with a reason that names the capability gap.
func TestAttemptSynthesis_ReviewerOnlyLeadSkipsWithCapabilityReason(t *testing.T) {
	_, skip := attemptSynthesis(context.Background(), reviewerOnlyFake{name: "agy"}, &review.ReviewInput{}, nil)
	if !strings.Contains(skip, "Synthesizer") {
		t.Errorf("skip reason should name the Synthesizer capability gap; got %q", skip)
	}
}

// TestAttemptSynthesis_NilLeadSkipsWithRegistryReason confirms a nil
// lead model (registry miss) is skipped with an explanation.
func TestAttemptSynthesis_NilLeadSkipsWithRegistryReason(t *testing.T) {
	_, skip := attemptSynthesis(context.Background(), nil, &review.ReviewInput{}, nil)
	if skip == "" {
		t.Fatal("nil lead must produce a skip reason; got empty")
	}
	if !strings.Contains(strings.ToLower(skip), "registered") && !strings.Contains(strings.ToLower(skip), "registry") {
		t.Errorf("skip reason should describe the registry miss; got %q", skip)
	}
}

// TestBuildRunSummary_IncludesSkipReasonsWhenSynthesisFails is the
// diffsmith-wfq regression: when synthesis was attempted but no
// candidate produced a result, the run summary must surface WHY
// (each candidate's skip reason) so the user sees a persistent audit
// trail instead of just a transient PhaseStatusMsg flash followed by
// 'synthesis failed' with no detail.
func TestBuildRunSummary_IncludesSkipReasonsWhenSynthesisFails(t *testing.T) {
	surviving := []*review.ModelReviewResult{
		{Model: "codex", Findings: make([]review.FindingCandidate, 5)},
		{Model: "claude", Findings: make([]review.FindingCandidate, 3)},
	}
	skips := []string{
		"codex: synthesis failed: budget exceeded",
		"claude: synthesis returned (nil, nil) — adapter must return either a non-nil result or an error",
	}
	got := buildRunSummary(nil, surviving, nil, "", 5, skips)
	if !strings.Contains(got, "budget exceeded") {
		t.Errorf("summary must include codex's skip reason; got:\n%s", got)
	}
	if !strings.Contains(got, "(nil, nil)") {
		t.Errorf("summary must include claude's skip reason; got:\n%s", got)
	}
}

// TestBuildRunSummary_OmitsSkipReasonsOnSynthesisSuccess confirms a
// successful synthesis run doesn't dump skip noise — the summary
// stays focused on the lead that won.
func TestBuildRunSummary_OmitsSkipReasonsOnSynthesisSuccess(t *testing.T) {
	surviving := []*review.ModelReviewResult{
		{Model: "codex", Findings: make([]review.FindingCandidate, 5)},
		{Model: "claude", Findings: make([]review.FindingCandidate, 3)},
	}
	// First candidate failed and was skipped; second succeeded as
	// lead. The failed-first reason is interesting but the success
	// is what the user cares about; don't dump the noise.
	skips := []string{"codex: synthesis failed: budget exceeded"}
	got := buildRunSummary(nil, surviving, nil, "claude", 4, skips)
	if !strings.Contains(got, "synthesized via claude") {
		t.Errorf("summary must report successful synthesis; got:\n%s", got)
	}
	if strings.Contains(got, "budget exceeded") {
		t.Errorf("successful run should not dump prior-candidate skip noise; got:\n%s", got)
	}
}

// Compile-time guard: synthFake must satisfy both interfaces and
// reviewerOnlyFake must satisfy only Reviewer.
var (
	_ model.Reviewer    = synthFake{}
	_ model.Synthesizer = synthFake{}
	_ model.Reviewer    = reviewerOnlyFake{}
)
