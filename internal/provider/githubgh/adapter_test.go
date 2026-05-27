package githubgh

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// recordedCall captures one invocation made by the adapter against the
// mock Runner. Tests assert against this rather than reaching for
// reflection on the function value.
type recordedCall struct {
	name string
	args []string
}

// scriptedRunner returns canned responses in order. Each call records its
// args; if responses run out, it fails the test.
func scriptedRunner(t *testing.T, responses [][]byte) (provider.Runner, *[]recordedCall) {
	t.Helper()
	var calls []recordedCall
	i := 0
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		calls = append(calls, recordedCall{name: name, args: append([]string(nil), args...)})
		if i >= len(responses) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		out := responses[i]
		i++
		return out, nil
	}
	return run, &calls
}

func readDiffFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "diffs", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestAdapterFetchHappyPath(t *testing.T) {
	metaJSON := []byte(`{
		"title": "Tighten token parsing",
		"author": {"login": "alice"},
		"headRefName": "feat/parse",
		"headRefOid": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"baseRefName": "main",
		"url": "https://github.com/owner/repo/pull/42"
	}`)
	rawDiff := readDiffFixture(t, "modified_simple.diff")

	run, calls := scriptedRunner(t, [][]byte{metaJSON, rawDiff})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42/files?w=1")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Two argv invocations, in order: pr view, then pr diff.
	if len(*calls) != 2 {
		t.Fatalf("call count: got %d, want 2", len(*calls))
	}
	wantView := []string{"pr", "view", "https://github.com/owner/repo/pull/42", "--json", "title,author,headRefName,headRefOid,baseRefName,url"}
	if (*calls)[0].name != "gh" || !reflect.DeepEqual((*calls)[0].args, wantView) {
		t.Errorf("view call: got %s %v, want gh %v", (*calls)[0].name, (*calls)[0].args, wantView)
	}
	wantDiff := []string{"pr", "diff", "https://github.com/owner/repo/pull/42", "--patch", "--color", "never"}
	if (*calls)[1].name != "gh" || !reflect.DeepEqual((*calls)[1].args, wantDiff) {
		t.Errorf("diff call: got %s %v, want gh %v", (*calls)[1].name, (*calls)[1].args, wantDiff)
	}

	// Normalized input checks.
	if got, want := input.Target.Host, review.HostGitHub; got != want {
		t.Errorf("Host: got %q, want %q", got, want)
	}
	if got, want := input.Target.Number, 42; got != want {
		t.Errorf("Number: got %d, want %d", got, want)
	}
	if got, want := input.Target.URL, "https://github.com/owner/repo/pull/42"; got != want {
		t.Errorf("Target.URL: got %q, want %q", got, want)
	}
	if got, want := input.Title, "Tighten token parsing"; got != want {
		t.Errorf("Title: got %q, want %q", got, want)
	}
	if got, want := input.Author, "alice"; got != want {
		t.Errorf("Author: got %q, want %q", got, want)
	}
	if got, want := input.Target.HeadRef, "feat/parse"; got != want {
		t.Errorf("HeadRef: got %q, want %q", got, want)
	}
	if got, want := input.Target.HeadSHA, "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"; got != want {
		t.Errorf("HeadSHA: got %q, want %q", got, want)
	}
	if got, want := input.Target.BaseRef, "main"; got != want {
		t.Errorf("BaseRef: got %q, want %q", got, want)
	}
	firstPath := ""
	if len(input.Files) > 0 {
		firstPath = input.Files[0].Path
	}
	if len(input.Files) != 1 || firstPath != "auth/session.go" {
		t.Errorf("Files: got %d files, first path %q; want 1 file, auth/session.go", len(input.Files), firstPath)
	}
}

func TestAdapterFetchRejectsNonGitHubURL(t *testing.T) {
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		t.Fatal("runner should not be invoked when URL parsing fails")
		return nil, nil
	}
	a := New(provider.Runner(run))

	_, err := a.Fetch(context.Background(), "https://gitlab.com/g/p/-/merge_requests/1")
	if err == nil {
		t.Fatal("want error for GitLab URL, got nil")
	}
}

