package diff

// FileKind classifies a changed file for prompt construction.
// The provider adapter uses this to decide what reaches the model.
type FileKind int

const (
	// FileText is a text file with reviewable hunks. Include in prompt.
	FileText FileKind = iota
	// FileRenameWithHunks was renamed and also modified. Include under
	// the post-rename path; preserve the old path as metadata.
	FileRenameWithHunks
	// FilePureRename was renamed without content changes. Metadata only.
	FilePureRename
	// FileDelete was deleted. Metadata only by default; the v1 schema does
	// not accept deleted-line comments.
	FileDelete
	// FileBinary is a binary file change. Metadata only; unsupported by
	// the model prompt.
	FileBinary
	// FileModeOnly changed file mode without content changes. Metadata only.
	FileModeOnly
)

// IncludeInPrompt reports whether this file's hunks should reach the model.
// Mirrors the classification rule in docs/provider-adapters.md § Diff Edge Cases.
func (k FileKind) IncludeInPrompt() bool {
	return k == FileText || k == FileRenameWithHunks
}

// String returns a short human-readable label for the kind.
func (k FileKind) String() string {
	switch k {
	case FileText:
		return "text"
	case FileRenameWithHunks:
		return "rename+edits"
	case FilePureRename:
		return "pure-rename"
	case FileDelete:
		return "delete"
	case FileBinary:
		return "binary"
	case FileModeOnly:
		return "mode-only"
	default:
		return "unknown"
	}
}

// LineClassification labels what a post-image line number resolves to in a
// changed file. The validator uses this to enforce the v1 finding schema
// invariant: only added/modified lines are valid finding targets.
type LineClassification int

const (
	// LineAdded is a brand-new line introduced by the diff (a "+" line in
	// a hunk where the deleted counterpart, if any, is absent).
	LineAdded LineClassification = iota
	// LineModified is an added line that replaces a deleted line in the
	// same hunk position. Distinguished from LineAdded so the TUI can
	// render edit highlights differently in M4.
	LineModified
	// LineContext is an unchanged context line shown for surrounding
	// readability. The v1 finding schema disallows context-line comments,
	// so the validator quarantines findings landing here.
	LineContext
	// LineOutsideHunk is a line number that falls outside every hunk in
	// the file. The schema disallows comments here; quarantine.
	LineOutsideHunk
	// LineFileNotFound is returned when the file isn't part of the diff
	// at all, or has a kind that excludes it from prompting.
	LineFileNotFound
)

// DiffFile is one changed file extracted from a unified diff.
//
// Path holds the post-change path (per the schema: findings address files by
// their post-change path). OldPath is set when Kind is a rename variant.
type DiffFile struct {
	Path    string
	OldPath string
	Kind    FileKind
	Hunks   []Hunk
}

// Hunk is one contiguous block of changes within a file, corresponding to a
// `@@ -a,b +c,d @@` section of the unified diff.
//
// Lines are kept in source order; each carries its post-image line number
// (zero for deleted lines, which have no post-image position).
type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []HunkLine
}

// HunkLine is one line within a hunk.
type HunkLine struct {
	Side    LineSide
	NewLine int    // post-image line number; zero when Side == LineDeleted
	Content string // line content, without the leading +/-/space marker
}

// LineSide names whether a hunk line was added, deleted, or context.
type LineSide int

const (
	SideContext LineSide = iota
	SideAdded
	SideDeleted
)
