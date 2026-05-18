package diff

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	godiff "github.com/sourcegraph/go-diff/diff"
)

// Parse turns a unified diff into a slice of DiffFile.
//
// Parsing delegates to sourcegraph/go-diff (per ADR 0006); this layer adds
// path normalization, file-kind classification, and per-line bookkeeping
// for the line-position oracle.
func Parse(unified string) ([]*DiffFile, error) {
	r := godiff.NewMultiFileDiffReader(bytes.NewReader([]byte(unified)))

	var out []*DiffFile
	for {
		fd, err := r.ReadFile()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read file diff: %w", err)
		}

		df := &DiffFile{
			Path:    stripDiffPrefix(fd.NewName),
			OldPath: stripDiffPrefix(fd.OrigName),
			Kind:    classify(fd),
			Hunks:   convertHunks(fd.Hunks),
		}
		// OldPath is only meaningful when it names a real previous path.
		// Collapse it to "" when it duplicates Path (unchanged path) or
		// when it's `/dev/null` (file is brand new, no prior path).
		// Deleted files keep OldPath so callers can recover the
		// pre-delete identity even though Path is `/dev/null`.
		if df.OldPath == df.Path || df.OldPath == "/dev/null" {
			df.OldPath = ""
		}
		out = append(out, df)
	}
	return out, nil
}

// stripDiffPrefix removes the leading `a/` or `b/` git introduces in diff
// headers. `/dev/null` is preserved verbatim — callers branch on it.
func stripDiffPrefix(p string) string {
	if p == "/dev/null" || p == "" {
		return p
	}
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

// classify maps a go-diff FileDiff to our FileKind. The signals come from
// the extended headers git emits above the unified diff (rename from / to,
// new/deleted file mode, old/new mode, Binary files ... differ).
func classify(fd *godiff.FileDiff) FileKind {
	for _, h := range fd.Extended {
		if strings.HasPrefix(h, "Binary files ") {
			return FileBinary
		}
	}

	hasRename := false
	hasModeChange := false
	for _, h := range fd.Extended {
		switch {
		case strings.HasPrefix(h, "rename from "), strings.HasPrefix(h, "rename to "):
			hasRename = true
		case strings.HasPrefix(h, "deleted file mode "):
			return FileDelete
		case strings.HasPrefix(h, "old mode "), strings.HasPrefix(h, "new mode "):
			hasModeChange = true
		}
	}

	if hasRename {
		if len(fd.Hunks) == 0 {
			return FilePureRename
		}
		return FileRenameWithHunks
	}
	if hasModeChange && len(fd.Hunks) == 0 {
		return FileModeOnly
	}
	return FileText
}

func convertHunks(hs []*godiff.Hunk) []Hunk {
	if len(hs) == 0 {
		return nil
	}
	out := make([]Hunk, 0, len(hs))
	for _, h := range hs {
		out = append(out, Hunk{
			OldStart: int(h.OrigStartLine),
			OldLines: int(h.OrigLines),
			NewStart: int(h.NewStartLine),
			NewLines: int(h.NewLines),
			Lines:    convertLines(int(h.NewStartLine), h.Body),
		})
	}
	return out
}

// convertLines walks the hunk body and assigns post-image line numbers.
//
// Unified-diff body format: each line begins with " " (context), "+"
// (added), or "-" (deleted). Lines that begin with "\" are no-newline
// markers and are skipped (they belong to the previous content line).
func convertLines(newStart int, body []byte) []HunkLine {
	if len(body) == 0 {
		return nil
	}
	var out []HunkLine
	lineNo := newStart
	for _, raw := range splitBody(body) {
		if len(raw) == 0 {
			continue
		}
		marker, content := raw[0], raw[1:]
		switch marker {
		case ' ':
			out = append(out, HunkLine{Side: SideContext, NewLine: lineNo, Content: content})
			lineNo++
		case '+':
			out = append(out, HunkLine{Side: SideAdded, NewLine: lineNo, Content: content})
			lineNo++
		case '-':
			out = append(out, HunkLine{Side: SideDeleted, NewLine: 0, Content: content})
		case '\\':
			// "\ No newline at end of file" — annotation, not a real line.
			continue
		default:
			// Defensive: an unexpected marker means the diff is malformed.
			// Skip rather than crash; the validator will surface zero
			// findings if the whole file is unparseable.
		}
	}
	return out
}

// splitBody splits a hunk body into its constituent lines, keeping each
// line's leading marker but stripping the trailing newline. An empty
// trailing element from a final newline is dropped.
func splitBody(body []byte) []string {
	s := string(body)
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
