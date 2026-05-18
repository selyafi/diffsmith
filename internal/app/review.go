package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/model"
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
			"in M3b; for now --print-prompt and --dry-run are the supported end-to-end\n" +
			"smoke tests.",
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
		_, err := io.WriteString(cmd.OutOrStdout(), model.BuildPrompt(input))
		return err
	case flags.dryRun:
		fmt.Fprintf(cmd.OutOrStdout(), "fetched %d file(s) from %s (model call skipped: --dry-run)\n", len(input.Files), input.Target.URL)
		return nil
	default:
		return errors.New("model invocation lands in M3b (Codex adapter). Try --print-prompt or --dry-run")
	}
}
