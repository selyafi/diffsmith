package post

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
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
			"body":"**diffsmith review** — high, model: codex\n\nstuff",
			"position":{"new_path":"a.go","new_line":11}
		}]},
		{"notes":[{
			"body":"LGTM — could you also add a test?",
			"position":{"new_path":"b.go","new_line":22}
		}]},
		{"notes":[{
			"body":"**diffsmith review** — low\n\nstuff",
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

func TestFetchExistingGitLabKeys_IgnoresThreadsWithoutPosition(t *testing.T) {
	// Top-level diffsmith notes (no position) shouldn't generate keys
	// since they aren't anchored to a file:line.
	raw := []byte(`[
		{"notes":[{
			"body":"**diffsmith review** — top-level note",
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
		{"body":"**diffsmith review** — high","path":"a.go","line":11},
		{"body":"Looks good to me","path":"b.go","line":22},
		{"body":"**diffsmith review** — low","path":"c.go","line":33}
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

// scriptedRunnerForRunner returns a typed provider.Runner that drives
// the same call recorder used elsewhere in the package, so dedup tests
// can compose with the existing scriptedGlab/scriptedGH helpers when
// integration tests are added below.
var _ provider.Runner = func(context.Context, io.Reader, string, ...string) ([]byte, error) { return nil, nil }
