package app

import "github.com/selyafi/diffsmith/internal/model"

// applyInputBudget delivers --input-budget=N to every selected model
// that implements model.InputBudgetSetter. Adapters without the
// capability (antigravity in v1) are silently skipped — they don't
// have a budget to override.
//
// budget<=0 means "flag unset / not requested"; in that case we leave
// every adapter's compiled-in default in place. Surfacing zero as a
// no-op (rather than passing it through) defends against an unset
// flag accidentally disabling enforcement.
func applyInputBudget(selected *model.SelectedModels, budget int) {
	if selected == nil || budget <= 0 {
		return
	}
	for _, m := range selected.All {
		if bs, ok := m.(model.InputBudgetSetter); ok {
			bs.SetInputBudget(budget)
		}
	}
}
