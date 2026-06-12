package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

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
	p := tea.NewProgram(loader, tea.WithAltScreen())
	go pipeline(func(msg tea.Msg) { p.Send(msg) })
	_, err := p.Run()
	return err
}

// submitPost is the seam between runReview and the GitHub poster. Tests
// swap this to assert whether the Submit branch ran (and with what
// findings) without shelling out to gh. Mirrors the runTUI pattern.
//
// oldPaths carries the rename map derived from the parsed diff so the
// GitLab path can populate position.old_path correctly for
// renamed-with-hunks files. The GitHub path ignores it.
var submitPost = func(ctx context.Context, out io.Writer, target review.ReviewTarget, marked []review.Finding, repost bool, oldPaths map[string]string) error {
	return (&post.Poster{Out: out, Repost: repost, OldPaths: oldPaths}).Submit(ctx, target, marked)
}

// registerPostFlowFlags adds the three flags that govern the
// post-flow on every entry point (review, inbox, bare diffsmith).
// Centralising the registration so the entry points stay symmetric;
// before this any drift (a flag added to review but not inbox) was
// invisible until users noticed it (diffsmith-3e8).
func registerPostFlowFlags(cmd *cobra.Command, flags *reviewFlags) {
	cmd.Flags().BoolVar(&flags.printPayload, "print-payload", false, "print the host-specific upstream payload(s) for findings marked with 'p' in the TUI (GitHub GraphQL addThread input on PRs; GitLab discussions API JSON on MRs), instead of posting upstream")
	cmd.Flags().BoolVar(&flags.repost, "repost", false, "bypass dedup and post every approved finding even if a diffsmith thread already exists at the same file:line")
	cmd.Flags().BoolVar(&flags.debug, "debug", false, "print each quarantined model finding's (file:line), title, and validator rejection reason after the TUI session ends")
}

// registerModelFlowFlags adds the flags that govern model invocation
// on every entry point (review, inbox, bare diffsmith). Like
// registerPostFlowFlags, the goal is to keep entry-point parity
// automatic: if --input-budget is registered on one but not the
// others, the inbox-launched review path would silently ignore the
// override. Currently this is just --input-budget; future model
// flags (e.g. --max-files) should join here rather than be inlined.
func registerModelFlowFlags(cmd *cobra.Command, flags *reviewFlags) {
	cmd.Flags().IntVar(&flags.inputBudget, "input-budget", 0, "override the per-adapter prompt-size cap in bytes (default: 1 MiB per adapter; 0 keeps the default)")
	cmd.Flags().DurationVar(&flags.modelTimeout, "model-timeout", 10*time.Minute, "per-model wall-clock cap; a model exceeding it is cancelled and dropped from the review (0 disables)")
	cmd.Flags().BoolVar(&flags.noContext, "no-context", false, "do not send the PR/MR description or fetch linked-issue acceptance criteria to the model (diff-only review)")
	cmd.Flags().StringArrayVar(&flags.exclude, "exclude", nil, "exclude files from the review diff (repeatable). Patterns: trailing '/' = directory tree at any depth ('vendor/'); no '/' = basename glob ('*.lock'); otherwise full-path glob ('internal/gen/*.go')")
}

// applyExcludes filters the fetched input per --exclude, mutating it in
// place. The returned note ("" when nothing was excluded) must reach the
// user — run summary on the TUI path, stderr on the bypass path — so
// exclusions are never silent. Excluding every file is an error: the
// review would be vacuous and a model call a waste.
func applyExcludes(input *review.ReviewInput, patterns []string) (string, error) {
	kept, keptRaw, excludedPaths, err := diff.Exclude(input.Files, input.RawDiff, patterns)
	if err != nil {
		return "", err
	}
	if len(excludedPaths) == 0 {
		return "", nil
	}
	if len(kept) == 0 {
		return "", fmt.Errorf("all %d changed file(s) matched --exclude; nothing left to review", len(excludedPaths))
	}
	removedBytes := len(input.RawDiff) - len(keptRaw)
	input.Files, input.RawDiff = kept, keptRaw
	return fmt.Sprintf("--exclude removed %d of %d changed file(s), %d bytes of diff",
		len(excludedPaths), len(excludedPaths)+len(kept), removedBytes), nil
}

