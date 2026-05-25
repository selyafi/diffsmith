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

// recordedGHCall captures one runGH invocation made during Submit so the
// test can assert on the sequence and the stdin body sent to gh.
type recordedGHCall struct {
	args  []string
	stdin string
}

// scriptedGH installs runGH with a stub that returns canned successful
// responses in order. Thin wrapper over scriptedGHResults for tests
// whose every gh call is expected to succeed.
func scriptedGH(t *testing.T, responses [][]byte) *[]recordedGHCall {
	t.Helper()
	results := make([]ghResult, len(responses))
	for i, r := range responses {
		results[i] = ghResult{out: r}
	}
	return scriptedGHResults(t, results)
}

func TestPoster_Submit_OrchestratesFourPhaseGraphQLFlow(t *testing.T) {
	responses := [][]byte{
		[]byte(`{"data":{"repository":{"pullRequest":{"id":"PR_kwDOExample"}}}}`),
		[]byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_kwDOExample"}}}}`),
		[]byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`),
		[]byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`),
		[]byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"url":"https://github.com/owner/repo/pull/42#pullrequestreview-999"}}}}`),
	}
	calls := scriptedGH(t, responses)

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{
		Owner:   "owner",
		Repo:    "repo",
		Number:  42,
		HeadSHA: "a1b2c3d4",
	}
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T1", SuggestedComment: "C1"},
		{File: "b.go", Line: 2, Severity: review.SeverityLow, Title: "T2", SuggestedComment: "C2"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if got, want := len(*calls), 5; got != want {
		t.Fatalf("call count: got %d, want %d (1 resolve + 1 begin + N threads + 1 submit)", got, want)
	}
	wantArgs := []string{"api", "graphql", "--input", "-"}
	for i, c := range *calls {
		if !sliceEqual(c.args, wantArgs) {
			t.Errorf("call %d args: got %v, want %v", i, c.args, wantArgs)
		}
	}

	// Each phase must send the right GraphQL mutation/query name.
	wantStdinSubstrings := []string{
		"pullRequest(number",         // resolve PR ID
		"addPullRequestReview(",      // begin pending review
		"addPullRequestReviewThread", // thread #1
		"addPullRequestReviewThread", // thread #2
		"submitPullRequestReview(",   // submit
	}
	for i, want := range wantStdinSubstrings {
		if !strings.Contains((*calls)[i].stdin, want) {
			t.Errorf("call %d stdin missing %q\nSTDIN:\n%s", i, want, (*calls)[i].stdin)
		}
	}

	// Thread calls must carry the capture-time HeadSHA.
	for _, i := range []int{2, 3} {
		if !strings.Contains((*calls)[i].stdin, target.HeadSHA) {
			t.Errorf("call %d (addThread) missing HeadSHA %q in stdin", i, target.HeadSHA)
		}
	}

	// Submit response URL must surface to the user via p.Out.
	if !strings.Contains(buf.String(), "pullrequestreview-999") {
		t.Errorf("Submit did not write review URL to Out\nGOT:\n%s", buf.String())
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestPoster_PrintPayload_WritesOneAnchoredJSONPerFinding(t *testing.T) {
	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{
		Owner:   "owner",
		Repo:    "repo",
		Number:  42,
		HeadSHA: "a1b2c3d4",
	}
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T1", SuggestedComment: "C1"},
		{File: "b.go", Line: 2, Severity: review.SeverityLow, Title: "T2", SuggestedComment: "C2"},
	}

	if err := p.PrintPayload(target, findings); err != nil {
		t.Fatalf("PrintPayload: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d output lines, want 2\nOUTPUT:\n%s", len(lines), buf.String())
	}
	for i, line := range lines {
		var got addThreadInput
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nLINE: %s", i, err, line)
		}
		if got.CommitOID != target.HeadSHA {
			t.Errorf("line %d: CommitOID = %q, want %q (capture-time HeadSHA)", i, got.CommitOID, target.HeadSHA)
		}
		if got.Path != findings[i].File {
			t.Errorf("line %d: Path = %q, want %q", i, got.Path, findings[i].File)
		}
		if got.Line != findings[i].Line {
			t.Errorf("line %d: Line = %d, want %d", i, got.Line, findings[i].Line)
		}
	}
}

// ghResult scripts one runGH call's outcome: either a canned response body
// (out) or a transport error (err). Used by scriptedGHResults to drive
// failure-path tests where the existing scriptedGH (which only models
// success) can't express thread-level failures.
type ghResult struct {
	out []byte
	err error
}

func scriptedGHResults(t *testing.T, results []ghResult) *[]recordedGHCall {
	t.Helper()
	prev := runGH
	t.Cleanup(func() { runGH = prev })

	var calls []recordedGHCall
	i := 0
	runGH = func(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		if name != "gh" {
			t.Errorf("unexpected command: %s", name)
		}
		var body []byte
		if stdin != nil {
			b, err := io.ReadAll(stdin)
			if err != nil {
				t.Fatalf("read stdin: %v", err)
			}
			body = b
		}
		calls = append(calls, recordedGHCall{
			args:  append([]string(nil), args...),
			stdin: string(body),
		})
		if i >= len(results) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		r := results[i]
		i++
		return r.out, r.err
	}
	return &calls
}

func TestPoster_Submit_ContinuesAfterPartialThreadFailure(t *testing.T) {
	// 3 findings; thread #2 fails. Threads #1 and #3 must still be
	// attempted, submitReview must still run (≥1 succeeded), the URL
	// must be printed, and a summary line "Posted: 2/3 ..." with the
	// failing finding's location and error must appear before the URL.
	results := []ghResult{
		{out: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_X"}}}}`)},
		{out: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_X"}}}}`)},
		{out: []byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`)},
		{err: errors.New("permission denied")},
		{out: []byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`)},
		{out: []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"url":"https://github.com/owner/repo/pull/42#pullrequestreview-999"}}}}`)},
	}
	calls := scriptedGHResults(t, results)

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "a1b2c3d4"}
	findings := []review.Finding{
		{File: "f1.go", Line: 10, Severity: review.SeverityHigh, Title: "T1", SuggestedComment: "C1"},
		{File: "f2.go", Line: 20, Severity: review.SeverityMedium, Title: "T2", SuggestedComment: "C2"},
		{File: "f3.go", Line: 30, Severity: review.SeverityLow, Title: "T3", SuggestedComment: "C3"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: expected nil on partial success, got %v", err)
	}

	// resolve + beginReview + 3 thread attempts (including the failure) + submitReview = 6 calls.
	if got, want := len(*calls), 6; got != want {
		t.Fatalf("call count: got %d, want %d (resolve+begin+3 threads+submit)", got, want)
	}

	// Thread #3 must still be attempted after #2 failed.
	if !strings.Contains((*calls)[4].stdin, "f3.go") {
		t.Errorf("call #5 should be addThread for f3.go (loop must continue past failure)\nSTDIN:\n%s", (*calls)[4].stdin)
	}

	// submitReview must run on partial success.
	if !strings.Contains((*calls)[5].stdin, "submitPullRequestReview(") {
		t.Errorf("call #6 should be submitReview on partial success\nSTDIN:\n%s", (*calls)[5].stdin)
	}

	out := buf.String()
	summaryIdx := strings.Index(out, "Posted: 2/3")
	urlIdx := strings.Index(out, "pullrequestreview-999")
	if summaryIdx < 0 {
		t.Errorf("summary 'Posted: 2/3 ...' missing from output:\n%s", out)
	}
	if urlIdx < 0 {
		t.Errorf("review URL missing from output:\n%s", out)
	}
	if summaryIdx >= 0 && urlIdx >= 0 && summaryIdx > urlIdx {
		t.Errorf("summary must appear BEFORE the URL; summaryIdx=%d urlIdx=%d\nOUTPUT:\n%s", summaryIdx, urlIdx, out)
	}
	if !strings.Contains(out, "f2.go:20") {
		t.Errorf("failed finding location (f2.go:20) missing from output:\n%s", out)
	}
	if !strings.Contains(out, "permission denied") {
		t.Errorf("failed finding error ('permission denied') missing from output:\n%s", out)
	}
}

func TestPoster_Submit_DeletesPendingReviewWhenAllThreadsFail(t *testing.T) {
	// All threads fail. submitReview MUST NOT run; deletePullRequestReview
	// MUST run to clean up the stranded pending review on GitHub. Submit
	// must return a non-nil error so callers can surface the failure.
	results := []ghResult{
		{out: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_X"}}}}`)},
		{out: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_X"}}}}`)},
		{err: errors.New("err1")},
		{err: errors.New("err2")},
		{out: []byte(`{"data":{"deletePullRequestReview":{"pullRequestReview":{"id":"PRR_X"}}}}`)},
	}
	calls := scriptedGHResults(t, results)

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "sha"}
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T1", SuggestedComment: "C1"},
		{File: "b.go", Line: 2, Severity: review.SeverityLow, Title: "T2", SuggestedComment: "C2"},
	}

	err := p.Submit(context.Background(), target, findings)
	if err == nil {
		t.Fatal("Submit: expected error when all threads fail, got nil")
	}

	// resolve + beginReview + 2 thread attempts + deleteReview = 5 calls. submitReview must NOT run.
	if got, want := len(*calls), 5; got != want {
		t.Fatalf("call count: got %d, want %d (resolve+begin+2 threads+delete; no submit)", got, want)
	}
	if !strings.Contains((*calls)[4].stdin, "deletePullRequestReview(") {
		t.Errorf("call #5 should be deletePullRequestReview when all threads fail\nSTDIN:\n%s", (*calls)[4].stdin)
	}
	for i, c := range *calls {
		if strings.Contains(c.stdin, "submitPullRequestReview(") {
			t.Errorf("submitReview must NOT run when all threads fail; found in call #%d\nSTDIN:\n%s", i+1, c.stdin)
		}
	}
}

