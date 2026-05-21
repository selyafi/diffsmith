# Plan: M6b — GitLab Preflight (glab on PATH + auth status)

## Context

Mirror `internal/provider/githubgh/preflight.go` for the `glab` CLI so users see actionable errors when their tooling is missing or unauthenticated, before any model invocation. This is the second M6 leaf ticket (after M6a — URL parser, already complete on disk) and is the last blocker on `diffsmith-qm8` (M6c — Adapter) along with M6a.

Reference implementation: `internal/provider/githubgh/preflight.go` + `preflight_test.go` (62 lines impl/test combined). The shape is fully prescribed by the acceptance criteria of `diffsmith-zgc`.

## Phase 1: RED — write failing tests + compile-only stubs

status: complete

### Changes

- file: `internal/provider/gitlabglab/preflight.go` — create with **compile-only stubs**: declare `Preflight` struct with `Run` and `LookPath` fields, declare `NewPreflight` returning `&Preflight{}` (no defaults yet), and `Check` returning `errors.New("not implemented")`. This makes the test file compile so failures are behavioral (and observable per sub-test) rather than a package-wide build error.
- file: `internal/provider/gitlabglab/preflight_test.go` — create with three tests modeled on the GitHub counterpart:
  - `TestPreflightMissingBinary`: `LookPath("glab")` returns ENOENT; assert `Check` returns an error containing `"glab CLI not found on PATH"`; assert the runner is NEVER invoked (fail the test from inside the runner if it is).
  - `TestPreflightAuthFailure`: `LookPath` succeeds; runner returns `errors.New("glab: exit 1: not logged in")`. Assert the error message contains the FULL contiguous actionable substring `"glab is not authenticated. Run \`glab auth login\` to authenticate"` (not just `"glab auth login"` — that would pass a draft where the underlying error splits the actionable text). Also assert `errors.Is` or `strings.Contains` for the underlying error so wrapping is preserved.
  - `TestPreflightHappyPath`: `LookPath` succeeds; runner asserts the exact argv shape `glab auth status` (name=="glab", args[0]=="auth", args[1]=="status"); `Check` returns nil.

### Test Gate

- [ ] `go test ./internal/provider/gitlabglab/ -run 'TestPreflight' -count=1` **builds successfully** (stubs ensure compile) and **all three new tests FAIL** with behavioral assertion errors (because `Check` returns the placeholder error, never invokes `LookPath` or `Run`, etc.). Failures must be sub-test-level (`--- FAIL: TestPreflightMissingBinary`, etc.), not a package compile error. If the tests pass on first run, the test is asserting existing behavior and must be re-examined.

## Phase 2: GREEN — implement Preflight

status: complete

### Changes

- file: `internal/provider/gitlabglab/preflight.go` — new file mirroring the `githubgh` counterpart's structure:
  - `Preflight` struct with exported `Run provider.Runner` and `LookPath func(string) (string, error)` fields.
  - `NewPreflight(run provider.Runner, lookPath func(string) (string, error)) *Preflight` — defaults `run` to `provider.DefaultRunner` and `lookPath` to `exec.LookPath` when nil.
  - `Check(ctx context.Context) error`:
    1. Call `LookPath("glab")`. On error: return `errors.New("glab CLI not found on PATH. Install it from https://gitlab.com/gitlab-org/cli")`.
    2. Call `Run(ctx, nil, "glab", "auth", "status")`. On error: return `fmt.Errorf("glab is not authenticated. Run \`glab auth login\` to authenticate: %w", err)`. NOTE: actionable text comes BEFORE the `%w` wrap so the contiguous acceptance-required string is preserved and `errors.Is`/`Unwrap` still surface the original gh-style transport error. This intentionally diverges from `githubgh.Preflight.Check`'s less-ideal ordering — that divergence is justified by the M6b acceptance criteria's explicit literal-string requirement, and a future follow-up can backport the cleaner wording to the github counterpart.
  - Doc comment on the type cites `docs/architecture.md § Pre-Flight Checks` (same pattern as `githubgh.Preflight`).

### Test Gate

- [ ] `go test ./internal/provider/gitlabglab/ -count=1` → all tests PASS (the M6a tests plus the three new Preflight tests).
- [ ] Error message for missing-binary contains the exact string `"glab CLI not found on PATH"` (asserted by test).
- [ ] Error message for auth-failure contains the exact string `"glab auth login"` (asserted by test).
- [ ] Runner is invoked exactly once (only on the auth-status call); never invoked when `LookPath` fails (asserted by test).

## Phase 3: Whole-repo verification

status: complete

### Changes

- (no production code changes — verification only)

### Test Gate

- [ ] `go test ./... -count=1` → all packages PASS, 0 failures, 0 regressions.
- [ ] `go vet ./...` → exit 0.
- [ ] `go build ./...` → exit 0.
- [ ] `gofmt -l internal/` → empty output (no formatting drift).

## Dependencies

- Phase 2 depends on Phase 1 (must see RED first, per TDD discipline established earlier this session).
- Phase 3 depends on Phase 2 (verification can only happen after impl).
- This whole plan is independent of M6a (`diffsmith-60k`) which is already complete on disk; no cross-file interaction.

## Out of scope

- Wiring `gitlabglab.New()` into `defaultRegistry()` — that's M6d.
- Adapter implementation (`Fetch`, metadata, diff parsing) — that's M6c.
- Capturing real `glab` output fixtures — deferred to M6c, where they're actually useful.
