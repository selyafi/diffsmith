package model_test

import (
	"context"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// stubModel is a Model fake used to test priority sorting.
type stubModel struct{ name string }

func (s stubModel) Name() string                                                             { return s.name }
func (s stubModel) Preflight(context.Context) error                                          { return nil }
func (s stubModel) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) { return nil, nil }
func (s stubModel) Synthesize(context.Context, *review.ReviewInput, []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return nil, nil
}

func TestNewSelectedModels_SortsByPriority(t *testing.T) {
	got := model.NewSelectedModels([]model.Model{
		stubModel{name: "antigravity"},
		stubModel{name: "codex"},
		stubModel{name: "claude"},
	})
	want := []string{"codex", "claude", "antigravity"}
	if len(got.All) != 3 {
		t.Fatalf("expected 3 models, got %d", len(got.All))
	}
	for i, m := range got.All {
		if m.Name() != want[i] {
			t.Errorf("position %d: got %s, want %s", i, m.Name(), want[i])
		}
	}
	// Callers that want "the synthesis lead" read All[0] directly.
	if got.All[0].Name() != "codex" {
		t.Errorf("lead: got %s, want codex", got.All[0].Name())
	}
}

func TestNewSelectedModels_LeadSkipsCodexWhenAbsent(t *testing.T) {
	got := model.NewSelectedModels([]model.Model{
		stubModel{name: "antigravity"},
		stubModel{name: "claude"},
	})
	if got.All[0].Name() != "claude" {
		t.Errorf("lead (All[0]): got %s, want claude", got.All[0].Name())
	}
	if got.All[1].Name() != "antigravity" {
		t.Errorf("order: got %s, want antigravity", got.All[1].Name())
	}
}

func TestNewSelectedModels_EmptyIsValid(t *testing.T) {
	got := model.NewSelectedModels(nil)
	if len(got.All) != 0 {
		t.Errorf("expected 0 models, got %d", len(got.All))
	}
}

func TestNewSelectedModels_UnknownNameLowestPriority(t *testing.T) {
	got := model.NewSelectedModels([]model.Model{
		stubModel{name: "future"},
		stubModel{name: "codex"},
	})
	if got.All[0].Name() != "codex" {
		t.Errorf("codex should still come first; got %s", got.All[0].Name())
	}
}
