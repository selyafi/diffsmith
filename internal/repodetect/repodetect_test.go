package repodetect

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
)

// stubResolver replaces sshHostResolver for the duration of the test and
// restores the previous value via t.Cleanup. Centralizes the save/restore
// pattern; not safe under t.Parallel — see sshHostResolver's docstring.
func stubResolver(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := sshHostResolver
	t.Cleanup(func() { sshHostResolver = prev })
	sshHostResolver = fn
}

// notCalledResolver fails the test if invoked. Use for cases that must
// take the dot-heuristic fast path.
func notCalledResolver(t *testing.T) func(string) (string, error) {
	t.Helper()
	return func(h string) (string, error) {
		t.Fatalf("resolver unexpectedly called with %q", h)
		return "", nil
	}
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
			// All cases above use either HTTPS or dotted SSH hosts, so
			// the resolver should not be invoked. If the dot-heuristic
			// fast path regresses, this will catch it.
			stubResolver(t, notCalledResolver(t))

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
	t.Run("dotless SSH host invokes the resolver", func(t *testing.T) {
		stubResolver(t, func(h string) (string, error) {
			if h == "github-shelyafi" {
				return "github.com", nil
			}
			return h, nil
		})

		got, err := parseRemoteURL("git@github-shelyafi:oddin-gg/lagertha-mono.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := provider.RepoCoord{Host: "github.com", Owner: "oddin-gg", Name: "lagertha-mono"}
		if got != want {
			t.Errorf("got %+v\nwant %+v", got, want)
		}
	})

	t.Run("dotted SSH host skips the resolver (no ssh dep for canonical hosts)", func(t *testing.T) {
		stubResolver(t, notCalledResolver(t))

		got, err := parseRemoteURL("git@github.com:owner/repo.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Host != "github.com" {
			t.Errorf("got host %q, want literal %q", got.Host, "github.com")
		}
	})

	t.Run("resolver error is propagated (no silent fallback to alias)", func(t *testing.T) {
		stubResolver(t, func(h string) (string, error) {
			return "", errors.New("ssh: command not found")
		})

		_, err := parseRemoteURL("git@github-shelyafi:owner/repo.git")
		if err == nil {
			t.Fatal("expected error when resolver fails, got nil")
		}
	})

	t.Run("empty resolved hostname is rejected", func(t *testing.T) {
		stubResolver(t, func(h string) (string, error) {
			return "", nil
		})

		_, err := parseRemoteURL("git@gh-shadow:owner/repo.git")
		if err == nil {
			t.Fatal("expected error for empty resolved hostname, got nil")
		}
		if !strings.Contains(err.Error(), "empty result") {
			t.Errorf("error %q should mention 'empty result'", err)
		}
	})

	t.Run("host starting with '-' is rejected before invoking resolver", func(t *testing.T) {
		stubResolver(t, notCalledResolver(t))

		_, err := parseRemoteURL("git@-oProxyCommand=evil:owner/repo.git")
		if err == nil {
			t.Fatal("expected error for dash-prefixed host, got nil")
		}
		if !strings.Contains(err.Error(), "'-'") {
			t.Errorf("error %q should mention the dash issue", err)
		}
	})

	t.Run("empty SSH host is rejected", func(t *testing.T) {
		stubResolver(t, notCalledResolver(t))

		_, err := parseRemoteURL("git@:owner/repo.git")
		if err == nil {
			t.Fatal("expected error for empty host, got nil")
		}
	})

	t.Run("HTTPS URLs skip the resolver", func(t *testing.T) {
		stubResolver(t, notCalledResolver(t))

		got, err := parseRemoteURL("https://github-shelyafi/owner/repo.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// HTTPS URL with an alias-shaped host stays literal — HTTPS doesn't
		// honor ssh_config, so this is the user's error to fix.
		if got.Host != "github-shelyafi" {
			t.Errorf("got host %q, want literal %q", got.Host, "github-shelyafi")
		}
	})

	t.Run("ssh:// scheme is handled and respects the alias gate", func(t *testing.T) {
		stubResolver(t, func(h string) (string, error) {
			if h == "github-shelyafi" {
				return "github.com", nil
			}
			t.Fatalf("unexpected resolver call for %q", h)
			return "", nil
		})

		got, err := parseRemoteURL("ssh://git@github-shelyafi/oddin-gg/lagertha-mono.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := provider.RepoCoord{Host: "github.com", Owner: "oddin-gg", Name: "lagertha-mono"}
		if got != want {
			t.Errorf("got %+v\nwant %+v", got, want)
		}
	})

	t.Run("ssh:// scheme with port and dotted host skips resolver", func(t *testing.T) {
		stubResolver(t, notCalledResolver(t))

		got, err := parseRemoteURL("ssh://git@github.com:22/owner/repo.git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Host != "github.com" {
			t.Errorf("got host %q, want %q", got.Host, "github.com")
		}
	})

	t.Run("real ssh -G round-trips an unknown host (integration)", func(t *testing.T) {
		// Verifies the real resolveSSHHost against actual OpenSSH. Skipped
		// when ssh isn't on PATH (minimal containers, some CI). Asserts
		// non-empty result rather than echo-back equality because users
		// with `CanonicalizeHostname always` + `CanonicalDomains` will
		// see the probe rewritten with a suffix.
		if _, err := exec.LookPath("ssh"); err != nil {
			t.Skip("ssh not on PATH")
		}
		probe := "diffsmith-test-host-no-alias-xyzzy"
		got, err := resolveSSHHost(probe)
		if err != nil {
			t.Fatalf("resolveSSHHost: %v", err)
		}
		if got == "" {
			t.Errorf("resolveSSHHost returned empty hostname for %q", probe)
		}
	})
}

func TestParseSSHGHostname(t *testing.T) {
	// Captured shape of real OpenSSH 10.x output. Tests the pure parser
	// in isolation — independent of whether the user has ssh on PATH.
	tests := []struct {
		name    string
		out     string
		want    string
		wantErr bool
	}{
		{
			name: "single hostname line",
			out:  "user git\nhostname github.com\nport 22\n",
			want: "github.com",
		},
		{
			name: "hostname line at start",
			out:  "hostname gitlab.example.com\nuser git\n",
			want: "gitlab.example.com",
		},
		{
			name: "no trailing newline",
			out:  "user git\nhostname github.com",
			want: "github.com",
		},
		{
			name:    "no hostname line",
			out:     "user git\nport 22\n",
			wantErr: true,
		},
		{
			name:    "empty value after key",
			out:     "user git\nhostname \nport 22\n",
			wantErr: true,
		},
		{
			name:    "empty output",
			out:     "",
			wantErr: true,
		},
		{
			name: "hostkey* prefix lines are not confused with hostname",
			out:  "hostkeyalgorithms ssh-ed25519\nhostkeyalias somealias\nhostname github.com\n",
			want: "github.com",
		},
		{
			name: "first hostname wins (single line per ssh -G contract)",
			out:  "hostname first.example.com\nhostname second.example.com\n",
			want: "first.example.com",
		},
		{
			name: "CRLF line endings tolerated (Windows ssh.exe)",
			out:  "user git\r\nhostname github.com\r\nport 22\r\n",
			want: "github.com",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSSHGHostname([]byte(tc.out))
			if (err != nil) != tc.wantErr {
				t.Fatalf("got err=%v want err=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestValidateSSHHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{name: "normal alias", host: "github-shelyafi", wantErr: false},
		{name: "dotted hostname", host: "github.com", wantErr: false},
		{name: "empty", host: "", wantErr: true},
		{name: "dash prefix (flag injection)", host: "-oProxyCommand=evil", wantErr: true},
		{name: "single dash", host: "-", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSSHHost(tc.host)
			if (err != nil) != tc.wantErr {
				t.Errorf("got err=%v want err=%v", err, tc.wantErr)
			}
		})
	}
}
