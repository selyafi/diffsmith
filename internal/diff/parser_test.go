package diff

import (
	"os"
	"path/filepath"
	"strings"
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

// TestParseRealGitHubPR exercises the parser against a real `gh pr diff
// --patch` capture (cli/cli#13491, 7 workflow files, ~10 KB). The fixture
// preserves the git-format-patch headers ("From <sha>... Subject:...") that
// gh emits in --patch mode, which the synthetic fixtures don't carry.
// This is the M2-followup deliverable per spike S1: a captured real-PR
// fixture so future regressions against real-world gh diff peculiarities
// are caught hermetically. To refresh: re-run
//
//	gh pr diff https://github.com/cli/cli/pull/13491 --patch --color never > \
//	  testdata/diffs/github_pr_cli_13491.diff
func TestParseRealGitHubPR(t *testing.T) {
	raw := readFixture(t, "github_pr_cli_13491.diff")

	files, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := len(files), 7; got != want {
		t.Fatalf("file count: got %d, want %d", got, want)
	}

	for _, f := range files {
		if f.Kind != FileText {
			t.Errorf("file %q: Kind = %v, want FileText", f.Path, f.Kind)
		}
	}

	// Anchor one known path so a future fixture refresh against an
	// unrelated PR doesn't silently pass with the wrong content.
	var found bool
	for _, f := range files {
		if f.Path == ".github/workflows/codeql.yml" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .github/workflows/codeql.yml in fixture; fixture may have been replaced")
	}
}

// TestConvertLinesRejectsMalformedMarker verifies convertLines fails
// loud when a hunk body contains an unexpected leading byte. Earlier
// the parser silently skipped such lines, which left subsequent rows
// with the wrong NewLine counter — meaning a finding anchored to a
// "valid" later line could land on the wrong row of a real PR/MR.
// The new behavior surfaces a categorized error so Parse fails before
// findings can be wrongly anchored.
//
// We exercise convertLines directly because go-diff's MultiFileDiffReader
// terminates a hunk at the first non-marker byte (treating it as a
// section break), so a synthetic malformed *file*-level diff would never
// reach our switch. In production this matters because go-diff hands us
// the body it parsed; if a future format change ever lets a stray
// marker through, convertLines is the layer that catches it.
func TestConvertLinesRejectsMalformedMarker(t *testing.T) {
	// Body where the second body line has a `?` marker — invalid in
	// unified-diff format. convertLines must refuse to assign positions
	// against such a body.
	body := []byte(" context\n?bogus marker\n+added\n")
	_, err := convertLines(10, body)
	if err == nil {
		t.Fatal("convertLines must reject a malformed marker; got nil error")
	}
	if !strings.Contains(err.Error(), "malformed hunk") {
		t.Errorf("error should categorize the failure as 'malformed hunk'; got: %v", err)
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

// TestParse_EmptyContextLineKeepsLineNumbering is the diffsmith-dsx
// regression: whitespace-trimmed patches store empty context lines as
// "" instead of " ", and go-diff deliberately passes them through.
// Skipping them without counting shifted every subsequent NewLine down
// by one — misanchoring validation and upstream posting, the exact
// failure convertLines' doc says must never happen silently.
func TestParse_EmptyContextLineKeepsLineNumbering(t *testing.T) {
	raw := "diff --git a/f.go b/f.go\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/f.go\n" +
		"+++ b/f.go\n" +
		"@@ -1,3 +1,4 @@\n" +
		" a\n" +
		"\n" + // empty context line, leading space trimmed
		"+added\n" +
		" b\n"
	files, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	lines := files[0].Hunks[0].Lines
	if len(lines) != 4 {
		t.Fatalf("want 4 hunk lines (incl. empty context), got %d", len(lines))
	}
	if lines[1].Side != SideContext || lines[1].NewLine != 2 {
		t.Errorf("empty line must be context @2; got side=%v line=%d", lines[1].Side, lines[1].NewLine)
	}
	if lines[2].Side != SideAdded || lines[2].NewLine != 3 {
		t.Errorf("added line must be @3; got side=%v line=%d", lines[2].Side, lines[2].NewLine)
	}
	if lines[3].NewLine != 4 {
		t.Errorf("final context line must be @4; got %d", lines[3].NewLine)
	}
}
