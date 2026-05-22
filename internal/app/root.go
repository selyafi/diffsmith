// Package app wires the diffsmith CLI command tree.
package app

import (
	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/model/claudecli"
	"github.com/selyafi/diffsmith/internal/model/codexcli"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/provider/githubgh"
	"github.com/selyafi/diffsmith/internal/provider/gitlabglab"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "diffsmith",
		Short: "Local, human-in-the-loop AI review cockpit for GitHub PRs and GitLab MRs",
		Long: "Diffsmith fetches a pull or merge request diff, asks a selected AI CLI to draft\n" +
			"review findings, validates them against the diff, and opens a terminal UI where\n" +
			"you inspect, edit, approve, dismiss, and copy comments. Diffsmith never posts.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newReviewCmd(defaultRegistry(), defaultModels()))
	return root
}

// defaultRegistry returns the provider registry wired to real CLIs. Tests
// build their own registry with stub providers and pass it to
// newReviewCmd directly.
func defaultRegistry() *provider.Registry {
	return provider.NewRegistry(githubgh.New(nil), gitlabglab.New(nil))
}

// defaultModels returns the model registry wired to real CLIs. Claude
// and Gemini land in M7; Gemini is experimental until spike S8 closes.
func defaultModels() map[string]model.Model {
	return map[string]model.Model{
		"codex":  codexcli.New(nil),
		"claude": claudecli.New(nil),
	}
}

// Execute parses argv and runs the matching command.
func Execute() error {
	return newRootCmd().Execute()
}
