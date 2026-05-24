package model

import "sort"

// priorityOrder defines the canonical priority for the multi-model
// flow: codex first, then claude, then antigravity, then any unknown
// names alphabetically after. Used to determine the synthesis lead
// (highest-priority surviving model among selected).
var priorityOrder = map[string]int{
	"codex":       0,
	"claude":      1,
	"antigravity": 2,
}

// SelectedModels is the user's picker choice carried through the
// review pipeline. All is sorted by priority (codex > claude >
// antigravity); Lead == All[0] when non-empty, nil otherwise.
type SelectedModels struct {
	All  []Model
	Lead Model
}

// NewSelectedModels returns a SelectedModels with All sorted by the
// canonical priority order. Unknown model names are sorted after the
// known ones (lowest priority).
func NewSelectedModels(ms []Model) *SelectedModels {
	sorted := make([]Model, len(ms))
	copy(sorted, ms)
	sort.SliceStable(sorted, func(i, j int) bool {
		return priorityOf(sorted[i].Name()) < priorityOf(sorted[j].Name())
	})
	var lead Model
	if len(sorted) > 0 {
		lead = sorted[0]
	}
	return &SelectedModels{All: sorted, Lead: lead}
}

func priorityOf(name string) int {
	if p, ok := priorityOrder[name]; ok {
		return p
	}
	return len(priorityOrder) // unknown → after all known
}
