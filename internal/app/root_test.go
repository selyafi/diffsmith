package app

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider/githubgh"
	"github.com/selyafi/diffsmith/internal/provider/gitlabglab"
)

func TestRootHelpListsReviewSubcommand(t *testing.T) {
	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--help should not error: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "review") {
		t.Fatalf("help output should advertise the review subcommand, got:\n%s", got)
	}
	if !strings.Contains(got, "diffsmith") {
		t.Fatalf("help output should mention the binary name, got:\n%s", got)
	}
}

func TestDefaultRegistryDispatchesGitHubAndGitLab(t *testing.T) {
	// The wiring under test: defaultRegistry() must contain both adapters
	// so the CLI dispatches correctly on URL shape. This is the load-bearing
	// test for M6d's acceptance.
	r := defaultRegistry()

	cases := []struct {
		name     string
		url      string
		wantType reflect.Type
	}{
		{"github PR URL", "https://github.com/owner/repo/pull/1", reflect.TypeOf((*githubgh.Adapter)(nil))},
		{"gitlab single-group MR URL", "https://gitlab.com/group/project/-/merge_requests/1", reflect.TypeOf((*gitlabglab.Adapter)(nil))},
		{"gitlab nested-group MR URL", "https://gitlab.com/group/sub/project/-/merge_requests/1", reflect.TypeOf((*gitlabglab.Adapter)(nil))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := r.Find(tc.url)
			if err != nil {
				t.Fatalf("Find(%s): unexpected error: %v", tc.url, err)
			}
			if got := reflect.TypeOf(p); got != tc.wantType {
				t.Errorf("Find(%s): got provider type %s, want %s", tc.url, got, tc.wantType)
			}
		})
	}

	// Unsupported URLs must produce a clear no-provider error.
	if _, err := r.Find("https://example.com/foo"); err == nil {
		t.Error("Find(example.com URL): want no-provider error, got nil")
	}
}

func TestReviewRequiresURLArgument(t *testing.T) {
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"review"})

	if err := root.Execute(); err == nil {
		t.Fatal("review without a URL argument should error")
	}
}