// renameMapFromFiles extracts the post-image → pre-image rename mapping
// from the parsed diff. Only renamed-with-hunks files get an entry;
// same-path files are absent from the map so callers can use it as a
// sparse lookup (`map[file]` returns "" for unchanged paths).
func renameMapFromFiles(files []*diff.DiffFile) map[string]string {
	var m map[string]string
	for _, f := range files {
		if f == nil || f.Kind != diff.FileRenameWithHunks {
			continue
		}
		if f.OldPath == "" || f.OldPath == f.Path {
			continue
		}
		if m == nil {
			m = make(map[string]string)
		}
		m[f.Path] = f.OldPath
	}
	return m
}

type reviewFlags struct {
	dryRun      bool
	printPrompt bool
	// exclude holds --exclude patterns applied to the fetched diff
	// before context enrichment and prompt build, on every entry
	// point. See applyExcludes for semantics.
	exclude []string
	// printSynthesisPrompt prints the multi-model synthesis prompt
	// (BuildSynthesisPrompt) using stub reviewer outputs, so operators
	// can inspect the lead model's input — rules, ordering, sentinels,
	// security warnings — without spending model quota. Disjoint from
	// printPrompt: both can be set; both prompts are then emitted with
	// a separator between them.
	printSynthesisPrompt bool
	printPayload         bool
	// repost bypasses the dedup-before-post step so every approved
	// finding is sent to the host API even if a diffsmith thread
	// already exists at the same (file, line). Default false:
	// duplicates are skipped with a summary line.
	repost bool
	// debug expands the post-TUI quarantine section: instead of a
	// single counter line pointing at --debug, each rejected candidate
	// is printed with its (file:line), title, and the validator's
	// reason. The default-off path stays compact for normal use.
	debug bool
	// inputBudget overrides every selected adapter's compiled-in
	// DefaultInputBudgetBytes. Zero (the cobra default for an unset
	// int flag) means "leave each adapter's default in place" — see
	// applyInputBudget for the no-op-on-zero contract.
	inputBudget int
	// noContext disables diffsmith-144 context enrichment: when set, the
	// PR/MR description is withheld from the prompt and linked-issue
	// acceptance criteria are not fetched. Default (false) sends the
	// description and resolves acceptance criteria.
	noContext bool
	// modelTimeout caps how long each model's Review may run before it is
	// cancelled and dropped from the review. A reviewer CLI can hang
	// (e.g. an MCP server cold-start), and because the models run in a
	// parallel fan-out that joins on all of them, one hang would
	// otherwise block the whole review. Zero disables the cap (inherit
	// the parent context only). See runModelsInParallel.
	modelTimeout time.Duration
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
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			// --print-prompt, --print-synthesis-prompt, and --dry-run
			// all bypass the model entirely; runReviewByURL short-circuits
			// before any model is invoked, so nil SelectedModels is safe.
			if flags.printPrompt || flags.printSynthesisPrompt || flags.dryRun {
				return runReview(cmd, args, flags, nil, registry)
			}

			items := preflightModels(ctx, models)
			selected, err := pickerRunner(items, models)
			if err != nil {
				return err
			}
			return runReview(cmd, args, flags, selected, registry)
		},
	}

	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "fetch and normalize the diff, then stop before the model call")
	cmd.Flags().BoolVar(&flags.printPrompt, "print-prompt", false, "print the single-model review prompt and exit without invoking the model")
	cmd.Flags().BoolVar(&flags.printSynthesisPrompt, "print-synthesis-prompt", false, "print the multi-model synthesis prompt (using stub reviewer outputs) and exit without invoking the model")
	registerModelFlowFlags(cmd, flags)
	registerPostFlowFlags(cmd, flags)

	return cmd
}

