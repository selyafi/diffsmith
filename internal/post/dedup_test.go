package post

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/review"
)

func TestFilterDuplicates_SkipsExistingFileLinePairs(t *testing.T) {
	existing := map[string]bool{
		dedupKey("a.go", 11): true,
		dedupKey("b.go", 22): true,
	}
	findings := []review.Finding{
		{File: "a.go", Line: 11, SuggestedComment: "already there"},
		{File: "a.go", Line: 12, SuggestedComment: "new"},
		{File: "b.go", Line: 22, SuggestedComment: "also already"},
		{File: "c.go", Line: 1, SuggestedComment: "totally new"},
	}
	toPost, skipped := filterDuplicates(findings, existing)
	if len(toPost) != 2 {
		t.Fatalf("toPost = %d, want 2; got %+v", len(toPost), toPost)
	}
	if toPost[0].File != "a.go" || toPost[0].Line != 12 {
		t.Errorf("toPost[0] = %s:%d, want a.go:12", toPost[0].File, toPost[0].Line)
	}
	if toPost[1].File != "c.go" || toPost[1].Line != 1 {
		t.Errorf("toPost[1] = %s:%d, want c.go:1", toPost[1].File, toPost[1].Line)
	}
	if len(skipped) != 2 {
		t.Errorf("skipped = %d, want 2", len(skipped))
	}
}

func TestFilterDuplicates_EmptyExistingPostsAll(t *testing.T) {
	findings := []review.Finding{
		{File: "a.go", Line: 1},
		{File: "b.go", Line: 2},
	}
	toPost, skipped := filterDuplicates(findings, map[string]bool{})
	if len(toPost) != 2 {
		t.Errorf("toPost = %d, want 2", len(toPost))
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %d, want 0", len(skipped))
	}
}

func TestFetchExistingGitLabKeys_SkipsNonDiffsmithThreads(t *testing.T) {
	// Two discussions: one diffsmith, one from a human reviewer.
	// Only the diffsmith one should be recorded as an existing key.
	raw := []byte(`[
		{"notes":[{
			"body":"<!-- diffsmith --> — high, model: codex\n\nstuff",
			"position":{"new_path":"a.go","new_line":11}
		}]},
		{"notes":[{
			"body":"LGTM — could you also add a test?",
			"position":{"new_path":"b.go","new_line":22}
		}]},
		{"notes":[{
			"body":"<!-- diffsmith --> — low\n\nstuff",
			"position":{"new_path":"c.go","new_line":33}
		}]}
	]`)
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		if name != "glab" {
			t.Errorf("expected glab, got %s", name)
		}
		// Endpoint should target the correct project + MR.
		if len(args) < 2 || !strings.Contains(args[1], "projects/g%2Fp/merge_requests/3650/discussions") {
			t.Errorf("unexpected endpoint args: %v", args)
		}
		return raw, nil
	}
	target := review.ReviewTarget{Host: review.HostGitLab, Owner: "g", Repo: "p", Number: 3650}
	got, err := fetchExistingGitLabKeys(context.Background(), run, target)
	if err != nil {
		t.Fatalf("fetchExistingGitLabKeys: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (two diffsmith-signed threads)", len(got))
	}
	if !got[dedupKey("a.go", 11)] {
		t.Error("missing a.go:11")
	}
	if !got[dedupKey("c.go", 33)] {
		t.Error("missing c.go:33")
	}
	if got[dedupKey("b.go", 22)] {
		t.Error("b.go:22 is a human comment; must not be in dedup set")
	}
}

