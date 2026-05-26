// Package repodetect resolves a provider.RepoCoord from the cwd's git
// remote. Used by the inbox flow which has a directory, not a URL.
package repodetect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/selyafi/diffsmith/internal/provider"
)

// sshResolverTimeout bounds the ssh -G subprocess. ssh_config can run
// arbitrary commands via `Match exec` and read includes over slow shares;
// without a deadline a hostile or misconfigured config would hang Detect()
// indefinitely.
const sshResolverTimeout = 5 * time.Second

// sshHostResolver maps an ssh_config alias (e.g. "github-shelyafi") to
// the real hostname (e.g. "github.com"). Exposed as a package variable
// so tests can stub it; production wiring points at resolveSSHHost.
//
// Other shell-out sites in this codebase (preflight, model adapters)
// use struct-field injection. This package is free-function style
// (Detect, runGit), so a package var is the least-invasive test seam
// here. Not safe for t.Parallel — tests that mutate it must run
// sequentially. Revisit if more state ever needs injection.
var sshHostResolver = resolveSSHHost

// resolveSSHHost shells out to `ssh -G <alias>` and returns the resolved
// hostname. ssh -G applies the user's ssh_config and emits one
// `hostname <value>` line. Bounded by sshResolverTimeout; stderr is
// captured so warnings on a still-exit-0 path are surfaced in errors.
func resolveSSHHost(alias string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), sshResolverTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ssh", "-G", alias)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("ssh -G %s: timed out after %s (check ssh_config for slow Match exec / Include directives)", alias, sshResolverTimeout)
		}
		if stderr.Len() > 0 {
			return "", fmt.Errorf("ssh -G %s: %w: %s", alias, err, strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	host, perr := parseSSHGHostname(out)
	if perr != nil {
		return "", fmt.Errorf("ssh -G %s: %w", alias, perr)
	}
	return host, nil
}

// parseSSHGHostname extracts the resolved hostname from ssh -G stdout.
// Returns an error if the output has no `hostname <value>` line or the
// value is empty — both signal a malformed ssh_config that callers
// should fail loud on, not silently treat as the literal alias.
func parseSSHGHostname(out []byte) (string, error) {
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "hostname ") {
			continue
		}
		host := strings.TrimSpace(strings.TrimPrefix(line, "hostname "))
		if host == "" {
			return "", errors.New("hostname line has empty value")
		}
		return host, nil
	}
	return "", errors.New("no hostname line in output")
}

// validateSSHHost rejects host strings that would be misinterpreted by
// ssh -G as command-line flags. A remote URL of `git@-oProxyCommand=...:`
// produces host=`-oProxyCommand=...`; passing that as argv to ssh lets a
// hostile repo control the user's ssh client. Same family as
// git CVE-2017-1000117. Empty hosts are rejected up-front too — they
// can never be valid ssh aliases or DNS names.
func validateSSHHost(host string) error {
	if host == "" {
		return errors.New("empty ssh host")
	}
	if strings.HasPrefix(host, "-") {
		return fmt.Errorf("ssh host %q starts with '-' (would be interpreted as a flag)", host)
	}
	return nil
}

// resolveSSHHostIfAlias gates ssh_config resolution on the dot-heuristic:
// only hosts WITHOUT a dot are treated as potential aliases and run
// through sshHostResolver. Real DNS hostnames (github.com, gitlab.com,
// any self-hosted forge) and IP literals always contain a dot and skip
// the resolver entirely. This avoids three sharp edges:
//
//   - A hard `ssh`-on-PATH dependency for the canonical case (minimal
//     containers without openssh-client previously parsed `git@github.com`
//     without invoking ssh).
//   - The `Host * / HostName proxy` wildcard hijack — a corp ProxyJump
//     config silently rewrites `github.com` to a bastion and breaks
//     provider dispatch even though the URL the user typed is a forge
//     identity, not an SSH transport target.
//   - ~30-150ms ssh fork cost on every Detect() in the common case.
func resolveSSHHostIfAlias(host string) (string, error) {
	if err := validateSSHHost(host); err != nil {
		return "", err
	}
	if strings.Contains(host, ".") {
		return host, nil
	}
	resolved, err := sshHostResolver(host)
	if err != nil {
		return "", fmt.Errorf("resolve ssh host %q: %w", host, err)
	}
	if resolved == "" {
		return "", fmt.Errorf("resolve ssh host %q: empty result", host)
	}
	return resolved, nil
}