func runReview(cmd *cobra.Command, args []string, flags *reviewFlags, selected *model.SelectedModels, registry *provider.Registry) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return runReviewByURL(ctx, cmd, args[0], flags, selected, registry)
}

// runReviewByURL is the URL-driven entry point used by both `review`
// (one-shot CLI) and `inbox` (interactive). It skips arg parsing and
// runs the existing fetch → model → validate → TUI → post pipeline for
// a single URL.
func runReviewByURL(ctx context.Context, cmd *cobra.Command, url string, flags *reviewFlags, selected *model.SelectedModels, registry *provider.Registry) error {
	p, err := registry.Find(url)
	if err != nil {
		return err
	}
	if err := p.Preflight(ctx); err != nil {
		return err
	}

	// --print-prompt, --print-synthesis-prompt, and --dry-run bypass
	// the TUI entirely: they need the fetched diff synchronously and
	// print to stdout. No spinner.
	if flags.printPrompt || flags.printSynthesisPrompt || flags.dryRun {
		input, err := p.Fetch(ctx, url)
		if err != nil {
			return err
		}
		// Filter before enrichment so --print-prompt and --dry-run
		// reflect exactly what a real run would send.
		excludeNote, err := applyExcludes(input, flags.exclude)
		if err != nil {
			return err
		}
		if excludeNote != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "diffsmith: %s\n", excludeNote)
		}
		// Enrich so --print-prompt reflects exactly what the model will
		// see (the # Intent section), and --no-context is honored here
		// too. Notes go to stderr to keep stdout a clean prompt/diff.
		fetcher, _ := p.(review.LinkedIssueFetcher)
		for _, n := range enrichWithContext(ctx, fetcher, input, flags.noContext) {
			fmt.Fprintf(cmd.ErrOrStderr(), "diffsmith: %s\n", n)
		}
		if flags.printPrompt {
			if _, err := io.WriteString(cmd.OutOrStdout(), model.BuildPrompt(input)); err != nil {
				return err
			}
		}
		if flags.printSynthesisPrompt {
			if flags.printPrompt {
				// Separator when both prompts are emitted in one run.
				if _, err := io.WriteString(cmd.OutOrStdout(), "\n--- synthesis prompt ---\n\n"); err != nil {
					return err
				}
			}
			// Stub reviewer outputs make the synthesis prompt's
			// structure visible (rules, sentinels, section markers)
			// without spending model quota. The placeholder text is
			// deliberately not valid JSON so an operator pasting the
			// printed prompt into a model cannot mistake it for a
			// real reviewer result.
			stubResults := []*review.ModelReviewResult{
				{Model: "<reviewer A>", RawOutput: "<reviewer A's JSON findings would appear here, between the BEGIN/END nonce markers>"},
				{Model: "<reviewer B>", RawOutput: "<reviewer B's JSON findings would appear here, between the BEGIN/END nonce markers>"},
			}
			if _, err := io.WriteString(cmd.OutOrStdout(), model.BuildSynthesisPrompt(input, stubResults)); err != nil {
				return err
			}
		}
		if flags.printPrompt || flags.printSynthesisPrompt {
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "fetched %d file(s) from %s (model call skipped: --dry-run)\n", len(input.Files), input.Target.URL)
		return nil
	}

	// Apply --input-budget BEFORE Preflight so the override is in place
	// for any side-effect a future Preflight implementation might have
	// (and so the runtime invariant — "budget is set before Review" —
	// is visibly local to one block rather than spread across the
	// pipeline goroutine).
	applyInputBudget(selected, flags.inputBudget)

	// Preflight all selected models before launching the TUI so missing
	// CLIs surface as clean errors rather than flashing the TUI open and
	// immediately closing it on a LoadErrorMsg.
	for _, m := range selected.All {
		if err := m.Preflight(ctx); err != nil {
			return fmt.Errorf("%s: %w", m.Name(), err)
		}
	}

	// The pipeline runs in a goroutine launched by runTUI; it pushes
	// PhaseStatusMsg as it transitions between fetch/model/validate, and
	// pushes LoadReadyMsg or LoadErrorMsg at the end. The closure
	// captures the per-session dependencies; nothing model-state-related
	// is shared mutably across the goroutine boundary.
	var input *review.ReviewInput
	var runSummary string // populated by pipeline; read by writeFindings after runTUI returns
	loader := tui.NewLoaderModel("Fetching diff…")
	pipeline := func(send func(tea.Msg)) {
		send(tui.PhaseStatusMsg("Fetching diff…"))
		var fetchErr error
		input, fetchErr = p.Fetch(ctx, url)
		if fetchErr != nil {
			send(tui.LoadErrorMsg{Err: fetchErr})
			return
		}

		// --exclude runs before enrichment and prompt build. Unlike
		// context enrichment this IS fatal on error: a bad pattern or
		// an everything-excluded result means the user's filter intent
		// can't be honored, and reviewing the unfiltered diff anyway
		// would silently ignore it.
		excludeNote, excludeErr := applyExcludes(input, flags.exclude)
		if excludeErr != nil {
			send(tui.LoadErrorMsg{Err: excludeErr})
			return
		}

		// diffsmith-144: enrich with the PR/MR description + linked-issue
		// acceptance criteria (gated by --no-context). Never fatal — any
		// failure becomes a surfaced note and the review proceeds.
		fetcher, _ := p.(review.LinkedIssueFetcher)
		if !flags.noContext {
			send(tui.PhaseStatusMsg("Fetching PR/issue context…"))
		}
		contextNotes := enrichWithContext(ctx, fetcher, input, flags.noContext)
		if excludeNote != "" {
			contextNotes = append([]string{excludeNote}, contextNotes...)
		}

		send(tui.PhaseStatusMsg("Reviewing with selected models…"))
		outcomes := runModelsInParallel(ctx, selected.All, input, send, flags.modelTimeout)
		surviving, dropped := splitOutcomes(outcomes)

		if len(surviving) == 0 {
			// Fold context notes into the error itself: the loader's
			// error view renders only the error (and loader.Err() is
			// what cobra prints on exit), so a PhaseStatusMsg sent here
			// would be clobbered before it could ever render.
			// diffsmith-h7a.
			err := aggregateErrors(dropped)
			if len(contextNotes) > 0 {
				err = fmt.Errorf("%w\n  context: %s", err, strings.Join(contextNotes, "; "))
			}
			send(tui.LoadErrorMsg{Err: err})
			return
		}

		final := surviving[0]
		synthesisLeadName := "" // empty == no synthesis happened
		var synthesisSkips []string
		if len(surviving) >= 2 {
			// Spec §14 AC8: try synthesis on each surviving model in
			// priority order. The first one that succeeds wins. If all
			// fail (budget bust, parse error, network), final stays as
			// surviving[0] — the highest-priority surviving model's own
			// findings. Per-failure status messages surface the cause
			// (esp. budget bust on large prompts) so users see why
			// synthesis was skipped.
			for _, candidate := range surviving {
				leadModel := findModelByName(selected.All, candidate.Model)
				if leadModel != nil {
					// Only announce the attempt when there's
					// actually a lead model to invoke. Without this
					// guard the "Synthesizing with X…" message
					// would flash even when the next call is going
					// to bail with "no matching model registered."
					send(tui.PhaseStatusMsg(fmt.Sprintf("Synthesizing with %s…", candidate.Model)))
				}
				synth, skipReason := attemptSynthesis(ctx, leadModel, input, surviving, flags.modelTimeout)
				if skipReason != "" {
					// Every skip surfaces a reason — including the
					// (nil, nil) silent-fallback that attemptSynthesis
					// catches explicitly (diffsmith-4f8). Without
					// this the loop could quietly advance and the
					// user would see surviving[0]'s own findings
					// under the impression synthesis succeeded.
					send(tui.PhaseStatusMsg(fmt.Sprintf("skipping synthesis with %s: %s", candidate.Model, skipReason)))
					synthesisSkips = append(synthesisSkips, fmt.Sprintf("%s: %s", candidate.Model, skipReason))
					continue
				}
				final = synth
				synthesisLeadName = candidate.Model
				break
			}
		}

		send(tui.PhaseStatusMsg("Validating findings against the diff…"))
		idx := diff.NewIndex(input.Files)
		valid, quarantined := review.Validate(final.Findings, final.Model, idx)

		runSummary = buildRunSummary(selected.All, surviving, dropped, synthesisLeadName, len(final.Findings), synthesisSkips, contextNotes)
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
			postErr = (&post.Poster{Out: cmd.OutOrStdout(), OldPaths: renameMapFromFiles(input.Files)}).PrintPayload(input.Target, marked)
		case confirmPost(cmd, len(marked), input.Target):
			postErr = submitPost(ctx, cmd.OutOrStdout(), input.Target, marked, flags.repost, renameMapFromFiles(input.Files))
		}
		if postErr != nil {
			return postErr
		}
	}

	writeFindings(cmd.OutOrStdout(), loader.GetApprovedFindings(), loader.Quarantined(), loader.TotalReviewed(), runSummary, flags.debug)
	return nil
}

