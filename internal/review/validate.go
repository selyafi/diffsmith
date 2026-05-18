package review

import (
	"fmt"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/model"
)

// Validate converts model candidates into validated Findings, enforcing
// every rule in docs/review-finding-schema.md:106-118.
//
// Each candidate is evaluated independently; both the accepted Findings
// and the Quarantined slice are returned so the TUI can show debug-only
// rejected items without dropping them on the floor.
func Validate(candidates []model.FindingCandidate, modelName string, idx *diff.Index) ([]Finding, []Quarantined) {
	var ok []Finding
	var bad []Quarantined

	for _, c := range candidates {
		finding, reason := validateOne(c, modelName, idx)
		if reason != "" {
			bad = append(bad, Quarantined{Candidate: c, Reason: reason})
			continue
		}
		ok = append(ok, finding)
	}
	return ok, bad
}

// validateOne returns a Finding on success or an empty Finding + non-empty
// reason on failure. Order matters: cheap field checks first, line
// classification last (it requires diff lookups).
func validateOne(c model.FindingCandidate, modelName string, idx *diff.Index) (Finding, string) {
	if c.File == "" {
		return Finding{}, "file is empty"
	}
	if c.SuggestedComment == "" {
		return Finding{}, "suggested_comment is empty"
	}
	if c.Confidence < 0 || c.Confidence > 1 {
		return Finding{}, fmt.Sprintf("confidence %.3f is outside [0.0, 1.0]", c.Confidence)
	}
	sev, err := ParseSeverity(c.Severity)
	if err != nil {
		return Finding{}, err.Error()
	}

	switch idx.Classify(c.File, c.Line) {
	case diff.LineFileNotFound:
		return Finding{}, fmt.Sprintf("file %q is not in the diff (or its kind is not addressable: binary, mode-only, or pure-rename)", c.File)
	case diff.LineContext:
		return Finding{}, fmt.Sprintf("line %d is a context line, not an added or modified line", c.Line)
	case diff.LineOutsideHunk:
		return Finding{}, fmt.Sprintf("line %d is outside any hunk in %s", c.Line, c.File)
	case diff.LineAdded, diff.LineModified:
		// accepted
	default:
		return Finding{}, "unexpected line classification"
	}

	return Finding{
		File:             c.File,
		Line:             c.Line,
		Severity:         sev,
		Title:            c.Title,
		Evidence:         c.Evidence,
		SuggestedComment: c.SuggestedComment,
		FixHint:          c.FixHint,
		Confidence:       c.Confidence,
		Model:            modelName,
		State:            StatePending,
	}, ""
}
