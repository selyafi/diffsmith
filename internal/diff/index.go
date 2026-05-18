package diff

// Index answers (file, post-image line) → LineClassification queries against
// a parsed set of DiffFiles. The validator uses it to enforce the v1 schema
// invariant that findings target only added or modified lines.
type Index struct {
	files map[string]*DiffFile
}

// NewIndex builds an Index from a parsed diff. Files whose Kind excludes
// them from the prompt (binary, mode-only, pure-rename, delete) are kept in
// the index so callers can still distinguish "not in diff" from "in diff
// but unaddressable"; this distinction matters in M3 when reporting why a
// finding was quarantined.
func NewIndex(files []*DiffFile) *Index {
	m := make(map[string]*DiffFile, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return &Index{files: m}
}

// Classify resolves a (file, post-image line) pair to a LineClassification.
// The first return value is the line's role inside the diff; callers that
// need the underlying file should use Lookup.
func (i *Index) Classify(file string, line int) LineClassification {
	f, ok := i.files[file]
	if !ok {
		return LineFileNotFound
	}
	if !f.Kind.IncludeInPrompt() {
		// The schema can't address binary/rename-only/mode-only/delete
		// files, so any line query against them is FileNotFound from the
		// validator's perspective.
		return LineFileNotFound
	}
	for _, h := range f.Hunks {
		if line < h.NewStart || line >= h.NewStart+h.NewLines {
			continue
		}
		hunkHasDelete := hunkHasDeletedLines(h)
		for _, hl := range h.Lines {
			if hl.NewLine != line {
				continue
			}
			switch hl.Side {
			case SideContext:
				return LineContext
			case SideAdded:
				if hunkHasDelete {
					return LineModified
				}
				return LineAdded
			}
		}
		// Inside the hunk's range but no matching line: treat as outside
		// to avoid lying about classification.
		return LineOutsideHunk
	}
	return LineOutsideHunk
}

// Lookup returns the DiffFile for a path, or nil if the file isn't in the
// diff. Useful for callers that need more than a single classification.
func (i *Index) Lookup(file string) *DiffFile {
	return i.files[file]
}

// hunkHasDeletedLines reports whether the hunk contains any deleted lines.
// This is the hunk-level signal that distinguishes a modification (any "-"
// present) from a pure insertion (no "-" present). Line-level pairing for
// finer "added vs modified" classification within a mixed hunk is left to
// a later iteration; the validator does not need the distinction.
func hunkHasDeletedLines(h Hunk) bool {
	for _, l := range h.Lines {
		if l.Side == SideDeleted {
			return true
		}
	}
	return false
}