// confirmPost prints a one-line preview to cmd.OutOrStdout and reads a
// single byte from cmd.InOrStdin to gate the upstream submit. The "press
// y, anything else to abort" framing means a reflex Enter (newline byte)
// bails safely; EOF (empty stdin) also bails so an unattached terminal
// can never auto-confirm. Capital and lowercase Y are both accepted so
// users don't have to switch shift state to confirm.
func confirmPost(cmd *cobra.Command, n int, target review.ReviewTarget) bool {
	// GitHub uses "PR #N", GitLab uses "MR !N" — match the platform's
	// own convention so users see familiar language.
	label := fmt.Sprintf("PR #%d", target.Number)
	if target.Host == review.HostGitLab {
		label = fmt.Sprintf("MR !%d", target.Number)
	}
	fmt.Fprintf(cmd.OutOrStdout(),
		"About to post %d comment(s) to %s. Press y to confirm, anything else to abort.\n",
		n, label)
	b, err := bufio.NewReader(cmd.InOrStdin()).ReadByte()
	if err != nil {
		return false
	}
	return b == 'y' || b == 'Y'
}

// buildRunSummary formats a one-line audit of a multi-model run for
// the post-quit terminal output. Examples:
//
//	"Models: codex (5 findings), claude (3 findings) → synthesized via codex into 2 findings."
//	"Models: codex (5 findings) → 5 findings."           (single model)
//	"Models: codex (5), claude (3); synthesis failed → using codex (5 findings)."
//	"Models: codex (5 findings); claude failed: gh exec: exit 1"
//
// finalCount is the count of findings BEFORE the validator pass — the
// validator's quarantine count is already surfaced separately.
//
// synthesisSkips carries one human-readable line per candidate that
// did NOT lead synthesis (no Synthesizer capability, error,
// (nil, nil), etc.). When synthesis ultimately failed for all
// candidates, the skip reasons are appended to the summary so the
// user has a persistent audit trail instead of relying on transient
// PhaseStatusMsg flashes (diffsmith-wfq).
func buildRunSummary(selectedAll []model.Model, surviving []*review.ModelReviewResult, dropped []modelOutcome, synthesisLead string, finalCount int, synthesisSkips []string, contextNotes []string) string {
	if len(selectedAll) == 0 && len(surviving) == 0 {
		// Tests can pass nil selectedAll if they only care about the
		// surviving+skips shape. Production always passes a non-nil
		// selected.All; the original guard was 'len(selectedAll) == 0
		// → return ""' but that ate the synthesis-skips audit case
		// where surviving!=empty but selectedAll is a test stub.
		return ""
	}
	parts := make([]string, 0, len(surviving)+len(dropped))
	// Surviving (priority order is preserved by splitOutcomes).
	for _, r := range surviving {
		parts = append(parts, fmt.Sprintf("%s (%d findings)", r.Model, len(r.Findings)))
	}
	for _, d := range dropped {
		parts = append(parts, fmt.Sprintf("%s failed: %v", d.Name, d.Err))
	}
	prefix := "Models: " + strings.Join(parts, ", ")

	var summary string
	switch {
	case len(surviving) == 1:
		summary = prefix + "."
	case synthesisLead != "":
		summary = fmt.Sprintf("%s → synthesized via %s into %d findings.", prefix, synthesisLead, finalCount)
	case len(surviving) >= 2:
		// Synthesis was attempted but all attempts failed; using surviving[0].
		summary = fmt.Sprintf("%s; synthesis failed → using %s (%d findings).", prefix, surviving[0].Model, finalCount)
		if len(synthesisSkips) > 0 {
			summary += "\n  Synthesis skips: " + strings.Join(synthesisSkips, "; ")
		}
	default:
		summary = prefix + "."
	}

	// Context enrichment notes (diffsmith-144) are surfaced regardless of
	// the synthesis path so a dropped/truncated description or acceptance
	// criterion is never silently lost.
	if len(contextNotes) > 0 {
		summary += "\n  Context: " + strings.Join(contextNotes, "; ")
	}
	return summary
}

