package githubgh

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
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
	wantView := []string{"pr", "view", "https://github.com/owner/repo/pull/42", "--json", "title,author,body,headRefName,headRefOid,baseRefName,url"}
	if (*calls)[0].name != "gh" || !reflect.DeepEqual((*calls)[0].args, wantView) {
		t.Errorf("view call: got %s %v, want gh %v", (*calls)[0].name, (*calls)[0].args, wantView)
	}
	// No --patch: that flag returns a per-commit format-patch mailbox
	// series (duplicate file sections, per-commit-tree line numbers
	// that misanchor the line oracle on multi-commit PRs). The default
	// response is the consolidated PR diff. diffsmith-a4o.
	wantDiff := []string{"pr", "diff", "https://github.com/owner/repo/pull/42", "--color", "never"}
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

// TestAdapterFetch_PopulatesDescription is diffsmith-144: Fetch must
// capture the PR body into ReviewInput.Description and request it in the
// gh pr view --json field set (free — same call).
func TestAdapterFetch_PopulatesDescription(t *testing.T) {
	metaJSON := []byte(`{
		"title": "Tighten token parsing",
		"author": {"login": "alice"},
		"headRefName": "feat/parse",
		"headRefOid": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"baseRefName": "main",
		"url": "https://github.com/owner/repo/pull/42",
		"body": "Implements the widget.\n\nCloses #7"
	}`)
	rawDiff := readDiffFixture(t, "modified_simple.diff")
	run, calls := scriptedRunner(t, [][]byte{metaJSON, rawDiff})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if want := "Implements the widget.\n\nCloses #7"; input.Description != want {
		t.Errorf("Description = %q, want %q", input.Description, want)
	}
	// Production must actually request the body field.
	if !strings.Contains(strings.Join((*calls)[0].args, " "), "body") {
		t.Errorf("gh pr view must request 'body' in --json; got args %v", (*calls)[0].args)
	}
}

// linkedIssuesTarget is the PR target used by the FetchLinkedIssues tests.
func linkedIssuesTarget() review.ReviewTarget {
	return review.ReviewTarget{
		Host:   review.HostGitHub,
		URL:    "https://github.com/owner/repo/pull/42",
		Owner:  "owner",
		Repo:   "repo",
		Number: 42,
	}
}

// TestFetchLinkedIssues_ResolvesClosingRefs is diffsmith-144: the GitHub
// adapter resolves the issues a PR closes (closingIssuesReferences) and
// fetches each issue's title/body via `gh issue view`.
func TestFetchLinkedIssues_ResolvesClosingRefs(t *testing.T) {
	refsJSON := []byte(`{"closingIssuesReferences":[
		{"number":7,"url":"https://github.com/owner/repo/issues/7","repository":{"name":"repo","owner":{"login":"owner"}}},
		{"number":9,"url":"https://github.com/owner/repo/issues/9","repository":{"name":"repo","owner":{"login":"owner"}}}
	]}`)
	issue7 := []byte(`{"number":7,"title":"Widget","body":"AC: it widgets","url":"https://github.com/owner/repo/issues/7"}`)
	issue9 := []byte(`{"number":9,"title":"Gadget","body":"AC: it gadgets","url":"https://github.com/owner/repo/issues/9"}`)
	run, calls := scriptedRunner(t, [][]byte{refsJSON, issue7, issue9})
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("FetchLinkedIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("want 2 issues, got %d", len(issues))
	}
	if issues[0].Number != 7 || issues[0].Title != "Widget" || !strings.Contains(issues[0].Body, "widgets") {
		t.Errorf("issue[0] decoded wrong: %+v", issues[0])
	}
	if len(notes) != 0 {
		t.Errorf("no notes expected on clean resolution; got %v", notes)
	}
	// First call resolves the closing refs; subsequent calls read issues.
	wantRefsCall := []string{"pr", "view", "https://github.com/owner/repo/pull/42", "--json", "closingIssuesReferences"}
	if (*calls)[0].name != "gh" || !reflect.DeepEqual((*calls)[0].args, wantRefsCall) {
		t.Errorf("first call (closingIssuesReferences): got %s %v, want gh %v", (*calls)[0].name, (*calls)[0].args, wantRefsCall)
	}
	// The issue-view call must request exactly the four JSON fields the adapter decodes.
	wantIssueView := []string{"issue", "view", "7", "--repo", "owner/repo", "--json", "number,title,body,url"}
	if (*calls)[1].name != "gh" || !reflect.DeepEqual((*calls)[1].args, wantIssueView) {
		t.Errorf("second call (issue view): got %s %v, want gh %v", (*calls)[1].name, (*calls)[1].args, wantIssueView)
	}
}

