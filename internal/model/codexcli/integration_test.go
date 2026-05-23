//go:build integration

// Package codexcli — opt-in live tests against the real codex CLI.
//
// Build tag: integration. Default `go test ./...` excludes these by
// design, since they hit the real model and cost real money. Run via:
//
//	go test -tags=integration ./internal/model/codexcli -run TestPromptInjectionLiveCodex -v
//
// Spike S10b (diffsmith-4ib) is the canonical home for these. See
// docs/dev-plan/spikes.md § S10b and docs/dev-plan/testing-strategy.md
// for the convention.
package codexcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/selyafi/diffsmith/internal/diff"
	"github.com/selyafi/diffsmith/internal/review"
)

// TestPromptInjectionLiveCodex runs the real codex CLI against each
// adversarial fixture from M3c (S10a) and asserts the response is both
// schema-valid (parses via model.ParseFindings without error) and
// structurally grounded (every finding's (file, line) maps to an Added
// or Modified line in the actual diff). It does NOT assert exact model
// wording — that would be flaky against any reasonable model.
//
// Side effect: each raw response is written to
//
//	testdata/findings/codex_<fixture-stem>.json
//
// so future hermetic tests (or release-prep regression runs) can replay
// the captured behavior without paying for another live call. The dir
// is created if missing.
//
// Cost: roughly one codex exec per fixture. As of writing there are 3
// fixtures, so a full run is 3 model calls. Cost varies with input size
// and account tier; budget tens of cents to single digits of dollars per
// full run depending on the codex pricing in effect.
func TestPromptInjectionLiveCodex(t *testing.T) {
	a := New(nil)
	if err := a.Preflight(context.Background()); err != nil {
		t.Skipf("codex preflight failed; skipping live test: %v", err)
	}

	outDir := filepath.Join("..", "..", "..", "testdata", "findings")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	fixtures := []string{
		"injection_escape_chars.diff",
		"injection_json_break.diff",
		"injection_unicode_control.diff",
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			input := loadFixture(t, name)

			result, err := a.Review(context.Background(), input)
			// Persist whatever we got before asserting, so a failed run
			// still produces a transcript the user can inspect.
			if result != nil {
				writeFindings(t, outDir, name, result.RawOutput)
			}
			if err != nil {
				t.Fatalf("a.Review against %s: %v", name, err)
			}

			// Schema validity is already proven by Review returning a
			// non-nil ModelReviewResult (it routes raw stdout through
			// model.ParseFindings, which errors on shape mismatches).
			// What remains: structural grounding.
			idx := diff.NewIndex(input.Files)
			for i, f := range result.Findings {
				cls := idx.Classify(f.File, f.Line)
				if cls != diff.LineAdded && cls != diff.LineModified {
					t.Errorf("finding[%d] (%s:%d) classified as %v; want Added or Modified — model returned an ungrounded location, which is the failure mode S10b watches for",
						i, f.File, f.Line, cls)
				}
			}
		})
	}
}

// loadFixture reads testdata/diffs/<name> at the repo root and builds a
// minimal ReviewInput suitable for handing to the real codex adapter.
// Target/Title/Author are synthetic since the fixture isn't from a real
// PR; the adversarial content lives in the diff itself.
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
		Target: review.ReviewTarget{
			URL: "fixture://" + name,
		},
		Title:   "S10b prompt-injection live smoke",
		Author:  "diffsmith-tests",
		Files:   files,
		RawDiff: string(raw),
	}
}

// writeFindings persists the model's raw response under
// testdata/findings/codex_<stem>.json. The .diff suffix on fixture
// names is stripped to keep the filenames readable; the codex_ prefix
// makes it easy to tell which model produced the capture once other
// adapters get their own live smokes.
func writeFindings(t *testing.T, outDir, fixtureName, body string) {
	t.Helper()
	stem := fixtureName
	if ext := filepath.Ext(stem); ext != "" {
		stem = stem[:len(stem)-len(ext)]
	}
	out := filepath.Join(outDir, "codex_"+stem+".json")
	if err := os.WriteFile(out, []byte(body), 0o644); err != nil {
		t.Errorf("write %s: %v", out, err)
		return
	}
	t.Logf("captured %d bytes -> %s", len(body), out)
}
