// Package repodetect resolves a provider.RepoCoord from the cwd's git
// remote. Used by the inbox flow which has a directory, not a URL.
package repodetect

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/selyafi/diffsmith/internal/provider"
)

// sshHostResolver maps an ssh_config alias (e.g. "github-shelyafi") to
// the real hostname (e.g. "github.com"). Exposed as a package variable
// so tests can stub it; production wiring points at resolveSSHHost.
var sshHostResolver = resolveSSHHost

// resolveSSHHost shells out to `ssh -G <alias>` and returns the resolved
// hostname. ssh -G applies the user's ssh_config and prints the canonical
// hostname on a `hostname <value>` line. For non-alias inputs, ssh -G
// echoes the input back as the hostname.
func resolveSSHHost(alias string) (string, error) {
	out, err := exec.Command("ssh", "-G", alias).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("ssh -G %s: %w: %s", alias, err, strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("ssh -G %s: %w", alias, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "hostname ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "hostname ")), nil
		}
	}
	return "", fmt.Errorf("ssh -G %s: no hostname in output", alias)
}

// parseRemoteURL maps a git remote URL (ssh or https) to a RepoCoord.
// Supports nested paths (GitLab subgroups) by treating everything up to
// the final path segment as the owner. SSH URLs have their host resolved
// through ssh_config so aliases like `git@github-shelyafi:...` map to the
// real hostname before provider dispatch.
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
		resolved, err := sshHostResolver(host)
		if err != nil {
			return provider.RepoCoord{}, fmt.Errorf("repodetect: resolve ssh host %q: %w", host, err)
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
