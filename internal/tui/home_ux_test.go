package tui

import (
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/scan"
)

func TestHomeSearchFiltersEntries(t *testing.T) {
	m := newModel(Options{Start: ScreenHome, NoColor: true})
	m.width, m.height = 100, 30
	m.filter = "sync"
	if v := m.visibleHome(); len(v) != 1 || homeItems[v[0]].id != ScreenSync {
		t.Fatalf("visible = %v", v)
	}
	out := m.renderHome()
	if !strings.Contains(out, "Nowt is touched") || !strings.Contains(out, "Ey up") {
		t.Fatalf("home view missing detail pane:\n%s", out)
	}
}

func TestSyncTargetsReplaceTypedIDWithPicker(t *testing.T) {
	m := newModel(Options{Start: ScreenSync, SyncServer: "https://reet.example"})
	m.width, m.height = 100, 30
	updated, _ := m.Update(syncTargetsMsg{choices: []scan.TargetChoice{{ID: "t2", Label: "codex (ns1)"}}})
	got := updated.(model)
	if got.syncValues.Target != "t2" || len(got.syncValues.Choices) != 1 {
		t.Fatalf("values = %+v", got.syncValues)
	}
}