// findModelByName looks up a Model in the slice by its Name() string.
// Used by the synthesis-chain fallback to find the Model corresponding
// to a surviving result (since surviving is []*ModelReviewResult, not
// []Model). Returns nil if not found.
func findModelByName(models []model.Model, name string) model.Model {
	for _, m := range models {
		if m.Name() == name {
			return m
		}
	}
	return nil
}

// aggregateErrors flattens dropped-model errors into a single error
// for the case where ALL models failed.
func aggregateErrors(dropped []modelOutcome) error {
	if len(dropped) == 0 {
		return fmt.Errorf("no models succeeded")
	}
	parts := make([]string, 0, len(dropped))
	for _, d := range dropped {
		parts = append(parts, fmt.Sprintf("%s: %v", d.Name, d.Err))
	}
	return fmt.Errorf("all selected models failed: %s", strings.Join(parts, "; "))
}

// writeFindings renders the post-TUI summary so approved findings can be
// piped or captured by non-interactive consumers. totalReviewed is the
// number of findings the user saw in the TUI (after validation): when it's
// zero, the model genuinely returned nothing; when it's non-zero but
// `valid` is empty, the user approved none — those two cases need
// different copy so a reviewer can tell them apart (per diffsmith-14p).
//
// debug controls how quarantined findings are surfaced: off (default)
// prints a one-line counter pointing at --debug; on dumps each
// quarantined candidate's (file:line), title, and the validator's
// reason so operators can see why a candidate was rejected.
func writeFindings(w io.Writer, valid []review.Finding, quarantined []review.Quarantined, totalReviewed int, runSummary string, debug bool) {
	if runSummary != "" {
		fmt.Fprintln(w, runSummary)
	}
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
		if !debug {
			fmt.Fprintf(w, "(%d finding(s) quarantined by validation; pass --debug to inspect.)\n", n)
			return
		}
		fmt.Fprintf(w, "Quarantined (%d):\n", n)
		for _, q := range quarantined {
			fmt.Fprintf(w, "  %s:%d  %s\n", q.Candidate.File, q.Candidate.Line, q.Candidate.Title)
			fmt.Fprintf(w, "    Reason: %s\n", q.Reason)
		}
	}
}
