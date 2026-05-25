# Diffsmith

Diffsmith is a local, human-in-the-loop AI code review cockpit for GitHub pull requests and GitLab merge requests.

It fetches a PR/MR diff, asks a selected AI CLI to draft review findings, validates the output, and opens a terminal UI where you inspect, edit, approve, dismiss, and copy comments manually.

Diffsmith is not an auto-comment bot. The reviewer stays in control.

Diffsmith has no server. The selected model CLI may still send diffs to its own provider, so users must choose a model that is acceptable for the repository being reviewed.

## Current Status

v0.1.0 release-prep (M8). The product is built end-to-end: GitHub + GitLab providers, Codex + Claude model adapters, three-pane TUI, clipboard/export, optional posting seam, prompt-injection-resilient parser. Acceptance happy-path runs against live PRs/MRs and clean-machine install verification remain before tagging. See [CHANGELOG.md](CHANGELOG.md) for the v0.1.0 release notes draft.

## Product Thesis

Most AI review tools fail by posting noisy comments directly into pull requests. Diffsmith takes the opposite approach:

- Local-first CLI/TUI.
- Explicit model selection.
- Suggested review comments are drafts.
- Every suggested review comment is editable.
- Publishing is manual copy/paste in v1.
- No backend.
- No auto-posting.

## Intended V1 Command

```sh
diffsmith review <github-pr-url|gitlab-mr-url> --model codex
diffsmith review <github-pr-url|gitlab-mr-url> --model claude
diffsmith review <github-pr-url|gitlab-mr-url> --model gemini
diffsmith review <github-pr-url|gitlab-mr-url> --model antigravity   # experimental in v1
```

V1 supports `codex`, `claude`, and `gemini` as fully tested adapters. `antigravity` (CLI binary: `agy`) is experimental in v1 and is currently disabled: selecting it returns a clear actionable error because the `agy` CLI has no non-interactive auth path. See `internal/model/antigravitycli/doc.go` for details.

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

If you have Go 1.22+ installed:

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

- `codex`, `claude`, or `gemini` CLI — at least one must be installed; the picker shows which are available at startup.
- (`agy` for Antigravity is not a supported install path in v1; the adapter ships disabled — see "Intended V1 Command" above)

## V1 Workflow

1. Detect GitHub or GitLab from the URL.
2. Fetch PR/MR metadata and diff through `gh` or `glab`.
3. Normalize changed files, hunks, and line positions.
4. Send a structured review prompt to the selected model CLI.
5. Parse model output as JSON findings.
6. Validate findings against the diff.
7. Show findings in a terminal UI.
8. Let the reviewer edit, approve, dismiss, and copy comments.

## V1 Non-Goals

- No direct posting to GitHub/GitLab.
- No CI bot mode.
- No hosted service.
- No session resume.
- No automatic patch application.
- No team policy engine.

## License

MIT — see [LICENSE](LICENSE). Diffsmith bundles no third-party code under a different license; runtime dependencies are listed in `go.mod`.
