package app

import (
	"context"
	"fmt"

	"github.com/selyafi/diffsmith/internal/review"
)

// enrichWithContext populates the review input's acceptance criteria from
// the provider's linked issues (when the provider supports them) and bounds
// the context to budget. It implements the diffsmith-144 contract:
//
//   - When noContext is set, ALL context is stripped (description cleared,
//     criteria nil) and the fetcher is never called — nothing extra reaches
//     the model and no extra network call is made.
//   - Otherwise, linked issues are resolved (if fetcher is non-nil). Context
//     enrichment is never a gate: a total fetch failure becomes one note and
//     the review proceeds with no criteria. Non-fatal fetcher notes and
//     budget-cap truncations are passed through.
//
// Returns the notes to surface in the run summary (nil when nothing was
// noteworthy). The fetcher is the provider type-asserted to
// review.LinkedIssueFetcher by the caller (nil when the provider does not
// implement it).
func enrichWithContext(ctx context.Context, fetcher review.LinkedIssueFetcher, input *review.ReviewInput, noContext bool) []string {
	if noContext {
		input.Description = ""
		input.AcceptanceCriteria = nil
		return nil
	}

	var notes []string
	if fetcher != nil {
		issues, fetchNotes, err := fetcher.FetchLinkedIssues(ctx, input.Target)
		if err != nil {
			// Total failure is non-fatal: surface it and proceed with no
			// criteria rather than aborting the review.
			notes = append(notes, fmt.Sprintf("acceptance criteria unavailable: %v", err))
		} else {
			input.AcceptanceCriteria = issues
			notes = append(notes, fetchNotes...)
		}
	}

	// Bound description + criteria so enrichment can never bust the input
	// budget; CapContext returns a note for every truncation/drop.
	notes = append(notes, input.CapContext()...)
	return notes
}
