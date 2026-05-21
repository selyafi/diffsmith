# Plan: M6d — Wire GitLab adapter into defaultRegistry + end-to-end smoke (revised after QA)

## Context

After M6a/b/c, the GitLab adapter is fully implemented and tested in isolation. M6d wires it into `internal/app/root.go:defaultRegistry()` and adds two tests that together cover the M6d acceptance:

1. **Dispatch test** (the load-bearing test for M6d): verifies `defaultRegistry()` actually contains gitlabglab and dispatches GitLab URLs to it. Without this test, the `defaultRegistry` wiring is unverified.
2. **End-to-end smoke** (independent integration value): verifies the real `gitlabglab.Adapter` works through the entire Cobra `runReview` path — Preflight → ParseURL → fetchMetadata → diff fetch → diff.Parse → BuildPrompt. This is NOT a dispatch test (the test uses a custom registry, bypassing `defaultRegistry`); its purpose is to catch wiring drift between the adapter and the app layer.

This plan was revised after Codex QA caught two critical issues in the original draft: (a) the e2e was being described as a defaultRegistry RED-test, which it isn't; and (b) the stub runner missed the `Preflight → glab auth status` call. Both addressed below.

## Phase 1: RED — write failing tests + hermetic infrastructure

status: complete

### Changes

- file: `internal/provider/gitlabglab/adapter.go` — add a parameterized constructor:
  ```go
  // NewWithLookPath constructs an Adapter with explicit run + lookPath
  // injection so cross-package tests (e.g. internal/app/) can build a
  // fully hermetic Adapter that needs neither real `glab` on PATH nor
  // real network. Passing nil for either falls back to defaults.
  func NewWithLookPath(run provider.Runner, lookPath func(string) (string, error)) *Adapter {
      if run == nil { run = provider.DefaultRunner }
      return &Adapter{run: run, preflight: NewPreflight(run, lookPath)}
  }
  ```
  Refactor `New(run)` to delegate: `return NewWithLookPath(run, nil)`. Existing tests must stay green.
- file: `internal/app/root_test.go` — add `TestDefaultRegistryDispatchesGitHubAndGitLab`:
  - Call `defaultRegistry()`. For URLs `https://github.com/owner/repo/pull/1`, `https://gitlab.com/group/project/-/merge_requests/1`, and `https://gitlab.com/group/sub/project/-/merge_requests/1`:
    - Call `r.Find(url)`. Assert `err == nil` FIRST. Only then compare `reflect.TypeOf(p)` against the expected `reflect.TypeOf((*githubgh.Adapter)(nil))` or `reflect.TypeOf((*gitlabglab.Adapter)(nil))`.
  - Assert `Find("https://example.com/foo")` returns a "no provider supports" error.
- file: `internal/app/review_test.go` — add `TestReviewPrintPromptHappyPathOnGitLabURL`:
  - Build a custom registry containing **only** `gitlabglab.NewWithLookPath(stubRunner, fakeLookPath)` where:
    - `fakeLookPath = func(string) (string, error) { return "/fake/glab", nil }` (no real `exec.LookPath`).
    - `stubRunner` is **argv-aware** — inspects `(name, args)` and returns:
      - `name=="glab" && args[0]=="auth" && args[1]=="status"` → `([]byte("ok"), nil)` (Preflight pass-through)
      - `name=="glab" && args[0]=="mr" && args[1]=="view"` → canned JSON shaped like `singleGroupMetadata` in `adapter_test.go`
      - `name=="glab" && args[0]=="mr" && args[1]=="diff"` → an inline synthetic diff string (no cross-package fixture coupling)
      - any other → `t.Fatalf("unexpected runner call: %s %v", name, args)`
  - Pass the registry to `newReviewCmd`. Drive `review <single-group-url> --print-prompt`. Assert output contains:
    - URL, title, author, source/target branch refs (echoes of the canned metadata)
    - The synthetic diff substring (`BuildPrompt` includes the raw diff in the context block)
  - This test's RED gate: WITHOUT `NewWithLookPath` existing yet, the test won't compile. That's the per-sub-test failure mode here.

### Test Gate

- [ ] `go test ./internal/provider/gitlabglab/ -count=1` — still all green (`NewWithLookPath` refactor preserves behavior).
- [ ] `go test ./internal/app/ -run 'TestDefaultRegistryDispatchesGitHubAndGitLab' -count=1` FAILS: `Find` for GitLab URLs returns the no-provider error, so `err != nil` and the test fails at the err-check.
- [ ] `go test ./internal/app/ -run 'TestReviewPrintPromptHappyPathOnGitLabURL' -count=1` FAILS: but per a real Cobra flow — once the test compiles (after `NewWithLookPath` lands), `runReview` reaches `Fetch` and the stub runner returns canned data. WAIT — this test passes against the CUSTOM registry; it doesn't need `defaultRegistry` to have gitlabglab. So this test will pass at Phase 1 once `NewWithLookPath` exists. THAT IS INTENTIONAL: this test is for "real Adapter wiring works through Cobra", not for "defaultRegistry dispatch". Its meaningful test-gate moment is: must compile and pass after `NewWithLookPath` is added; it's a regression guard, not a RED→GREEN driver.

## Phase 2: GREEN — wire gitlabglab into defaultRegistry

status: complete

### Changes

- file: `internal/app/root.go` — change `defaultRegistry()`:
  ```go
  import (
      ...
      "github.com/selyafi/diffsmith/internal/provider/gitlabglab"
  )
  ...
  func defaultRegistry() *provider.Registry {
      return provider.NewRegistry(githubgh.New(nil), gitlabglab.New(nil))
  }
  ```

### Test Gate

- [ ] `TestDefaultRegistryDispatchesGitHubAndGitLab` PASSES — gitlabglab is now in the registry, Find returns it, type assertion succeeds.
- [ ] `TestReviewPrintPromptHappyPathOnGitLabURL` still PASSES (was passing since Phase 1; this is just a regression-guard).
- [ ] All existing app tests still PASS.

## Phase 3: Whole-repo verification

status: complete

### Changes

- (no production code changes)

### Test Gate

- [ ] `go test ./... -count=1` — all packages PASS.
- [ ] `go vet ./...` → exit 0.
- [ ] `go build ./...` → exit 0.
- [ ] `gofmt -l internal/` → empty.

## Dependencies

- Phase 2 depends on Phase 1.
- Phase 3 depends on Phase 2.
- `NewWithLookPath` is added to gitlabglab in Phase 1 because the e2e test in `internal/app/` needs it to compile.

## Out of scope

- Spike S2 closure / doc updates — M6e.
- Adding `NewWithLookPath` symmetric helper to githubgh — future symmetry pass; not required for M6d acceptance and not blocking anything in v1.

## Notes on test-gate semantics (revised after QA)

- The **dispatch test** is the RED→GREEN driver for M6d. It directly verifies the M6d acceptance line "GitLab URLs dispatch to the GitLab adapter".
- The **e2e test** is NOT a RED→GREEN driver. It's a Phase-1-once-compiled, then-passes regression guard. It catches drift in the Adapter↔app integration that the M6c unit tests can't see (e.g., if `BuildPrompt` were broken for a GitLab ReviewInput). The acceptance line about `--print-prompt` against a stubbed runner is satisfied by this test, even though the test uses a custom registry instead of `defaultRegistry`.
- The asymmetry is deliberate and documented: dispatch test = M6d acceptance core; e2e test = Adapter-app wiring smoke.
