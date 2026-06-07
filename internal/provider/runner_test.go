package provider_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/provider"
)

// TestIsolatedRunnerRunsInFreshTempDir proves the core of diffsmith-4tz:
// reviewer CLIs must NOT run in the caller's working directory (where a
// project's .agents/skills/, AGENTS.md, or CLAUDE.md would be autoloaded).
// We exec `pwd` through the isolated runner and assert the child reported
// a directory that is (a) not the caller's cwd and (b) gone after the call
// returned — i.e. a fresh per-call temp dir that was cleaned up.
func TestIsolatedRunnerRunsInFreshTempDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	cwdResolved, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("evalsymlinks cwd: %v", err)
	}

	run := provider.IsolatedRunner()
	out, err := run(context.Background(), nil, "pwd")
	if err != nil {
		t.Fatalf("IsolatedRunner pwd: %v", err)
	}
	childDir := strings.TrimSpace(string(out))

	if childDir == cwd || childDir == cwdResolved {
		t.Errorf("child ran in caller cwd %q; want an isolated temp dir", childDir)
	}

	// The per-call temp dir must be removed once the command returns.
	if _, statErr := os.Stat(childDir); !os.IsNotExist(statErr) {
		t.Errorf("temp dir %q still exists after run (stat err: %v); want cleaned up", childDir, statErr)
	}
}