// TestFetchLinkedIssues_CapsRefsToMax: when a PR closes more than
// review.MaxLinkedIssues issues, only the first review.MaxLinkedIssues
// are fetched (no excess gh issue view calls) and a note is appended
// describing how many were skipped.
func TestFetchLinkedIssues_CapsRefsToMax(t *testing.T) {
	// Build a closingIssuesReferences JSON with MaxLinkedIssues+1 refs.
	n := review.MaxLinkedIssues + 1
	refsEntries := make([]string, n)
	for i := 0; i < n; i++ {
		num := i + 1
		refsEntries[i] = fmt.Sprintf(`{"number":%d,"url":"https://github.com/owner/repo/issues/%d","repository":{"name":"repo","owner":{"login":"owner"}}}`, num, num)
	}
	refsJSON := []byte(`{"closingIssuesReferences":[` + strings.Join(refsEntries, ",") + `]}`)

	// Provide canned responses for the refs call + exactly MaxLinkedIssues issue-view calls.
	responses := make([]scriptedResponse, 1+review.MaxLinkedIssues)
	responses[0] = scriptedResponse{out: refsJSON}
	for i := 0; i < review.MaxLinkedIssues; i++ {
		num := i + 1
		responses[1+i] = scriptedResponse{out: []byte(fmt.Sprintf(
			`{"number":%d,"title":"Issue %d","body":"body %d","url":"https://github.com/owner/repo/issues/%d"}`,
			num, num, num, num,
		))}
	}

	run, calls := scriptedRunnerSeq(t, responses)
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("FetchLinkedIssues: %v", err)
	}

	// (i) Exactly MaxLinkedIssues issues returned.
	if got := len(issues); got != review.MaxLinkedIssues {
		t.Errorf("want %d issues (cap), got %d", review.MaxLinkedIssues, got)
	}

	// (ii) A cap note is present.
	if len(notes) == 0 {
		t.Fatal("want a cap note; got no notes")
	}
	found := false
	for _, note := range notes {
		if strings.Contains(note, "not fetched") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cap note must describe skipped refs; got notes: %v", notes)
	}

	// (iii) Only 1 + review.MaxLinkedIssues runner calls: the closingIssuesReferences
	// call plus one issue-view per kept ref (no calls for the excess ref).
	wantCalls := 1 + review.MaxLinkedIssues
	if got := len(*calls); got != wantCalls {
		t.Errorf("want %d runner calls (1 refs + %d issue-views), got %d — excess refs were fetched",
			wantCalls, review.MaxLinkedIssues, got)
	}
}

// TestFetchLinkedIssues_DropsFailingIssueWithNote: a per-issue fetch
// failure is non-fatal — the issue is dropped and a note is surfaced, the
// rest still resolve. (No silent fallback.)
func TestFetchLinkedIssues_DropsFailingIssueWithNote(t *testing.T) {
	refsJSON := []byte(`{"closingIssuesReferences":[
		{"number":7,"url":"u7","repository":{"name":"repo","owner":{"login":"owner"}}},
		{"number":9,"url":"u9","repository":{"name":"repo","owner":{"login":"owner"}}}
	]}`)
	issue7 := []byte(`{"number":7,"title":"Widget","body":"b","url":"u7"}`)
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: refsJSON},
		{out: issue7},
		{err: errors.New("HTTP 404: Not Found")},
	})
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("per-issue failure must be non-fatal; got err %v", err)
	}
	if len(issues) != 1 || issues[0].Number != 7 {
		t.Fatalf("want only issue #7 surviving; got %+v", issues)
	}
	if len(notes) == 0 {
		t.Error("a dropped issue must produce a surfaced note")
	}
}

// TestFetchLinkedIssues_TotalFailureReturnsError: if the closing-refs
// query itself fails, that's a total failure surfaced as err (the caller
// turns it into one note and proceeds with no criteria).
func TestFetchLinkedIssues_TotalFailureReturnsError(t *testing.T) {
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{{err: errors.New("gh exploded")}})
	a := New(run)

	_, _, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err == nil {
		t.Fatal("a failed closingIssuesReferences query must return err")
	}
}