// TestFetchExistingGitLabKeys_RenamedFile_KeysByPostImagePath is the
// diffsmith-jc5 regression: GitLab dedup is keyed on post-image path
// because that's what review.Finding.File carries. The implicit
// contract is that GitLab's API rewrites Position.NewPath on threads
// anchored to a pre-rename revision so it always reflects the
// post-image path — only then does the key built here match the key
// built from a fresh finding on the renamed file.
//
// This test pins the happy path: when the GitLab API returns
// Position.NewPath == post-image-path, dedup catches the rename
// correctly. If a future GitLab API change (or a self-hosted
// provider that doesn't rewrite NewPath) breaks the assumption,
// this test still passes but the production dedup silently misses
// renamed files — which is why a companion failure-mode case is
// documented in the body.
func TestFetchExistingGitLabKeys_RenamedFile_KeysByPostImagePath(t *testing.T) {
	// Scenario: a file was renamed from old_name.go to renamed.go
	// between two MR pushes. A previous diffsmith run posted an
	// inline thread on the file. GitLab's API now returns the
	// thread with Position.NewPath rewritten to the post-image path.
	raw := []byte(`[
		{"notes":[{
			"body":"<!-- diffsmith --> existing",
			"position":{"new_path":"internal/store/renamed.go","new_line":11}
		}]}
	]`)
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return raw, nil
	}
	got, err := fetchExistingGitLabKeys(context.Background(), run,
		review.ReviewTarget{Host: review.HostGitLab, Owner: "g", Repo: "p", Number: 1})
	if err != nil {
		t.Fatalf("fetchExistingGitLabKeys: %v", err)
	}
	// Key must use the post-image path so filterDuplicates can match
	// a new finding whose File field carries the post-image path
	// (the only path the model knows about).
	if !got[dedupKey("internal/store/renamed.go", 11)] {
		t.Errorf("rename dedup key missing for renamed.go:11; got keys: %v", got)
	}

	// Failure-mode documentation: if a provider ever returns
	// Position.NewPath as the OLD path (pre-rename), the key set
	// would be missing the post-image-path entry and dedup would
	// silently re-post on every run. The following case is the
	// canary the maintainer should watch in production logs:
	rawHostile := []byte(`[
		{"notes":[{
			"body":"<!-- diffsmith --> existing",
			"position":{"new_path":"internal/store/old_name.go","new_line":11}
		}]}
	]`)
	runHostile := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return rawHostile, nil
	}
	hostileKeys, _ := fetchExistingGitLabKeys(context.Background(), runHostile,
		review.ReviewTarget{Host: review.HostGitLab, Owner: "g", Repo: "p", Number: 1})
	// Documented current behavior: if Position.NewPath is the OLD
	// path, dedup keys it by OLD path and silently misses the rename.
	// The test pins this so any future change to the matcher (e.g. a
	// reverse-lookup against the OldPaths map at fetch time) is a
	// deliberate, test-changing decision rather than an accident.
	if hostileKeys[dedupKey("internal/store/renamed.go", 11)] {
		t.Errorf("current dedup keys by Position.NewPath verbatim; got an unexpected renamed.go:11 key from an old-path NewPath — matcher behavior changed")
	}
	if !hostileKeys[dedupKey("internal/store/old_name.go", 11)] {
		t.Errorf("documenting current behavior: if Position.NewPath is the pre-rename path, dedup keys by that — expected old_name.go:11 in the key set")
	}
}

func TestFetchExistingGitLabKeys_IgnoresThreadsWithoutPosition(t *testing.T) {
	// Top-level diffsmith notes (no position) shouldn't generate keys
	// since they aren't anchored to a file:line.
	raw := []byte(`[
		{"notes":[{
			"body":"<!-- diffsmith --> — top-level note",
			"position":null
		}]}
	]`)
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return raw, nil
	}
	got, _ := fetchExistingGitLabKeys(context.Background(), run, review.ReviewTarget{Host: review.HostGitLab})
	if len(got) != 0 {
		t.Errorf("len = %d, want 0; positionless threads should be ignored", len(got))
	}
}

func TestFetchExistingGitLabKeys_PropagatesFetchError(t *testing.T) {
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("glab: 401 unauthorized")
	}
	_, err := fetchExistingGitLabKeys(context.Background(), run, review.ReviewTarget{Host: review.HostGitLab, Owner: "g", Repo: "p", Number: 1})
	if err == nil {
		t.Fatal("expected error to propagate")
	}
	if !strings.Contains(err.Error(), "fetch existing gitlab discussions") {
		t.Errorf("error should be wrapped with context; got: %v", err)
	}
}

