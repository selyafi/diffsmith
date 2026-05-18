package app

import (
	"errors"

	"github.com/spf13/cobra"
)

type reviewFlags struct {
	model       string
	dryRun      bool
	printPrompt bool
}

func newReviewCmd() *cobra.Command {
	flags := &reviewFlags{}

	cmd := &cobra.Command{
		Use:   "review <pr-or-mr-url>",
		Short: "Draft review comments for a GitHub PR or GitLab MR",
		Long: "Fetch the diff for the given PR/MR, run the selected model CLI, validate the\n" +
			"findings against the diff, and open the review TUI. Not implemented yet.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented: review pipeline lands in M2 (GitHub provider) and M3 (Codex adapter)")
		},
	}

	cmd.Flags().StringVar(&flags.model, "model", "codex", "model adapter to use (codex|claude|gemini)")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "fetch and normalize the diff, then stop before the model call")
	cmd.Flags().BoolVar(&flags.printPrompt, "print-prompt", false, "print the model prompt and exit without invoking the model")

	return cmd
}
