package app

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/post"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
	"github.com/selyafi/diffsmith/internal/tui"
)

// runTUI is the seam between runReview and the interactive Bubble Tea
// program. Tests swap this to drive the model without a TTY.
var runTUI = tui.Run

type reviewFlags struct {
	model        string
	dryRun       bool
	printPrompt  bool
	printPayload bool
}

func newReviewCmd(registry *provider.Registry, models map[string]model.Model) *cobra.Command {
	flags := &reviewFlags{}

	cmd := &cobra.Command{
		Use:   "review <pr-or-mr-url>",
		Short: "Draft review comments for a GitHub PR or GitLab MR",
		Long: "Fetch the diff for the given PR/MR, run the selected model CLI, validate the\n" +
			"findings against the diff, and open the review TUI for approval. Approved\n" +
			"findings are written to stdout on quit.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, args, flags, registry, models)
		},
	}

	cmd.Flags().StringVar(&flags.model, "model", "codex", "model adapter to use (codex)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "fetch and normalize the diff, then stop before the model call")
	cmd.Flags().BoolVar(&flags.printPrompt, "print-prompt", false, "print the model prompt and exit without invoking the model")
	cmd.Flags().BoolVar(&flags.printPayload, "print-payload", false, "print the GraphQL payload(s) for findings marked with 'p' in the TUI, instead of posting upstream")

	return cmd
}

func runReview(cmd *cobra.Command, args []string, flags *reviewFlags, registry *provider.Registry, models map[string]model.Model) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	url := args[0]
	p, err := registry.Find(url)
	if err != nil {
		return err
	}
	if err := p.Preflight(ctx); err != nil {
		return err
	}

	input, err := p.Fetch(ctx, url)
	if err != nil {
		return err
	}

	switch {
	case flags.printPrompt:
		_, err := io.WriteString(cmd.OutOrStdout(), model.BuildPrompt(input))
		return err
	case flags.dryRun:
		fmt.Fprintf(cmd.OutOrStdout(), "fetched %d file(s) from %s (model call skipped: --dry-run)\n", len(input.Files), input.Target.URL)
		return nil
	}

	m, ok := models[flags.model]
	if !ok {
		return fmt.Errorf("unknown model %q (supported: codex)", flags.model)
	}
	if err := m.Preflight(ctx); err != nil {
		return err
	}
	result, err := m.Review(ctx, input)
	if err != nil {
		return err
	}

	idx := diff.NewIndex(input.Files)
	valid, quarantined := review.Validate(result.Findings, m.Name(), idx)

	tm := tui.NewModel(valid)
	if err := runTUI(tm); err != nil {
		return err
	}

	if marked := tm.GetFindingsMarkedForPost(); len(marked) > 0 {
		poster := &post.Poster{Out: cmd.OutOrStdout()}
		var postErr error
		if flags.printPayload {
			postErr = poster.PrintPayload(input.Target, marked)
		} else {
			postErr = poster.Submit(ctx, input.Target, marked)
		}
		if postErr != nil {
			return postErr
		}
	}

	writeFindings(cmd.OutOrStdout(), tm.GetApprovedFindings(), quarantined)
	return nil
}

// writeFindings renders the post-TUI summary so approved findings can be
// piped or captured by non-interactive consumers.
func writeFindings(w io.Writer, valid []review.Finding, quarantined []review.Quarantined) {
	if len(valid) == 0 && len(quarantined) == 0 {
		fmt.Fprintln(w, "No findings.")
		return
	}
	for _, f := range valid {
		fmt.Fprintf(w, "%s:%d [%s] (%s, confidence %.2f)\n", f.File, f.Line, f.Severity, f.Model, f.Confidence)
		fmt.Fprintf(w, "  Title:   %s\n", f.Title)
		fmt.Fprintf(w, "  Comment: %s\n", f.SuggestedComment)
		if f.Evidence != "" {
			fmt.Fprintf(w, "  Why:     %s\n", f.Evidence)
		}
		if f.FixHint != "" {
			fmt.Fprintf(w, "  Fix:     %s\n", f.FixHint)
		}
		fmt.Fprintln(w)
	}
	if n := len(quarantined); n > 0 {
		fmt.Fprintf(w, "(%d finding(s) quarantined by validation; pass --debug to inspect.)\n", n)
	}
}