func TestFetchExistingGitHubKeys_FiltersBySignature(t *testing.T) {
	raw := []byte(`[
		{"body":"<!-- diffsmith --> — high","path":"a.go","line":11},
		{"body":"Looks good to me","path":"b.go","line":22},
		{"body":"<!-- diffsmith --> — low","path":"c.go","line":33}
	]`)
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		if name != "gh" {
			t.Errorf("expected gh, got %s", name)
		}
		if len(args) < 2 || !strings.Contains(args[1], "repos/o/r/pulls/7/comments") {
			t.Errorf("unexpected endpoint args: %v", args)
		}
		return raw, nil
	}
	target := review.ReviewTarget{Host: review.HostGitHub, Owner: "o", Repo: "r", Number: 7}
	got, err := fetchExistingGitHubKeys(context.Background(), run, target)
	if err != nil {
		t.Fatalf("fetchExistingGitHubKeys: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[dedupKey("a.go", 11)] || !got[dedupKey("c.go", 33)] {
		t.Errorf("missing diffsmith-authored keys; got: %v", got)
	}
}

// TestFetchExistingGitHubKeys_RecognisesBodyFromFormatBody is the
// regression that pins the GitHub format-vs-dedup contract: the comment
// body that Submit posts (via formatBody) must be recognised as a
// diffsmith thread by fetchExistingGitHubKeys when it sees it again on
// the next run. Without this, a previously-posted comment looks like
// human content and dedup re-posts duplicates.
func TestFetchExistingGitHubKeys_RecognisesBodyFromFormatBody(t *testing.T) {
	f := review.Finding{
		File:             "a.go",
		Line:             11,
		Severity:         review.SeverityHigh,
		Title:            "Boom",
		SuggestedComment: "Don't do that.",
	}
	body := formatBody(f)

	// Build the gh-api response shape using the body that Submit would
	// actually have posted on the previous run.
	resp, err := json.Marshal([]ghReviewComment{{
		Body: body,
		Path: f.File,
		Line: f.Line,
	}})
	if err != nil {
		t.Fatalf("marshal scripted gh response: %v", err)
	}
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return resp, nil
	}
	got, err := fetchExistingGitHubKeys(context.Background(), run,
		review.ReviewTarget{Host: review.HostGitHub, Owner: "o", Repo: "r", Number: 7})
	if err != nil {
		t.Fatalf("fetchExistingGitHubKeys: %v", err)
	}
	if !got[dedupKey(f.File, f.Line)] {
		t.Errorf("a real formatBody-produced comment was NOT recognised as a diffsmith thread\nBODY:\n%s\nKEYS: %v",
			body, got)
	}
}

