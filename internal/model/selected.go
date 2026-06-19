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
// antigravity). The synthesis-lead-priority concept is encoded in
// the All ordering itself — callers that need "the highest-priority
// surviving model" iterate All in order; the first match wins.
type SelectedModels struct {
	All []Model
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
	return &SelectedModels{All: sorted}
}

func priorityOf(name string) int {
	if p, ok := priorityOrder[name]; ok {
		return p
	}
	return len(priorityOrder) // unknown → after all known
}
