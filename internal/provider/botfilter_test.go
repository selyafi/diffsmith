package provider

import "testing"

func TestIsBotLogin(t *testing.T) {
	bots := []string{"copilot-pull-request-reviewer", "coderabbitai", "greptile-apps",
		"github-actions", "dependabot[bot]", "renovate-bot", "Copilot"}
	for _, b := range bots {
		if !IsBotLogin(b) {
			t.Errorf("IsBotLogin(%q) = false, want true", b)
		}
	}
	humans := []string{"Balvajs", "prathoss", "yung-madamm", "shelyafi", ""}
	for _, h := range humans {
		if IsBotLogin(h) {
			t.Errorf("IsBotLogin(%q) = true, want false", h)
		}
	}
}
