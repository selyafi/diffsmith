# Diffsmith

Diffsmith is a local, human-in-the-loop AI code review cockpit for GitHub pull requests and GitLab merge requests.

It fetches a PR/MR diff, asks a selected AI CLI to draft review findings, validates the output, and opens a terminal UI where you inspect, edit, approve, dismiss, and copy comments manually.

Diffsmith is not an auto-comment bot. The reviewer stays in control.

Diffsmith has no server. The selected model CLI may still send diffs to its own provider, so users must choose a model that is acceptable for the repository being reviewed.

## Current Status

[v0.1.0-rc1](https://github.com/selyafi/diffsmith/releases/tag/v0.1.0-rc1) released (M8). The product is built end-to-end: GitHub + GitLab providers; Codex, Claude, and Antigravity model adapters running in parallel with synthesis via a lead model; three-pane TUI; clipboard/export; inline review-thread posting back to GitHub PRs and GitLab MRs (gated by explicit confirmation); dedup-before-post against existing diffsmith threads; prompt-injection-resilient parser. See [CHANGELOG.md](CHANGELOG.md) for the v0.1.0 release notes.

## Product Thesis

Most AI review tools fail by posting noisy comments directly into pull requests. Diffsmith takes the opposite approach:

- Local-first CLI/TUI.
- Explicit model selection at startup via an interactive picker.
- Suggested review comments are drafts.
- Every suggested review comment is editable.
- Posting is opt-in per finding (`p` in the TUI) and requires an explicit `y` confirmation before any network call.
- No backend.
- No auto-posting.

## V1 Command

```sh
diffsmith review <github-pr-url|gitlab-mr-url>     # review a specific PR/MR
diffsmith                                          # inbox: pick from your repo's open PRs/MRs
```

At startup, diffsmith probes which AI CLIs are installed (`codex`, `claude`, and `agy` for Antigravity) and shows an interactive picker. Any subset can be selected; their findings are merged by a synthesis pass via the highest-priority surviving model (priority order: codex → claude → antigravity). The Antigravity adapter requires a one-time interactive `agy` login (after which its OAuth token persists); see `internal/model/antigravitycli/doc.go`.

To sharpen findings, diffsmith also sends the PR/MR description and the acceptance criteria from any issues the PR/MR formally closes (resolved via `gh`/`glab`) so reviewers can flag scope drift and unmet criteria. This is on by default; pass `--no-context` for a diff-only review that withholds the description and skips the linked-issue fetch. Context fetching is never a gate — if it fails, the review proceeds and the reason is surfaced in the run summary.

Large diffs can exceed the per-model input budget (1 MiB by default). Pass `--exclude <pattern>` (repeatable) to drop noise files from the review — lockfiles, vendored deps, generated code — before the prompt is built: a trailing `/` excludes a directory tree at any depth (`vendor/`), a pattern without `/` matches basenames anywhere (`*.lock`), anything else is a full-path glob (`internal/gen/*.go`). Exclusions are surfaced in the run summary, never silent, and excluding every changed file is an error rather than an empty review.

`--include <pattern>` (repeatable) is the allowlist counterpart: it keeps only the matching files and drops the rest, using the same pattern rules. It runs first, then `--exclude` carves exceptions out of the kept set — so `--include 'internal/' --exclude 'internal/gen/'` reviews everything under `internal/` except the generated tree. Like exclusions, the narrowing is surfaced in the run summary, and an `--include` that matches no changed file is an error rather than an empty review.

After review, `p` in the TUI marks findings for upstream posting. On quit, diffsmith asks for explicit `y` confirmation, then posts approved findings as inline review threads on the PR/MR. Findings whose `(file, line)` already has a diffsmith thread upstream are skipped with a summary line; pass `--repost` to bypass that dedup gate.

## Install

### One-line install (macOS and Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/selyafi/diffsmith/main/install.sh | sh
```

The script detects your OS and architecture, downloads the matching tarball from the latest GitHub Release, verifies its SHA256 against the published `SHA256SUMS`, and installs the binary to `/usr/local/bin` (or `$HOME/.local/bin` if `/usr/local/bin` is not writable without sudo).

To pin a specific version or change the install directory:

```bash
DIFFSMITH_VERSION=v0.1.0 INSTALL_DIR="$HOME/bin" \
  curl -fsSL https://raw.githubusercontent.com/selyafi/diffsmith/main/install.sh | sh
```

### Via `go install`

If you have Go 1.24+ installed:

```bash
go install github.com/selyafi/diffsmith/cmd/diffsmith@latest
```

### Manual download

Grab the tarball for your platform from the [latest release](https://github.com/selyafi/diffsmith/releases/latest), extract it, and move the `diffsmith` binary somewhere on your `PATH`.

### Updating

Re-run the install command — `install.sh` always resolves to the most recent release, so running it again upgrades in place. For `go install`, repeat the `@latest` invocation.

### Verifying

```bash
diffsmith --version
```

### Uninstalling

```bash
rm "$(command -v diffsmith)"
```

## V1 Dependencies

For repository access:

- `gh` for GitHub pull requests
- `glab` for GitLab merge requests

For AI review:

- `codex`, `claude`, or `agy` (Antigravity) CLI — at least one must be installed and authenticated. The picker shows which are available at startup; all selected models run in parallel and a lead model synthesizes the final findings.
- `agy` requires a one-time interactive login (run `agy` once); after that its OAuth token persists and diffsmith drives it non-interactively.

## V1 Workflow

1. Detect GitHub or GitLab from the URL.
2. Fetch PR/MR metadata and diff through `gh` or `glab`.
3. Normalize changed files, hunks, and line positions.
4. Send a structured review prompt to each selected AI CLI in parallel.
5. Parse each model's output as JSON findings (defensively — fenced or prose-wrapped output is unwrapped before parsing).
6. Synthesize findings across models via a lead model that picks, merges, and rewrites.
7. Validate findings against the diff (anchor file/line into the diff index).
8. Show findings in a three-pane TUI; let the reviewer edit, approve, dismiss, copy, or mark for posting.
9. On quit, with explicit `y` confirmation, post approved findings as inline review threads on the upstream PR/MR — skipping any whose `(file, line)` already has a diffsmith thread.

## V1 Non-Goals

- No automatic posting — every post requires explicit `y` confirmation.
- No CI bot mode.
- No hosted service.
- No session resume.
- No automatic patch application.
- No team policy engine.

## License

MIT — see [LICENSE](LICENSE). Diffsmith bundles no third-party code under a different license; runtime dependencies are listed in `go.mod`.
