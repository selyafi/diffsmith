package app

import (
	"bufio"
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/post"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
	"github.com/selyafi/diffsmith/internal/tui"
)

// runTUI is the seam between runReview and the interactive Bubble Tea
// program. The default impl wires up a tui.LoaderModel + bubbletea
// program + async pipeline goroutine that pushes PhaseStatusMsg /
// LoadErrorMsg / LoadReadyMsg into the loader. Tests swap this for a
// fake that drives the pipeline synchronously and operates on the inner
// ReviewModel via tui.LoaderModel.ReviewModel — see withFakeTUI in
// review_test.go.
var runTUI = func(loader *tui.LoaderModel, pipeline func(send func(tea.Msg))) error {
	p := tea.NewProgram(loader)
	go pipeline(func(msg tea.Msg) { p.Send(msg) })
	_, err := p.Run()
	return err
}

// submitPost is the seam between runReview and the GitHub poster. Tests
// swap this to assert whether the Submit branch ran (and with what
// findings) without shelling out to gh. Mirrors the runTUI pattern.
var submitPost = func(ctx context.Context, out io.Writer, target review.ReviewTarget, marked []review.Finding) error {
	return (&post.Poster{Out: out}).Submit(ctx, target, marked)
}

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

	cmd.Flags().StringVar(&flags.model, "model", "codex", "model adapter to use (codex, claude)")
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
	return runReviewByURL(ctx, cmd, args[0], flags, registry, models)
}

// runReviewByURL is the URL-driven entry point used by both `review`
// (one-shot CLI) and `inbox` (interactive). It skips arg parsing and
// runs the existing fetch → model → validate → TUI → post pipeline for
// a single URL.
func runReviewByURL(ctx context.Context, cmd *cobra.Command, url string, flags *reviewFlags, registry *provider.Registry, models map[string]model.Model) error {
	p, err := registry.Find(url)
	if err != nil {
		return err
	}
	if err := p.Preflight(ctx); err != nil {
		return err
	}

	// --print-prompt and --dry-run bypass the TUI entirely: they need
	// the fetched diff synchronously and print to stdout. No spinner.
	if flags.printPrompt || flags.dryRun {
		input, err := p.Fetch(ctx, url)
		if err != nil {
			return err
		}
		if flags.printPrompt {
			_, err := io.WriteString(cmd.OutOrStdout(), model.BuildPrompt(input))
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "fetched %d file(s) from %s (model call skipped: --dry-run)\n", len(input.Files), input.Target.URL)
		return nil
	}

	m, ok := models[flags.model]
	if !ok {
		return fmt.Errorf("unknown model %q (supported: codex, claude)", flags.model)
	}
	// Preflight before launching the TUI so a missing CLI surfaces as a
	// clean error rather than flashing the TUI open and immediately
	// closing it on a LoadErrorMsg.
	if err := m.Preflight(ctx); err != nil {
		return err
	}

	// The pipeline runs in a goroutine launched by runTUI; it pushes
	// PhaseStatusMsg as it transitions between fetch/model/validate, and
	// pushes LoadReadyMsg or LoadErrorMsg at the end. The closure
	// captures the per-session dependencies; nothing model-state-related
	// is shared mutably across the goroutine boundary.
	var input *review.ReviewInput
	loader := tui.NewLoaderModel("Fetching diff…")
	pipeline := func(send func(tea.Msg)) {
		send(tui.PhaseStatusMsg("Fetching diff…"))
		var fetchErr error
		input, fetchErr = p.Fetch(ctx, url)
		if fetchErr != nil {
			send(tui.LoadErrorMsg{Err: fetchErr})
			return
		}

		send(tui.PhaseStatusMsg(fmt.Sprintf("Calling %s (this can take 30–90s)…", m.Name())))
		result, modelErr := m.Review(ctx, input)
		if modelErr != nil {
			send(tui.LoadErrorMsg{Err: modelErr})
			return
		}

		send(tui.PhaseStatusMsg("Validating findings against the diff…"))
		idx := diff.NewIndex(input.Files)
		valid, quarantined := review.Validate(result.Findings, m.Name(), idx)

		send(tui.LoadReadyMsg{Findings: valid, Quarantined: quarantined})
	}
	if err := runTUI(loader, pipeline); err != nil {
		return err
	}
	if loaderErr := loader.Err(); loaderErr != nil {
		return loaderErr
	}
	if input == nil {
		// Pipeline never produced an input (cancelled before fetch). No
		// further work to do; just exit cleanly.
		return nil
	}

	if marked := loader.GetFindingsMarkedForPost(); len(marked) > 0 {
		var postErr error
		switch {
		case flags.printPayload:
			postErr = (&post.Poster{Out: cmd.OutOrStdout()}).PrintPayload(input.Target, marked)
		case confirmPost(cmd, len(marked), input.Target.Number):
			postErr = submitPost(ctx, cmd.OutOrStdout(), input.Target, marked)
		}
		if postErr != nil {
			return postErr
		}
	}

	writeFindings(cmd.OutOrStdout(), loader.GetApprovedFindings(), loader.Quarantined(), loader.TotalReviewed())
	return nil
}

// confirmPost prints a one-line preview to cmd.OutOrStdout and reads a
// single byte from cmd.InOrStdin to gate the upstream submit. The "press
// y, anything else to abort" framing means a reflex Enter (newline byte)
// bails safely; EOF (empty stdin) also bails so an unattached terminal
// can never auto-confirm. Capital and lowercase Y are both accepted so
// users don't have to switch shift state to confirm.
func confirmPost(cmd *cobra.Command, n, prNumber int) bool {
	fmt.Fprintf(cmd.OutOrStdout(),
		"About to post %d comment(s) to PR #%d. Press y to confirm, anything else to abort.\n",
		n, prNumber)
	b, err := bufio.NewReader(cmd.InOrStdin()).ReadByte()
	if err != nil {
		return false
	}
	return b == 'y' || b == 'Y'
}

// writeFindings renders the post-TUI summary so approved findings can be
// piped or captured by non-interactive consumers. totalReviewed is the
// number of findings the user saw in the TUI (after validation): when it's
// zero, the model genuinely returned nothing; when it's non-zero but
// `valid` is empty, the user approved none — those two cases need
// different copy so a reviewer can tell them apart (per diffsmith-14p).
func writeFindings(w io.Writer, valid []review.Finding, quarantined []review.Quarantined, totalReviewed int) {
	if totalReviewed == 0 && len(quarantined) == 0 {
		fmt.Fprintln(w, "No findings.")
		return
	}
	if len(valid) == 0 && len(quarantined) == 0 {
		fmt.Fprintf(w, "0 of %d findings approved.\n", totalReviewed)
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
