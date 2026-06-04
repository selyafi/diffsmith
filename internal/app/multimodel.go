package app

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
	"github.com/selyafi/diffsmith/internal/tui"
)

// modelOutcome captures one model's contribution: either a Review
// result or the error that caused it to drop out.
type modelOutcome struct {
	Name   string
	Result *review.ModelReviewResult // nil if Err != nil
	Err    error
}

// runModelsInParallel runs each model's Review concurrently and
// streams ModelStatusMsg updates via send. Returns one modelOutcome
// per input model after all have completed. Order is non-deterministic
// (callers should look up by Name).
func runModelsInParallel(ctx context.Context, models []model.Model, input *review.ReviewInput, send func(tea.Msg), timeout time.Duration) []modelOutcome {
	if len(models) == 0 {
		return nil
	}
	results := make([]modelOutcome, len(models))
	var wg sync.WaitGroup
	for i, m := range models {
		wg.Add(1)
		go func(idx int, m model.Model) {
			defer wg.Done()
			// Each model gets its own deadline so one hung reviewer (e.g.
			// an MCP cold-start) is cancelled and dropped without blocking
			// the others — the join below waits on every goroutine. A
			// non-positive timeout disables the cap.
			runCtx := ctx
			if timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}
			send(tui.ModelStatusMsg{Name: m.Name(), State: "running"})
			r, err := m.Review(runCtx, input)
			if err != nil {
				send(tui.ModelStatusMsg{Name: m.Name(), State: "failed", Err: err})
				results[idx] = modelOutcome{Name: m.Name(), Err: err}
				return
			}
			send(tui.ModelStatusMsg{Name: m.Name(), State: "done"})
			results[idx] = modelOutcome{Name: m.Name(), Result: r}
		}(i, m)
	}
	wg.Wait()
	return results
}

// splitOutcomes separates surviving (non-error) results from dropped
// (error) ones. Surviving order matches input order (since the input
// slice is priority-ordered upstream).
func splitOutcomes(outcomes []modelOutcome) (surviving []*review.ModelReviewResult, dropped []modelOutcome) {
	for _, o := range outcomes {
		if o.Err != nil {
			dropped = append(dropped, o)
			continue
		}
		surviving = append(surviving, o.Result)
	}
	return
}
