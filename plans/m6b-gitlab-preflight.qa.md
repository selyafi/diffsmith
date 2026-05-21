# QA Report: m6b-gitlab-preflight

- Reviewer: codex (codex-cli 0.130.0)
- Date: 2026-05-21T15:30:00Z
- Prompt: plans/.qa-prompt.md (deleted in cleanup phase)
- Schema: plans/.qa-schema.json (local hot-fix; upstream xdev schema bug — `suggested_phase` not in `required`)

## Raw Response

```json
{"verdict":"findings","findings":[{"phase":"2","severity":"critical","title":"Auth-failure message does not match acceptance text","detail":"The ticket acceptance requires the auth failure to wrap the underlying error with the contiguous actionable text \"glab is not authenticated. Run `glab auth login` to authenticate\". Phase 2 specifies `fmt.Errorf(\"glab is not authenticated: %w. Run `glab auth login` to authenticate\", err)`, which renders the underlying error between \"authenticated\" and \"Run\" and therefore will not match that required text. Use a shape like `fmt.Errorf(\"glab is not authenticated. Run `glab auth login` to authenticate: %w\", err)` to preserve wrapping while keeping the required message intact.","suggested_phase":""},{"phase":"1","severity":"warning","title":"Auth-failure test is too weak for the required message","detail":"Phase 1 only asserts that the error contains \"glab auth login\". That would pass the incorrect Phase 2 text above, so the test gate would not catch a mismatch with the ticket acceptance. Strengthen the test to assert the full actionable substring \"glab is not authenticated. Run `glab auth login` to authenticate\" and keep a separate wrapping check if needed.","suggested_phase":""},{"phase":"1","severity":"warning","title":"RED gate contradicts Go compile behavior","detail":"Phase 1 says `go test ./internal/provider/gitlabglab/ -run 'TestPreflight' -count=1` should build successfully while all three tests fail because `NewPreflight`, `Preflight`, and `Check` do not exist. Against the actual package, undefined `NewPreflight` means the test package will not compile, so no individual tests run. Either make the gate explicitly accept that compile failure as RED, or add minimal compile-only stubs before expecting behavioral test failures.","suggested_phase":"1.5"}]}
```

## Integration Decisions

- **Finding 1 (Phase 2, critical, auth-failure format string) — INTEGRATED.** Plan's Phase 2 updated to use `fmt.Errorf("glab is not authenticated. Run \`glab auth login\` to authenticate: %w", err)` per Codex's suggested ordering. Acknowledges divergence from `githubgh.Preflight` (which has the same wording bug); documented as a follow-up candidate.
- **Finding 2 (Phase 1, warning, auth-failure test too weak) — INTEGRATED.** Plan's Phase 1 strengthened: assertion now requires the FULL contiguous substring `"glab is not authenticated. Run \`glab auth login\` to authenticate"`, plus a separate wrapping check via `errors.Is` / `strings.Contains` for the underlying transport error.
- **Finding 3 (Phase 1, warning, RED gate vs Go compile semantics) — INTEGRATED.** Plan's Phase 1 now explicitly includes writing compile-only stubs in `preflight.go` BEFORE running the tests, so the RED is per-sub-test behavioral failure rather than a package-wide build error. Test-gate language tightened to require sub-test-level FAIL lines. No new Phase 1.5 needed — folded into Phase 1.
