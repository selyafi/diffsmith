package geminicli

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

// DefaultInputBudgetBytes caps the prompt size sent to gemini. Matches
// the claudecli budget (1 MiB after diffsmith-uc1) so users get
// consistent behavior regardless of model choice. See codexcli for the
// underlying rationale.
const DefaultInputBudgetBytes = 1024 * 1024

// Adapter implements the model.Model interface against the Gemini CLI.
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
		// Isolate gemini from the caller's cwd so it can't onboard from a
		// project AGENTS.md / CLAUDE.md or autoload project MCP config.
		// diffsmith-4tz.
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

// Name returns the model identifier surfaced to users via the picker
// and attached to validated findings.
func (a *Adapter) Name() string { return "gemini" }

// Preflight verifies the gemini binary is on PATH. The model is never
// invoked when this fails; the user sees an actionable install hint
// instead of a stack trace from os/exec.
func (a *Adapter) Preflight(_ context.Context) error {
	if _, err := a.lookPath("gemini"); err != nil {
		return errors.New("gemini CLI not found on PATH. Install: https://github.com/google-gemini/gemini-cli")
	}
	return nil
}

// Review invokes gemini with `-o text --skip-trust`. Stdin piping,
// JSON shape, and validation are prompt-engineered (see
// prompt-contract.md): the model is instructed to emit a
// {"findings":[...]} JSON object as its entire response, so text mode
// returns exactly that.
//
// We deliberately do NOT use `-o json`, which wraps the model output in
// a {"response": ..., "stats": ...} envelope. That envelope would have
// to be unwrapped before parsing; text mode skips that step.
//
// --skip-trust bypasses gemini's per-directory workspace-trust gate.
// Diffsmith pipes the full diff via stdin and gemini never reads files
// from the CWD, so the trust check has no semantic meaning here — and
// without the flag, gemini exits 55 ("not running in a trusted
// directory") whenever diffsmith is run from a repo the user hasn't
// trusted via gemini's interactive prompt.
func (a *Adapter) Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildPrompt(input))
}

// Synthesize runs gemini against the synthesis prompt that combines
// the diff with N other reviewers' findings.
func (a *Adapter) Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildSynthesisPrompt(input, results))
}

// executeWithPrompt runs gemini against the given prompt and returns
// the parsed result. Shared by Review (normal review prompt) and
// Synthesize (synthesis prompt).
func (a *Adapter) executeWithPrompt(ctx context.Context, prompt string) (*review.ModelReviewResult, error) {
	if len(prompt) > a.inputBudget {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR, filter files with --include/--exclude, or raise --input-budget",
			len(prompt), a.inputBudget, a.Name())
	}

	out, err := a.run(ctx, strings.NewReader(prompt), "gemini", "-o", "text", "--skip-trust")
	if err != nil {
		return nil, fmt.Errorf("gemini: %w", err)
	}
	findings, err := model.ParseFindings(out)
	if err != nil {
		return nil, fmt.Errorf("gemini output: %w", err)
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
