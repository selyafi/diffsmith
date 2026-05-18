// Package app wires the diffsmith CLI command tree.
package app

import (
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "diffsmith",
		Short: "Local, human-in-the-loop AI review cockpit for GitHub PRs and GitLab MRs",
		Long: "Diffsmith fetches a pull or merge request diff, asks a selected AI CLI to draft\n" +
			"review findings, validates them against the diff, and opens a terminal UI where\n" +
			"you inspect, edit, approve, dismiss, and copy comments. Diffsmith never posts.",
		SilenceUsage: true,
	}
	root.AddCommand(newReviewCmd())
	return root
}

// Execute parses argv and runs the matching command.
func Execute() error {
	return newRootCmd().Execute()
}
