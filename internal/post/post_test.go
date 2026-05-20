package post

import (
	"bytes"
	"context"
	"encoding/json"
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

// scriptedGH installs runGH with a stub that returns canned responses in
// order and records each call. It restores the original runGH on Cleanup.
func scriptedGH(t *testing.T, responses [][]byte) *[]recordedGHCall {
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
		if i >= len(responses) {
			t.Fatalf("unexpected call #%d: %s %v", i+1, name, args)
		}
		out := responses[i]
		i++
		return out, nil
	}
	return &calls
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
