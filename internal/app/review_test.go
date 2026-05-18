package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/provider"
)

// stubProvider is a hand-rolled Provider for testing review.go without
// needing a real CLI runner. Each field is the canned outcome for the
// corresponding interface method.
type stubProvider struct {
	supports     func(string) bool
	preflightErr error
	fetchInput   *provider.ReviewInput
	fetchErr     error
	preflightHit bool
	fetchHit     bool
}

func (s *stubProvider) Supports(u string) bool { return s.supports(u) }

func (s *stubProvider) Preflight(context.Context) error {
	s.preflightHit = true
	return s.preflightErr
}

func (s *stubProvider) Fetch(context.Context, string) (*provider.ReviewInput, error) {
	s.fetchHit = true
	return s.fetchInput, s.fetchErr
}

func newTestRoot(stub *stubProvider) (*cobra.Command, *bytes.Buffer) {
	registry := provider.NewRegistry(stub)
	root := &cobra.Command{Use: "diffsmith", SilenceUsage: true}
	root.AddCommand(newReviewCmd(registry))
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	return root, buf
}

func sampleReviewInput() *provider.ReviewInput {
	return &provider.ReviewInput{
		Target: provider.ReviewTarget{
			Host:    provider.HostGitHub,
			URL:     "https://github.com/owner/repo/pull/42",
			Owner:   "owner",
			Repo:    "repo",
			Number:  42,
			HeadRef: "feat/x",
			BaseRef: "main",
		},
		Title:  "Tighten token parsing",
		Author: "alice",
		Files: []*diff.DiffFile{
			{Path: "auth/session.go", Kind: diff.FileText, Hunks: []diff.Hunk{{}}},
			{Path: "docs/changelog.md", Kind: diff.FileText, Hunks: []diff.Hunk{{}, {}}},
		},
		RawDiff: "diff --git a/auth/session.go b/auth/session.go\n...\n",
	}
}

func TestReviewPrintPromptHappyPath(t *testing.T) {
	stub := &stubProvider{
		supports:   func(string) bool { return true },
		fetchInput: sampleReviewInput(),
	}
	root, out := newTestRoot(stub)
	root.SetArgs([]string{"review", "https://github.com/owner/repo/pull/42", "--print-prompt"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !stub.preflightHit {
		t.Error("Preflight was not called before Fetch")
	}
	if !stub.fetchHit {
		t.Error("Fetch was not called")
	}

	got := out.String()
	for _, want := range []string{
		"URL: https://github.com/owner/repo/pull/42",
		"Title: Tighten token parsing",
		"Author: alice",
		"Branch: feat/x -> main",
		"# Files (2)",
		"- auth/session.go (text, 1 hunk(s))",
		"- docs/changelog.md (text, 2 hunk(s))",
		"diff --git a/auth/session.go b/auth/session.go",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q.\nFull output:\n%s", want, got)
		}
	}
}

func TestReviewDryRunSkipsModel(t *testing.T) {
	stub := &stubProvider{
		supports:   func(string) bool { return true },
		fetchInput: sampleReviewInput(),
	}
	root, out := newTestRoot(stub)
	root.SetArgs([]string{"review", "https://github.com/owner/repo/pull/42", "--dry-run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "fetched 2 file(s)") || !strings.Contains(got, "--dry-run") {
		t.Errorf("dry-run output should confirm fetch and skip; got:\n%s", got)
	}
}

func TestReviewWithoutFlagsErrorsAtModelStep(t *testing.T) {
	stub := &stubProvider{
		supports:   func(string) bool { return true },
		fetchInput: sampleReviewInput(),
	}
	root, _ := newTestRoot(stub)
	root.SetArgs([]string{"review", "https://github.com/owner/repo/pull/42"})

	err := root.Execute()
	if err == nil {
		t.Fatal("want error pointing to M3, got nil")
	}
	if !strings.Contains(err.Error(), "M3") {
		t.Errorf("error should reference M3; got: %v", err)
	}
}

func TestReviewPreflightFailureSkipsFetch(t *testing.T) {
	stub := &stubProvider{
		supports:     func(string) bool { return true },
		preflightErr: errors.New("gh not authenticated"),
	}
	root, _ := newTestRoot(stub)
	root.SetArgs([]string{"review", "https://github.com/owner/repo/pull/42", "--print-prompt"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "gh not authenticated") {
		t.Errorf("preflight error should propagate; got: %v", err)
	}
	if stub.fetchHit {
		t.Error("Fetch must not run when Preflight fails")
	}
}

func TestReviewUnsupportedURL(t *testing.T) {
	stub := &stubProvider{supports: func(string) bool { return false }}
	root, _ := newTestRoot(stub)
	root.SetArgs([]string{"review", "https://example.com/random", "--print-prompt"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "no provider") {
		t.Errorf("unsupported URL should error; got: %v", err)
	}
}
