package update

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIsReleaseVersion(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"v0.1.0", true},
		{"v1.0.0-rc1", true},
		{"v2.5.0", true},
		{"", false},
		{"dev", false},
		{"v0.1.0-dirty", false},
		{"f91518a-dirty", false},
		{"f91518a", false}, // bare SHA from git describe --always (no leading v)
	}
	for _, c := range cases {
		if got := isReleaseVersion(c.v); got != c.want {
			t.Errorf("isReleaseVersion(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v0.2.0", "v0.1.0", +1},
		{"v0.1.0", "v0.2.0", -1},
		{"v0.1.0", "v0.1.0", 0},
		{"v1.0.0", "v0.9.0", +1},
		{"v0.1.0", "0.1.0", 0}, // leading 'v' optional
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		// Normalize to sign for comparison since strings.Compare may
		// return values other than ±1.
		gotSign := 0
		switch {
		case got > 0:
			gotSign = 1
		case got < 0:
			gotSign = -1
		}
		if gotSign != c.want {
			t.Errorf("compareVersions(%q, %q) sign = %d, want %d", c.a, c.b, gotSign, c.want)
		}
	}
}

// withTestCache redirects the cache dir to a per-test tempdir and
// resets httpFetcher between tests so test order doesn't matter.
func withTestCache(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	prev := httpFetcher
	t.Cleanup(func() { httpFetcher = prev })
}

func TestCheck_SkipsDevBuilds(t *testing.T) {
	withTestCache(t)
	called := false
	httpFetcher = func(_ context.Context) (string, error) {
		called = true
		return "v999.0.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "dev", &buf)
	if called {
		t.Error("httpFetcher must NOT be called for dev builds")
	}
	if buf.Len() > 0 {
		t.Errorf("dev builds must produce no output; got: %s", buf.String())
	}
}

func TestCheck_SkipsDirtyBuilds(t *testing.T) {
	withTestCache(t)
	called := false
	httpFetcher = func(_ context.Context) (string, error) {
		called = true
		return "v999.0.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "f91518a-dirty", &buf)
	if called {
		t.Error("httpFetcher must NOT be called for *-dirty builds")
	}
	if buf.Len() > 0 {
		t.Errorf("dirty builds must produce no output; got: %s", buf.String())
	}
}

func TestCheck_NotifiesOnNewerVersion(t *testing.T) {
	withTestCache(t)
	httpFetcher = func(_ context.Context) (string, error) {
		return "v0.2.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	out := buf.String()
	if !strings.Contains(out, "v0.2.0 available") {
		t.Errorf("expected notification mentioning v0.2.0; got: %q", out)
	}
	if !strings.Contains(out, "v0.1.0") {
		t.Errorf("notification should also name current version v0.1.0; got: %q", out)
	}
	if !strings.Contains(out, "install.sh") {
		t.Errorf("notification should include install.sh URL; got: %q", out)
	}
}

func TestCheck_SilentOnSameVersion(t *testing.T) {
	withTestCache(t)
	httpFetcher = func(_ context.Context) (string, error) {
		return "v0.1.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	if buf.Len() > 0 {
		t.Errorf("equal versions must produce no output; got: %q", buf.String())
	}
}

func TestCheck_SilentOnOlderRemote(t *testing.T) {
	// If GitHub somehow reports an older version than what we're
	// running (shouldn't happen in practice, but cache age + tag
	// deletion edge cases exist), do nothing — don't suggest a
	// "downgrade."
	withTestCache(t)
	httpFetcher = func(_ context.Context) (string, error) {
		return "v0.0.9", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	if buf.Len() > 0 {
		t.Errorf("older remote version must not produce output; got: %q", buf.String())
	}
}

func TestCheck_SilentOnFetchError(t *testing.T) {
	withTestCache(t)
	httpFetcher = func(_ context.Context) (string, error) {
		return "", errors.New("simulated network failure")
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	if buf.Len() > 0 {
		t.Errorf("fetch error must produce no output; got: %q", buf.String())
	}
}

func TestCheck_UsesCacheWithinLifetime(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	// Seed a fresh cache entry pointing at v0.5.0.
	cacheFile := filepath.Join(cacheRoot, "diffsmith", "latest-version.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	seeded := `{"latest_version":"v0.5.0","checked_at":"` + time.Now().Format(time.RFC3339Nano) + `"}`
	if err := os.WriteFile(cacheFile, []byte(seeded), 0o644); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	prev := httpFetcher
	t.Cleanup(func() { httpFetcher = prev })
	called := false
	httpFetcher = func(_ context.Context) (string, error) {
		called = true
		return "v999.0.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	if called {
		t.Error("fresh cache should prevent network call")
	}
	if !strings.Contains(buf.String(), "v0.5.0 available") {
		t.Errorf("notification should reflect cached value v0.5.0; got: %q", buf.String())
	}
}

func TestCheck_RefetchesWhenCacheStale(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	// Seed a cache entry older than cacheLifetime.
	cacheFile := filepath.Join(cacheRoot, "diffsmith", "latest-version.json")
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
	stale := time.Now().Add(-cacheLifetime - time.Hour).Format(time.RFC3339Nano)
	seeded := `{"latest_version":"v0.5.0","checked_at":"` + stale + `"}`
	if err := os.WriteFile(cacheFile, []byte(seeded), 0o644); err != nil {
		t.Fatalf("seed stale cache: %v", err)
	}
	prev := httpFetcher
	t.Cleanup(func() { httpFetcher = prev })
	called := false
	httpFetcher = func(_ context.Context) (string, error) {
		called = true
		return "v0.3.0", nil
	}
	var buf bytes.Buffer
	Check(context.Background(), "v0.1.0", &buf)
	if !called {
		t.Error("stale cache should trigger a refetch")
	}
	if !strings.Contains(buf.String(), "v0.3.0 available") {
		t.Errorf("notification should use refetched value v0.3.0; got: %q", buf.String())
	}
}
