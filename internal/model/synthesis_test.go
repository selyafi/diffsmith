package model_test

import (
	"strings"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

func TestBuildSynthesisPrompt_IncludesAllReviewerNames(t *testing.T) {
	input := &review.ReviewInput{
		Target:  review.ReviewTarget{URL: "https://example/pr/1"},
		Title:   "test PR",
		Author:  "alice",
		RawDiff: "diff --git a/x b/x\n+foo",
	}
	results := []*review.ModelReviewResult{
		{Model: "codex", RawOutput: `{"findings":[{"file":"x","line":1,"severity":"low","title":"a","evidence":"e","suggested_comment":"c","fix_hint":"f","confidence":0.9}]}`},
		{Model: "claude", RawOutput: `{"findings":[]}`},
	}
	got := model.BuildSynthesisPrompt(input, results)

	if !strings.Contains(got, `Reviewer "codex"`) {
		t.Error("prompt should mention codex by name")
	}
	if !strings.Contains(got, `Reviewer "claude"`) {
		t.Error("prompt should mention claude by name")
	}
	if !strings.Contains(strings.ToLower(got), "dedup") {
		t.Error("prompt should instruct deduplication")
	}
	if !strings.Contains(got, input.RawDiff) {
		t.Error("prompt should include the diff for grounding verification")
	}
}

func TestBuildSynthesisPrompt_HandlesEmptyResults(t *testing.T) {
	input := &review.ReviewInput{Title: "t", RawDiff: "d"}
	got := model.BuildSynthesisPrompt(input, nil)
	if got == "" {
		t.Error("prompt should be non-empty even with no results")
	}
}
