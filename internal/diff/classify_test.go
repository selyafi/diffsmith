package diff

import "testing"

func TestParseClassification(t *testing.T) {
	cases := []struct {
		fixture string
		path    string
		oldPath string
		kind    FileKind
		hunks   int
	}{
		{"modified_simple.diff", "auth/session.go", "", FileText, 1},
		{"added_simple.diff", "util/clock.go", "", FileText, 1},
		{"deleted_simple.diff", "/dev/null", "legacy/old.go", FileDelete, 1},
		{"pure_rename.diff", "new/path.go", "old/path.go", FilePureRename, 0},
		{"rename_with_hunks.diff", "handler/new_name.go", "handler/old_name.go", FileRenameWithHunks, 1},
		{"binary_change.diff", "assets/logo.png", "", FileBinary, 0},
		{"mode_only.diff", "scripts/run.sh", "", FileModeOnly, 0},
		{"multi_hunk.diff", "parser/parse.go", "", FileText, 2},
		{"no_newline_at_eof.diff", "text/note.txt", "", FileText, 1},
	}
	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			files, err := Parse(readFixture(t, tc.fixture))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(files) != 1 {
				t.Fatalf("want 1 file, got %d", len(files))
			}
			f := files[0]
			if f.Path != tc.path {
				t.Errorf("Path: got %q, want %q", f.Path, tc.path)
			}
			if f.OldPath != tc.oldPath {
				t.Errorf("OldPath: got %q, want %q", f.OldPath, tc.oldPath)
			}
			if f.Kind != tc.kind {
				t.Errorf("Kind: got %v, want %v", f.Kind, tc.kind)
			}
			if len(f.Hunks) != tc.hunks {
				t.Errorf("Hunks count: got %d, want %d", len(f.Hunks), tc.hunks)
			}
		})
	}
}

func TestParseMultipleFiles(t *testing.T) {
	files, err := Parse(readFixture(t, "multiple_files.diff"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if files[0].Path != "a/first.go" || files[0].Kind != FileText {
		t.Errorf("file[0]: got Path=%q Kind=%v", files[0].Path, files[0].Kind)
	}
	if files[1].Path != "b/second.go" || files[1].Kind != FileText {
		t.Errorf("file[1]: got Path=%q Kind=%v", files[1].Path, files[1].Kind)
	}
}

func TestIncludeInPrompt(t *testing.T) {
	cases := []struct {
		kind FileKind
		want bool
	}{
		{FileText, true},
		{FileRenameWithHunks, true},
		{FilePureRename, false},
		{FileDelete, false},
		{FileBinary, false},
		{FileModeOnly, false},
	}
	for _, tc := range cases {
		if got := tc.kind.IncludeInPrompt(); got != tc.want {
			t.Errorf("FileKind(%d).IncludeInPrompt(): got %v, want %v", tc.kind, got, tc.want)
		}
	}
}
