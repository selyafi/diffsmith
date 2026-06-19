package app

import (
	"context"
	"testing"

	"github.com/selyafi/diffsmith/internal/model"
	"github.com/selyafi/diffsmith/internal/review"
)

// modelSettingFake implements model.ModelSetter and records SetModel calls.
type modelSettingFake struct {
	name     string
	gotModel string
}

func (f *modelSettingFake) Name() string                    { return f.name }
func (f *modelSettingFake) Preflight(context.Context) error { return nil }
func (f *modelSettingFake) Review(context.Context, *review.ReviewInput) (*review.ModelReviewResult, error) {
	return nil, nil
}
func (f *modelSettingFake) SetModel(name string) { f.gotModel = name }

var _ model.ModelSetter = (*modelSettingFake)(nil)

func TestApplyAntigravityModel_FlagWins(t *testing.T) {
	t.Setenv(antigravityModelEnv, "env-model")
	f := &modelSettingFake{name: "antigravity"}
	applyAntigravityModel(&model.SelectedModels{All: []model.Model{f}}, "flag-model")
	if f.gotModel != "flag-model" {
		t.Errorf("flag must win over env; got %q", f.gotModel)
	}
}

func TestApplyAntigravityModel_EnvFallback(t *testing.T) {
	t.Setenv(antigravityModelEnv, "env-model")
	f := &modelSettingFake{name: "antigravity"}
	applyAntigravityModel(&model.SelectedModels{All: []model.Model{f}}, "")
	if f.gotModel != "env-model" {
		t.Errorf("env must apply when flag is empty; got %q", f.gotModel)
	}
}

func TestApplyAntigravityModel_NoneIsNoOp(t *testing.T) {
	t.Setenv(antigravityModelEnv, "")
	f := &modelSettingFake{name: "antigravity"}
	applyAntigravityModel(&model.SelectedModels{All: []model.Model{f}}, "")
	if f.gotModel != "" {
		t.Errorf("no flag/env must be a no-op (keep adapter default); got %q", f.gotModel)
	}
}
