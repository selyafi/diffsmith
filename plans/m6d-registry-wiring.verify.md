# Verify Report: m6d-registry-wiring

- Reviewer: codex (codex-cli 0.130.0)
- Date: 2026-05-21T16:40:00Z
- Prompt: plans/.verify-prompt.md (deleted in cleanup phase)
- Diff: plans/.verify-diff.patch (190 lines, deleted in cleanup phase)
- Schema: plans/.verify-schema.json (local hot-fix)

## Raw Response

```json
{"verdict":"findings","findings":[
  {"severity":"warning","title":"E2E runner stub is not exact-call aware","detail":"The stub accepts any glab auth status, glab mr view, or glab mr diff invocation with at least two args, and it does not assert call count. Duplicate calls or missing/changed flags would still pass, so it does not fully prove the exact 3 expected calls. Fix by matching the full expected argv sequence and failing when calls are exhausted or extra calls occur.","file":"internal/app/review_test.go","line":577},
  {"severity":"warning","title":"E2E diff assertion is too weak","detail":"The plan requires asserting a synthetic diff substring to prove BuildPrompt includes the raw diff. The test only checks svc/handler.go, which can be satisfied by the Files section even if the raw Diff block is removed. Fix by asserting a raw-diff-only substring such as the diff --git header or the added context.WithTimeout(req.Context(), ...) line.","file":"internal/app/review_test.go","line":617}
]}
```

## Integration Decisions

- **Finding 1 (warning, stub not exact-call-aware) — INTEGRATED.** Rewrote the stub to use `reflect.DeepEqual` against the full expected argv shape for each of the 3 calls. Unmatched argv (including duplicates, missing flags, wrong order) hits `t.Fatalf`. Added a post-execute assertion that `len(calls) == 3` so missing calls also fail. Drift in either direction (mismatched flags OR unexpected extra calls) now fails the test.
- **Finding 2 (warning, diff substring too weak) — INTEGRATED.** Replaced the `svc/handler.go` filename assertion (which could be satisfied by the Files section alone) with a unified-diff-header assertion: `"diff --git a/svc/handler.go"`. The `diff --git` prefix appears ONLY in the raw-diff block of BuildPrompt's output; if RawDiff were dropped, this substring would not appear.

## Codex's Independently-Reproduced Evidence

While running in `-s read-only` sandbox, Codex ran `go test ./internal/app/ -run 'TestReviewPrintPromptHappyPathOnGitLabURL' -count=1 -vet=off` and confirmed it passed (1151ms). No sandbox-limitation warnings this round.

## Verdict (final)

**Verified** — both test-rigor warnings addressed in-place. No critical findings. No plan drift. Implementation matches the (revised) plan.