// TestPoster_Submit_GitHubDedupSkipsExistingThreads is the end-to-end
// proof: when a previously-posted diffsmith comment exists at a.go:1,
// rerunning Submit with findings at a.go:1 and b.go:2 must NOT add a
// thread for a.go:1 — only b.go:2 — and must print the skip summary.
func TestPoster_Submit_GitHubDedupSkipsExistingThreads(t *testing.T) {
	// Build the existing-comment body from the real formatter so this
	// test breaks if anyone ever decouples the dedup contract again.
	existing := formatBody(review.Finding{
		File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "Old", SuggestedComment: "old comment",
	})
	dedupResp, err := json.Marshal([]ghReviewComment{{Body: existing, Path: "a.go", Line: 1}})
	if err != nil {
		t.Fatalf("marshal dedup response: %v", err)
	}

	results := []ghResult{
		{out: dedupResp}, // 1) dedup fetch
		{out: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_X"}}}}`)},                       // 2) resolve
		{out: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_X"}}}}`)},      // 3) begin
		{out: []byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`)},                      // 4) addThread for b.go:2
		{out: []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"url":"https://github.com/o/r/pull/7#pullrequestreview-1"}}}}`)}, // 5) submit
	}
	calls := scriptedGHResults(t, results)

	var buf bytes.Buffer
	p := &Poster{Out: &buf} // Repost=false: dedup must run
	target := review.ReviewTarget{Host: review.HostGitHub, Owner: "o", Repo: "r", Number: 7, HeadSHA: "sha"}
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "Dupe", SuggestedComment: "dupe"},
		{File: "b.go", Line: 2, Severity: review.SeverityLow, Title: "New", SuggestedComment: "new"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if got, want := len(*calls), 5; got != want {
		t.Fatalf("call count: got %d, want %d (dedup + resolve + begin + 1 thread + submit)", got, want)
	}
	// Call 0 is the dedup fetch on the comments endpoint.
	if !strings.Contains((*calls)[0].args[1], "repos/o/r/pulls/7/comments") {
		t.Errorf("call 0 should be the dedup fetch; got args[1]=%q", (*calls)[0].args[1])
	}
	if !contains((*calls)[0].args, "--paginate") {
		t.Error("dedup fetch must use --paginate to cover all comment pages")
	}
	// Call 3 is the only addThread — and it must be for b.go:2, not a.go:1.
	threadStdin := (*calls)[3].stdin
	if !strings.Contains(threadStdin, "addPullRequestReviewThread") {
		t.Errorf("call 3 should be addThread; stdin:\n%s", threadStdin)
	}
	if !strings.Contains(threadStdin, `"path":"b.go"`) || !strings.Contains(threadStdin, `"line":2`) {
		t.Errorf("addThread should be for b.go:2, not a.go:1; stdin:\n%s", threadStdin)
	}
	if strings.Contains(threadStdin, `"path":"a.go"`) {
		t.Errorf("addThread for a.go:1 must be filtered out by dedup; stdin:\n%s", threadStdin)
	}

	out := buf.String()
	if !strings.Contains(out, "Skipping 1 finding(s) already posted") {
		t.Errorf("expected dedup summary; got:\n%s", out)
	}
	if !strings.Contains(out, "a.go:1") {
		t.Errorf("dedup summary should name the skipped finding; got:\n%s", out)
	}
	if !strings.Contains(out, "pullrequestreview-1") {
		t.Errorf("review URL should still print for the surviving finding; got:\n%s", out)
	}
}

// TestPoster_Submit_GitLabDedupSkipsExistingThreads is the end-to-end
// proof that the dedup wire-up is correct: when an existing diffsmith
// thread is present at a.go:1, a new finding at a.go:1 must NOT
// produce a POST, and the user must see the "Skipping N finding(s)…"
// summary. The new finding at b.go:2 still posts.
func TestPoster_Submit_GitLabDedupSkipsExistingThreads(t *testing.T) {
	// First glab call: dedup fetch returns one existing thread at a.go:1.
	// Second call: inline POST for the surviving b.go:2 finding.
	calls := scriptedGlab(t, []ghResult{
		{out: []byte(`[{"notes":[{"body":"<!-- diffsmith --> — high","position":{"new_path":"a.go","new_line":1}}]}]`)},
		{out: []byte(`{"id":"def"}`)},
	})

	var buf bytes.Buffer
	p := &Poster{Out: &buf} // Repost=false: dedup must run
	target := review.ReviewTarget{
		Host:     review.HostGitLab,
		Owner:    "g",
		Repo:     "p",
		Number:   1,
		HeadSHA:  "h",
		BaseSHA:  "b",
		StartSHA: "s",
	}
	findings := []review.Finding{
		{File: "a.go", Line: 1, SuggestedComment: "dupe"},
		{File: "b.go", Line: 2, SuggestedComment: "new"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if len(*calls) != 2 {
		t.Fatalf("expected 2 glab calls (dedup-fetch + 1 inline POST); got %d", len(*calls))
	}
	// First call is the dedup-fetch.
	if (*calls)[0].args[1] != "projects/g%2Fp/merge_requests/1/discussions" {
		t.Errorf("first call should be the dedup fetch; got args[1] = %q", (*calls)[0].args[1])
	}
	if !contains((*calls)[0].args, "--paginate") {
		t.Error("dedup fetch must use --paginate to cover all discussion pages")
	}
	// Second call is the inline POST for the b.go:2 finding (not a.go:1).
	second := (*calls)[1]
	if !contains(second.args, "--method") || !contains(second.args, "POST") {
		t.Errorf("second call should be a POST; got %v", second.args)
	}
	if !strings.Contains(second.stdin, `"new_path":"b.go"`) || !strings.Contains(second.stdin, `"new_line":2`) {
		t.Errorf("POST should be for b.go:2, not a.go:1; stdin: %s", second.stdin)
	}

	out := buf.String()
	if !strings.Contains(out, "Skipping 1 finding(s) already posted") {
		t.Errorf("expected dedup summary; got: %s", out)
	}
	if !strings.Contains(out, "a.go:1") {
		t.Errorf("dedup summary should name the skipped finding; got: %s", out)
	}
	if !strings.Contains(out, "Posted 1 inline thread(s)") {
		t.Errorf("expected truthful posted-count line; got: %s", out)
	}
}

// TestFetchExistingGitHubKeys_MultiPagePaginateOutput is the
// diffsmith-kjk regression: gh --paginate emits one JSON array PER
// PAGE, back to back. Unmarshalling that as a single array fails on
// >100-comment PRs, which silently disabled dedup exactly where it
// matters. Keys from every page must land.
func TestFetchExistingGitHubKeys_MultiPagePaginateOutput(t *testing.T) {
	raw := []byte(`[{"body":"<!-- diffsmith --> one","path":"a.go","line":11}]` + "\n" +
		`[{"body":"<!-- diffsmith --> two","path":"b.go","line":22}]`)
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return raw, nil
	}
	target := review.ReviewTarget{Host: review.HostGitHub, Owner: "o", Repo: "r", Number: 7}
	got, err := fetchExistingGitHubKeys(context.Background(), run, target)
	if err != nil {
		t.Fatalf("multi-page paginate output must parse: %v", err)
	}
	if !got[dedupKey("a.go", 11)] || !got[dedupKey("b.go", 22)] {
		t.Errorf("keys from both pages must land; got %v", got)
	}
}

// TestFetchExistingGitLabKeys_MultiPagePaginateOutput: same
// diffsmith-kjk regression for the glab fetcher.
func TestFetchExistingGitLabKeys_MultiPagePaginateOutput(t *testing.T) {
	raw := []byte(`[{"notes":[{"body":"<!-- diffsmith --> one","position":{"new_path":"a.go","new_line":11}}]}]` + "\n" +
		`[{"notes":[{"body":"<!-- diffsmith --> two","position":{"new_path":"b.go","new_line":22}}]}]`)
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		return raw, nil
	}
	target := review.ReviewTarget{Host: review.HostGitLab, Owner: "o", Repo: "r", Number: 7}
	got, err := fetchExistingGitLabKeys(context.Background(), run, target)
	if err != nil {
		t.Fatalf("multi-page paginate output must parse: %v", err)
	}
	if !got[dedupKey("a.go", 11)] || !got[dedupKey("b.go", 22)] {
		t.Errorf("keys from both pages must land; got %v", got)
	}
}

// TestFetchExistingGitLabKeys_PinsHostnameFromTargetURL is
// diffsmith-1bk: glab resolves bare api calls against its configured
// default host, which can be a different instance than the MR's. The
// dedup fetch must pin --hostname from the target URL.
func TestFetchExistingGitLabKeys_PinsHostnameFromTargetURL(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
		gotArgs = append([]string(nil), args...)
		return []byte(`[]`), nil
	}
	target := review.ReviewTarget{
		Host: review.HostGitLab, Owner: "o", Repo: "r", Number: 7,
		URL: "https://gitlab.example.com/o/r/-/merge_requests/7",
	}
	if _, err := fetchExistingGitLabKeys(context.Background(), run, target); err != nil {
		t.Fatalf("fetchExistingGitLabKeys: %v", err)
	}
	joined := strings.Join(gotArgs, " ")
	if !strings.Contains(joined, "--hostname gitlab.example.com") {
		t.Errorf("glab dedup fetch must pin --hostname from target URL; got args %v", gotArgs)
	}
}
