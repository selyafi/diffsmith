package diff

import (
	"strings"
	"testing"
)

// Include reuses the threeFileDiff fixture and parseThreeFileDiff helper
// defined in exclude_test.go — same package, same three targets (a
// root lockfile, a nested source file, a vendored file).

func TestInclude_NoPatternsReturnsInputUnchanged(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, dropped, err := Include(files, threeFileDiff, nil)
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 3 || keptRaw != threeFileDiff || len(dropped) != 0 {
		t.Errorf("no patterns must be a no-op (review everything); kept=%d dropped=%d rawChanged=%v",
			len(kept), len(dropped), keptRaw != threeFileDiff)
	}
}

// TestInclude_BasenameGlobKeepsOnlyMatches: --include is the inverse of
// --exclude — it keeps matching files and drops the rest.
func TestInclude_BasenameGlobKeepsOnlyMatches(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, dropped, err := Include(files, threeFileDiff, []string{"*.json"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 1 || kept[0].Path != "package-lock.json" {
		t.Fatalf("want only package-lock.json kept, got %d files", len(kept))
	}
	if len(dropped) != 2 {
		t.Errorf("dropped = %v, want the two non-matching files", dropped)
	}
	if !strings.Contains(keptRaw, "package-lock.json") {
		t.Error("kept raw missing the included file's segment")
	}
	for _, gone := range []string{"internal/auth/session.go", "vendor/lib/dep.go"} {
		if strings.Contains(keptRaw, gone) {
			t.Errorf("kept raw should not contain dropped file %q", gone)
		}
	}
}

func TestInclude_TrailingSlashKeepsTreeAtAnyDepth(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, dropped, err := Include(files, threeFileDiff, []string{"auth/"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 1 || kept[0].Path != "internal/auth/session.go" {
		t.Fatalf("auth/ should keep internal/auth/ at depth; kept=%d", len(kept))
	}
	if len(dropped) != 2 {
		t.Errorf("want 2 dropped, got %v", dropped)
	}
	if strings.Contains(keptRaw, "vendor/lib/dep.go") {
		t.Error("kept raw should not contain a non-matching segment")
	}
}

func TestInclude_FullPathGlob(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, _, dropped, err := Include(files, threeFileDiff, []string{"internal/auth/*.go"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 1 || kept[0].Path != "internal/auth/session.go" || len(dropped) != 2 {
		t.Errorf("full-path glob: kept=%d dropped=%v", len(kept), dropped)
	}
}

// TestInclude_MultiplePatternsUnion: a file is kept if it matches ANY
// pattern, so two patterns keep the union of their matches.
func TestInclude_MultiplePatternsUnion(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, _, dropped, err := Include(files, threeFileDiff, []string{"*.json", "vendor/"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 2 || len(dropped) != 1 || dropped[0] != "internal/auth/session.go" {
		t.Errorf("union of *.json and vendor/ should keep 2, drop the source file; kept=%d dropped=%v",
			len(kept), dropped)
	}
}

// TestInclude_RenameMatchesOldPath: a file renamed away from a matching
// path is still kept — symmetry with Exclude so a rename can't smuggle a
// file out of an allowlist any more than into a blocklist.
func TestInclude_RenameMatchesOldPath(t *testing.T) {
	const renameDiff = `diff --git a/notes.lock b/notes.txt
similarity index 90%
rename from notes.lock
rename to notes.txt
index 1111111..2222222 100644
--- a/notes.lock
+++ b/notes.txt
@@ -1,1 +1,1 @@
-x
+y
`
	files, err := Parse(renameDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kept, _, dropped, err := Include(files, renameDiff, []string{"*.lock"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 1 || len(dropped) != 0 {
		t.Errorf("rename whose OldPath matches must be kept; kept=%d dropped=%v", len(kept), dropped)
	}
}

// TestInclude_NothingMatchesDropsAll: when no file matches, Include keeps
// nothing and reports every path as dropped. The app layer turns this
// into a clean pre-model error.
func TestInclude_NothingMatchesDropsAll(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, dropped, err := Include(files, threeFileDiff, []string{"*.rs"})
	if err != nil {
		t.Fatalf("Include: %v", err)
	}
	if len(kept) != 0 || len(dropped) != 3 {
		t.Errorf("kept=%d dropped=%d; want 0/3", len(kept), len(dropped))
	}
	if strings.TrimSpace(keptRaw) != "" {
		t.Errorf("kept raw should be empty, got %d bytes", len(keptRaw))
	}
}

// TestInclude_InvalidPatternErrors: a malformed glob must error up front,
// naming the pattern and the --include flag, even if no file would have
// matched it.
func TestInclude_InvalidPatternErrors(t *testing.T) {
	files := parseThreeFileDiff(t)
	_, _, _, err := Include(files, threeFileDiff, []string{"[unclosed"})
	if err == nil || !strings.Contains(err.Error(), "[unclosed") || !strings.Contains(err.Error(), "--include") {
		t.Errorf("invalid pattern should error naming the pattern and --include; got %v", err)
	}
}

// TestInclude_SegmentCountMismatchErrors: raw-segment to parsed-file
// correspondence is positional, exactly as in Exclude.
func TestInclude_SegmentCountMismatchErrors(t *testing.T) {
	files := parseThreeFileDiff(t)
	oneSegment := threeFileDiff[:strings.Index(threeFileDiff, "diff --git a/internal")]
	_, _, _, err := Include(files, oneSegment, []string{"*.json"})
	if err == nil || !strings.Contains(err.Error(), "segment") {
		t.Errorf("mismatched raw/files must error; got %v", err)
	}
}
