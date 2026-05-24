// Package app wires the diffsmith CLI command tree.
package app

import (
	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/model/antigravitycli"
	"github.com/selyafi/diffsmith/internal/model/claudecli"
	"github.com/selyafi/diffsmith/internal/model/codexcli"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/provider/githubgh"
	"github.com/selyafi/diffsmith/internal/provider/gitlabglab"
)

// version is set via SetVersion from main at startup. The literal "dev"
// is the local-build sentinel; the release workflow stamps the real tag
// via -ldflags (see Makefile).
var version = "dev"

// SetVersion lets cmd/diffsmith inject the build-stamped version string
// before Execute runs. Keeping the seam in the app package means tests
// can override the version without touching the main package.
func SetVersion(v string) {
	if v != "" {
		version = v
	}
}

func newRootCmd() *cobra.Command {
	registry := defaultRegistry()
	models := defaultModels()
	// Bare `diffsmith` (no subcommand) runs the inbox flow against the
	// current git repo. The `inbox` subcommand remains registered for
	// muscle-memory users; both share runInboxCommand.
	rootFlags := &reviewFlags{model: "codex"}
	root := &cobra.Command{
		Use:   "diffsmith",
		Short: "Local, human-in-the-loop AI review cockpit for GitHub PRs and GitLab MRs",
		Long: "Diffsmith fetches a pull or merge request diff, asks a selected AI CLI to draft\n" +
			"review findings, validates them against the diff, and opens a terminal UI where\n" +
			"you inspect, edit, approve, dismiss, and copy comments. Diffsmith never posts.\n\n" +
			"Run without arguments inside a git repo to enter the inbox.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInboxCommand(cmd, rootFlags, registry, models)
		},
	}
	root.AddCommand(newReviewCmd(registry, models))
	root.AddCommand(newInboxCmd(registry, models))
	return root
}

// defaultRegistry returns the provider registry wired to real CLIs. Tests
// build their own registry with stub providers and pass it to
// newReviewCmd directly.
func defaultRegistry() *provider.Registry {
	return provider.NewRegistry(githubgh.New(nil), gitlabglab.New(nil))
}

// defaultModels returns the model registry wired to real CLIs. Codex
// and Claude are required v1 adapters. Antigravity (agy) is registered
// so `--model antigravity` surfaces the actionable Preflight error from
// spike S8b instead of an "unknown model" CLI error; the adapter itself
// refuses to run because agy has no non-interactive auth path in v1.
func defaultModels() map[string]model.Model {
	return map[string]model.Model{
		"codex":       codexcli.New(nil),
		"claude":      claudecli.New(nil),
		"antigravity": antigravitycli.New(nil),
	}
}

// Execute parses argv and runs the matching command.
func Execute() error {
	return newRootCmd().Execute()
}
