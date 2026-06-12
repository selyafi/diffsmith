package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/repodetect"
	"github.com/selyafi/diffsmith/internal/tui"
)

// inboxAction mirrors tui.InboxAction at the app layer so tests don't
// need to reach into the TUI package. Translated 1:1.
type inboxAction int

const (
	inboxActionOpen    inboxAction = iota + 1 // never 0 — that's "no action seen"
	inboxActionRefresh
	inboxActionQuit
)

// inboxLister is the seam tests use to inject a fake "show the list,
// get a pick" implementation. Production code wires this to a fresh
// tea.NewProgram(InboxModel).
type inboxLister func() (*provider.PRSummary, inboxAction, error)

// inboxOpener is the seam for "review this URL". Production wires it
// to runReviewByURL (with the user's flag state captured at command
// construction time).
type inboxOpener func(ctx context.Context, cmd *cobra.Command, url string) error

func newInboxCmd(registry *provider.Registry, models map[string]model.Model) *cobra.Command {
	flags := &reviewFlags{}
	cmd := &cobra.Command{
		Use:   "inbox",
		Short: "Interactively review open PRs/MRs for the current git repo",
		Long: "Detects the current repo from `git remote`, lists its open PRs/MRs,\n" +
			"and opens the picked one in the review TUI. Quit review to return to\n" +
			"the list. `r` refreshes; `q` exits.\n\n" +
			"This is also the default behavior of bare `diffsmith` (no subcommand).",
		Args: cobra.NoArgs,
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
			return runInboxCommandWithSelected(cmd, flags, registry, selected)
		},
	}
	registerModelFlowFlags(cmd, flags)
	registerPostFlowFlags(cmd, flags)
	return cmd
}

// runInboxCommandWithSelected is the cobra RunE shared by the `inbox`
// subcommand and bare `diffsmith` (no subcommand). Both paths run the
// picker upstream and pass the resolved SelectedModels here.
func runInboxCommandWithSelected(cmd *cobra.Command, flags *reviewFlags, registry *provider.Registry, selected *model.SelectedModels) error {
	// ctx is not used in this function — repodetect/registry calls don't take
	// one, and the opener closure below has its own ctx parameter. The
	// runInbox helper resolves cmd.Context() itself where needed.
	repo, err := repodetect.Detect()
	if err != nil {
		return err
	}
	p, err := registry.ByHost(repo.Host)
	if err != nil {
		return err
	}

	opener := func(ctx context.Context, cmd *cobra.Command, url string) error {
		return runReviewByURL(ctx, cmd, url, flags, selected, registry)
	}

	var current *tui.InboxModel
	lister := func() (*provider.PRSummary, inboxAction, error) {
		if current == nil {
			return nil, 0, fmt.Errorf("inbox: lister called before model initialized")
		}
		// Clear the previous session's exit state so a teardown without
		// a keypress reads as a clean quit, not a replay of the last
		// open. diffsmith-qe5.
		current.ResetSession()
		prog := tea.NewProgram(current, tea.WithAltScreen())
		if _, err := prog.Run(); err != nil {
			return nil, 0, err
		}
		return current.Pick(), inboxAction(current.Action()), nil
	}

	return runInbox(cmd, p, repo, &current, lister, opener)
}

// runInbox is the single source of truth for the loop. modelPtr is
// optional: production code passes &current to keep the lister closure
// in sync with refreshes; tests pass nil because the test lister is
// pre-scripted.
func runInbox(cmd *cobra.Command, p provider.Provider, repo provider.RepoCoord,
	modelPtr **tui.InboxModel, lister inboxLister, opener inboxOpener) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.PreflightList(ctx); err != nil {
		return err
	}
	summaries, err := p.List(ctx, repo)
	if err != nil {
		return err
	}
	if modelPtr != nil {
		*modelPtr = tui.NewInboxModel(summaries, repo.Owner, repo.Name)
	}

	for {
		pick, action, err := lister()
		if err != nil {
			return err
		}
		switch action {
		case 0:
			// inboxAction zero-value: the Bubble Tea program exited
			// without Update setting an action (e.g., SIGINT, SIGWINCH-
			// driven teardown). Treat as a clean quit, not a usage error.
			return nil
		case inboxActionQuit:
			return nil
		case inboxActionRefresh:
			summaries, err := p.List(ctx, repo)
			if err != nil {
				return err
			}
			if modelPtr != nil {
				*modelPtr = tui.NewInboxModel(summaries, repo.Owner, repo.Name)
			}
		case inboxActionOpen:
			if pick == nil {
				return fmt.Errorf("inbox: open action with nil pick")
			}
			if err := opener(ctx, cmd, pick.URL); err != nil {
				return err
			}
			// model stays put — cached list persists across reviews per spec §7
		default:
			return fmt.Errorf("inbox: unknown action %d", action)
		}
	}
}

// runInboxWithDeps is the test entry point — same behavior as runInbox
// without managing a real *tui.InboxModel.
func runInboxWithDeps(cmd *cobra.Command, p provider.Provider, repo provider.RepoCoord,
	lister inboxLister, opener inboxOpener) error {
	return runInbox(cmd, p, repo, nil, lister, opener)
}
