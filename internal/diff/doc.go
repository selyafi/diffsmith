// Package diff parses unified diffs into a structured DiffFile model and maps
// (file, post-image line) pairs back to added, modified, context, or
// out-of-hunk locations.
//
// Parsing delegates to sourcegraph/go-diff (ADR 0006); this package adds
// classification, line-position bookkeeping, and the validator-facing Index.
package diff
