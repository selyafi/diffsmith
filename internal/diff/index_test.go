package diff

import "testing"

func TestClassifyLineAddedFile(t *testing.T) {
	files, err := Parse(readFixture(t, "added_simple.diff"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := NewIndex(files)

	cases := []struct {
		name string
		line int
		want LineClassification
	}{
		// added_simple.diff inserts 5 lines into a brand-new file
		// starting at post-image line 1. The hunk has no "-" lines, so
		// every "+" line classifies as LineAdded, not LineModified.
		{"first added line", 1, LineAdded},
		{"middle added line", 3, LineAdded},
		{"last added line", 5, LineAdded},
		{"line past end is outside", 6, LineOutsideHunk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.Classify("util/clock.go", tc.line)
			if got != tc.want {
				t.Errorf("Classify line %d: got %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestClassifyLineMultiHunk(t *testing.T) {
	files, err := Parse(readFixture(t, "multi_hunk.diff"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := NewIndex(files)

	cases := []struct {
		name string
		line int
		want LineClassification
	}{
		// First hunk @@ -3,7 +3,7 @@: lines 3..9 in post-image,
		// modification at line 6.
		{"hunk-1 modified line", 6, LineModified},
		{"hunk-1 context", 4, LineContext},
		// Gap between hunks (lines 10..19) is outside any hunk.
		{"between hunks", 15, LineOutsideHunk},
		// Second hunk @@ -20,5 +20,7 @@: lines 20..26 in post-image,
		// pure insertion at lines 23 and 24 (hunk has no deletes, so
		// these are LineAdded, not LineModified).
		{"hunk-2 added line", 23, LineAdded},
		{"hunk-2 second added", 24, LineAdded},
		{"hunk-2 context", 22, LineContext},
		{"past last hunk", 99, LineOutsideHunk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.Classify("parser/parse.go", tc.line)
			if got != tc.want {
				t.Errorf("Classify line %d: got %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}

func TestClassifyLineRenameWithHunks(t *testing.T) {
	// A finding against a renamed file must target the post-rename path.
	files, err := Parse(readFixture(t, "rename_with_hunks.diff"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := NewIndex(files)

	if got := idx.Classify("handler/new_name.go", 3); got != LineModified {
		t.Errorf("post-rename path line 3: got %v, want LineModified", got)
	}
	if got := idx.Classify("handler/old_name.go", 3); got != LineFileNotFound {
		t.Errorf("pre-rename path should not be addressable: got %v, want LineFileNotFound", got)
	}
}

func TestClassifyExcludedKindsAreUnaddressable(t *testing.T) {
	// Binary, pure-rename, delete, and mode-only files exist in the
	// index for diagnostics, but the schema can't address them. Every
	// line query on them must return LineFileNotFound.
	cases := []struct {
		fixture string
		path    string
	}{
		{"binary_change.diff", "assets/logo.png"},
		{"pure_rename.diff", "new/path.go"},
		{"mode_only.diff", "scripts/run.sh"},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			files, err := Parse(readFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			idx := NewIndex(files)
			if got := idx.Classify(tc.path, 1); got != LineFileNotFound {
				t.Errorf("line 1 of %s: got %v, want LineFileNotFound", tc.path, got)
			}
		})
	}
}

func TestClassifyNoNewlineAtEOF(t *testing.T) {
	// The trailing "\ No newline at end of file" markers must not be
	// counted as real lines. Post-image line 3 should classify as the
	// updated text, not as the no-newline marker.
	files, err := Parse(readFixture(t, "no_newline_at_eof.diff"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := NewIndex(files)

	if got := idx.Classify("text/note.txt", 3); got != LineModified {
		t.Errorf("line 3: got %v, want LineModified", got)
	}
	if got := idx.Classify("text/note.txt", 4); got != LineOutsideHunk {
		t.Errorf("line 4 should not exist as a hunk line: got %v, want LineOutsideHunk", got)
	}
}