func TestGraphqlCall_ReturnsErrorOnErrorsOnlyResponse(t *testing.T) {
	// gh exits 0 with {data:null, errors:[{...}]} for permission denials,
	// invalid input, etc. graphqlCall must surface the first errors[] entry
	// as a clean 'graphql: <msg>' before per-step decoders see the null data
	// and produce the confusing 'empty PR ID in response' message.
	scriptedGHResults(t, []ghResult{
		{out: []byte(`{"data":null,"errors":[{"message":"Resource not accessible by integration"}]}`)},
	})

	_, err := graphqlCall(context.Background(), "query{x}", nil)
	if err == nil {
		t.Fatal("graphqlCall: expected error on errors-only response, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "graphql:") {
		t.Errorf("error should start with 'graphql:' prefix; got %q", msg)
	}
	if !strings.Contains(msg, "Resource not accessible by integration") {
		t.Errorf("error should include the gh-supplied message; got %q", msg)
	}
}

func TestGraphqlCall_ReturnsErrorOnPartialResponse(t *testing.T) {
	// GraphQL spec allows errors[] alongside non-null data (partial success).
	// We treat any non-empty errors[] as failure so callers don't silently
	// proceed on half-broken responses. Two messages must be joined.
	scriptedGHResults(t, []ghResult{
		{out: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_X"}}},"errors":[{"message":"oops one"},{"message":"oops two"}]}`)},
	})

	_, err := graphqlCall(context.Background(), "query{x}", nil)
	if err == nil {
		t.Fatal("graphqlCall: expected error when errors[] non-empty alongside data, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "oops one") || !strings.Contains(msg, "oops two") {
		t.Errorf("error should join all messages; got %q", msg)
	}
}

func TestPoster_Submit_RoutesGraphQLBodyErrorsThroughFailureSummary(t *testing.T) {
	// Integration: a thread call that returns gh-200 with errors[] should
	// behave exactly like a transport failure — the finding lands in the
	// failure summary (via p4j's threadFailure plumbing) and the loop
	// continues. This is the payoff of doing the errors[] check inside
	// graphqlCall: addThread sees a normal error, no new plumbing needed.
	results := []ghResult{
		{out: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_X"}}}}`)},
		{out: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_X"}}}}`)},
		{out: []byte(`{"data":{"addPullRequestReviewThread":{"thread":{}}}}`)},
		{out: []byte(`{"data":null,"errors":[{"message":"Resource not accessible by integration"}]}`)},
		{out: []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"url":"https://github.com/o/r/pull/42#pullrequestreview-7"}}}}`)},
	}
	scriptedGHResults(t, results)

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{Owner: "owner", Repo: "repo", Number: 42, HeadSHA: "sha"}
	findings := []review.Finding{
		{File: "a.go", Line: 1, Severity: review.SeverityHigh, Title: "T1", SuggestedComment: "C1"},
		{File: "b.go", Line: 2, Severity: review.SeverityHigh, Title: "T2", SuggestedComment: "C2"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: expected nil on partial success, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Posted: 1/2") {
		t.Errorf("summary should show 1/2 posted; got:\n%s", out)
	}
	if !strings.Contains(out, "b.go:2") {
		t.Errorf("failed finding location missing from summary; got:\n%s", out)
	}
	if !strings.Contains(out, "graphql:") {
		t.Errorf("body-error failure should carry the 'graphql:' prefix; got:\n%s", out)
	}
	if !strings.Contains(out, "Resource not accessible by integration") {
		t.Errorf("body-error message should appear in summary; got:\n%s", out)
	}
}

func TestPoster_PrintPayload_EmptyFindingsWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	p := &Poster{Out: &buf}

	if err := p.PrintPayload(review.ReviewTarget{HeadSHA: "x"}, nil); err != nil {
		t.Fatalf("PrintPayload: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty findings produced output: %q", buf.String())
	}
}

// scriptedGlab installs runGlab with a canned-response stub mirroring
// scriptedGH. Used by the GitLab MR posting tests.
func scriptedGlab(t *testing.T, results []ghResult) *[]recordedGHCall {
	t.Helper()
	prev := runGlab
	t.Cleanup(func() { runGlab = prev })

	var calls []recordedGHCall
	i := 0
	runGlab = func(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		if name != "glab" {
			t.Errorf("unexpected command: %s", name)
		}
		var body []byte
		if stdin != nil {
			b, _ := io.ReadAll(stdin)
			body = b
		}
		calls = append(calls, recordedGHCall{
			args:  append([]string(nil), args...),
			stdin: string(body),
		})
		if i >= len(results) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		r := results[i]
		i++
		return r.out, r.err
	}
	return &calls
}

func TestPoster_Submit_GitLabPostsInlineDiscussions(t *testing.T) {
	calls := scriptedGlab(t, []ghResult{
		{out: []byte(`{"id":"abc","notes":[{"id":1}]}`)},
		{out: []byte(`{"id":"def","notes":[{"id":2}]}`)},
	})

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{
		Host:     review.HostGitLab,
		Owner:    "g",
		Repo:     "p",
		Number:   3650,
		HeadSHA:  "head123",
		BaseSHA:  "base456",
		StartSHA: "start789",
	}
	findings := []review.Finding{
		{File: "a.go", Line: 11, Severity: review.SeverityHigh, Model: "codex", Confidence: 0.9, SuggestedComment: "first"},
		{File: "b.go", Line: 22, Severity: review.SeverityLow, Model: "claude", Confidence: 0.7, SuggestedComment: "second"},
	}

	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(*calls) != 2 {
		t.Fatalf("expected 2 glab calls, got %d", len(*calls))
	}

	// Endpoint + flags assertions.
	first := (*calls)[0].args
	if first[0] != "api" {
		t.Errorf("expected first arg `api`; got %v", first[0])
	}
	if first[1] != "projects/g%2Fp/merge_requests/3650/discussions" {
		t.Errorf("unexpected endpoint: %q", first[1])
	}
	if !contains(first, "--method") || !contains(first, "POST") {
		t.Errorf("expected --method POST; got %v", first)
	}
	if !contains(first, "--input") || !contains(first, "-") {
		t.Errorf("expected --input -; got %v", first)
	}
	if !contains(first, "Content-Type: application/json") {
		t.Errorf("expected Content-Type: application/json header; got %v", first)
	}

	// Stdin must be a JSON object with body + nested position. This is the
	// load-bearing assertion: glab's -F does NOT nest bracket syntax, so we
	// must send the position as a proper sub-object via the request body.
	var parsed struct {
		Body     string `json:"body"`
		Position struct {
			BaseSHA      string `json:"base_sha"`
			HeadSHA      string `json:"head_sha"`
			StartSHA     string `json:"start_sha"`
			PositionType string `json:"position_type"`
			NewPath      string `json:"new_path"`
			OldPath      string `json:"old_path"`
			NewLine      int    `json:"new_line"`
		} `json:"position"`
	}
	if err := json.Unmarshal([]byte((*calls)[0].stdin), &parsed); err != nil {
		t.Fatalf("first call stdin is not valid JSON: %v; raw: %s", err, (*calls)[0].stdin)
	}
	if !strings.Contains(parsed.Body, "first") {
		t.Errorf("body should contain 'first'; got: %s", parsed.Body)
	}
	if parsed.Position.BaseSHA != "base456" {
		t.Errorf("position.base_sha = %q, want base456", parsed.Position.BaseSHA)
	}
	if parsed.Position.HeadSHA != "head123" {
		t.Errorf("position.head_sha = %q, want head123", parsed.Position.HeadSHA)
	}
	if parsed.Position.StartSHA != "start789" {
		t.Errorf("position.start_sha = %q, want start789", parsed.Position.StartSHA)
	}
	if parsed.Position.PositionType != "text" {
		t.Errorf("position.position_type = %q, want text", parsed.Position.PositionType)
	}
	if parsed.Position.NewPath != "a.go" || parsed.Position.OldPath != "a.go" {
		t.Errorf("position paths = (%q, %q), want (a.go, a.go)", parsed.Position.NewPath, parsed.Position.OldPath)
	}
	if parsed.Position.NewLine != 11 {
		t.Errorf("position.new_line = %d, want 11", parsed.Position.NewLine)
	}

	out := buf.String()
	if !strings.Contains(out, "Posted 2 inline thread(s)") {
		t.Errorf("expected 'Posted 2 inline thread(s)' summary; got: %s", out)
	}
}

func TestPoster_Submit_GitLabSurfacesInlineFailuresWithoutFallback(t *testing.T) {
	calls := scriptedGlab(t, []ghResult{
		// First finding: inline post fails — must NOT fall back to top-level.
		{err: errors.New("glab: exit 1: 400 Bad Request line not in diff")},
		// Second finding: inline post succeeds.
		{out: []byte(`{"id":"def"}`)},
	})

	var buf bytes.Buffer
	p := &Poster{Out: &buf}
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
		{File: "a.go", Line: 1, SuggestedComment: "first"},
		{File: "b.go", Line: 2, SuggestedComment: "second"},
	}
	if err := p.Submit(context.Background(), target, findings); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// Exactly 2 glab calls: one per finding. No fallback call.
	if len(*calls) != 2 {
		t.Fatalf("expected 2 glab calls (one inline per finding, no fallback); got %d; args of each: %+v", len(*calls), *calls)
	}
	for i, c := range *calls {
		if c.args[0] != "api" {
			t.Errorf("call %d expected `api` (inline post), got %q — fallback to non-api command is forbidden", i, c.args[0])
		}
	}
	out := buf.String()
	if !strings.Contains(out, "Posted: 1/2 (1 failed)") {
		t.Errorf("expected summary 'Posted: 1/2 (1 failed)'; got: %s", out)
	}
	if !strings.Contains(out, "a.go:1") || !strings.Contains(out, "line not in diff") {
		t.Errorf("expected failure summary to name a.go:1 with the glab error; got: %s", out)
	}
	if !strings.Contains(out, "Posted 1 inline thread(s)") {
		t.Errorf("expected 'Posted 1 inline thread(s)' final line; got: %s", out)
	}
}

func TestPoster_Submit_GitLabRequiresDiffRefs(t *testing.T) {
	var buf bytes.Buffer
	p := &Poster{Out: &buf}
	target := review.ReviewTarget{
		Host:   review.HostGitLab,
		Owner:  "g",
		Repo:   "p",
		Number: 1,
		// All SHAs intentionally empty.
	}
	err := p.Submit(context.Background(),
		target,
		[]review.Finding{{File: "a.go", Line: 1, SuggestedComment: "c"}})
	if err == nil || !strings.Contains(err.Error(), "missing diff-refs") {
		t.Errorf("expected 'missing diff-refs' error; got %v", err)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
