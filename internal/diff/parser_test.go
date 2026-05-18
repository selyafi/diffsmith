package diff

import (
	"os"
	"path/filepath"
	"testing"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	// Tests run from the package directory; testdata/ is at the repo root,
	// reachable via ../../testdata.
	path := filepath.Join("..", "..", "testdata", "diffs", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(data)
}

func TestParseModifiedSimple(t *testing.T) {
	raw := readFixture(t, "modified_simple.diff")

	files, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("want 1 file, got %d", len(files))
	}

	f := files[0]
	if got, want := f.Path, "auth/session.go"; got != want {
		t.Errorf("Path: got %q, want %q", got, want)
	}
	if got, want := f.Kind, FileText; got != want {
		t.Errorf("Kind: got %v, want FileText", got)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(f.Hunks))
	}

	h := f.Hunks[0]
	if got, want := h.NewStart, 10; got != want {
		t.Errorf("NewStart: got %d, want %d", got, want)
	}
	if got, want := h.NewLines, 7; got != want {
		t.Errorf("NewLines: got %d, want %d", got, want)
	}
}

func TestClassifyLineModifiedSimple(t *testing.T) {
	raw := readFixture(t, "modified_simple.diff")
	files, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := NewIndex(files)

	cases := []struct {
		name string
		file string
		line int
		want LineClassification
	}{
		{"replaced-line is Modified", "auth/session.go", 13, LineModified},
		{"context-line above hunk start", "auth/session.go", 10, LineContext},
		{"context-line just before edit", "auth/session.go", 12, LineContext},
		{"context-line just after edit", "auth/session.go", 14, LineContext},
		{"line past hunk end is OutsideHunk", "auth/session.go", 99, LineOutsideHunk},
		{"unknown file is FileNotFound", "nope/missing.go", 1, LineFileNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := idx.Classify(tc.file, tc.line)
			if got != tc.want {
				t.Errorf("Classify(%q, %d): got %v, want %v", tc.file, tc.line, got, tc.want)
			}
		})
	}
}
