package gitlabglab

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/provider"
	"github.com/selyafi/diffsmith/internal/review"
)

// recordedCall captures one invocation made by the adapter against the
// mock Runner.
type recordedCall struct {
	name string
	args []string
}

// scriptedRunner returns canned responses in order. If responses run out,
// it fails the test. Each entry can be either a (body, nil) success or a
// (nil, err) failure.
type scriptResult struct {
	out []byte
	err error
}

func scriptedRunner(t *testing.T, results []scriptResult) (provider.Runner, *[]recordedCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []recordedCall
	i := 0
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, recordedCall{name: name, args: append([]string(nil), args...)})
		if i >= len(results) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		r := results[i]
		i++
		return r.out, r.err
	}
	return run, &calls
}

func readDiffFixture(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "synthetic.diff")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read synthetic.diff: %v", err)
	}
	return data
}

// singleGroupMetadata returns a valid mr-view JSON body for the
// single-group happy-path test. Snake_case fields mirror GitLab's REST
// API response shape so the real Fetch can decode the same JSON.
func singleGroupMetadata() []byte {
	return []byte(`{
		"title": "Pin context to request scope",
		"author": {"username": "alice"},
		"source_branch": "feat/ctx-scope",
		"target_branch": "main",
		"sha": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"web_url": "https://gitlab.com/group/project/-/merge_requests/42"
	}`)
}

func nestedGroupMetadata() []byte {
	return []byte(`{
		"title": "Nested-group MR",
		"author": {"username": "bob"},
		"source_branch": "feat/nested",
		"target_branch": "main",
		"sha": "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"web_url": "https://gitlab.com/group/sub/project/-/merge_requests/9001"
	}`)
}

func TestAdapterFetchHappyPathSingleGroup(t *testing.T) {
	rawDiff := readDiffFixture(t)
	run, calls := scriptedRunner(t, []scriptResult{
		{out: singleGroupMetadata()},
		{out: rawDiff},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(),
		"https://gitlab.com/group/project/-/merge_requests/42/diffs?tab=overview")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("call count: got %d, want 2 (mr view + mr diff)", len(*calls))
	}
	wantView := []string{"mr", "view", "42", "-R", "https://gitlab.com/group/project", "--output", "json"}
	if (*calls)[0].name != "glab" || !reflect.DeepEqual((*calls)[0].args, wantView) {
		t.Errorf("view call: got %s %v, want glab %v", (*calls)[0].name, (*calls)[0].args, wantView)
	}
	wantDiff := []string{"mr", "diff", "42", "-R", "https://gitlab.com/group/project", "--raw", "--color", "never"}
	if (*calls)[1].name != "glab" || !reflect.DeepEqual((*calls)[1].args, wantDiff) {
		t.Errorf("diff call: got %s %v, want glab %v", (*calls)[1].name, (*calls)[1].args, wantDiff)
	}

	if got, want := input.Target.Host, review.HostGitLab; got != want {
		t.Errorf("Host: got %q, want %q", got, want)
	}
	if got, want := input.Target.Number, 42; got != want {
		t.Errorf("Number: got %d, want %d", got, want)
	}
	if got, want := input.Target.Owner, "group"; got != want {
		t.Errorf("Owner: got %q, want %q", got, want)
	}
	if got, want := input.Target.Repo, "project"; got != want {
		t.Errorf("Repo: got %q, want %q", got, want)
	}
	if got, want := input.Target.URL, "https://gitlab.com/group/project/-/merge_requests/42"; got != want {
		t.Errorf("Target.URL: got %q, want %q", got, want)
	}
	if got, want := input.Title, "Pin context to request scope"; got != want {
		t.Errorf("Title: got %q, want %q", got, want)
	}
	if got, want := input.Author, "alice"; got != want {
		t.Errorf("Author: got %q, want %q", got, want)
	}
	if got, want := input.Target.HeadRef, "feat/ctx-scope"; got != want {
		t.Errorf("HeadRef: got %q, want %q", got, want)
	}
	if got, want := input.Target.HeadSHA, "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"; got != want {
		t.Errorf("HeadSHA: got %q, want %q", got, want)
	}
	if got, want := input.Target.BaseRef, "main"; got != want {
		t.Errorf("BaseRef: got %q, want %q", got, want)
	}
	if input.RawDiff != string(rawDiff) {
		t.Errorf("RawDiff: byte-for-byte mismatch against fixture")
	}
	if len(input.Files) != 1 {
		t.Errorf("Files: got %d files, want 1", len(input.Files))
	} else if input.Files[0].Path != "svc/handler.go" {
		t.Errorf("Files[0].Path: got %q, want %q", input.Files[0].Path, "svc/handler.go")
	}
}

