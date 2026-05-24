package tui

import (
	"strings"
	"testing"
)

func TestLoader_RendersModelStatusSection(t *testing.T) {
	m := NewLoaderModel("Reviewing…")
	m.Update(ModelStatusMsg{Name: "codex", State: "running"})
	m.Update(ModelStatusMsg{Name: "claude", State: "done"})

	view := m.View()
	if !strings.Contains(view, "codex") {
		t.Errorf("view should show codex; got: %s", view)
	}
	if !strings.Contains(view, "running") {
		t.Errorf("view should show running; got: %s", view)
	}
	if !strings.Contains(view, "claude") || !strings.Contains(view, "done") {
		t.Errorf("view should show claude done; got: %s", view)
	}
}

func TestLoader_ModelStatusMsgUpdatesExisting(t *testing.T) {
	m := NewLoaderModel("Reviewing…")
	m.Update(ModelStatusMsg{Name: "codex", State: "running"})
	m.Update(ModelStatusMsg{Name: "codex", State: "done"})

	view := m.View()
	if !strings.Contains(view, "done") {
		t.Errorf("view should show updated state 'done'; got: %s", view)
	}
	if strings.Contains(view, "running") {
		t.Errorf("view should no longer show 'running' after update; got: %s", view)
	}
}
