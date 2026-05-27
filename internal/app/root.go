// Package app wires the diffsmith CLI command tree.
package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/model/antigravitycli"
	"github.com/selyafi/diffsmith/internal/model/claudecli"
	"github.com/selyafi/diffsmith/internal/model/codexcli"
	"github.com/selyafi/diffsmith/internal/model/geminicli"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/provider/githubgh"
	"github.com/selyafi/diffsmith/internal/provider/gitlabglab"
	"github.com/selyafi/diffsmith/internal/tui"
	"github.com/selyafi/diffsmith/internal/update"
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
	// muscle-memory users; both share runInboxCommandWithSelected.
	rootFlags := &reviewFlags{}
	root := &cobra.Command{
		Use:   "diffsmith",
		Short: "Local, human-in-the-loop AI review cockpit for GitHub PRs and GitLab MRs",
		Long: "Diffsmith fetches a pull or merge request diff, asks a selected AI CLI to draft\n" +
			"review findings, validates them against the diff, and opens a terminal UI where\n" +
			"you inspect, edit, approve, dismiss, and copy comments. Approved findings can\n" +
			"optionally be posted upstream as inline review comments; posting is opt-in,\n" +
			"requires you to press 'p' on an approved finding, and never runs without an\n" +
			"explicit confirmation prompt.\n\n" +
			"Run without arguments inside a git repo to enter the inbox.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			items := preflightModels(ctx, models)
			selected, err := pickerRunner(items, models)
			if err != nil {
				return err
			}
			return runInboxCommandWithSelected(cmd, rootFlags, registry, selected)
		},
		// PersistentPreRun fires BEFORE every subcommand (review, inbox)
		// and before the bare-root RunE. Running at startup means users
		// see an upgrade notice immediately, and the check still fires
		// when the subcommand later errors — which is when an upgrade
		// hint is most useful. update.Check is silent on any failure
		// and bounded by a 3-second HTTP timeout; the common cache-hit
		// path is an O(1) file read.
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			update.Check(ctx, version, cmd.ErrOrStderr())
		},
	}
	registerPostFlowFlags(root, rootFlags)
	root.AddCommand(newReviewCmd(registry, models))
	root.AddCommand(newInboxCmd(registry, models))
	return root
}

// preflightModels probes each adapter and returns a slice of picker
// items annotated with availability. Order is stable: codex, claude,
// gemini, antigravity.
func preflightModels(ctx context.Context, models map[string]model.Model) []tui.ModelPickerItem {
	order := []string{"codex", "claude", "gemini", "antigravity"}
	items := make([]tui.ModelPickerItem, 0, len(order))
	for _, name := range order {
		m, ok := models[name]
		if !ok {
			continue
		}
		err := m.Preflight(ctx)
		items = append(items, tui.ModelPickerItem{
			Name:        name,
			Available:   err == nil,
			Unavailable: errMsg(err),
		})
	}
	return items
}

func errMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// pickerRunner is the seam tests use to bypass the interactive picker
// TUI. Production code uses runPickerForModels; tests can swap this
// to return a fixed *model.SelectedModels.
var pickerRunner = func(items []tui.ModelPickerItem, models map[string]model.Model) (*model.SelectedModels, error) {
	return runPickerForModels(items, models)
}

// runPickerForModels shows the picker TUI and returns the resulting
// SelectedModels, or nil + error if cancelled / nothing selected.
func runPickerForModels(items []tui.ModelPickerItem, models map[string]model.Model) (*model.SelectedModels, error) {
	available := 0
	for _, it := range items {
		if it.Available {
			available++
		}
	}
	if available == 0 {
		return nil, fmt.Errorf("no review CLIs available; install/auth at least one of: codex, claude, gemini, antigravity")
	}

	picker := tui.NewModelPickerModel(items)
	prog := tea.NewProgram(picker, tea.WithAltScreen())
	if _, err := prog.Run(); err != nil {
		return nil, fmt.Errorf("picker: %w", err)
	}
	if picker.Cancelled() {
		return nil, fmt.Errorf("review cancelled")
	}
	names := picker.SelectedNames()
	if len(names) == 0 {
		return nil, fmt.Errorf("no models selected")
	}
	chosen := make([]model.Model, 0, len(names))
	for _, n := range names {
		if m, ok := models[n]; ok {
			chosen = append(chosen, m)
		}
	}
	return model.NewSelectedModels(chosen), nil
}

// defaultRegistry returns the provider registry wired to real CLIs. Tests
// build their own registry with stub providers and pass it to
// newReviewCmd directly.
func defaultRegistry() *provider.Registry {
	return provider.NewRegistry(githubgh.New(nil), gitlabglab.New(nil))
}

// defaultModels returns the model registry wired to real CLIs. Codex,
// Claude, and Gemini are the working v1 adapters. Antigravity (agy) is
// still registered so a user who selects it sees the actionable
// Preflight error from spike S8b (no non-interactive auth path) rather
// than an "unknown model" CLI error; the adapter itself refuses to run.
func defaultModels() map[string]model.Model {
	return map[string]model.Model{
		"codex":       codexcli.New(nil),
		"claude":      claudecli.New(nil),
		"gemini":      geminicli.New(nil),
		"antigravity": antigravitycli.New(nil),
	}
}

// Execute parses argv and runs the matching command.
func Execute() error {
	return newRootCmd().Execute()
}