func TestAdapterFetchSurfacesRunnerError(t *testing.T) {
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		return nil, errors.New("gh: exit 4: not authenticated")
	}
	a := New(provider.Runner(run))

	_, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/1")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "gh pr view") {
		t.Errorf("error should identify the failing command, got: %v", err)
	}
}

func TestAdapterFetchSurfacesMalformedJSON(t *testing.T) {
	run, _ := scriptedRunner(t, [][]byte{[]byte("not json")})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/1")
	if err == nil || !strings.Contains(err.Error(), "decode gh pr view JSON") {
		t.Errorf("want decode error, got: %v", err)
	}
}

func TestAdapter_PreflightList_Success(t *testing.T) {
	run, calls := scriptedRunner(t, [][]byte{
		[]byte("Logged in to github.com as selyafi\n"),
	})
	a := New(run)

	if err := a.PreflightList(context.Background()); err != nil {
		t.Fatalf("PreflightList: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(*calls))
	}
	if got := (*calls)[0].args; got[0] != "auth" || got[1] != "status" {
		t.Errorf("expected gh auth status, got %v", got)
	}
}

func TestAdapter_PreflightList_NotAuthenticated(t *testing.T) {
	failingRun := func(ctx context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		return []byte("You are not logged in to any GitHub host.\n"), errors.New("exit status 1")
	}
	a := New(failingRun)

	err := a.PreflightList(context.Background())
	if err == nil {
		t.Fatal("expected error when gh is unauthenticated")
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("error should mention gh auth login; got: %v", err)
	}
}

func TestAdapter_List_Success(t *testing.T) {
	canned := []byte(`[
		{"number":13491,"title":"fix: handle empty PR descriptions","author":{"login":"alice"},"updatedAt":"2026-05-20T12:00:00Z","url":"https://github.com/cli/cli/pull/13491","isDraft":false},
		{"number":13485,"title":"docs: clarify auth","author":{"login":"bob"},"updatedAt":"2026-05-18T09:30:00Z","url":"https://github.com/cli/cli/pull/13485","isDraft":true}
	]`)
	run, calls := scriptedRunner(t, [][]byte{canned})
	a := New(run)

	got, err := a.List(context.Background(), provider.RepoCoord{Host: "github.com", Owner: "cli", Name: "cli"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d summaries, want 2", len(got))
	}
	if got[0].Number != 13491 || got[0].Author != "alice" || got[0].Draft {
		t.Errorf("row 0 mismatch: %+v", got[0])
	}
	if got[1].Number != 13485 || got[1].Author != "bob" || !got[1].Draft {
		t.Errorf("row 1 mismatch: %+v", got[1])
	}
	if got := (*calls)[0].args; got[0] != "pr" || got[1] != "list" {
		t.Errorf("expected gh pr list, got %v", got)
	}
	gotArgs := (*calls)[0].args
	if !contains(gotArgs, "--repo") || !contains(gotArgs, "cli/cli") {
		t.Errorf("expected --repo cli/cli in args, got %v", gotArgs)
	}
}

func TestAdapter_List_Empty(t *testing.T) {
	run, _ := scriptedRunner(t, [][]byte{[]byte(`[]`)})
	a := New(run)
	got, err := a.List(context.Background(), provider.RepoCoord{Host: "github.com", Owner: "x", Name: "y"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

func TestAdapter_List_MalformedJSON(t *testing.T) {
	run, _ := scriptedRunner(t, [][]byte{[]byte(`{"not":"an array"}`)})
	a := New(run)
	if _, err := a.List(context.Background(), provider.RepoCoord{Host: "github.com", Owner: "x", Name: "y"}); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// contains is a helper that checks if needle is present in haystack.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// scriptedResponse pairs a canned stdout payload with an optional error,
// letting tests script per-call outcomes (e.g. the diff call returning
// the GitHub 20K-line-cap 406 error before the files-API fallback runs).
type scriptedResponse struct {
	out []byte
	err error
}

func scriptedRunnerSeq(t *testing.T, responses []scriptedResponse) (provider.Runner, *[]recordedCall) {
	t.Helper()
	var calls []recordedCall
	i := 0
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		calls = append(calls, recordedCall{name: name, args: append([]string(nil), args...)})
		if i >= len(responses) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		r := responses[i]
		i++
		return r.out, r.err
	}
	return run, &calls
}

// TestAdapterFetch_FallsBackToFilesAPIOn20KLineCap is the diffsmith-5n4
// regression. When `gh pr diff` fails with the GitHub server-side 20000-
// line cap (HTTP 406, "exceeded the maximum number of lines"), the
// adapter must fall back to `gh api repos/{o}/{r}/pulls/{N}/files
// --paginate`, reassemble a unified diff from the per-file patches, and
// return a parsed ReviewInput. Without this, every PR over the cap
// aborts with an opaque "gh pr diff" error and the user can't review at
// all (as happened on oddin-gg/lagertha-mono#248).
func TestAdapterFetch_FallsBackToFilesAPIOn20KLineCap(t *testing.T) {
	metaJSON := []byte(`{
		"title": "Big PR",
		"author": {"login": "alice"},
		"headRefName": "feat/big",
		"headRefOid": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"baseRefName": "main",
		"url": "https://github.com/owner/repo/pull/42"
	}`)
	// The exact 406 body gh surfaces when the diff is too large.
	diffErr := errors.New("gh: exit 1: could not find pull request diff: HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)")
	filesJSON := []byte(`[
		{
			"filename": "a/first.go",
			"status": "modified",
			"patch": "@@ -1,3 +1,3 @@\n package a\n \n-var X = 1\n+var X = 2"
		},
		{
			"filename": "b/second.go",
			"status": "added",
			"patch": "@@ -0,0 +1,3 @@\n+package b\n+\n+const Y = \"hello\""
		}
	]`)

	run, calls := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON},
		{err: diffErr},
		{out: filesJSON},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("Fetch with 20K-cap fallback must succeed; got: %v", err)
	}

	if len(*calls) != 3 {
		t.Fatalf("call count: got %d, want 3 (pr view, pr diff, api files)", len(*calls))
	}
	apiCall := (*calls)[2]
	if apiCall.name != "gh" {
		t.Errorf("fallback should invoke gh, got %q", apiCall.name)
	}
	if len(apiCall.args) < 2 || apiCall.args[0] != "api" {
		t.Errorf("fallback first arg must be 'api'; got %v", apiCall.args)
	}
	wantPath := "repos/owner/repo/pulls/42/files"
	foundPath := false
	for _, arg := range apiCall.args {
		if arg == wantPath {
			foundPath = true
		}
	}
	if !foundPath {
		t.Errorf("fallback args must include %q; got %v", wantPath, apiCall.args)
	}
	if !contains(apiCall.args, "--paginate") {
		t.Errorf("fallback must paginate to handle large PRs; got %v", apiCall.args)
	}

	if len(input.Files) != 2 {
		t.Fatalf("want 2 files reassembled, got %d", len(input.Files))
	}
	if input.Files[0].Path != "a/first.go" {
		t.Errorf("file 0 path: got %q, want a/first.go", input.Files[0].Path)
	}
	if input.Files[1].Path != "b/second.go" {
		t.Errorf("file 1 path: got %q, want b/second.go", input.Files[1].Path)
	}
	if input.Target.HeadSHA != "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" {
		t.Errorf("HeadSHA lost across fallback: got %q", input.Target.HeadSHA)
	}
}

// TestAdapterFetch_FallbackPropagatesNon406DiffErrors confirms we don't
// over-trigger: a generic `gh pr diff` error (auth, network, missing PR)
// must surface unchanged, not silently fall back to the files API.
func TestAdapterFetch_FallbackPropagatesNon406DiffErrors(t *testing.T) {
	metaJSON := []byte(`{"title":"X","author":{"login":"a"},"headRefName":"h","headRefOid":"deadbeef","baseRefName":"main","url":"u"}`)
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON},
		{err: errors.New("gh: exit 4: not authenticated")},
	})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err == nil {
		t.Fatal("non-406 errors must surface, not trigger fallback")
	}
	if !strings.Contains(err.Error(), "not authenticated") {
		t.Errorf("error should preserve the underlying cause; got %v", err)
	}
}