func TestAdapterFetchHappyPathNestedGroup(t *testing.T) {
	rawDiff := readDiffFixture(t)
	run, calls := scriptedRunner(t, []scriptResult{
		{out: nestedGroupMetadata()},
		{out: rawDiff},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(),
		"https://gitlab.com/group/sub/project/-/merge_requests/9001")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	wantRepoURL := "https://gitlab.com/group/sub/project"
	wantView := []string{"mr", "view", "9001", "-R", wantRepoURL, "--output", "json"}
	if !reflect.DeepEqual((*calls)[0].args, wantView) {
		t.Errorf("view call argv: got %v, want %v", (*calls)[0].args, wantView)
	}
	wantDiff := []string{"mr", "diff", "9001", "-R", wantRepoURL, "--raw", "--color", "never"}
	if !reflect.DeepEqual((*calls)[1].args, wantDiff) {
		t.Errorf("diff call argv: got %v, want %v", (*calls)[1].args, wantDiff)
	}

	if got, want := input.Target.Owner, "group/sub"; got != want {
		t.Errorf("Owner (nested): got %q, want %q", got, want)
	}
	if got, want := input.Target.Repo, "project"; got != want {
		t.Errorf("Repo (nested): got %q, want %q", got, want)
	}
	if got, want := input.Target.Number, 9001; got != want {
		t.Errorf("Number: got %d, want %d", got, want)
	}
	if input.RawDiff != string(rawDiff) {
		t.Errorf("RawDiff: byte-for-byte mismatch against fixture")
	}
}

// TestAdapterFetch_PopulatesDescription is diffsmith-144: Fetch must
// capture the MR description (already present in the glab mr view JSON,
// previously discarded) into ReviewInput.Description.
func TestAdapterFetch_PopulatesDescription(t *testing.T) {
	meta := []byte(`{
		"title": "Pin context",
		"author": {"username": "alice"},
		"source_branch": "feat/ctx",
		"target_branch": "main",
		"sha": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"web_url": "https://gitlab.com/group/project/-/merge_requests/42",
		"description": "Implements the widget.\n\nCloses #7"
	}`)
	run, _ := scriptedRunner(t, []scriptResult{{out: meta}, {out: readDiffFixture(t)}})
	a := New(run)

	input, err := a.Fetch(context.Background(),
		"https://gitlab.com/group/project/-/merge_requests/42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "Implements the widget.\n\nCloses #7"; input.Description != want {
		t.Errorf("Description = %q, want %q", input.Description, want)
	}
}

func linkedIssuesTarget() review.ReviewTarget {
	return review.ReviewTarget{
		Host:   review.HostGitLab,
		URL:    "https://gitlab.com/group/project/-/merge_requests/42",
		Owner:  "group",
		Repo:   "project",
		Number: 42,
	}
}

// TestFetchLinkedIssues_ResolvesClosesIssues is diffsmith-144: the GitLab
// adapter resolves the MR's closing issues via the closes_issues API,
// which returns title + description + web_url in a single call.
func TestFetchLinkedIssues_ResolvesClosesIssues(t *testing.T) {
	resp := []byte(`[
		{"iid":7,"title":"Widget","description":"AC: it widgets","web_url":"https://gitlab.com/group/project/-/issues/7"},
		{"iid":9,"title":"Gadget","description":"AC: it gadgets","web_url":"https://gitlab.com/group/project/-/issues/9"}
	]`)
	run, calls := scriptedRunner(t, []scriptResult{{out: resp}})
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("FetchLinkedIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %d", len(issues))
	}
	if issues[0].Number != 7 || issues[0].Title != "Widget" || !strings.Contains(issues[0].Body, "widgets") || issues[0].URL == "" {
		t.Errorf("issue[0] decoded wrong: %+v", issues[0])
	}
	if issues[1].Number != 9 || issues[1].Title != "Gadget" {
		t.Errorf("issue[1] decoded wrong: got Number=%d Title=%q, want Number=9 Title=%q", issues[1].Number, issues[1].Title, "Gadget")
	}
	if len(notes) != 0 {
		t.Errorf("no notes expected (single call); got %v", notes)
	}
	want := []string{"api", "projects/group%2Fproject/merge_requests/42/closes_issues", "--hostname", "gitlab.com"}
	if (*calls)[0].name != "glab" || !reflect.DeepEqual((*calls)[0].args, want) {
		t.Errorf("call: got %s %v, want glab %v", (*calls)[0].name, (*calls)[0].args, want)
	}
}

