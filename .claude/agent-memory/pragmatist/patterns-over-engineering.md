---
name: patterns-over-engineering
description: Recurring shapes of waste seen in diffsmith audits — defensive nil re-init, dead getters, single-call helpers, empty milestone packages.
metadata:
  type: project
---

Common over-engineering patterns observed in diffsmith Go audits:

1. **Defensive nil-init that can't fire** — fields set unconditionally in `New(...)` get re-initialized inside methods (e.g., `githubgh/adapter.go` `Preflight` re-inits `a.preflight`). Unexported struct + unexported field means no zero-value literal escapes; the guard is dead code.

2. **Unused getters on TUI models** — `LoaderModel.RunSummary()` defined, field assigned, but production caller passes a local closure variable. Pattern: getter introduced "for tests or future use" then never wired.

3. **Computed convenience fields** — `SelectedModels.Lead = All[0]` precomputed in constructor but production paths only read `All`; only the test file exercises `Lead`.

4. **Empty milestone packages** — `internal/export/` ships with only `doc.go` for M5 ("Lands in M5") with no callers. Per project rules: don't ship empty packages ahead of need.

5. **Package-level globals consumed once** — `probeURLs` in `registry.go` is used only by `ByHost`; package-level scope implies reuse that doesn't exist.

6. **Wrapper functions with one defensive case** — `aggregateErrors` handles `len(dropped)==0` defensively, but the only caller guarantees non-zero. The branch is unreachable.

**Why:** Today's session shipped a real CI failure on dead `ctx := cmd.Context()` in `inbox.go` — the dead-variable class is live in this codebase.

**How to apply:** When auditing, check (a) every package-level `var`/`func` for actual call sites, (b) every nil-check against the constructor's contract, (c) every method on internal-only types for production callers (not test-only).
