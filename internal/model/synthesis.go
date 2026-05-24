package model

import (
	"fmt"
	"strings"

	"github.com/selyafi/diffsmith/internal/review"
)

// BuildSynthesisPrompt constructs the synthesis prompt sent to the
// lead model. The lead receives the original diff plus per-model
// findings from other reviewers and is asked to dedupe, merge, drop
// false positives, and re-emit a unified findings set in its own
// voice. The output schema is the SAME as a normal review (an object
// with a "findings" array) so existing parser/validator paths apply
// unchanged.
func BuildSynthesisPrompt(input *review.ReviewInput, results []*review.ModelReviewResult) string {
	var b strings.Builder

	b.WriteString("You are the lead reviewer synthesizing findings from multiple AI reviewers on the same pull request.\n\n")
	b.WriteString("Each reviewer ran independently against the same diff. Your job:\n\n")
	b.WriteString("1. Dedupe overlapping findings (same file:line, same root cause → keep ONE).\n")
	b.WriteString("2. Drop findings that look like false positives, hallucinations, or suggestions that don't ground to the diff.\n")
	b.WriteString("3. Merge complementary findings (e.g., one says 'X is broken', another adds the fix 'do Y' → combine).\n")
	b.WriteString("4. Re-emit the surviving findings in your own voice: short, direct, evidence-grounded suggested comments.\n\n")
	b.WriteString("Output format: the same JSON schema as a normal review. An object with a \"findings\" array of {file, line, severity, title, evidence, suggested_comment, fix_hint, confidence} entries.\n\n")

	if input.Title != "" {
		fmt.Fprintf(&b, "== PR TITLE ==\n%s\n\n", input.Title)
	}
	if input.Author != "" {
		fmt.Fprintf(&b, "== PR AUTHOR ==\n%s\n\n", input.Author)
	}

	b.WriteString("== DIFF ==\n")
	b.WriteString(input.RawDiff)
	b.WriteString("\n\n")

	b.WriteString("== REVIEWER OUTPUTS ==\n\n")
	if len(results) == 0 {
		b.WriteString("(no reviewer output)\n\n")
	}
	for _, r := range results {
		fmt.Fprintf(&b, "Reviewer %q:\n%s\n\n", r.Model, r.RawOutput)
	}

	b.WriteString("Emit the unified findings JSON now. Do not include any prose outside the JSON.\n")
	return b.String()
}