// parseRemoteURL maps a git remote URL to a RepoCoord. Three forms are
// supported: scp-style (`git@host:owner/.../name`), `ssh://[user@]host[:port]/path`,
// and `https?://host/path`. Nested paths (GitLab subgroups) are handled
// by treating everything up to the final segment as the owner. SSH
// hosts that look like aliases (no dot) are resolved through ssh_config
// via `ssh -G`; dotted hosts are taken literally.
func parseRemoteURL(raw string) (provider.RepoCoord, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return provider.RepoCoord{}, errors.New("repodetect: empty remote URL")
	}

	var host, path string
	switch {
	case strings.HasPrefix(s, "git@"):
		// git@host:owner/.../name(.git)?
		rest := strings.TrimPrefix(s, "git@")
		colon := strings.Index(rest, ":")
		if colon < 0 {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: malformed ssh url %q", raw)
		}
		host = rest[:colon]
		path = rest[colon+1:]
		resolved, err := resolveSSHHostIfAlias(host)
		if err != nil {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: %w (in %q)", err, raw)
		}
		host = resolved
	case strings.HasPrefix(s, "ssh://"):
		// ssh://[user@]host[:port]/owner/.../name(.git)?
		rest := strings.TrimPrefix(s, "ssh://")
		if at := strings.Index(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: malformed ssh url %q", raw)
		}
		host = rest[:slash]
		path = rest[slash+1:]
		if colon := strings.Index(host, ":"); colon >= 0 {
			host = host[:colon]
		}
		resolved, err := resolveSSHHostIfAlias(host)
		if err != nil {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: %w (in %q)", err, raw)
		}
		host = resolved
	case strings.HasPrefix(s, "https://"), strings.HasPrefix(s, "http://"):
		rest := s
		rest = strings.TrimPrefix(rest, "https://")
		rest = strings.TrimPrefix(rest, "http://")
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: malformed https url %q", raw)
		}
		host = rest[:slash]
		path = rest[slash+1:]
	default:
		return provider.RepoCoord{}, fmt.Errorf("repodetect: unsupported url scheme in %q", raw)
	}

	path = strings.TrimSuffix(path, ".git")
	if path == "" {
		return provider.RepoCoord{}, fmt.Errorf("repodetect: missing path in %q", raw)
	}

	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		return provider.RepoCoord{}, fmt.Errorf("repodetect: missing owner/name in %q", raw)
	}
	owner := path[:lastSlash]
	name := path[lastSlash+1:]
	if owner == "" || name == "" {
		return provider.RepoCoord{}, fmt.Errorf("repodetect: missing owner or name in %q", raw)
	}

	return provider.RepoCoord{Host: host, Owner: owner, Name: name}, nil
}

// Detect resolves the current working directory's git remote into a
// RepoCoord. Looks at remote.origin first; falls back to the only
// remote if origin isn't set; errors on 0 or multiple-non-origin
// remotes per spec §6.
//
// For SSH remotes whose host looks like an ssh_config alias (no dot in
// the host portion), Detect shells out to `ssh -G` to resolve the real
// hostname before returning. This requires `ssh` on PATH when such an
// alias is in use; canonical hosts like `git@github.com:...` skip the
// resolver and have no new runtime dependency.
func Detect() (provider.RepoCoord, error) {
	originURL, err := runGit("config", "--get", "remote.origin.url")
	if err == nil && strings.TrimSpace(originURL) != "" {
		return parseRemoteURL(originURL)
	}

	// No origin — see what we do have.
	remotes, err := listRemotes()
	if err != nil {
		return provider.RepoCoord{}, err
	}
	switch len(remotes) {
	case 0:
		return provider.RepoCoord{}, errors.New("repodetect: no git remotes configured")
	case 1:
		var only string
		for _, url := range remotes {
			only = url
		}
		return parseRemoteURL(only)
	default:
		names := make([]string, 0, len(remotes))
		for n := range remotes {
			names = append(names, n)
		}
		return provider.RepoCoord{}, fmt.Errorf("repodetect: multiple remotes (%s); set 'origin' or cd to a single-remote clone", strings.Join(names, ", "))
	}
}

// listRemotes returns a name→url map by parsing `git remote -v`.
func listRemotes() (map[string]string, error) {
	out, err := runGit("remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("repodetect: not in a git repository (or git not installed): %w", err)
	}
	remotes := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// fields[0] = name, fields[1] = url, fields[2] = (fetch)|(push)
		remotes[fields[0]] = fields[1]
	}
	return remotes, nil
}

func runGit(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
	}
	return string(out), err
}
