# QA Report: m6c-gitlab-adapter

- Reviewer: codex (codex-cli 0.130.0)
- Date: 2026-05-21T15:50:00Z
- Prompt: plans/.qa-prompt.md (deleted in cleanup phase)
- Schema: plans/.qa-schema.json (local hot-fix; upstream schema bug)

## Raw Response

```json
{"verdict":"findings","findings":[{"phase":"1","severity":"warning","title":"Adapter tests do not cover second glab command failures","detail":"The planned `TestAdapterFetchSurfacesRunnerError` only describes a runner error generally, and the GitHub precedent uses a runner that fails on the first call. For this adapter there are two separate external calls (`glab mr view` and `glab mr diff`). Without a case where metadata succeeds and `mr diff` fails, the plan does not verify the Phase 2 requirement to wrap diff fetch errors with `glab mr diff`.","suggested_phase":"1"},{"phase":"1","severity":"warning","title":"Missing required metadata validation coverage","detail":"`review.ReviewTarget.HeadSHA` is the capture-time commit anchor in `internal/review/input.go`, and Phase 2 maps it from `meta.SHA`. The plan never says to reject empty `sha`, and the five tests do not include a missing-`sha` JSON case. If `glab mr view --output json` omits or changes this field, Fetch would silently return `HeadSHA=\"\"`, which downstream payload generation cannot safely anchor.","suggested_phase":"1"},{"phase":"1","severity":"warning","title":"RawDiff is not asserted in happy-path tests","detail":"`internal/model/prompt.go` builds the review prompt from `ReviewInput.RawDiff`, not from parsed `Files`. The Phase 2 implementation explicitly sets `RawDiff = string(rawDiff)`, but the planned happy-path assertions mention normalized fields and parsed files only. Add an assertion that `input.RawDiff` exactly matches the diff runner output so a parser-only implementation cannot pass.","suggested_phase":"1"}]}
```

## Integration Decisions

- **Finding 1 (warning, missing diff-command failure test) — INTEGRATED.** Added `TestAdapterFetchSurfacesDiffCommandError` to Phase 1: scripted runner where `mr view` succeeds with valid metadata but the second invocation (`mr diff`) returns an error. Assert Fetch returns an error whose message contains `"glab mr diff"` so the diff-command error wrap is independently verified.
- **Finding 2 (warning, missing empty-SHA validation) — INTEGRATED.** Added `TestAdapterFetchRejectsEmptySHA` to Phase 1 (runner returns metadata JSON with empty `sha` field). Updated Phase 2 to add explicit validation: after `json.Unmarshal`, if `meta.SHA == ""` return an error like `"glab mr view returned empty sha for MR <iid>"`. NOTE this is asymmetric with the github counterpart (which doesn't validate `meta.HeadRefOid`) — flagged as a future-backport candidate in the plan, similar to the M6b auth-message format divergence.
- **Finding 3 (warning, RawDiff not asserted) — INTEGRATED.** Both happy-path tests (single-group AND nested-group) now assert `input.RawDiff == string(diffFixture)` so a Phase 2 implementation that forgets to set RawDiff can't pass.

(`approved` verdict is not applicable since findings were issued; with all three integrated, the revised plan is ready for user approval per the xdev contract.)
