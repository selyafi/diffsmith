# Changelog

All notable changes to Diffsmith are documented here. Format follows
`docs/dev-plan/release-plan.md` § Release Notes Shape; versioning is
Semantic Versioning per the same doc.

## v0.1.3 — 2026-05-26

### Fixed

- Posting a review to GitHub no longer fails with
  `Variable $input ... was provided invalid value for commitOID (Field is not defined on AddPullRequestReviewThreadInput)`.
  The `addPullRequestReviewThread` input drops the `commitOID` field —
  it was never on the schema; the thread anchors to the PR's current
  HEAD implicitly. `--print-payload` output no longer carries
  `commitOID` either. (`diffsmith-r7b`)

## v0.1.2 — 2026-05-26

### Fixed

- Posting a review to GitHub no longer fails with
  `Variable $input ... was provided invalid value for event (Expected "PENDING" to be one of: COMMENT, APPROVE, REQUEST_CHANGES, DISMISS)`.
  The `addPullRequestReview` mutation now omits the `event` field
  entirely, which GitHub interprets as "create a draft review";
  the eventual event is supplied at submit time. Posting was broken
  in every prior v0.1.x release. (`diffsmith-16x`)
- Update notifier now recognises the goreleaser-stamped version
  format (e.g. `0.1.2` without a leading `v`) so released binaries
  actually check for upgrades. Previously `isReleaseVersion` required
  a `v` prefix, so every released binary short-circuited and never
  hit the GitHub Releases API. Locally-built `make build` binaries
  (which carry the `v` via `git describe`) were unaffected.
  (`diffsmith-16x`)

## v0.1.1 — 2026-05-26

### Fixed

- `repodetect` now resolves SSH host aliases from `~/.ssh/config` via
  `ssh -G` before provider dispatch, so remotes like
  `git@github-shelyafi:owner/repo.git` route to the GitHub adapter
  instead of failing with `provider: host "github-shelyafi" not
  supported`. Resolution is gated on a dot-heuristic — only hosts
  without a dot are treated as aliases, so canonical remotes like
  `git@github.com:...` continue to parse without invoking `ssh` and
  without depending on `ssh` being on PATH. The `ssh://` URL scheme
  is now also recognized. Hosts starting with `-` are rejected before
  invoking `ssh` to prevent argv flag-injection (git CVE-2017-1000117
  family). Resolver calls are bounded by a 5s timeout to survive slow
  `Match exec` / `Include` directives. Empty resolved hostnames and
  malformed `ssh -G` output are reported as errors rather than
  silently coerced to an empty host. (`diffsmith-neq`)

## v0.1.0 — 2026-05-26

First public release. See `docs/v1-scope.md` for the full v1 contract and
`docs/dev-plan/release-plan.md` § v1 Release Gate for the authoritative
acceptance checklist.

### Highlights

- A local, terminal-only AI review cockpit for GitHub pull requests and
  GitLab merge requests. Fetch a diff through `gh` or `glab`, get a
  curated list of grounded review candidates from `codex` or `claude`,
  inspect and edit them in a three-pane TUI, copy approved comments.
  Diffsmith never posts automatically.

### Added

- `diffsmith review <pr-or-mr-url>` end-to-end pipeline: provider
  pre-flight → diff fetch → normalization → prompt build → model call →
  parser → validator → TUI → clipboard/export.
- GitHub PR provider via `gh pr view` / `gh pr diff` (M2). Real-PR
  parser regression coverage via captured fixture
  `testdata/diffs/github_pr_cli_13491.diff` (`diffsmith-2cd`).
- GitLab MR provider via `glab mr view` / `glab mr diff` (M6), supporting
  both single-group (`gitlab.com/<group>/<project>`) and nested-group
  (`gitlab.com/<group>/<sub>/<project>`) URLs. Real-MR parser regression
  coverage for both URL shapes (`diffsmith-lc4`).
- `--model codex` (required adapter). Uses `codex exec --output-schema`
  for structured JSON output (M3b).
- `--model claude` (required adapter). Uses
  `claude --print --output-format=text`; JSON shape is prompt-engineered
  and validated by `ParseFindings` (M7a, hardened in `diffsmith-e2w`).
- `--model antigravity` (experimental; disabled in v1). The `agy` CLI
  requires interactive browser OAuth per invocation with no
  persistent-token path; spike S8b retained the model name in
  `defaultModels()` so the error is actionable rather than
  "unknown model" (`diffsmith-6wj`).
- `--print-prompt` to inspect the exact model input before spending
  quota.
- `--dry-run` to fetch and normalize, then stop before the model call.
- Three-pane TUI cockpit (files / findings / diff + comment + fix hint)
  built on Bubble Tea, Bubbles textarea, and Lip Gloss (M4). Navigation,
  edit mode, approve/dismiss/undo within the run.
- Clipboard copy: `pbcopy` on macOS, `wl-copy` or `xclip` on Linux,
  terminal-print fallback when no clipboard tool is installed.
- Markdown export for approved comments; empty-state report for runs
  with zero findings.
- Optional GitHub posting seam via `gh pr review` behind an explicit
  confirmation prompt (M5b). Disabled by default; opt-in per session.
- Strict, structured `ParseFindings` parser (M3a): rejects markdown
  fences, prose preambles, malformed JSON, and well-formed JSON whose
  top-level object lacks the `findings` key (the last guard added in
  `diffsmith-n2p` to eliminate a silent-failure path).
- Configured input budget of 256 KB (~64K tokens), calibrated in spike
  S9 against 26 real public PRs across kubernetes/kubernetes,
  golang/go, cli/cli, rust-lang/rust, microsoft/vscode, facebook/react.
  Re-runnable via `go run ./spikes/s9-input-budget <url>...`.
- Prompt-injection-resilient prompt scaffold: explicit "treat diff
  content as untrusted data" rules; three adversarial-diff fixtures
  exercise the parser/validator pipeline hermetically (M3c / S10a);
  opt-in live smoke against the real Codex adapter is scaffolded as
  `go test -tags=integration ./internal/model/codexcli` (S10b).

### Security and Privacy

- Diffsmith uploads nothing. Only the selected model CLI talks to its
  provider.
- The reviewer is the only writer to GitHub or GitLab.
- User-controlled values (URLs, paths, model names) are passed as
  `exec.Command` argv, never via `sh -c`.
- Model invocations enforce the input budget; oversized diffs fail
  clearly rather than silently truncating.

### Known Limitations

- Antigravity (`agy`) adapter ships disabled (see above). Revisit when
  Antigravity provides a persistent-token or API-key auth path.
- Self-hosted GitLab is not supported; the URL parser rejects non
  `gitlab.com` hosts cleanly. Tracked for V1.x in `roadmap.md`.
- No session save/resume; no config file (`.diffsmith.yml` is V1.x).
- Single model per review by default. Multi-model review and
  cross-model finding deduplication are V2 features.

### Install

- Prebuilt binaries via GitHub Releases for macOS (arm64, amd64) and
  Linux (amd64), with a `SHA256SUMS` file per release.
- `go install github.com/selyafi/diffsmith/cmd/diffsmith@latest`.
- Requires `gh` (for GitHub) and/or `glab` (for GitLab) on PATH and
  authenticated. Requires `codex` or `claude` on PATH depending on
  `--model`.