// TestFetchLinkedIssues_EncodesNestedProjectPath verifies the project path
// is URL-encoded (slashes → %2F) so nested groups resolve correctly.
func TestFetchLinkedIssues_EncodesNestedProjectPath(t *testing.T) {
	run, calls := scriptedRunner(t, []scriptResult{{out: []byte(`[]`)}})
	a := New(run)
	target := review.ReviewTarget{
		Host:   review.HostGitLab,
		URL:    "https://gitlab.com/group/sub/project/-/merge_requests/9001",
		Owner:  "group/sub",
		Repo:   "project",
		Number: 9001,
	}
	if _, _, err := a.FetchLinkedIssues(context.Background(), target); err != nil {
		t.Fatalf("FetchLinkedIssues: %v", err)
	}
	want := "projects/group%2Fsub%2Fproject/merge_requests/9001/closes_issues"
	if (*calls)[0].args[1] != want {
		t.Errorf("api path: got %q, want %q", (*calls)[0].args[1], want)
	}
}

// TestFetchLinkedIssues_TotalFailureReturnsError: a failed closes_issues
// query is total — returned as err for the caller to surface as one note.
func TestFetchLinkedIssues_TotalFailureReturnsError(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{{err: errors.New("glab api exploded")}})
	a := New(run)

	if _, _, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget()); err == nil {
		t.Fatal("a failed closes_issues query must return err")
	}
}

// TestFetchLinkedIssues_NoIssuesIsEmpty: an MR that closes no issues yields
// empty criteria with no error.
func TestFetchLinkedIssues_NoIssuesIsEmpty(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{{out: []byte(`[]`)}})
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("no issues should not error: %v", err)
	}
	if len(issues) != 0 || len(notes) != 0 {
		t.Errorf("expected empty; got %d issues, notes %v", len(issues), notes)
	}
}

func TestAdapterFetchRejectsNonGitLabURL(t *testing.T) {
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		t.Fatal("runner must not be invoked when URL parsing fails")
		return nil, nil
	}
	a := New(provider.Runner(run))

	_, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/1")
	if err == nil {
		t.Fatal("want error for GitHub URL, got nil")
	}
	// The error must originate from ParseURL (host-guard), NOT from a
	// downstream stub returning a generic error. This distinguishes "we
	// bailed at URL parsing" from "we ran something and it failed".
	if !strings.Contains(err.Error(), "expected gitlab.com") {
		t.Errorf("want ParseURL host-guard error mentioning 'expected gitlab.com', got: %v", err)
	}
}

func TestAdapterFetchSurfacesViewCommandError(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{
		{err: errors.New("glab: exit 4: not authenticated")},
	})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://gitlab.com/group/project/-/merge_requests/1")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "glab mr view") {
		t.Errorf("error should identify the failing command 'glab mr view', got: %v", err)
	}
}

func TestAdapterFetchSurfacesDiffCommandError(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{
		{out: singleGroupMetadata()},
		{err: errors.New("glab: exit 1: diff failed")},
	})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://gitlab.com/group/project/-/merge_requests/42")
	if err == nil {
		t.Fatal("want diff-command error, got nil")
	}
	if !strings.Contains(err.Error(), "glab mr diff") {
		t.Errorf("error should identify the failing command 'glab mr diff' (NOT 'glab mr view'), got: %v", err)
	}
	if strings.Contains(err.Error(), "glab mr view") {
		t.Errorf("diff-error must not be mis-attributed to 'glab mr view'; got: %v", err)
	}
}

func TestAdapterFetchSurfacesMalformedJSON(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{
		{out: []byte("not json")},
	})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://gitlab.com/group/project/-/merge_requests/1")
	if err == nil || !strings.Contains(err.Error(), "decode glab mr view JSON") {
		t.Errorf("want decode error mentioning 'decode glab mr view JSON', got: %v", err)
	}
}

