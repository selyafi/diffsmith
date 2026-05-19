package review

// FindingCandidate is the transport-layer shape of a finding returned by
// a model CLI. Severity is kept as a string here; Validate converts it to
// a typed Severity.
//
// Field ordering and JSON tags mirror docs/review-finding-schema.md
// exactly so the same struct can be used for both encoding the schema
// example in the prompt and decoding the model's reply.
type FindingCandidate struct {
	File             string  `json:"file"`
	Line             int     `json:"line"`
	Severity         string  `json:"severity"`
	Title            string  `json:"title"`
	Evidence         string  `json:"evidence"`
	SuggestedComment string  `json:"suggested_comment"`
	FixHint          string  `json:"fix_hint"`
	Confidence       float64 `json:"confidence"`
}

// ModelReviewResult is what a model adapter returns after invocation.
// RawOutput preserves the model's stdout so the TUI's debug surface can
// show it when validation rejects everything.
type ModelReviewResult struct {
	Model     string
	Findings  []FindingCandidate
	RawOutput string
}