// TestAdapterFetch_FallbackReassemblesRenamesAndDeletes confirms the
// reassembled diff captures the file-kind signals the classifier looks
// for: a renamed file must round-trip through diff.Parse with its old
// path preserved on OldPath, and a removed file must classify as
// FileDelete (not FileText). Without this, post-fallback the post
// flow's rename-aware position would lose the old_path mapping and
// inline comments would land on /dev/null.
func TestAdapterFetch_FallbackReassemblesRenamesAndDeletes(t *testing.T) {
	metaJSON := []byte(`{"title":"X","author":{"login":"a"},"headRefName":"h","headRefOid":"deadbeef","baseRefName":"main","url":"u"}`)
	diffErr := errors.New("gh: exit 1: HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)")
	filesJSON := []byte(`[
		{
			"filename": "newname.go",
			"previous_filename": "oldname.go",
			"status": "renamed",
			"patch": "@@ -1,1 +1,1 @@\n-old\n+new"
		},
		{
			"filename": "gone.go",
			"status": "removed",
			"patch": "@@ -1,1 +0,0 @@\n-bye"
		}
	]`)
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON},
		{err: diffErr},
		{out: filesJSON},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(input.Files) != 2 {
		t.Fatalf("want 2 files, got %d", len(input.Files))
	}
	// Renamed entry must keep its old path so the rename-aware GitLab
	// poster can route inline comments via old_path.
	if input.Files[0].Path != "newname.go" || input.Files[0].OldPath != "oldname.go" {
		t.Errorf("rename: got Path=%q OldPath=%q; want newname.go / oldname.go",
			input.Files[0].Path, input.Files[0].OldPath)
	}
	// Removed entry must be classified by the parser as a delete via
	// the `deleted file mode` extended header we emit.
	if input.Files[1].Path != "/dev/null" {
		t.Errorf("removed: Path should be /dev/null after parser normalization; got %q", input.Files[1].Path)
	}
	if input.Files[1].OldPath != "gone.go" {
		t.Errorf("removed: OldPath should preserve original name; got %q", input.Files[1].OldPath)
	}
}

