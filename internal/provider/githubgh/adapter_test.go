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
