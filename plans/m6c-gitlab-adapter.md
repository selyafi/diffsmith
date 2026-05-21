# Plan: M6c — GitLab Adapter (Fetch metadata + diff via glab)

## Context

The core of M6: an Adapter implementing `provider.Provider` for GitLab MRs. Composes M6a (URL parser, already complete) and M6b (Preflight, already complete) into a working `Fetch` that shells out to `glab`, parses the diff via `internal/diff`, and returns a normalized `review.ReviewInput`.

Reference: `internal/provider/githubgh/adapter.go` (117 lines) + `adapter_test.go` (156 lines). Mirror its shape except where GitLab's CLI/API differs:
- `glab mr view <iid> -R <repo-url> --output json` (NOT URL-form like gh)
- `glab mr diff <iid> -R <repo-url> --raw --color never`
- JSON fields are snake_case (`iid`, `source_branch`, `target_branch`, `sha`, `web_url`, `author.username`), NOT gh's camelCase
- `iid` is the user-facing MR number (internal id within project); `id` would be the global GitLab id, which is not what users type in URLs

Local environment confirmed during planning: `glab 1.93.0` on PATH, authenticated against gitlab.com and gitlab.dbi.ru. `glab mr view --help` shows `-F/--output text|json` (also long-form `--output json` works). `glab mr diff --help` confirms `--color` and `--raw`.

## Phase 1: RED — write failing tests + compile-only stubs

status: complete

### Changes

- file: `internal/provider/gitlabglab/adapter.go` — create with **compile-only stubs**:
  - `Adapter` struct with `run provider.Runner` and `preflight *Preflight` unexported fields
  - `New(run provider.Runner) *Adapter` returning a zero-valued Adapter (real defaults applied in Phase 2)
  - `Supports(rawURL string) bool` delegates to the package-level `Supports` (already implemented in M6a; should return false for the package method too via the package-level until impl). Or simpler: just delegate.
  - `Preflight(ctx) error` returns `errors.New("not implemented")` for now
  - `Fetch(ctx, rawURL) (*review.ReviewInput, error)` returns `nil, errors.New("not implemented")`
