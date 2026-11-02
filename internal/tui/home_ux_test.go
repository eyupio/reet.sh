package tui

import (
	"strings"
	"testing"
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
