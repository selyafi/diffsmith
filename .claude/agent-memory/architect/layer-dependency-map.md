---
name: layer-dependency-map
description: Observed import direction across diffsmith internal packages. Domain types now live in review/; provider and model both depend on review (the inner domain leaf), reversing the older inversion.
metadata:
  type: project
---

Observed import edges (refreshed 2026-05-26 on main):

- `internal/diff` — std-lib only. Domain leaf #1 (diff parsing).
- `internal/review` — imports only `internal/diff`. Domain leaf #2: owns `ReviewInput`, `ReviewTarget`, `Finding`, `FindingCandidate`, `Severity`, `ModelReviewResult`, `validate.go`. This is the canonical "inner" package now.
- `internal/provider` — imports `internal/review`. Defines `Provider`, `Runner`, `RepoCoord`, `PRSummary`. (provider.go:7, registry.go)
- `internal/model` — imports `internal/review` (types.go:14, prompt.go:7, parse.go:8, synthesis.go:7). No longer imports `internal/provider`.
- `internal/model/{codexcli,claudecli,geminicli}` — import `internal/model`, `internal/provider` (for Runner), `internal/review`, `internal/diff`.
- `internal/model/antigravitycli` — imports only `internal/provider` + `internal/review` (stub: never invokes runner, see S8b).
- `internal/provider/{githubgh,gitlabglab}` — import `internal/diff`, `internal/provider`, `internal/review`.
- `internal/post` — imports `internal/provider`, `internal/review` (post.go, dedup.go). Both seams (`runGH`, `runGlab`) are `provider.Runner` typed.
- `internal/tui` — imports `internal/review`, `internal/clipboard`, `internal/provider` (inbox.go pulls PRSummary). Bubble Tea confined here.
- `internal/repodetect` — imports `internal/provider` only (returns `provider.RepoCoord`).
- `internal/clipboard`, `internal/export` — std-lib only.
- `internal/update` — std-lib only. Pure leaf, called from app/root.go.
- `internal/app` — imports all of the above; owns Cobra.

**Why this matters:** the older "review depends on model" inversion has been fixed. The dependency graph is now a clean DAG with review+diff as the domain leaves and app at the top. provider and model are sibling adapter layers that both depend inward on review.

**How to apply:** Reject any new edge from review/diff outward, or from a leaf (update, clipboard, export, repodetect) into review/model/provider beyond what's listed here. tui importing provider is the only non-obvious edge (PRSummary for the inbox view); accept it but flag if tui starts reaching into model.
