package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eyupio/reet.sh/internal/lifecycle"
)

func testModel() model {
	m := newModel(Options{Version: "v1.2.3", Commit: "abcdef1234"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(model)
}

func TestHomeNavigationAndOpen(t *testing.T) {
	m := testModel()
	if m.page != ScreenHome {
		t.Fatalf("page = %q, want home", m.page)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(model)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cursor)
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.page != ScreenHealth || !m.loading || cmd == nil {
		t.Fatalf("page=%q loading=%t cmd=%v", m.page, m.loading, cmd != nil)
	}
}

func TestDoctorResultRendersChecks(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenDoctor)
	m = next.(model)
	if !m.loading {
		t.Fatal("doctor screen did not start loading")
	}
	report := lifecycle.Report{
		Version: "v1.2.3",
		Commit:  "abcdef1234",
		Checks: []lifecycle.Diagnostic{
			{Name: "binary", Status: "pass", Message: "Running /usr/bin/reet"},
			{Name: "database", Status: "warn", Message: "No database yet", Advice: "Start the server."},
		},
	}
	next, _ = m.Update(resultMsg{page: ScreenDoctor, value: report})
	m = next.(model)
	if m.loading {
		t.Fatal("doctor screen still loading")
	}
	view := m.View()
	for _, want := range []string{"[PASS]", "[WARN]", "binary", "No database yet", "Start the server."} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestEscapeReturnsHomeAndRefreshReloads(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenAgent)
	m = next.(model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.page != ScreenHome {
		t.Fatalf("page = %q, want home", m.page)
	}
	next, _ = m.open(ScreenDoctor)
	m = next.(model)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(model)
	if !m.loading || cmd == nil {
		t.Fatal("refresh did not start a load")
	}
}

func TestVersionScreenRendersWithoutLoading(t *testing.T) {
	m := newModel(Options{Version: "v1.2.3", Commit: "abcdef1234", Start: ScreenVersion})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	view := m.View()
	for _, want := range []string{"v1.2.3", "abcdef1234", "Platform"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
