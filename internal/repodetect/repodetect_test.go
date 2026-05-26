package repodetect

import (
	"errors"
	"os"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
)

// TestMain swaps in an identity resolver so the table below doesn't shell
// out to the host's real ssh binary (whose ssh_config we don't control).
// Individual tests override sshHostResolver via t.Cleanup as needed.
func TestMain(m *testing.M) {
	prev := sshHostResolver
	sshHostResolver = func(h string) (string, error) { return h, nil }
	code := m.Run()
	sshHostResolver = prev
	os.Exit(code)
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want provider.RepoCoord
		err  bool
	}{
		{
			name: "github SSH",
			raw:  "git@github.com:selyafi/diffsmith.git",
			want: provider.RepoCoord{Host: "github.com", Owner: "selyafi", Name: "diffsmith"},
		},
		{
			name: "github HTTPS with .git",
			raw:  "https://github.com/selyafi/diffsmith.git",
			want: provider.RepoCoord{Host: "github.com", Owner: "selyafi", Name: "diffsmith"},
		},
		{
			name: "github HTTPS without .git",
			raw:  "https://github.com/selyafi/diffsmith",
			want: provider.RepoCoord{Host: "github.com", Owner: "selyafi", Name: "diffsmith"},
		},
		{
			name: "gitlab SSH nested group",
			raw:  "git@gitlab.com:my-group/subgroup/widget.git",
			want: provider.RepoCoord{Host: "gitlab.com", Owner: "my-group/subgroup", Name: "widget"},
		},
		{
			name: "gitlab HTTPS nested",
			raw:  "https://gitlab.com/my-group/subgroup/widget",
			want: provider.RepoCoord{Host: "gitlab.com", Owner: "my-group/subgroup", Name: "widget"},
		},
		{
			name: "self-hosted gitlab",
			raw:  "git@gitlab.example.com:team/repo.git",
			want: provider.RepoCoord{Host: "gitlab.example.com", Owner: "team", Name: "repo"},
		},
		{
			name: "trailing newline tolerated",
			raw:  "https://github.com/a/b\n",
			want: provider.RepoCoord{Host: "github.com", Owner: "a", Name: "b"},
		},
		{name: "empty string", raw: "", err: true},
		{name: "garbage", raw: "not a url", err: true},
		{name: "ftp scheme rejected", raw: "ftp://github.com/a/b", err: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRemoteURL(tc.raw)
			if (err != nil) != tc.err {
				t.Fatalf("got err=%v want err=%v", err, tc.err)
			}
			if tc.err {
				return
			}
			if got != tc.want {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestParseRemoteURL_SSHAliasResolution(t *testing.T) {
	t.Run("SSH alias is resolved to real host", func(t *testing.T) {
		prev := sshHostResolver
		t.Cleanup(func() { sshHostResolver = prev })
		sshHostResolver = func(h string) (string, error) {
			if h == "github-shelyafi" {
				return "github.com", nil
			}
			return h, nil
		}

		got, err := parseRemoteURL("git@github-shelyafi:oddin-gg/lagertha-mono.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := provider.RepoCoord{Host: "github.com", Owner: "oddin-gg", Name: "lagertha-mono"}
		if got != want {
			t.Errorf("got %+v\nwant %+v", got, want)
		}
	})

	t.Run("resolver error is propagated (no silent fallback to alias)", func(t *testing.T) {
		prev := sshHostResolver
		t.Cleanup(func() { sshHostResolver = prev })
		sshHostResolver = func(h string) (string, error) {
			return "", errors.New("ssh: command not found")
		}

		_, err := parseRemoteURL("git@github-shelyafi:owner/repo.git")
		if err == nil {
			t.Fatal("expected error when resolver fails, got nil")
		}
	})

	t.Run("HTTPS URLs skip the resolver", func(t *testing.T) {
		prev := sshHostResolver
		t.Cleanup(func() { sshHostResolver = prev })
		called := false
		sshHostResolver = func(h string) (string, error) {
			called = true
			return h, nil
		}

		got, err := parseRemoteURL("https://github-shelyafi/owner/repo.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("resolver should not be called for HTTPS URLs")
		}
		// HTTPS URL with an alias-shaped host stays literal — HTTPS doesn't
		// honor ssh_config, so this is the user's error to fix.
		if got.Host != "github-shelyafi" {
			t.Errorf("got host %q, want literal %q", got.Host, "github-shelyafi")
		}
	})
}
