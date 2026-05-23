// Package claudecli implements the Claude model adapter via `claude --print
// --output-format=text`. Prompts are piped via stdin per ADR 0007.
package claudecli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// DefaultInputBudgetBytes caps the prompt size sent to Claude. Calibrated
// by spike S9 against 26 real public PRs (median 7.9 KB, max non-outlier
// 47.4 KB, one 2.2 MB mega-PR rejected). 256 KB cleanly bisects: every
// reviewable PR passes with a ~5x safety margin, unreviewable mega-PRs
// fail with an actionable message. See docs/model-adapters.md § Diff Size
// and Context Budget for the rationale; spikes/s9-input-budget/main.go is
// the measurement tool — re-run when models change or the prompt scaffold
// grows.
const DefaultInputBudgetBytes = 256 * 1024

// Adapter implements the model.Model interface against the Claude CLI.
type Adapter struct {
	run      provider.Runner
	lookPath func(name string) (string, error)
}

// New constructs an Adapter. Passing nil uses provider.DefaultRunner;
// lookPath defaults to exec.LookPath. Tests override fields directly
// (the package is internal-only).
func New(run provider.Runner) *Adapter {
	if run == nil {
		run = provider.DefaultRunner
	}
	return &Adapter{
		run:      run,
		lookPath: exec.LookPath,
	}
}

// Name returns the model identifier surfaced to users via --model and
// attached to validated findings.
func (a *Adapter) Name() string { return "claude" }

// Preflight verifies the claude binary is on PATH. The model is never
// invoked when this fails; the user sees an actionable install hint
// instead of a stack trace from os/exec.
func (a *Adapter) Preflight(_ context.Context) error {
	if _, err := a.lookPath("claude"); err != nil {
		return errors.New("claude CLI not found on PATH. Install instructions: https://claude.ai/download")
	}
	return nil
}

// Review invokes claude with --print --output-format=text. Stdin piping,
// JSON shape, and validation are prompt-engineered (see prompt-contract.md):
// the model is instructed to emit a {"findings":[...]} JSON object as its
// entire response, so text mode returns exactly that.
//
// We deliberately do NOT use --output-format=json, which returns a JSON
// array of event records ({"type":"system",...}, {"type":"assistant",...},
// {"type":"result","result":"<model text>",...}). That envelope would have
// to be unwrapped before parsing; text mode skips that step. Verified via
// spike M7a-followup (diffsmith-e2w).
func (a *Adapter) Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error) {
	prompt := model.BuildPrompt(input)
	if len(prompt) > DefaultInputBudgetBytes {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR or filter files",
			len(prompt), DefaultInputBudgetBytes, a.Name())
	}

	out, err := a.run(ctx, strings.NewReader(prompt), "claude", "--print", "--output-format=text")
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	findings, err := model.ParseFindings(out)
	if err != nil {
		return nil, fmt.Errorf("claude output: %w", err)
	}
	return &review.ModelReviewResult{
		Model:     a.Name(),
		Findings:  findings,
		RawOutput: string(out),
	}, nil
}
