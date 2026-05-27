package app

import (
	"context"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// budgetSettingFake is a model.Model that also implements
// model.InputBudgetSetter. Records every SetInputBudget call so tests
// can assert the override was actually delivered (not "called the
// helper, hoped it worked").
type budgetSettingFake struct {
	name     string
	gotCalls []int
}

func (f *budgetSettingFake) Name() string                    { return f.name }
func (f *budgetSettingFake) Preflight(context.Context) error { return nil }
func (f *budgetSettingFake) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	return nil, nil
}
func (f *budgetSettingFake) SetInputBudget(bytes int) {
	f.gotCalls = append(f.gotCalls, bytes)
}

var (
	_ model.Reviewer          = (*budgetSettingFake)(nil)
	_ model.InputBudgetSetter = (*budgetSettingFake)(nil)
)

// budgetlessFake is a model.Model that does NOT implement
// InputBudgetSetter. applyInputBudget must silently skip it instead of
// panicking.
type budgetlessFake struct{ name string }

func (f *budgetlessFake) Name() string                    { return f.name }
func (f *budgetlessFake) Preflight(context.Context) error { return nil }
func (f *budgetlessFake) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	return nil, nil
}

// TestApplyInputBudget_AppliesToSettersOnly is the diffsmith-uc1 unit:
// when the user passes --input-budget=N, every selected model that
// implements InputBudgetSetter must receive SetInputBudget(N) exactly
// once. Models without the capability (e.g. antigravity in v1) are
// silently skipped — they don't have a budget to override.
func TestApplyInputBudget_AppliesToSettersOnly(t *testing.T) {
	setter := &budgetSettingFake{name: "codex"}
	other := &budgetlessFake{name: "antigravity"}
	selected := &model.SelectedModels{All: []model.Model{setter, other}}

	applyInputBudget(selected, 512*1024)

	if len(setter.gotCalls) != 1 || setter.gotCalls[0] != 512*1024 {
		t.Errorf("setter should receive exactly one SetInputBudget(524288); got %v", setter.gotCalls)
	}
}

// TestApplyInputBudget_ZeroIsNoOp covers the unset-flag case. cobra
// leaves an unset --input-budget int flag at 0; applyInputBudget must
// treat that as "user didn't ask for an override" and leave each
// adapter's default in place.
func TestApplyInputBudget_ZeroIsNoOp(t *testing.T) {
	setter := &budgetSettingFake{name: "codex"}
	selected := &model.SelectedModels{All: []model.Model{setter}}

	applyInputBudget(selected, 0)

	if len(setter.gotCalls) != 0 {
		t.Errorf("budget=0 means 'flag unset'; setter should not be called. got %v", setter.gotCalls)
	}
}

// TestApplyInputBudget_NegativeIsNoOp guards against operator typos:
// a negative byte count is meaningless and must not propagate to the
// adapter (the adapter itself also rejects it, but defense in depth).
func TestApplyInputBudget_NegativeIsNoOp(t *testing.T) {
	setter := &budgetSettingFake{name: "codex"}
	selected := &model.SelectedModels{All: []model.Model{setter}}

	applyInputBudget(selected, -1)

	if len(setter.gotCalls) != 0 {
		t.Errorf("negative budget must be rejected before reaching the adapter; got %v", setter.gotCalls)
	}
}

// TestApplyInputBudget_NilSelectedIsNoOp covers the dry-run/print-only
// paths where the picker is bypassed and runReviewByURL has nil
// SelectedModels. applyInputBudget must not panic.
func TestApplyInputBudget_NilSelectedIsNoOp(t *testing.T) {
	applyInputBudget(nil, 512*1024) // must not panic
}
