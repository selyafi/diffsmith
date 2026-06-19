package antigravitycli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// DefaultInputBudgetBytes caps the prompt size sent to agy. Matches the
// codex/claude budget (1 MiB) so users get consistent behavior regardless
// of model choice. See codexcli for the underlying rationale.
const DefaultInputBudgetBytes = 1024 * 1024

// noDeadlinePrintTimeout is the --print-timeout passed when the call's ctx
// has no deadline (i.e. --model-timeout 0, documented as "disables the
// cap"). agy's intrinsic default is 5m, which would cap antigravity while
// codex/claude run unbounded; a large ceiling honors the disabled cap
// without leaving a truly unbounded interactive hang.
const noDeadlinePrintTimeout = "24h"

// printTimeout derives agy's --print-timeout from the call's ctx deadline
// so the user's --model-timeout governs antigravity exactly as it governs
// codex/claude — which pass no internal CLI timeout and are capped solely
// by ctx. agy's intrinsic 5m default would otherwise cap antigravity below
// a longer --model-timeout (default 10m) and ignore --model-timeout 0.
// With a deadline we pass the remaining budget, so agy self-aborts at the
// same point exec.CommandContext would cancel it.
func printTimeout(ctx context.Context) string {
	dl, ok := ctx.Deadline()
	if !ok {
		return noDeadlinePrintTimeout
	}
	// at/past the deadline: stop promptly rather than passing 0/negative.
	remaining := max(time.Until(dl), time.Second)
	return remaining.Round(time.Second).String()
}

// Adapter implements the model.Model interface against the Antigravity CLI
// (`agy`). agy 1.0.9 resolved the S8b auth blocker (persistent OAuth
// tokens), so this is a full peer: it reviews, synthesizes, and honors an
// input budget, matching the codex/claude adapters.
type Adapter struct {
	run         provider.Runner
	lookPath    func(name string) (string, error)
	inputBudget int
}

// New constructs an Adapter. Passing nil uses provider.IsolatedRunner so
// agy can't onboard from the caller's cwd; lookPath defaults to
// exec.LookPath. Tests override fields directly (the package is
// internal-only).
func New(run provider.Runner) *Adapter {
	if run == nil {
		// Isolate agy from the caller's cwd: the whole diff is piped via
		// stdin, so the reviewer needs no workspace, and a neutral temp dir
		// keeps reviews deterministic. diffsmith-4tz.
		run = provider.IsolatedRunner()
	}
	return &Adapter{
		run:         run,
		lookPath:    exec.LookPath,
		inputBudget: DefaultInputBudgetBytes,
	}
}

// SetInputBudget overrides the default prompt-size cap. Values <= 0 are
// ignored so an unset --input-budget flag can't silently disable
// enforcement and let an arbitrarily large prompt slip through.
func (a *Adapter) SetInputBudget(bytes int) {
	if bytes > 0 {
		a.inputBudget = bytes
	}
}

// Name returns the model identifier surfaced to users via the picker and
// attached to validated findings.
func (a *Adapter) Name() string { return "antigravity" }

// Preflight verifies the agy binary is on PATH. Auth failures (a user who
// has never run an interactive `agy` login) surface at Review time via
// agy's own stderr, which the runner propagates — matching codex/claude.
func (a *Adapter) Preflight(_ context.Context) error {
	if _, err := a.lookPath("agy"); err != nil {
		return errors.New("agy (Antigravity CLI) not found on PATH. Install it and run `agy` once to authenticate, or select --model codex or --model claude")
	}
	return nil
}

// Review invokes agy against the standard review prompt.
func (a *Adapter) Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildPrompt(input))
}

// Synthesize runs agy against the synthesis prompt that combines the diff
// with N other reviewers' findings. Output is parsed identically to Review.
func (a *Adapter) Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildSynthesisPrompt(input, results))
}

// executeWithPrompt runs agy against the given prompt and returns the
// parsed result. Shared by Review and Synthesize.
//
// Invocation: `agy --print=- --print-timeout <dur>` with the prompt piped
// via stdin. agy's --print is a string flag that requires a value; `-` is
// the conventional stdin marker, and when stdin is a pipe agy reads the
// prompt from it (verified — see the design spec). Output is raw model
// text with no envelope, so stdout pipes straight into ParseFindings
// (unlike gemini's -o json wrapper). We deliberately omit
// --dangerously-skip-permissions: agy must not auto-execute tools.
func (a *Adapter) executeWithPrompt(ctx context.Context, prompt string) (*review.ModelReviewResult, error) {
	if len(prompt) > a.inputBudget {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR, filter files with --include/--exclude, or raise --input-budget",
			len(prompt), a.inputBudget, a.Name())
	}

	out, err := a.run(ctx, strings.NewReader(prompt), "agy", "--print=-", "--print-timeout", printTimeout(ctx))
	if err != nil {
		return nil, fmt.Errorf("antigravity: %w", err)
	}
	findings, err := model.ParseFindings(out)
	if err != nil {
		// agy has no schema flag, so non-JSON output is the failure shape
		// for an unauthenticated agy (it emits auth/login text, not
		// findings). Point the user at the one-time login rather than
		// surfacing a bare parse error.
		return nil, fmt.Errorf("antigravity output not parseable as findings (a likely cause is an unauthenticated agy — run `agy` once to log in): %w", err)
	}
	return &review.ModelReviewResult{
		Model:     a.Name(),
		Findings:  findings,
		RawOutput: string(out),
	}, nil
}

// Compile-time interface guards: agy is a full peer (diffsmith-0hy).
var (
	_ model.Reviewer          = (*Adapter)(nil)
	_ model.Synthesizer       = (*Adapter)(nil)
	_ model.InputBudgetSetter = (*Adapter)(nil)
)
