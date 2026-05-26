// Package update implements a passive update-notification check that
// runs after the main diffsmith flow. It compares the running binary's
// version against the latest GitHub Release and prints a one-line
// suggestion if a newer release exists.
//
// Design properties:
//   - Silent on failure. Network errors, parse errors, missing cache
//     directories — none of them surface to the user. A notification
//     is a courtesy, not a load-bearing path.
//   - Skips local builds. The "dev" and "*-dirty" version strings
//     (stamped by Makefile's git-describe) bypass the check entirely
//     so dev work isn't nagged about a release "upgrade" that's
//     actually a downgrade.
//   - Caches for 24h. We hit the GitHub Releases API at most once per
//     day per user, even when the user runs diffsmith many times.
//     The cache lives at $XDG_CACHE_HOME/diffsmith/latest-version.json
//     (or $HOME/.cache/diffsmith/...).
//   - Uses a swappable httpFetcher seam so tests don't hit the network.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	apiURL        = "https://api.github.com/repos/selyafi/diffsmith/releases/latest"
	installURL    = "https://raw.githubusercontent.com/selyafi/diffsmith/main/install.sh"
	cacheLifetime = 24 * time.Hour
	httpTimeout   = 3 * time.Second
)

// httpFetcher is the test seam for the GitHub Releases API call.
// Tests reassign this package-level variable to return canned tags
// without hitting the real network.
var httpFetcher = defaultHTTPFetcher

// Check prints a one-line update notification to w if a newer release
// exists on GitHub. Silent on every failure mode — never blocks, never
// returns errors. Safe to call from a cobra PersistentPostRun hook.
//
// The cache hit path is O(1) read of a small JSON file; only the
// once-per-24h cache miss does network I/O, bounded by httpTimeout.
func Check(ctx context.Context, current string, w io.Writer) {
	if !isReleaseVersion(current) {
		return
	}
	latest, ok := cachedLatest()
	if !ok {
		fetched, err := httpFetcher(ctx)
		if err != nil || fetched == "" {
			return
		}
		latest = fetched
		_ = writeCache(latest)
	}
	if compareVersions(latest, current) > 0 {
		fmt.Fprintf(w, "↑ diffsmith %s available (you have %s). Update: curl -fsSL %s | sh\n",
			latest, current, installURL)
	}
}

// isReleaseVersion classifies a version string as one stamped by a
// real release tag (vX.Y.Z[-suffix]) vs a local development build.
// Local builds are filtered out so they never get a "newer version
// available" prompt that would actually be a downgrade.
func isReleaseVersion(v string) bool {
	if v == "" || v == "dev" {
		return false
	}
	if strings.Contains(v, "-dirty") {
		return false
	}
	return strings.HasPrefix(v, "v")
}

// compareVersions returns +1 if a > b, -1 if a < b, 0 if equal.
// Accepts versions with or without a leading 'v' — we normalise to
// the 'v'-prefixed form before delegating to golang.org/x/mod/semver,
// which requires the prefix.
//
// Earlier implementation used strings.Compare and got bitten the
// instant any segment crossed a digit-width boundary: lexicographic
// ordering says "10" < "9", so v0.10.0 looked older than v0.9.0 to
// the notifier. semver.Compare also handles prerelease suffixes
// correctly (v0.1.0-rc1 < v0.1.0), which we need for our own
// rc-prefixed tags.
func compareVersions(a, b string) int {
	if !strings.HasPrefix(a, "v") {
		a = "v" + a
	}
	if !strings.HasPrefix(b, "v") {
		b = "v" + b
	}
	return semver.Compare(a, b)
}

func defaultHTTPFetcher(ctx context.Context) (string, error) {
	reqCtx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github releases api: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	return payload.TagName, nil
}

type cacheEntry struct {
	LatestVersion string    `json:"latest_version"`
	CheckedAt     time.Time `json:"checked_at"`
}

// cacheDir returns the per-user cache directory for diffsmith, or ""
// if neither XDG_CACHE_HOME nor HOME is set. Caching is silently
// disabled in that pathological case.
func cacheDir() string {
	if d := os.Getenv("XDG_CACHE_HOME"); d != "" {
		return filepath.Join(d, "diffsmith")
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".cache", "diffsmith")
	}
	return ""
}

// cachedLatest returns the cached latest-version string and a boolean
// indicating whether the cache was both present and fresh (newer than
// cacheLifetime ago).
func cachedLatest() (string, bool) {
	dir := cacheDir()
	if dir == "" {
		return "", false
	}
	data, err := os.ReadFile(filepath.Join(dir, "latest-version.json"))
	if err != nil {
		return "", false
	}
	var e cacheEntry
	if json.Unmarshal(data, &e) != nil {
		return "", false
	}
	if time.Since(e.CheckedAt) > cacheLifetime {
		return "", false
	}
	return e.LatestVersion, true
}

// writeCache persists the fetched latest version. Failures are silent;
// the worst case is "we'll re-fetch next run."
func writeCache(latest string) error {
	dir := cacheDir()
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(cacheEntry{
		LatestVersion: latest,
		CheckedAt:     time.Now(),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "latest-version.json"), data, 0o644)
}
