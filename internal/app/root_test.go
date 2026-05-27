package app

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

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

// TestRootHelpDescribesOptInPosting pins the trust-boundary contract:
// the root help must NOT say "Diffsmith never posts" (that was true
// pre-M5b; it has been false since the explicit posting path landed).
// It must also tell the user that posting is opt-in and gated on
// confirmation — otherwise the help text is dangerously stale for a
// local-first tool handling private code.
func TestRootHelpDescribesOptInPosting(t *testing.T) {
	root := newRootCmd()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("--help: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "never posts") {
		t.Errorf("help must not claim 'never posts'; posting has been opt-in since M5b\nGOT:\n%s", got)
	}
	// The help must surface BOTH the approval gate and the explicit
	// confirmation prompt so a first-time reader understands the trust
	// boundary without reading the README.
	for _, want := range []string{"approve", "post"} {
		if !strings.Contains(strings.ToLower(got), want) {
			t.Errorf("help must mention %q; got:\n%s", want, got)
		}
	}
}

// TestPostFlowFlagsAreSymmetricAcrossEntryPoints is the diffsmith-3e8
// regression: --repost, --print-payload, and --debug all affect the
// review post-flow, and they must be available from every entry
// point that exercises that flow. Before this fix, bare `diffsmith`
// exposed only --repost and `diffsmith inbox` exposed only
// --print-payload, so users had to reach for `diffsmith review` just
// to get --debug.
func TestPostFlowFlagsAreSymmetricAcrossEntryPoints(t *testing.T) {
	root := newRootCmd()

	cases := []struct {
		name    string
		findCmd func() *cobra.Command
		wantOK  []string // flags that must be present
	}{
		{"bare root", func() *cobra.Command { return root }, []string{"repost", "print-payload", "debug"}},
		{"inbox", func() *cobra.Command {
			for _, c := range root.Commands() {
				if c.Name() == "inbox" {
					return c
				}
			}
			return nil
		}, []string{"repost", "print-payload", "debug"}},
		{"review", func() *cobra.Command {
			for _, c := range root.Commands() {
				if c.Name() == "review" {
					return c
				}
			}
			return nil
		}, []string{"repost", "print-payload", "debug"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.findCmd()
			if cmd == nil {
				t.Fatalf("could not find %s subcommand", tc.name)
			}
			for _, flag := range tc.wantOK {
				if cmd.Flags().Lookup(flag) == nil {
					t.Errorf("%s command missing --%s flag", tc.name, flag)
				}
			}
		})
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
