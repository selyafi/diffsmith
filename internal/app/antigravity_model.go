package app

import (
	"os"

	"github.com/selyafi/diffsmith/internal/model"
)

// antigravityModelEnv lets users pick the agy model without a flag (e.g.
// in CI). The --antigravity-model flag takes precedence when set.
const antigravityModelEnv = "DIFFSMITH_ANTIGRAVITY_MODEL"

// applyAntigravityModel overrides the agy model on every selected adapter
// that implements model.ModelSetter (only the antigravity adapter does).
//
// Precedence: the flag value wins; otherwise $DIFFSMITH_ANTIGRAVITY_MODEL;
// otherwise nothing is applied and the adapter keeps its compiled-in
// DefaultModel. An empty resolved value is a no-op so an unset flag/env
// can't blank out the pinned default (the adapter's SetModel also guards
// this, but resolving it here keeps the no-op visible at the call site).
func applyAntigravityModel(selected *model.SelectedModels, flagVal string) {
	name := flagVal
	if name == "" {
		name = os.Getenv(antigravityModelEnv)
	}
	if selected == nil || name == "" {
		return
	}
	for _, m := range selected.All {
		if ms, ok := m.(model.ModelSetter); ok {
			ms.SetModel(name)
		}
	}
}
