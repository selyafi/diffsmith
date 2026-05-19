---
name: layer-dependency-map
description: Observed import direction across diffsmith internal packages, including the provider<-model<-review chain that inverts Clean Architecture's inner/outer rule.
metadata:
  type: project
---

Observed import edges (as of 2026-05-19, branch main):

- `internal/diff` — std-lib only. True domain leaf.
- `internal/provider` — imports `internal/diff` (for `DiffFile` inside `ReviewInput`). Also owns `Runner` (wraps `os/exec`).
- `internal/model` — imports `internal/provider` (consumes `provider.ReviewInput` directly in `Model.Review`, `BuildPrompt`, and `FindingCandidate` neighbors). See `internal/model/types.go:12`, `internal/model/prompt.go:7`.
- `internal/review` — imports `internal/model` AND `internal/diff` (validates `model.FindingCandidate` against `diff.Index`). See `internal/review/validate.go:6-7`, `internal/review/types.go:12`.
- `internal/tui` — imports `internal/review` and `internal/clipboard`. Bubble Tea confined here (`internal/tui/bubble_tea.go:4`).
- `internal/app` — imports diff, model, provider, review, tui; owns Cobra.
- `internal/model/codexcli` — imports `internal/model` and `internal/provider`; uses `os/exec`.
- `internal/provider/githubgh` — imports `internal/diff` and `internal/provider`; uses `os/exec`.

**Why this matters:** Conceptually `review` is the most-domain package, but the import chain makes `provider` the innermost layer (everyone imports it; it imports only `diff`). The `ReviewInput` type acts as a god-DTO that pulls `provider` into the center even though the type itself is data, not transport.

**How to apply:** If a future refactor wants Clean-Architecture-style purity, move `ReviewInput` and the `Model`/`Provider` interfaces into a neutral package (e.g. `internal/core` or `internal/review`) so adapters depend inward. Until then, do not flag this as a bug — it is a deliberate small-CLI trade-off, but call it out on any boundary change that deepens the inversion.