- file: `internal/provider/gitlabglab/adapter_test.go` — create with these tests:
  - `TestAdapterFetchHappyPathSingleGroup`: stub runner returns canned MR JSON + diff fixture for URL `https://gitlab.com/group/project/-/merge_requests/42/diffs?tab=overview`. Assert: 2 invocations in order (`mr view` then `mr diff`), exact argv shapes, normalized `ReviewInput` fields (Host=HostGitLab, Number=42, Owner="group", Repo="project", Title, Author, HeadRef, HeadSHA, BaseRef, URL canonical), `Files` parsed from the diff fixture, AND `input.RawDiff == string(diffFixture)` byte-for-byte (so a forgot-to-set-RawDiff impl can't pass).
  - `TestAdapterFetchHappyPathNestedGroup`: same as above but URL is `https://gitlab.com/group/sub/project/-/merge_requests/9001`. Assert: argv carries `-R https://gitlab.com/group/sub/project`, Owner="group/sub", Repo="project", Number=9001, AND `input.RawDiff == string(diffFixture)`.
  - `TestAdapterFetchRejectsNonGitLabURL`: GitHub URL → error from ParseURL surfaces; runner must NEVER be invoked.
  - `TestAdapterFetchSurfacesViewCommandError`: first scripted runner call (`mr view`) returns error → Fetch returns an error whose message identifies `"glab mr view"`. (Renamed from the earlier generic "SurfacesRunnerError" to make the scope explicit.)
  - `TestAdapterFetchSurfacesDiffCommandError`: first call (`mr view`) returns valid metadata JSON; second call (`mr diff`) returns error. Fetch must return an error whose message identifies `"glab mr diff"` (NOT `"glab mr view"`). This independently verifies the diff-error wrap that the previous test cannot.
  - `TestAdapterFetchSurfacesMalformedJSON`: runner returns `"not json"` for view → Fetch returns an error mentioning JSON decode.
  - `TestAdapterFetchRejectsEmptySHA`: runner returns metadata JSON where `sha` is the empty string (`"sha": ""`). Fetch must return an error (no downstream nil-anchor) — message should mention something like "empty sha" or "missing sha".
- file: `internal/provider/gitlabglab/testdata/synthetic.diff` — small unified diff fixture for the happy-path tests. Synthetic content is sufficient (no need to capture against a real MR; the diff parser doesn't care about URL shape). One file, one hunk, a few +/- lines.

### Test Gate

- [ ] `go test ./internal/provider/gitlabglab/ -run 'TestAdapter' -count=1` builds successfully (stubs ensure compile) and **all seven new tests FAIL** with sub-test-level behavioral assertion errors (because stub `Fetch` returns `"not implemented"`). Failures must NOT be a package compile error. If any test passes on first run, the test is asserting existing behavior — re-examine.

## Phase 2: GREEN — implement Adapter, Fetch, fetchMetadata

status: complete

### Changes

- file: `internal/provider/gitlabglab/adapter.go` — replace stubs with real impl:
  - `New(run)` applies defaults: `run = provider.DefaultRunner` if nil; `preflight = NewPreflight(run, nil)`.
  - `Supports` delegates to package-level `Supports` (already in `url.go`).
  - `Preflight` delegates to `a.preflight.Check(ctx)`.
  - `Fetch(ctx, rawURL)`:
    1. `ref, err := ParseURL(rawURL)` — fail-fast on bad URL.
    2. `meta, err := a.fetchMetadata(ctx, ref)` — invokes `glab mr view <iid> -R <repo-url> --output json`. Decodes into `mrMetadata`.
    3. `rawDiff, err := a.run(ctx, nil, "glab", "mr", "diff", strconv.Itoa(ref.Number), "-R", ref.RepoURL, "--raw", "--color", "never")` — fail-wrap with `"glab mr diff"`.
    4. `files, err := diff.Parse(string(rawDiff))` — fail-wrap with `"parse diff"`.
    5. Build `ReviewInput`:
       - `Target.Host = review.HostGitLab`
       - `Target.URL = ref.URL` (canonical from ParseURL; meta.WebURL is a fallback if non-empty)
       - `Target.Owner, Target.Repo` = split `ref.ProjectPath` at last `/`
       - `Target.Number = ref.Number`
       - `Target.HeadRef = meta.SourceBranch`
       - `Target.HeadSHA = meta.SHA`
       - `Target.BaseRef = meta.TargetBranch`
       - `Title = meta.Title`
       - `Author = meta.Author.Username`
       - `Files = files`
       - `RawDiff = string(rawDiff)`
  - `mrMetadata` struct (snake_case JSON tags, based on GitLab REST API conventions — see https://docs.gitlab.com/api/merge_requests/#response):
    ```go
    type mrMetadata struct {
        Title  string `json:"title"`
        Author struct {
            Username string `json:"username"`
        } `json:"author"`
        SourceBranch string `json:"source_branch"`
        TargetBranch string `json:"target_branch"`
        SHA          string `json:"sha"`
        WebURL       string `json:"web_url"`
    }
    ```
  - `fetchMetadata(ctx, ref)`: `a.run(ctx, nil, "glab", "mr", "view", strconv.Itoa(ref.Number), "-R", ref.RepoURL, "--output", "json")`; `json.Unmarshal` into `mrMetadata`. Wrap errors with `"glab mr view"` / `"decode glab mr view JSON"` per gh's pattern. **After decode, validate `meta.SHA != ""`** — if empty, return `fmt.Errorf("glab mr view returned empty sha for MR #%d (cannot anchor review comments without a head commit)", ref.Number)`. This is asymmetric with `githubgh.Adapter` (which doesn't validate `meta.HeadRefOid`); the gh side should be brought into line in a follow-up — adding the check on the GitLab side now closes the QA finding without regressing anything.
  - Helper `splitProjectPath(p string) (owner, repo string)`: `strings.LastIndex(p, "/")`; everything before → owner; everything after → repo. Assumed there's always at least one `/` because `ParseURL` enforces it (the "single-segment path" test case in `url_test.go` rejects single-segment paths).

### Test Gate

- [ ] `go test ./internal/provider/gitlabglab/ -count=1` → all tests PASS (M6a's TestParseURL/TestSupports + M6b's three Preflight tests + all five new Adapter tests).
- [ ] Argv for `mr view` matches: `["mr", "view", "<iid>", "-R", "<repo-url>", "--output", "json"]`.
- [ ] Argv for `mr diff` matches: `["mr", "diff", "<iid>", "-R", "<repo-url>", "--raw", "--color", "never"]`.
- [ ] Owner/Repo split correctly for both single-group (`Owner="group", Repo="project"`) and nested-group (`Owner="group/sub", Repo="project"`).
- [ ] Host is `review.HostGitLab`.

## Phase 3: Whole-repo verification

status: complete

### Changes

- (no production code changes — verification only)

### Test Gate

- [ ] `go test ./... -count=1` → all packages PASS.
- [ ] `go vet ./...` → exit 0.
- [ ] `go build ./...` → exit 0.
- [ ] `gofmt -l internal/` → empty.

## Dependencies

- Phase 2 depends on Phase 1 (TDD discipline).
- Phase 3 depends on Phase 2.
- Adapter depends on `MergeRequestRef` (M6a, complete) and `Preflight`/`NewPreflight` (M6b, complete).

## Out of scope

- Wiring `gitlabglab.New()` into `defaultRegistry()` — that's M6d.
- Closing spike S2 + docs — that's M6e.
- Capturing real-world `glab mr view` output against a known public MR — the synthetic fixture suffices for hermetic tests; the JSON shape is documented in this plan and Codex QA will catch any discrepancy with GitLab's actual REST API response.

## Risk register

1. **GitLab JSON field names** — best-guessed from GitLab REST API conventions. Codex's QA pass should cross-check against any installed `glab mr view --help` schema docs or the GitLab API reference.
2. **`iid` vs `id`** — `iid` is the project-local user-visible MR number; `id` is the global GitLab ID. We send `iid` because that's what users type. The mrMetadata struct doesn't decode either (we already have `ref.Number` from ParseURL) — but the argv must use the iid form (a plain integer, not the GitLab global id).
3. **Owner/Repo split for nested groups** — convention "everything before the last slash" is a forward decision. The user-facing `Target.URL` and the `glab -R` flag both use the full `ref.ProjectPath`, so the split is mostly cosmetic for downstream consumers. Document the convention so a future reader doesn't try to reverse-engineer it.