// TestAdapterFetch_FallbackSkipsNullPatchFiles covers GitHub's per-file
// ~3MB patch cap: when a file's `patch` field is null, we skip that file
// (warning, not error) and continue with the rest. Failing the whole
// review because one file is too big would be a worse UX than reviewing
// the other files without it.
func TestAdapterFetch_FallbackSkipsNullPatchFiles(t *testing.T) {
	metaJSON := []byte(`{"title":"X","author":{"login":"a"},"headRefName":"h","headRefOid":"deadbeef","baseRefName":"main","url":"u"}`)
	diffErr := errors.New("gh: exit 1: HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)")
	// One reviewable file, one too-large file (patch:null).
	filesJSON := []byte(`[
		{"filename":"small.go","status":"modified","patch":"@@ -1,1 +1,1 @@\n-old\n+new"},
		{"filename":"huge.go","status":"modified","patch":null}
	]`)
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON},
		{err: diffErr},
		{out: filesJSON},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("null-patch entries must be skipped, not fatal; got %v", err)
	}
	if len(input.Files) != 1 {
		t.Fatalf("want 1 file (huge.go skipped), got %d", len(input.Files))
	}
	if input.Files[0].Path != "small.go" {
		t.Errorf("kept file path: got %q, want small.go", input.Files[0].Path)
	}
}
