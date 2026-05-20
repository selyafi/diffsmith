// Package post submits approved review findings back to the upstream
// hosting service as inline PR review comments.
package post

import (
	"fmt"
	"strings"

	"github.com/selyafi/diffsmith/internal/review"
)

// formatBody renders a validated Finding as the Markdown body of a single
// GitHub inline review comment. The file and line are intentionally not
// repeated here — the surrounding GraphQL mutation already anchors the
// comment to (path, line), so duplicating them would just be noise.
func formatBody(f review.Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**[%s] %s**\n\n", f.Severity, f.Title)
	fmt.Fprintf(&b, "%s\n", f.SuggestedComment)
	if f.Evidence != "" {
		fmt.Fprintf(&b, "\nEvidence:\n```\n%s\n```\n", f.Evidence)
	}
	if f.FixHint != "" {
		fmt.Fprintf(&b, "\nSuggested fix:\n```\n%s\n```\n", f.FixHint)
	}
	return b.String()
}