// TestFetchLinkedIssues_NoRefsIsEmpty: a PR that closes no issues yields
// empty criteria with no error and no extra calls.
func TestFetchLinkedIssues_NoRefsIsEmpty(t *testing.T) {
	run, calls := scriptedRunner(t, [][]byte{[]byte(`{"closingIssuesReferences":[]}`)})
	a := New(run)

	issues, notes, err := a.FetchLinkedIssues(context.Background(), linkedIssuesTarget())
	if err != nil {
		t.Fatalf("no refs should not error: %v", err)
	}
	if len(issues) != 0 || len(notes) != 0 {
		t.Errorf("expected no issues/notes; got %d issues, notes %v", len(issues), notes)
	}
	if len(*calls) != 1 {
		t.Errorf("no issues to fetch → only the refs call; got %d calls", len(*calls))
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

// TestListEnrichesFromGraphQL verifies that List issues a single `gh api
// graphql` call and correctly derives CommentCount, thread counts, and
// HumanCommenters (bot + author excluded).
func TestListEnrichesFromGraphQL(t *testing.T) {
	resp := []byte(`{"data":{"search":{"nodes":[
		{"number":269,"title":"wire systests","url":"https://github.com/o/r/pull/269",
		 "updatedAt":"2026-06-04T00:00:00Z","isDraft":false,"author":{"login":"shelyafi","__typename":"User"},
		 "comments":{"totalCount":8},
		 "reviewThreads":{"nodes":[
		   {"isResolved":true,"comments":{"totalCount":1,"nodes":[{"author":{"login":"Balvajs","__typename":"User"}}]}},
		   {"isResolved":false,"comments":{"totalCount":1,"nodes":[{"author":{"login":"copilot","__typename":"Bot"}}]}},
		   {"isResolved":false,"comments":{"totalCount":1,"nodes":[{"author":{"login":"Balvajs","__typename":"User"}}]}}
		 ]},
		 "reviews":{"nodes":[{"author":{"login":"Balvajs","__typename":"User"}},{"author":{"login":"shelyafi","__typename":"User"}}]}}
	]}}}`)
	run, calls := scriptedRunner(t, [][]byte{resp})
	a := New(run)

	got, err := a.List(context.Background(), provider.RepoCoord{Owner: "o", Name: "r"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0].args[0] != "api" || (*calls)[0].args[1] != "graphql" {
		t.Fatalf("expected one `gh api graphql` call, got %v", *calls)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 summary, got %d", len(got))
	}
	s := got[0]
	if s.Number != 269 || s.URL == "" || !s.Enriched {
		t.Errorf("base/enriched wrong: %+v", s)
	}
	if s.CommentCount != 11 { // 8 conversation + 3 inline thread comments
		t.Errorf("CommentCount = %d, want 11", s.CommentCount)
	}
	if s.ResolvedThreads != 1 || s.UnresolvedThreads != 2 {
		t.Errorf("threads = ✔%d/✖%d, want ✔1/✖2", s.ResolvedThreads, s.UnresolvedThreads)
	}
	if len(s.HumanCommenters) != 1 || s.HumanCommenters[0] != "Balvajs" {
		t.Errorf("HumanCommenters = %v, want [Balvajs] (bot + author excluded)", s.HumanCommenters)
	}
}

// TestAdapter_List_Success migrated from the old gh-pr-list shape to GraphQL.
// Verifies that two PRs are correctly mapped from the GraphQL response.
func TestAdapter_List_Success(t *testing.T) {
	canned := []byte(`{"data":{"search":{"nodes":[
		{"number":13491,"title":"fix: handle empty PR descriptions","author":{"login":"alice","__typename":"User"},"updatedAt":"2026-05-20T12:00:00Z","url":"https://github.com/cli/cli/pull/13491","isDraft":false,
		 "comments":{"totalCount":0},"reviewThreads":{"nodes":[]},"reviews":{"nodes":[]}},
		{"number":13485,"title":"docs: clarify auth","author":{"login":"bob","__typename":"User"},"updatedAt":"2026-05-18T09:30:00Z","url":"https://github.com/cli/cli/pull/13485","isDraft":true,
		 "comments":{"totalCount":0},"reviewThreads":{"nodes":[]},"reviews":{"nodes":[]}}
	]}}}`)
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
	if args := (*calls)[0].args; args[0] != "api" || args[1] != "graphql" {
		t.Errorf("expected gh api graphql, got %v", args)
	}
}

func TestAdapter_List_Empty(t *testing.T) {
	run, _ := scriptedRunner(t, [][]byte{[]byte(`{"data":{"search":{"nodes":[]}}}`)})
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
	run, _ := scriptedRunner(t, [][]byte{[]byte(`not-json-at-all`)})
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
	// One reviewable file, one too-large file. A genuinely size-capped
	// entry has a null patch WITH a non-zero change count — null patch
	// with changes:0 means binary/rename/mode-only and is synthesized
	// as metadata instead (diffsmith-oks).
	filesJSON := []byte(`[
		{"filename":"small.go","status":"modified","changes":2,"patch":"@@ -1,1 +1,1 @@\n-old\n+new"},
		{"filename":"huge.go","status":"modified","changes":80000,"patch":null}
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

// TestAdapterFetch_FilesAPIFallbackMultiPage is the diffsmith-kjk
// regression on the diffsmith-5n4 fallback: gh --paginate emits one
// JSON array PER PAGE, so PRs with >100 changed files (the norm for
// over-20K-line PRs) produced concatenated arrays the old single
// json.Unmarshal could not parse — the fallback failed on exactly the
// PRs it exists for. Files from every page must reach the diff.
func TestAdapterFetch_FilesAPIFallbackMultiPage(t *testing.T) {
	metaJSON := []byte(`{
		"title": "Big PR", "author": {"login": "alice"},
		"headRefName": "feat/big",
		"headRefOid": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"baseRefName": "main", "url": "https://github.com/owner/repo/pull/42"
	}`)
	diffErr := errors.New("gh: exit 1: could not find pull request diff: HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)")
	pagedFiles := []byte(`[{"filename":"a/first.go","status":"modified","patch":"@@ -1,1 +1,1 @@\n-var X = 1\n+var X = 2"}]` + "\n" +
		`[{"filename":"b/second.go","status":"added","patch":"@@ -0,0 +1,1 @@\n+const Y = 1"}]`)

	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON},
		{err: diffErr},
		{out: pagedFiles},
	})
	a := New(run)

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("multi-page files fallback must succeed; got: %v", err)
	}
	var paths []string
	for _, f := range input.Files {
		paths = append(paths, f.Path)
	}
	for _, want := range []string{"a/first.go", "b/second.go"} {
		found := false
		for _, p := range paths {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("file %s from paginated response missing; got %v", want, paths)
		}
	}
}

// TestAdapterFetch_FilesAPINilPatchKinds is the diffsmith-oks
// regression: GitHub returns no `patch` for binary files, pure renames,
// and mode-only changes — not just for size-capped text. The fallback
// dropped all of them with a misleading "exceeds per-file size cap"
// warning, so unlike the normal `gh pr diff` path they vanished from
// the prompt's metadata section. Zero-change nil-patch entries must be
// synthesized as metadata-only diff segments; only changes>0 entries
// are genuinely capped and skipped.
func TestAdapterFetch_FilesAPINilPatchKinds(t *testing.T) {
	metaJSON := []byte(`{
		"title": "Big PR", "author": {"login": "alice"},
		"headRefName": "feat/big",
		"headRefOid": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"baseRefName": "main", "url": "https://github.com/owner/repo/pull/42"
	}`)
	diffErr := errors.New("gh: exit 1: HTTP 406: Sorry, the diff exceeded the maximum number of lines (20000)")
	filesJSON := []byte(`[
		{"filename":"normal.go","status":"modified","changes":2,"patch":"@@ -1,1 +1,1 @@\n-a\n+b"},
		{"filename":"logo.png","status":"modified","changes":0},
		{"filename":"renamed.go","previous_filename":"orig.go","status":"renamed","changes":0},
		{"filename":"huge.gen.go","status":"modified","changes":50000}
	]`)
	var warnBuf strings.Builder
	run, _ := scriptedRunnerSeq(t, []scriptedResponse{
		{out: metaJSON}, {err: diffErr}, {out: filesJSON},
	})
	a := New(run)
	a.warn = &warnBuf

	input, err := a.Fetch(context.Background(), "https://github.com/owner/repo/pull/42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	kinds := map[string]diff.FileKind{}
	for _, f := range input.Files {
		kinds[f.Path] = f.Kind
	}
	if kinds["logo.png"] != diff.FileBinary {
		t.Errorf("binary nil-patch entry must surface as FileBinary metadata; got %v (files: %v)", kinds["logo.png"], kinds)
	}
	if kinds["renamed.go"] != diff.FilePureRename {
		t.Errorf("pure-rename nil-patch entry must surface as FilePureRename; got %v", kinds["renamed.go"])
	}
	if _, present := kinds["huge.gen.go"]; present {
		t.Error("size-capped entry must still be skipped, not synthesized")
	}
	warn := warnBuf.String()
	if !strings.Contains(warn, "huge.gen.go") || !strings.Contains(warn, "size cap") {
		t.Errorf("size-cap warning must name the capped file; got %q", warn)
	}
	if strings.Contains(warn, "logo.png") || strings.Contains(warn, "renamed.go") {
		t.Errorf("metadata-only files must not be warned about as size-capped; got %q", warn)
	}
}
