package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

// fakeFetcher is a scripted review.LinkedIssueFetcher.
type fakeFetcher struct {
	issues []review.IssueContext
	notes  []string
	err    error
	called bool
}

func (f *fakeFetcher) FetchLinkedIssues(_ context.Context, _ review.ReviewTarget) ([]review.IssueContext, []string, error) {
	f.called = true
	return f.issues, f.notes, f.err
}

func ctxInput() *review.ReviewInput {
	return &review.ReviewInput{
		Target:      review.ReviewTarget{Host: review.HostGitHub, Number: 42},
		Description: "the body",
	}
}

// TestEnrichWithContext_NoContextStripsEverything: --no-context clears the
// description and acceptance criteria and never calls the fetcher, so
// nothing extra reaches the model and no extra network call is made.
func TestEnrichWithContext_NoContextStripsEverything(t *testing.T) {
	in := ctxInput()
	in.AcceptanceCriteria = []review.IssueContext{{Number: 1}}
	f := &fakeFetcher{issues: []review.IssueContext{{Number: 9}}}

	notes := enrichWithContext(context.Background(), f, in, true)

	if in.Description != "" || in.AcceptanceCriteria != nil {
		t.Errorf("--no-context must strip context; got desc=%q ac=%v", in.Description, in.AcceptanceCriteria)
	}
	if f.called {
		t.Error("--no-context must not call the fetcher")
	}
	if notes != nil {
		t.Errorf("no notes expected; got %v", notes)
	}
}

// TestEnrichWithContext_PopulatesAcceptanceCriteria: the resolved issues
// land on the input and clean resolution surfaces no notes.
func TestEnrichWithContext_PopulatesAcceptanceCriteria(t *testing.T) {
	in := ctxInput()
	f := &fakeFetcher{issues: []review.IssueContext{{Number: 7, Title: "Widget"}, {Number: 9}}}

	notes := enrichWithContext(context.Background(), f, in, false)

	if len(in.AcceptanceCriteria) != 2 || in.AcceptanceCriteria[0].Number != 7 {
		t.Fatalf("acceptance criteria not populated: %+v", in.AcceptanceCriteria)
	}
	if len(notes) != 0 {
		t.Errorf("clean resolution should surface no notes; got %v", notes)
	}
}

// TestEnrichWithContext_FetchErrorIsNonFatalNote: a total fetch failure
// must not panic or abort — it surfaces one note and leaves criteria empty.
func TestEnrichWithContext_FetchErrorIsNonFatalNote(t *testing.T) {
	in := ctxInput()
	f := &fakeFetcher{err: errors.New("gh exploded")}

	notes := enrichWithContext(context.Background(), f, in, false)

	if len(in.AcceptanceCriteria) != 0 {
		t.Errorf("criteria should stay empty on fetch failure; got %v", in.AcceptanceCriteria)
	}
	if len(notes) == 0 || !strings.Contains(strings.Join(notes, " "), "gh exploded") {
		t.Errorf("fetch failure must surface a note carrying the cause; got %v", notes)
	}
}

// TestEnrichWithContext_SurfacesFetcherNotes: non-fatal per-issue notes from
// the fetcher are passed through.
func TestEnrichWithContext_SurfacesFetcherNotes(t *testing.T) {
	in := ctxInput()
	f := &fakeFetcher{
		issues: []review.IssueContext{{Number: 7}},
		notes:  []string{"linked issue #9: fetch failed"},
	}

	notes := enrichWithContext(context.Background(), f, in, false)

	if len(notes) == 0 || !strings.Contains(strings.Join(notes, " "), "#9") {
		t.Errorf("fetcher notes must be surfaced; got %v", notes)
	}
}

// TestBuildRunSummary_SurfacesContextNotes: context enrichment notes
// (fetch failure, truncation) appear in the run summary so they're never
// silently lost.
func TestBuildRunSummary_SurfacesContextNotes(t *testing.T) {
	surviving := []*review.ModelReviewResult{{Model: "codex", Findings: make([]review.FindingCandidate, 2)}}
	notes := []string{"acceptance criteria unavailable: gh exploded", "description truncated from 9000 to 8192 bytes"}
	got := buildRunSummary(nil, surviving, nil, "", 2, nil, notes)
	if !strings.Contains(got, "gh exploded") || !strings.Contains(got, "truncated") {
		t.Errorf("run summary must surface context notes; got:\n%s", got)
	}
}

// TestEnrichWithContext_NilFetcherStillCaps: a provider that doesn't support
// linked issues yields no criteria, but the description is still bounded.
func TestEnrichWithContext_NilFetcherStillCaps(t *testing.T) {
	in := ctxInput()
	in.Description = strings.Repeat("x", review.MaxDescriptionBytes+500)

	notes := enrichWithContext(context.Background(), nil, in, false)

	if len(in.Description) > review.MaxDescriptionBytes {
		t.Errorf("description must be capped even with no fetcher; got %d bytes", len(in.Description))
	}
	if len(notes) == 0 {
		t.Error("truncation must surface a note")
	}
}
