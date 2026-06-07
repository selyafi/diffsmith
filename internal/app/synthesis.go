package app

import (
	"context"
	"fmt"
	"time"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// attemptSynthesis runs one synthesis attempt against a single lead
// candidate. Returns (result, "") on success; (nil, skipReason) when
// the attempt was skipped or failed. The caller surfaces skipReason
// as a user-visible status so the user always sees why a candidate
// was bypassed instead of seeing a silent fallback to the next one.
//
// Four skip paths are handled here, in declaration order:
//
//  1. nil leadModel — registry miss (drift between the reviewer's
//     surviving outcome's Model name and the selected.All set).
//  2. lead doesn't satisfy model.Synthesizer — review-only adapter
//     (e.g. antigravity in v1); diffsmith-dvz.7 made this explicit.
//  3. Synthesize returned an error — typical: budget bust, parse
//     failure, network.
//  4. Synthesize returned (nil, nil) — undefined per the adapter
//     contract but legal Go. Without this branch the loop would
//     silently advance; diffsmith-4f8 introduced the explicit check.
func attemptSynthesis(ctx context.Context, leadModel model.Reviewer, input *review.ReviewInput, surviving []*review.ModelReviewResult, timeout time.Duration) (*review.ModelReviewResult, string) {
	if leadModel == nil {
		return nil, "no matching model registered in the picker selection"
	}
	leadSynth, ok := leadModel.(model.Synthesizer)
	if !ok {
		return nil, "model does not implement the Synthesizer capability"
	}
	// Cap the lead model the same way the parallel reviewers are capped:
	// a hung synthesis CLI must not block the whole review. A non-positive
	// timeout disables the cap. A deadline surfaces as a skip reason via
	// the err path below — never a silent fallback.
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	synth, err := leadSynth.Synthesize(ctx, input, surviving)
	if err != nil {
		return nil, fmt.Sprintf("synthesis failed: %v", err)
	}
	if synth == nil {
		return nil, "synthesis returned (nil, nil) — adapter must return either a non-nil result or an error"
	}
	return synth, ""
}
