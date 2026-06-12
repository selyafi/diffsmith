package diff

import (
	"fmt"
	"path"
	"strings"
)

// Exclude drops files matching any of the gitignore-lite patterns from
// both the parsed file list and the raw unified diff, returning the
// kept files, the rebuilt raw diff, and the excluded paths (in diff
// order). With no patterns it returns its inputs unchanged.
//
// Pattern rules (documented on the --exclude flag):
//   - trailing "/"  — directory tree, at any depth ("vendor/", "gen/")
//   - no "/"        — basename glob via path.Match ("*.lock")
//   - otherwise     — full-path glob via path.Match ("internal/gen/*.go")
//
// Renamed files are excluded when either their old or new path matches,
// so `--exclude '*.lock'` cannot be dodged by a rename in the diff.
//
// Raw-segment correspondence is positional: rawDiff must contain exactly
// one `diff --git` segment per entry in files (the invariant Parse
// guarantees for its own output). A mismatch is an error, never a guess.
func Exclude(files []*DiffFile, rawDiff string, patterns []string) (kept []*DiffFile, keptRaw string, excluded []string, err error) {
	if len(patterns) == 0 {
		return files, rawDiff, nil, nil
	}
	for _, pat := range patterns {
		// Surface malformed globs immediately — a typo'd pattern that
		// silently matches nothing would defeat the exclusion's purpose.
		if !strings.HasSuffix(pat, "/") {
			if _, err := path.Match(pat, "probe"); err != nil {
				return nil, "", nil, fmt.Errorf("invalid --exclude pattern %q: %w", pat, err)
			}
		}
	}

	segments := splitRawSegments(rawDiff)
	if len(segments) != len(files) {
		return nil, "", nil, fmt.Errorf("raw diff has %d segment(s) but %d parsed file(s); refusing to exclude against a mismatched diff", len(segments), len(files))
	}

	var rawKept strings.Builder
	for i, f := range files {
		drop, err := fileMatchesAny(f, patterns)
		if err != nil {
			return nil, "", nil, err
		}
		if drop {
			excluded = append(excluded, f.Path)
			continue
		}
		kept = append(kept, f)
		rawKept.WriteString(segments[i])
	}
	return kept, rawKept.String(), excluded, nil
}

// fileMatchesAny reports whether any pattern matches the file's path or,
// for renames/deletes, its previous path.
func fileMatchesAny(f *DiffFile, patterns []string) (bool, error) {
	for _, pat := range patterns {
		for _, p := range []string{f.Path, f.OldPath} {
			if p == "" || p == "/dev/null" {
				continue
			}
			ok, err := matchPattern(pat, p)
			if err != nil {
				return false, fmt.Errorf("invalid --exclude pattern %q: %w", pat, err)
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// matchPattern applies one gitignore-lite rule to one slash path.
func matchPattern(pat, p string) (bool, error) {
	if strings.HasSuffix(pat, "/") {
		return strings.HasPrefix(p, pat) || strings.Contains(p, "/"+pat), nil
	}
	if !strings.Contains(pat, "/") {
		return path.Match(pat, path.Base(p))
	}
	return path.Match(pat, p)
}

// splitRawSegments cuts a unified diff at each `diff --git` header line.
// Text before the first header (if any) is attached to the first
// segment so no bytes are invented or lost.
func splitRawSegments(raw string) []string {
	const header = "diff --git "
	var starts []int
	for i := 0; i < len(raw); {
		lineEnd := strings.IndexByte(raw[i:], '\n')
		if strings.HasPrefix(raw[i:], header) {
			starts = append(starts, i)
		}
		if lineEnd < 0 {
			break
		}
		i += lineEnd + 1
	}
	if len(starts) == 0 {
		return nil
	}
	segs := make([]string, 0, len(starts))
	for n, s := range starts {
		end := len(raw)
		if n+1 < len(starts) {
			end = starts[n+1]
		}
		begin := s
		if n == 0 {
			begin = 0 // keep any preamble with the first segment
		}
		segs = append(segs, raw[begin:end])
	}
	return segs
}