func TestAdapterFetchRejectsEmptySHA(t *testing.T) {
	emptySHAMeta := []byte(`{
		"title": "no sha",
		"author": {"username": "alice"},
		"source_branch": "x",
		"target_branch": "main",
		"sha": "",
		"web_url": "https://gitlab.com/group/project/-/merge_requests/7"
	}`)
	run, _ := scriptedRunner(t, []scriptResult{
		{out: emptySHAMeta},
	})
	a := New(run)

	_, err := a.Fetch(context.Background(), "https://gitlab.com/group/project/-/merge_requests/7")
	if err == nil {
		t.Fatal("want error when sha is empty, got nil")
	}
	// Message should reference the empty/missing sha condition so the user
	// understands why the fetch was rejected (vs. e.g. a transport error).
	msg := err.Error()
	if !strings.Contains(msg, "sha") {
		t.Errorf("error must mention 'sha' to localise the cause; got: %v", err)
	}
}

// readRealFixture loads one of the captured-real-glab fixtures alongside
// synthetic.diff in testdata/. Used by the M6c followup tests below.
func readRealFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return data
}

// TestParseRealGitLabMRSingleGroup parses a captured `glab mr diff --raw`
// body from gitlab-org/cli!3313 (single-group URL shape:
// gitlab.com/<group>/<project>). The hermetic adapter tests above prove
// the argv shape; this test proves the diff parser still consumes real
// glab output, so a parser swap or library upgrade can't silently break
// the single-group path. Pair with the nested-group test below.
//
// Refresh:
//
//	glab mr diff 3313 -R gitlab-org/cli --raw --color never > \
//	  internal/provider/gitlabglab/testdata/gitlab_mr_cli_3313.diff
func TestParseRealGitLabMRSingleGroup(t *testing.T) {
	raw := readRealFixture(t, "gitlab_mr_cli_3313.diff")

	files, err := diff.Parse(string(raw))
	if err != nil {
		t.Fatalf("diff.Parse: %v", err)
	}
	if got, want := len(files), 4; got != want {
		t.Fatalf("file count: got %d, want %d", got, want)
	}
	for _, f := range files {
		if f.Kind != diff.FileText {
			t.Errorf("file %q: Kind = %v, want FileText", f.Path, f.Kind)
		}
	}
	// Anchor one known path so a fixture replacement that targets a
	// different MR fails loudly rather than silently passing with wrong
	// content.
	var found bool
	for _, f := range files {
		if f.Path == "internal/commands/update/check_update.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected internal/commands/update/check_update.go; fixture may have been replaced")
	}
}

