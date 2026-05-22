# Diffsmith

Diffsmith is a local, human-in-the-loop AI code review cockpit for GitHub pull requests and GitLab merge requests.

It fetches a PR/MR diff, asks a selected AI CLI to draft review findings, validates the output, and opens a terminal UI where you inspect, edit, approve, dismiss, and copy comments manually.

Diffsmith is not an auto-comment bot. The reviewer stays in control.

Diffsmith has no server. The selected model CLI may still send diffs to its own provider, so users must choose a model that is acceptable for the repository being reviewed.

## Current Status

Design draft for v1. No implementation yet.

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
diffsmith review <github-pr-url|gitlab-mr-url> --model antigravity   # experimental in v1
```

V1 supports `codex` and `claude` as fully tested adapters. `antigravity` (CLI binary: `agy`) is experimental in v1 and may fail with a clear "adapter spike required" message until its invocation spike closes.

## V1 Dependencies

For repository access:

- `gh` for GitHub pull requests
- `glab` for GitLab merge requests

For AI review:

- `codex`, `claude`, or `agy` (Antigravity) CLI, depending on `--model`

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

## Documentation

- [V1 Design Overview](docs/diffsmith-v1-design.md)
- [Vision](docs/vision.md)
- [V1 Scope](docs/v1-scope.md)
- [Architecture](docs/architecture.md)
- [TUI Workflow](docs/tui-workflow.md)
- [Review Finding Schema](docs/review-finding-schema.md)
- [Provider Adapters](docs/provider-adapters.md)
- [Model Adapters](docs/model-adapters.md)
- [Prompt Contract](docs/prompt-contract.md)
- [Security and Privacy](docs/security-and-privacy.md)
- [Confidence and Validation](docs/confidence-and-validation.md)
- [Roadmap](docs/roadmap.md)
- [Open Questions](docs/open-questions.md)
- [Local Notes](docs/NOTES.md)
- [Decisions](docs/decisions)
- [Dev Plan](docs/dev-plan)
