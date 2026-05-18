package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/provider"
)

type reviewFlags struct {
	model       string
	dryRun      bool
	printPrompt bool
}

func newReviewCmd(registry *provider.Registry) *cobra.Command {
	flags := &reviewFlags{}

	cmd := &cobra.Command{
		Use:   "review <pr-or-mr-url>",
		Short: "Draft review comments for a GitHub PR or GitLab MR",
		Long: "Fetch the diff for the given PR/MR, run the selected model CLI, validate the\n" +
			"findings against the diff, and open the review TUI. The model-call path lands\n" +
			"in M3; for now --print-prompt is the supported end-to-end smoke test.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, args, flags, registry)
		},
	}

	cmd.Flags().StringVar(&flags.model, "model", "codex", "model adapter to use (codex|claude|gemini)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "fetch and normalize the diff, then stop before the model call")
	cmd.Flags().BoolVar(&flags.printPrompt, "print-prompt", false, "print the model prompt and exit without invoking the model")

	return cmd
}

func runReview(cmd *cobra.Command, args []string, flags *reviewFlags, registry *provider.Registry) error {
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
		return writeStubPrompt(cmd.OutOrStdout(), input)
	case flags.dryRun:
		fmt.Fprintf(cmd.OutOrStdout(), "fetched %d file(s) from %s (model call skipped: --dry-run)\n", len(input.Files), input.Target.URL)
		return nil
	default:
		return errors.New("model invocation lands in M3 (Codex adapter). Try --print-prompt or --dry-run")
	}
}

// writeStubPrompt produces a M2-shaped summary of the fetch result. The
// final structured prompt (per docs/prompt-contract.md) is M3 work; this
// is the smoke-test artifact that proves the GitHub + diff-parser pipeline
// works end-to-end.
func writeStubPrompt(w io.Writer, input *provider.ReviewInput) error {
	fmt.Fprintln(w, "# Review Target")
	fmt.Fprintf(w, "URL: %s\n", input.Target.URL)
	fmt.Fprintf(w, "Title: %s\n", input.Title)
	fmt.Fprintf(w, "Author: %s\n", input.Author)
	if input.Target.HeadRef != "" || input.Target.BaseRef != "" {
		fmt.Fprintf(w, "Branch: %s -> %s\n", input.Target.HeadRef, input.Target.BaseRef)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "# Files (%d)\n", len(input.Files))
	for _, f := range input.Files {
		fmt.Fprintf(w, "- %s (%s, %d hunk(s))\n", f.Path, f.Kind, len(f.Hunks))
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "# Diff")
	if _, err := io.WriteString(w, input.RawDiff); err != nil {
		return err
	}
	return nil
}
