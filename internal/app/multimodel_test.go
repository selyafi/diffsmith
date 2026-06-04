package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
	"github.com/selyafi/diffsmith/internal/tui"
)

type fakeModel struct {
	name   string
	result *review.ModelReviewResult
	err    error
}

func (f fakeModel) Name() string                    { return f.name }
func (f fakeModel) Preflight(context.Context) error { return nil }
func (f fakeModel) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	r := *f.result
	r.Model = f.name
	return &r, nil
}
func (f fakeModel) Synthesize(context.Context, *review.ReviewInput, []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return nil, nil
}

var _ model.Model = fakeModel{}

func TestRunModelsInParallel_AllSucceed(t *testing.T) {
	models := []model.Model{
		fakeModel{name: "codex", result: &review.ModelReviewResult{}},
		fakeModel{name: "claude", result: &review.ModelReviewResult{}},
	}
	var statusCount int32
	send := func(msg tea.Msg) {
		if _, ok := msg.(tui.ModelStatusMsg); ok {
			atomic.AddInt32(&statusCount, 1)
		}
	}
	results := runModelsInParallel(context.Background(), models, &review.ReviewInput{}, send, 0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("expected nil err for %s; got %v", r.Name, r.Err)
		}
	}
	// Each model emits at least 2 status msgs (running, done).
	if atomic.LoadInt32(&statusCount) < 4 {
		t.Errorf("expected ≥4 ModelStatusMsg; got %d", statusCount)
	}
}

func TestRunModelsInParallel_OneFails(t *testing.T) {
	models := []model.Model{
		fakeModel{name: "codex", result: &review.ModelReviewResult{}},
		fakeModel{name: "claude", err: errors.New("simulated failure")},
	}
	results := runModelsInParallel(context.Background(), models, &review.ReviewInput{}, func(tea.Msg) {}, 0)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	byName := map[string]modelOutcome{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["codex"].Err != nil {
		t.Error("codex should succeed")
	}
	if byName["claude"].Err == nil {
		t.Error("claude should have an error")
	}
}

func TestRunModelsInParallel_EmptyInput(t *testing.T) {
	results := runModelsInParallel(context.Background(), nil, &review.ReviewInput{}, func(tea.Msg) {}, 0)
	if len(results) != 0 {
		t.Errorf("empty input should give empty results; got %d", len(results))
	}
}

// blockingModel.Review blocks until either its context is cancelled (then
// it reports ctx.Err()) or fallback elapses (then it succeeds). It lets a
// test prove a per-model timeout fires: with a timeout shorter than
// fallback the context cancels first; with no timeout the model "succeeds"
// after fallback, which is the RED state we want to see fail.
type blockingModel struct {
	name     string
	fallback time.Duration
}

func (b blockingModel) Name() string                    { return b.name }
func (b blockingModel) Preflight(context.Context) error { return nil }
func (b blockingModel) Review(ctx context.Context, _ *review.ReviewInput) (*review.ModelReviewResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(b.fallback):
		return &review.ModelReviewResult{Model: b.name}, nil
	}
}

var _ model.Model = blockingModel{}

// TestRunModelsInParallel_SlowModelTimesOut proves diffsmith-ptr: a model
// that hangs past the per-model timeout drops out as an error while fast
// models still return their results. Without the timeout the whole call
// would block on the slow model (wg.Wait), defeating parallelism.
func TestRunModelsInParallel_SlowModelTimesOut(t *testing.T) {
	models := []model.Model{
		fakeModel{name: "codex", result: &review.ModelReviewResult{}},
		blockingModel{name: "gemini", fallback: 500 * time.Millisecond},
	}
	results := runModelsInParallel(context.Background(), models, &review.ReviewInput{}, func(tea.Msg) {}, 20*time.Millisecond)

	byName := map[string]modelOutcome{}
	for _, r := range results {
		byName[r.Name] = r
	}
	if byName["codex"].Err != nil {
		t.Errorf("fast model codex should succeed; got %v", byName["codex"].Err)
	}
	if byName["gemini"].Err == nil {
		t.Error("slow model gemini should drop out with a timeout error")
	}
}
