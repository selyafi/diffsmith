package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestRoot_UpdateCheckFiresOnStartupEvenWhenSubcommandErrors(t *testing.T) {
	// Regression: the update notification must fire BEFORE the user's
	// subcommand runs (PersistentPreRun), not after it (PostRun). Two
	// reasons: users see the prompt before they start working, and the
	// check still runs when the subcommand errors out — which is when
	// users most need an "upgrade available" hint.
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	cacheFile := filepath.Join(cacheRoot, "diffsmith", "latest-version.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	seeded := `{"latest_version":"v9.9.9","checked_at":"` + time.Now().Format(time.RFC3339Nano) + `"}`
	if err := os.WriteFile(cacheFile, []byte(seeded), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	prevVersion := version
	t.Cleanup(func() { version = prevVersion })
	version = "v0.1.0" // any valid release tag older than the cached "v9.9.9"

	root := newRootCmd()
	var stderr bytes.Buffer
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&stderr)
	// A URL that passes Args validation (ExactArgs(1)) but no registered
	// provider supports — RunE errors. With PreRun, the update check
	// still fires; with PostRun, it would not.
	root.SetArgs([]string{"review", "https://example.com/owner/repo/pull/1"})

	_ = root.Execute() // we expect an error here; the test cares about stderr

	if !strings.Contains(stderr.String(), "v9.9.9 available") {
		t.Errorf("update notification must fire even when subcommand errors; stderr:\n%s", stderr.String())
	}
}
