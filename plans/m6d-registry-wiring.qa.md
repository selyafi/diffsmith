# QA Report: m6d-registry-wiring

- Reviewer: codex (codex-cli 0.130.0)
- Date: 2026-05-21T16:25:00Z
- Prompt: plans/.qa-prompt.md (deleted in cleanup phase)
- Schema: plans/.qa-schema.json (local hot-fix)

## Raw Response

```json
{"verdict":"findings","findings":[
  {"phase":"1","severity":"critical","title":"GitLab e2e is not a RED test for defaultRegistry","detail":"The plan says TestReviewPrintPromptHappyPathOnGitLabURL should build a custom registry with gitlabglab.New(stubRunner) and pass it to newReviewCmd. runReview uses that injected registry at internal/app/review.go:67, so the current defaultRegistry omission in internal/app/root.go:30-32 is bypassed completely. The stated RED gate that it fails because the real default registry does not support GitLab URLs is therefore false: once the stub is correct, this test can pass before Phase 2.","suggested_phase":"1"},
  {"phase":"1","severity":"critical","title":"Stub runner contract misses Preflight call","detail":"runReview calls p.Preflight(ctx) before Fetch at internal/app/review.go:71-75. gitlabglab.New(stubRunner) installs a Preflight using the same runner at internal/provider/gitlabglab/adapter.go:24-31, and Preflight.Check invokes glab auth status at internal/provider/gitlabglab/preflight.go:41-47. A two-response runner returning only MR JSON and diff is misaligned: the auth-status call consumes the first response, then mr view receives the diff and JSON decoding fails. The test fake must handle auth status plus mr view plus mr diff, preferably by matching name/args.","suggested_phase":"1"},
  {"phase":"1","severity":"warning","title":"Preflight still depends on real PATH lookup","detail":"Even with a stub runner, gitlabglab.New(stubRunner) leaves LookPath as exec.LookPath, so Preflight still checks for a real glab binary before reaching Fetch. That contradicts the plan's hermetic-test intent and the out-of-scope note for real-glab integration. In package app the test cannot set Adapter.preflight directly, so the plan should specify a hermetic LookPath strategy such as a temp PATH entry containing a fake glab executable, or add an explicit seam before relying on this e2e in CI.","suggested_phase":"1"},
  {"phase":"1","severity":"info","title":"Dispatch assertion shape is acceptable but should check err first","detail":"A reflect.TypeOf(provider) comparison will catch the current defaultRegistry state because Registry.Find returns nil plus a no-provider error for a GitLab URL, and reflect.TypeOf(nil) is not *gitlabglab.Adapter. Still, the clearer test shape is to require err == nil before comparing reflect.TypeOf(p) to reflect.TypeOf((*gitlabglab.Adapter)(nil)); do not use a single-result concrete assertion before checking the Find error.","suggested_phase":"1"},
  {"phase":"1","severity":"info","title":"Extra GitHub negative dispatch check is optional","detail":"githubgh.Supports already rejects GitLab URLs in internal/provider/githubgh/url_test.go:118-127, and a registry dispatch test with GitHub registered before GitLab would also fail if githubgh ever started matching GitLab URLs because Registry.Find returns the first supporting provider. An explicit githubgh-does-not-support-gitlab assertion in the root registry test is useful clarity, not required coverage.","suggested_phase":""}
]}
```

## Integration Decisions

User chose "Apply all fixes; add NewWithLookPath constructor."

- **Finding 1 (critical, e2e bypasses defaultRegistry) — REDEFINED.** The e2e test stays in the plan but is recategorized: it's now explicitly NOT the RED→GREEN driver for M6d. Its purpose is regression-guard for the Adapter↔app wiring (catches `BuildPrompt`/registry drift); the **dispatch test** is the load-bearing driver. Test-gate language in Phase 1 documents this asymmetry honestly.
- **Finding 2 (critical, stub misses Preflight) — INTEGRATED.** The plan now specifies an **argv-aware** stub runner that branches on `(name, args)`: handles `glab auth status`, `glab mr view`, `glab mr diff` distinctly. Any unrecognized call calls `t.Fatalf`. Three canned responses, not two.
- **Finding 3 (warning, LookPath dependency) — INTEGRATED.** Plan now adds a `NewWithLookPath(run, lookPath)` constructor to `gitlabglab.Adapter` (Phase 1 change), parallel to the existing `NewPreflight` shape. `New(run)` delegates. The e2e calls `gitlabglab.NewWithLookPath(stubRunner, fakeLookPath)` for full hermeticity. Note: a symmetric `NewWithLookPath` could be added to `githubgh` later, but it's out of scope for M6d.
- **Finding 4 (info, dispatch should check err first) — INTEGRATED.** Plan now mandates `err == nil` check BEFORE `reflect.TypeOf` comparison.
- **Finding 5 (info, GitHub-doesn't-support-GitLab assertion optional) — REJECTED.** Already covered by `githubgh/url_test.go:TestSupports` (line 118-127). Adding it to the registry dispatch test would be duplicate coverage with no signal.
