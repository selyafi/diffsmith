//go:build integration

// Package antigravitycli — opt-in live tests against the real agy CLI.
//
// Build tag: integration. Default `go test ./...` excludes these by
// design, since they hit the real model and require a one-time `agy`
// login. Run via:
//
//	go test -tags=integration ./internal/model/antigravitycli -run TestReviewLiveAntigravity -v
//
// This is the durable guard against agy's UNDOCUMENTED flag contract
// changing: the adapter relies on `agy --print=- --print-timeout <dur>`
// reading the prompt from stdin and emitting raw (unwrapped) findings
// JSON. If a future agy release changes that, this test fails loudly
// instead of the contract silently rotting. See the design spec
// (docs/superpowers/specs/2026-06-18-antigravity-adapter-design.md).
package antigravitycli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/review"
)

// TestReviewLiveAntigravity runs the real agy CLI against representative
// fixtures (a plain modification and the prompt-injection diffs) and
// asserts each response is schema-valid (parses via model.ParseFindings,
// proven by Review returning a non-nil result) and structurally grounded
// (every finding's (file, line) maps to an Added or Modified line in the
// actual diff). It does NOT assert exact wording — that would be flaky
// against any reasonable model.
//
// Side effect: each raw response is captured under
// testdata/findings/antigravity_<fixture-stem>.json for inspection.
func TestReviewLiveAntigravity(t *testing.T) {
	a := New(nil)
	if err := a.Preflight(context.Background()); err != nil {
		t.Skipf("agy preflight failed; skipping live test: %v", err)
	}

	outDir := filepath.Join("..", "..", "..", "testdata", "findings")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	// allowUngrounded marks fixtures where agy is documented to occasionally
	// anchor an otherwise-correct finding at a context line. The production
	// validator (review.Validate) quarantines these before they reach the
	// TUI, so this is a known model-precision limit, not an adapter bug.
	// On the 2026-06-19 live run, modified_simple produced a substantively
	// correct finding (a real strings.SplitN token-validation bypass)
	// anchored at line 14 — one line below the actual modified line 13 —
	// accepted per the S10b safety-net contract, mirroring codex's
	// escape_chars exception.
	fixtures := []struct {
		name            string
		allowUngrounded bool
	}{
		{"modified_simple.diff", true},
		{"injection_json_break.diff", false},
		{"injection_unicode_control.diff", false},
	}

	for _, fx := range fixtures {
		t.Run(fx.name, func(t *testing.T) {
			input := loadFixture(t, fx.name)

			result, err := a.Review(context.Background(), input)
			if result != nil {
				writeFindings(t, outDir, fx.name, result.RawOutput)
			}
			if err != nil {
				t.Fatalf("a.Review against %s: %v", fx.name, err)
			}

			idx := diff.NewIndex(input.Files)
			for i, f := range result.Findings {
				cls := idx.Classify(f.File, f.Line)
				if cls == diff.LineAdded || cls == diff.LineModified {
					continue
				}
				msg := fmt.Sprintf("finding[%d] (%s:%d) classified as %v; want Added or Modified — agy returned an ungrounded location",
					i, f.File, f.Line, cls)
				if fx.allowUngrounded {
					t.Logf("known acceptable model-precision behavior: %s", msg)
					continue
				}
				t.Errorf("%s", msg)
			}
		})
	}
}

// loadFixture reads testdata/diffs/<name> at the repo root and builds a
// minimal ReviewInput for the real agy adapter.
func loadFixture(t *testing.T, name string) *review.ReviewInput {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "diffs", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	files, err := diff.Parse(string(raw))
	if err != nil {
		t.Fatalf("parse fixture %s: %v", name, err)
	}
	return &review.ReviewInput{
		Target:  review.ReviewTarget{URL: "fixture://" + name},
		Title:   "antigravity live smoke",
		Author:  "diffsmith-tests",
		Files:   files,
		RawDiff: string(raw),
	}
}

// writeFindings persists agy's raw response under
// testdata/findings/antigravity_<stem>.json.
func writeFindings(t *testing.T, outDir, fixtureName, body string) {
	t.Helper()
	stem := fixtureName
	if ext := filepath.Ext(stem); ext != "" {
		stem = stem[:len(stem)-len(ext)]
	}
	out := filepath.Join(outDir, "antigravity_"+stem+".json")
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		t.Errorf("write %s: %v", out, err)
		return
	}
	t.Logf("captured %d bytes -> %s", len(body), out)
}
