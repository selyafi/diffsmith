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

// DefaultInputBudgetBytes caps the prompt size sent to Claude. Originally
// calibrated by spike S9 at 256 KiB; raised to 1 MiB (diffsmith-uc1).
// See codexcli.DefaultInputBudgetBytes for the rationale — Claude shares
// the same envelope and the bump is intentionally uniform across all
// three adapters so users get consistent behavior regardless of which
// model they pick.
const DefaultInputBudgetBytes = 1024 * 1024

// Adapter implements the model.Model interface against the Claude CLI.
type Adapter struct {
	run         provider.Runner
	lookPath    func(name string) (string, error)
	inputBudget int
}

// New constructs an Adapter. Passing nil uses provider.DefaultRunner;
// lookPath defaults to exec.LookPath. Tests override fields directly
// (the package is internal-only).
func New(run provider.Runner) *Adapter {
	if run == nil {
		// Isolate claude from the caller's cwd so it can't autoload a
		// project CLAUDE.md / .claude config during a review. diffsmith-4tz.
		run = provider.IsolatedRunner()
	}
	return &Adapter{
		run:         run,
		lookPath:    exec.LookPath,
		inputBudget: DefaultInputBudgetBytes,
	}
}

// SetInputBudget overrides the default prompt-size cap for this
// adapter. Values <= 0 are ignored so an unset flag can't silently
// disable enforcement.
func (a *Adapter) SetInputBudget(bytes int) {
	if bytes > 0 {
		a.inputBudget = bytes
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
	return a.executeWithPrompt(ctx, model.BuildPrompt(input))
}

// Synthesize runs claude against the synthesis prompt that combines
// the diff with N other reviewers' findings.
func (a *Adapter) Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildSynthesisPrompt(input, results))
}

// executeWithPrompt runs claude against the given prompt and returns
// the parsed result. Shared by Review (normal review prompt) and
// Synthesize (synthesis prompt).
func (a *Adapter) executeWithPrompt(ctx context.Context, prompt string) (*review.ModelReviewResult, error) {
	if len(prompt) > a.inputBudget {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR, filter files with --include/--exclude, or raise --input-budget",
			len(prompt), a.inputBudget, a.Name())
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

// Compile-time interface guards: catch any future refactor that
// accidentally drops a capability. diffsmith-0hy.
var (
	_ model.Reviewer          = (*Adapter)(nil)
	_ model.Synthesizer       = (*Adapter)(nil)
	_ model.InputBudgetSetter = (*Adapter)(nil)
)
