// Package codexcli implements the Codex model adapter via `codex exec
// --output-schema`. Prompts are piped via stdin per ADR 0007.
package codexcli

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

//go:embed schema.json
var schemaJSON []byte

// DefaultInputBudgetBytes caps the prompt size sent to codex. Calibrated
// by spike S9 against 26 real public PRs (median 7.9 KB, max non-outlier
// 47.4 KB, one 2.2 MB mega-PR rejected). 256 KB cleanly bisects: every
// reviewable PR passes with a ~5x safety margin, unreviewable mega-PRs
// fail with an actionable message. See docs/model-adapters.md § Diff Size
// and Context Budget for the rationale; spikes/s9-input-budget/main.go is
// the measurement tool — re-run when models change or the prompt scaffold
// grows.
const DefaultInputBudgetBytes = 256 * 1024

// Adapter implements the model.Model interface against the Codex CLI.
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
func (a *Adapter) Name() string { return "codex" }

// Preflight verifies the codex binary is on PATH. The model is never
// invoked when this fails; the user sees an actionable install hint
// instead of a stack trace from os/exec.
func (a *Adapter) Preflight(_ context.Context) error {
	if _, err := a.lookPath("codex"); err != nil {
		return errors.New("codex CLI not found on PATH. Install instructions: https://github.com/openai/codex")
	}
	return nil
}

// Review invokes codex with an --output-schema path. Stdin piping,
// schema temp-file writing, and output parsing are added by subsequent
// TDD cycles as their tests drive them.
func (a *Adapter) Review(ctx context.Context, input *review.ReviewInput) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildPrompt(input))
}

// Synthesize runs codex against the synthesis prompt that combines
// the diff with N other reviewers' findings. Output is parsed and
// validated identically to Review.
func (a *Adapter) Synthesize(ctx context.Context, input *review.ReviewInput, results []*review.ModelReviewResult) (*review.ModelReviewResult, error) {
	return a.executeWithPrompt(ctx, model.BuildSynthesisPrompt(input, results))
}

// executeWithPrompt runs codex against the given prompt and returns
// the parsed result. Shared by Review (normal review prompt) and
// Synthesize (synthesis prompt).
func (a *Adapter) executeWithPrompt(ctx context.Context, prompt string) (*review.ModelReviewResult, error) {
	if len(prompt) > DefaultInputBudgetBytes {
		return nil, fmt.Errorf("prompt size %d bytes exceeds input budget %d bytes for %s; review a smaller PR or filter files",
			len(prompt), DefaultInputBudgetBytes, a.Name())
	}

	schemaPath, cleanup, err := writeSchema()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	out, err := a.run(ctx, strings.NewReader(prompt), "codex", "exec", "--output-schema", schemaPath)
	if err != nil {
		return nil, fmt.Errorf("codex exec: %w", err)
	}
	findings, err := model.ParseFindings(out)
	if err != nil {
		return nil, fmt.Errorf("codex output: %w", err)
	}
	return &review.ModelReviewResult{
		Model:     a.Name(),
		Findings:  findings,
		RawOutput: string(out),
	}, nil
}

// writeSchema persists the embedded JSON Schema to a temp file and
// returns the path along with a cleanup func. The temp file is created
// per-invocation so concurrent runs don't collide.
func writeSchema() (string, func(), error) {
	f, err := os.CreateTemp("", "diffsmith-codex-schema-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("create temp schema: %w", err)
	}
	if _, err := f.Write(schemaJSON); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("write temp schema: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, fmt.Errorf("close temp schema: %w", err)
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

// Compile-time interface guards: catch any future refactor that
// accidentally drops a capability. diffsmith-0hy.
var (
	_ model.Reviewer    = (*Adapter)(nil)
	_ model.Synthesizer = (*Adapter)(nil)
)