// TestParseRealGitLabMRNestedGroup mirrors the single-group test for a
// nested-group URL: gitlab-org/cluster-integration/gitlab-agent!3650
// (gitlab.com/<group>/<subgroup>/<project>). Diff content is independent
// of URL shape so the two tests can drift apart without one masking
// regressions in the other.
//
// Refresh:
//
//	glab mr diff 3650 -R gitlab-org/cluster-integration/gitlab-agent \
//	  --raw --color never > \
//	  internal/provider/gitlabglab/testdata/gitlab_mr_gitlab_agent_3650.diff
func TestAdapter_PreflightList_Success(t *testing.T) {
	run, calls := scriptedRunner(t, []scriptResult{
		{out: []byte("Logged in to gitlab.com as selyafi (token)\n")},
	})
	a := New(run)

	if err := a.PreflightList(context.Background()); err != nil {
		t.Fatalf("PreflightList: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].args[0] != "auth" {
		t.Errorf("expected glab auth status call, got %+v", (*calls)[0])
	}
}

func TestAdapter_PreflightList_NotAuthenticated(t *testing.T) {
	failingRun := func(ctx context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		return []byte("Not logged in.\n"), errors.New("exit status 1")
	}
	a := New(failingRun)

	err := a.PreflightList(context.Background())
	if err == nil {
		t.Fatal("expected error when glab is unauthenticated")
	}
	if !strings.Contains(err.Error(), "glab auth login") {
		t.Errorf("error should mention glab auth login; got: %v", err)
	}
}

func TestParseRealGitLabMRNestedGroup(t *testing.T) {
	raw := readRealFixture(t, "gitlab_mr_gitlab_agent_3650.diff")

	files, err := diff.Parse(string(raw))
	if err != nil {
		t.Fatalf("diff.Parse: %v", err)
	}
	if got, want := len(files), 5; got != want {
		t.Fatalf("file count: got %d, want %d", got, want)
	}
	for _, f := range files {
		if f.Kind != diff.FileText {
			t.Errorf("file %q: Kind = %v, want FileText", f.Path, f.Kind)
		}
	}
	var found bool
	for _, f := range files {
		if f.Path == "internal/module/autoflow/rpc/rpc.proto" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected internal/module/autoflow/rpc/rpc.proto; fixture may have been replaced")
	}
}

func TestAdapter_List_Success(t *testing.T) {
	// Use one MR to keep the scripted runner deterministic under concurrent
	// goroutines (mr-list call first, then the single discussions call).
	canned := []byte(`[
		{"iid":3650,"title":"feat: kubernetes agent v2","author":{"username":"alice"},"updated_at":"2026-05-19T08:00:00Z","web_url":"https://gitlab.com/gitlab-org/cluster-integration/gitlab-agent/-/merge_requests/3650","draft":false}
	]`)
	emptyDiscussions := []byte(`[]`)
	run, calls := scriptedRunner(t, []scriptResult{{out: canned}, {out: emptyDiscussions}})
	a := New(run)

	got, err := a.List(context.Background(), provider.RepoCoord{Host: "gitlab.com", Owner: "gitlab-org/cli", Name: "cli"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	if got[0].Number != 3650 || got[0].Author != "alice" || got[0].Draft {
		t.Errorf("row 0 mismatch: %+v", got[0])
	}
	if args := (*calls)[0].args; args[0] != "mr" || args[1] != "list" {
		t.Errorf("expected glab mr list, got %v", args)
	}
}

func TestAdapter_List_Empty(t *testing.T) {
	run, _ := scriptedRunner(t, []scriptResult{{out: []byte(`[]`)}})
	a := New(run)
	got, err := a.List(context.Background(), provider.RepoCoord{Host: "gitlab.com", Owner: "x", Name: "y"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d", len(got))
	}
}

// glab prints warnings (deprecations, update-available notices, etc.)
// to stdout, prefixed before the JSON. Confirm the parser skips the
// preamble instead of failing.
func TestAdapter_List_StripsWarningPreamble(t *testing.T) {
	canned := []byte("Flag --opened has been deprecated, default value if neither --closed, --locked or --merged is used.\n[{\"iid\":42,\"title\":\"t\",\"author\":{\"username\":\"u\"},\"draft\":false,\"web_url\":\"https://example/42\"}]")
	// The enrichment phase makes one discussions call per MR; provide an
	// empty response so the runner doesn't run out of scripted results.
	run, _ := scriptedRunner(t, []scriptResult{{out: canned}, {out: []byte(`[]`)}})
	a := New(run)
	got, err := a.List(context.Background(), provider.RepoCoord{Host: "gitlab.com", Owner: "g", Name: "p"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Number != 42 {
		t.Errorf("expected 1 row with Number=42; got %+v", got)
	}
}

func TestListEnrichesFromDiscussions(t *testing.T) {
	mrList := []byte(`[{"iid":7,"title":"add market","author":{"username":"shelyafi"},
		"updated_at":"2026-06-16T00:00:00Z","web_url":"https://gitlab.com/o/r/-/merge_requests/7",
		"draft":false,"user_notes_count":4}]`)
	discussions := []byte(`[
		{"notes":[{"system":false,"resolvable":true,"resolved":true,"author":{"username":"prathoss"}}]},
		{"notes":[{"system":false,"resolvable":true,"resolved":false,"author":{"username":"yung-madamm"}}]},
		{"notes":[{"system":true,"resolvable":false,"resolved":false,"author":{"username":"someone"}}]},
		{"notes":[{"system":false,"resolvable":true,"resolved":false,"author":{"username":"coderabbitai"}}]}
	]`)
	run, _ := scriptedRunner(t, []scriptResult{{out: mrList}, {out: discussions}})
	a := New(run)

	got, err := a.List(context.Background(), provider.RepoCoord{Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	s := got[0]
	if s.Number != 7 || !s.Enriched {
		t.Errorf("base/enriched wrong: %+v", s)
	}
	if s.CommentCount != 4 {
		t.Errorf("CommentCount = %d, want 4 (user_notes_count)", s.CommentCount)
	}
	if s.ResolvedThreads != 1 || s.UnresolvedThreads != 2 {
		t.Errorf("threads = ✔%d/✖%d, want ✔1/✖2 (system discussion ignored)", s.ResolvedThreads, s.UnresolvedThreads)
	}
	if len(s.HumanCommenters) != 2 || s.HumanCommenters[0] != "prathoss" || s.HumanCommenters[1] != "yung-madamm" {
		t.Errorf("HumanCommenters = %v, want [prathoss yung-madamm] (bot + system excluded)", s.HumanCommenters)
	}
}

// TestList_PerMRDiscussionFailure verifies that when one MR's discussions call
// fails, that row is marked Enriched=false with its base fields (Number,
// Author, CommentCount) intact, while the other MR still enriches successfully.
// List must return nil error. A bespoke runner is used (instead of
// scriptedRunner) because the two discussions calls run concurrently and cannot
// be matched by position.
func TestList_PerMRDiscussionFailure(t *testing.T) {
	// Two MRs: iid 10 (discussions will fail) and iid 20 (discussions succeed).
	mrListJSON := []byte(`[
		{"iid":10,"title":"MR ten","author":{"username":"alice"},"updated_at":"2026-06-01T00:00:00Z","web_url":"https://gitlab.com/o/r/-/merge_requests/10","draft":false,"user_notes_count":2},
		{"iid":20,"title":"MR twenty","author":{"username":"bob"},"updated_at":"2026-06-01T00:00:00Z","web_url":"https://gitlab.com/o/r/-/merge_requests/20","draft":false,"user_notes_count":0}
	]`)

	discussionsIID20 := []byte(`[
		{"notes":[{"system":false,"resolvable":true,"resolved":true,"author":{"username":"carol"}}]}
	]`)

	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		// mr list call
		for i, a := range args {
			if a == "list" && i > 0 && args[i-1] == "mr" {
				return mrListJSON, nil
			}
		}
		// discussions calls: match by API path
		for _, a := range args {
			if strings.Contains(a, "/merge_requests/10/discussions") {
				return nil, errors.New("simulated discussions failure for MR 10")
			}
			if strings.Contains(a, "/merge_requests/20/discussions") {
				return discussionsIID20, nil
			}
		}
		t.Errorf("unexpected call: %s %v", name, args)
		return nil, errors.New("unexpected call")
	}

	a := New(provider.Runner(run))
	got, err := a.List(context.Background(), provider.RepoCoord{Host: "gitlab.com", Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("List must return nil error even when one discussions call fails; got: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %d", len(got))
	}

	// Find rows by Number (concurrent goroutines may write in any order).
	var row10, row20 *provider.PRSummary
	for i := range got {
		switch got[i].Number {
		case 10:
			row10 = &got[i]
		case 20:
			row20 = &got[i]
		}
	}
	if row10 == nil || row20 == nil {
		t.Fatalf("could not find both rows by Number; got %+v", got)
	}

	// iid 10: enrichment failed — Enriched=false, but base fields intact.
	if row10.Enriched {
		t.Errorf("iid 10: Enriched should be false after discussions failure")
	}
	if row10.Number != 10 {
		t.Errorf("iid 10: Number = %d, want 10", row10.Number)
	}
	if row10.Author != "alice" {
		t.Errorf("iid 10: Author = %q, want alice", row10.Author)
	}
	if row10.CommentCount != 2 {
		t.Errorf("iid 10: CommentCount = %d, want 2", row10.CommentCount)
	}

	// iid 20: enrichment succeeded.
	if !row20.Enriched {
		t.Errorf("iid 20: Enriched should be true")
	}
	if row20.ResolvedThreads != 1 || row20.UnresolvedThreads != 0 {
		t.Errorf("iid 20: threads = ✔%d/✖%d, want ✔1/✖0", row20.ResolvedThreads, row20.UnresolvedThreads)
	}
	if len(row20.HumanCommenters) != 1 || row20.HumanCommenters[0] != "carol" {
		t.Errorf("iid 20: HumanCommenters = %v, want [carol]", row20.HumanCommenters)
	}
}

// Confirm we no longer pass the deprecated --opened flag.
func TestAdapter_List_OmitsDeprecatedOpenedFlag(t *testing.T) {
	run, calls := scriptedRunner(t, []scriptResult{{out: []byte(`[]`)}})
	a := New(run)
	if _, err := a.List(context.Background(), provider.RepoCoord{Host: "gitlab.com", Owner: "g", Name: "p"}); err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, arg := range (*calls)[0].args {
		if arg == "--opened" {
			t.Fatal("--opened is deprecated and should no longer be passed; glab uses 'open' as the default")
		}
	}
}
