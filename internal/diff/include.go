package diff

import (
	"fmt"
	"path"
	"strings"
)

// Include is the allowlist inverse of Exclude: it keeps only the files
// matching at least one of the gitignore-lite patterns, dropping the rest
// from both the parsed file list and the raw unified diff. It returns the
// kept files, the rebuilt raw diff, and the dropped paths (in diff
// order). With no patterns it returns its inputs unchanged — an absent
// --include means "review everything", not "review nothing".
//
// Pattern rules are identical to Exclude (see matchPattern):
//   - trailing "/"  — directory tree, at any depth ("internal/", "auth/")
//   - no "/"        — basename glob via path.Match ("*.go")
//   - otherwise     — full-path glob via path.Match ("internal/app/*.go")
//
// A renamed file is kept when either its old or new path matches, the
// same OldPath/Path union Exclude uses — so a rename can't smuggle a file
// out of an allowlist any more than it can dodge a blocklist.
//
// Raw-segment correspondence is positional, exactly as in Exclude: a
// segment/file count mismatch is an error, never a guess.
func Include(files []*DiffFile, rawDiff string, patterns []string) (kept []*DiffFile, keptRaw string, dropped []string, err error) {
	if len(patterns) == 0 {
		return files, rawDiff, nil, nil
	}
	for _, pat := range patterns {
		// Surface malformed globs immediately, naming --include — a
		// typo'd pattern that silently matches nothing would filter the
		// whole diff away under an allowlist.
		if !strings.HasSuffix(pat, "/") {
			if _, err := path.Match(pat, "probe"); err != nil {
				return nil, "", nil, fmt.Errorf("invalid --include pattern %q: %w", pat, err)
			}
		}
	}

	segments := splitRawSegments(rawDiff)
	if len(segments) != len(files) {
		return nil, "", nil, fmt.Errorf("raw diff has %d segment(s) but %d parsed file(s); refusing to filter against a mismatched diff", len(segments), len(files))
	}

	var rawKept strings.Builder
	for i, f := range files {
		keep, err := fileMatchesAny(f, patterns)
		if err != nil {
			return nil, "", nil, err
		}
		if !keep {
			dropped = append(dropped, f.Path)
			continue
		}
		kept = append(kept, f)
		rawKept.WriteString(segments[i])
	}
	return kept, rawKept.String(), dropped, nil
}
