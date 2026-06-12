package diff

import (
	"strings"
	"testing"
)

// threeFileDiff has one root-level lockfile, one nested source file, and
// one vendored file, so each gitignore-lite rule has a target.
const threeFileDiff = `diff --git a/package-lock.json b/package-lock.json
index 1111111..2222222 100644
--- a/package-lock.json
+++ b/package-lock.json
@@ -1,1 +1,1 @@
-old lock
+new lock
diff --git a/internal/auth/session.go b/internal/auth/session.go
index 3333333..4444444 100644
--- a/internal/auth/session.go
+++ b/internal/auth/session.go
@@ -1,1 +1,1 @@
-old code
+new code
diff --git a/vendor/lib/dep.go b/vendor/lib/dep.go
index 5555555..6666666 100644
--- a/vendor/lib/dep.go
+++ b/vendor/lib/dep.go
@@ -1,1 +1,1 @@
-old dep
+new dep
`

func parseThreeFileDiff(t *testing.T) []*DiffFile {
	t.Helper()
	files, err := Parse(threeFileDiff)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("fixture should parse to 3 files, got %d", len(files))
	}
	return files
}

func TestExclude_NoPatternsReturnsInputUnchanged(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, excluded, err := Exclude(files, threeFileDiff, nil)
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 3 || keptRaw != threeFileDiff || len(excluded) != 0 {
		t.Errorf("no patterns must be a no-op; kept=%d excluded=%d rawChanged=%v",
			len(kept), len(excluded), keptRaw != threeFileDiff)
	}
}

func TestExclude_BasenameGlobMatchesAnyDepth(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, excluded, err := Exclude(files, threeFileDiff, []string{"*.json"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("want 2 kept, got %d", len(kept))
	}
	if len(excluded) != 1 || excluded[0] != "package-lock.json" {
		t.Errorf("excluded = %v, want [package-lock.json]", excluded)
	}
	if strings.Contains(keptRaw, "package-lock.json") {
		t.Error("kept raw still contains the excluded file's segment")
	}
	for _, want := range []string{"internal/auth/session.go", "vendor/lib/dep.go", "+new code", "+new dep"} {
		if !strings.Contains(keptRaw, want) {
			t.Errorf("kept raw missing %q", want)
		}
	}
}

func TestExclude_TrailingSlashExcludesTree(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, excluded, err := Exclude(files, threeFileDiff, []string{"vendor/"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 2 || len(excluded) != 1 || excluded[0] != "vendor/lib/dep.go" {
		t.Fatalf("kept=%d excluded=%v; want 2 kept, vendor file excluded", len(kept), excluded)
	}
	if strings.Contains(keptRaw, "vendor/lib/dep.go") {
		t.Error("kept raw still contains vendored segment")
	}
}

// TestExclude_TrailingSlashMatchesNestedDir: gitignore semantics — a
// directory pattern matches at any depth, not only at the repo root.
func TestExclude_TrailingSlashMatchesNestedDir(t *testing.T) {
	const nested = `diff --git a/pkg/gen/types.go b/pkg/gen/types.go
index 1111111..2222222 100644
--- a/pkg/gen/types.go
+++ b/pkg/gen/types.go
@@ -1,1 +1,1 @@
-a
+b
`
	files, err := Parse(nested)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kept, _, excluded, err := Exclude(files, nested, []string{"gen/"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 0 || len(excluded) != 1 {
		t.Errorf("gen/ should match pkg/gen/ at depth; kept=%d excluded=%v", len(kept), excluded)
	}
}

func TestExclude_FullPathGlob(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, _, excluded, err := Exclude(files, threeFileDiff, []string{"internal/auth/*.go"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 2 || len(excluded) != 1 || excluded[0] != "internal/auth/session.go" {
		t.Errorf("full-path glob: kept=%d excluded=%v", len(kept), excluded)
	}
}

// TestExclude_RenameMatchesOldPath: a file renamed away from a matching
// path is still excluded — otherwise `--exclude '*.lock'` misses
// `foo.lock -> foo.txt` and the old content rides along.
func TestExclude_RenameMatchesOldPath(t *testing.T) {
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
	kept, _, excluded, err := Exclude(files, renameDiff, []string{"*.lock"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 0 || len(excluded) != 1 {
		t.Errorf("rename whose OldPath matches must be excluded; kept=%d excluded=%v", len(kept), excluded)
	}
}

func TestExclude_AllExcludedReturnsEmpty(t *testing.T) {
	files := parseThreeFileDiff(t)
	kept, keptRaw, excluded, err := Exclude(files, threeFileDiff, []string{"*"})
	if err != nil {
		t.Fatalf("Exclude: %v", err)
	}
	if len(kept) != 0 || len(excluded) != 3 {
		t.Errorf("kept=%d excluded=%d; want 0/3", len(kept), len(excluded))
	}
	if strings.TrimSpace(keptRaw) != "" {
		t.Errorf("kept raw should be empty, got %d bytes", len(keptRaw))
	}
}

// TestExclude_InvalidPatternErrors: a malformed glob must error up front
// (naming the pattern) even if no file would have matched it — a typo'd
// exclude that silently matches nothing defeats the user's intent.
func TestExclude_InvalidPatternErrors(t *testing.T) {
	files := parseThreeFileDiff(t)
	_, _, _, err := Exclude(files, threeFileDiff, []string{"[unclosed"})
	if err == nil || !strings.Contains(err.Error(), "[unclosed") {
		t.Errorf("invalid pattern should error naming the pattern; got %v", err)
	}
}

// TestExclude_SegmentCountMismatchErrors: raw-segment to parsed-file
// correspondence is positional; if the caller hands a raw diff that
// doesn't match the files slice, Exclude must refuse rather than
// excluding the wrong segments.
func TestExclude_SegmentCountMismatchErrors(t *testing.T) {
	files := parseThreeFileDiff(t)
	oneSegment := threeFileDiff[:strings.Index(threeFileDiff, "diff --git a/internal")]
	_, _, _, err := Exclude(files, oneSegment, []string{"*.json"})
	if err == nil || !strings.Contains(err.Error(), "segment") {
		t.Errorf("mismatched raw/files must error; got %v", err)
	}
}
