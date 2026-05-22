// Package claudecli implements the Claude model adapter via `claude --print
// --output-format=json`. Prompts are piped via stdin per ADR 0007.
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

// DefaultInputBudgetBytes caps the prompt size sent to Claude. The value
// is intentionally conservative for v1; spike S9 calibrates a real
// number before M8 by measuring real public-PR diffs.
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

// Review invokes claude with --print --output-format=json. Stdin piping,
// output parsing and validation are built-in to the Claude CLI.
func (a *Adapter) Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error) {
	prompt := model.BuildPrompt(input)
	if len(prompt) > DefaultInputBudgetBytes {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR or filter files",
			len(prompt), DefaultInputBudgetBytes, a.Name())
	}

	out, err := a.run(ctx, strings.NewReader(prompt), "claude", "--print", "--output-format=json")
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
