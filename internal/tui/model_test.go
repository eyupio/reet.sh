package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eyupio/reet.sh/internal/lifecycle"
	"github.com/eyupio/reet.sh/internal/scan"
	"github.com/eyupio/reet.sh/internal/store"
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

func TestScanFormOpensAndEscapeReturnsHome(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenScan)
	m = next.(model)
	if m.page != ScreenScan || !m.formActive() || m.scanForm == nil {
		t.Fatalf("page=%q form=%v loading=%t", m.page, m.scanForm != nil, m.loading)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.page != ScreenHome {
		t.Fatalf("page = %q, want home", m.page)
	}
}

func TestScanResultRendersFindingsAndDryRun(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenScan)
	m = next.(model)
	next, cmd := m.startScan()
	m = next.(model)
	if !m.loading || cmd == nil {
		t.Fatal("scan did not start")
	}
	report := scan.Report{
		Host:   "laptop",
		DryRun: true,
		Found:  []scan.Found{{Name: "alpha", Source: "owner/repo", Agent: "claude"}},
		Envs:   []scan.ScannedEnv{{Host: "laptop", Agent: "claude", Path: ".claude/skills"}},
	}
	next, _ = m.Update(resultMsg{page: ScreenScan, value: report})
	m = next.(model)
	view := m.View()
	for _, want := range []string{"Scan", "alpha", "dry run", "1 skills found"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestScanDirsSplitsCommasAndNewlines(t *testing.T) {
	got := scanDirs(" /one, /two\n/three ,, ")
	want := []string{"/one", "/two", "/three"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
}

func TestSyncFormOpensAndEscapeReturnsHome(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenSync)
	m = next.(model)
	if m.page != ScreenSync || !m.formActive() || m.syncForm == nil {
		t.Fatalf("page=%q form=%v loading=%t", m.page, m.syncForm != nil, m.loading)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if m.page != ScreenHome {
		t.Fatalf("page = %q, want home", m.page)
	}
}

func TestSyncPreviewRendersAndApplyStarts(t *testing.T) {
	m := testModel()
	next, _ := m.open(ScreenSync)
	m = next.(model)
	next, _ = m.startSync(false)
	m = next.(model)
	report := scan.SyncReport{
		PlanSet: true,
		Hash:    "plan-hash",
		Plan: scan.SyncPlan{
			Directory: "/home/sam/.claude/skills",
			Revision:  "rev-one",
			Actions: []scan.SyncAction{{
				Add:   true,
				Skill: store.SyncSkill{Slug: "beta", Source: "owner/repo"},
			}},
		},
	}
	next, _ = m.Update(resultMsg{page: ScreenSync, value: syncOutcome{report: report}})
	m = next.(model)
	view := m.View()
	for _, want := range []string{"INSTALL", "beta", "Preview only", "a applies"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(model)
	if !m.loading || cmd == nil {
		t.Fatal("apply did not start")
	}
}

func TestSyncAutoApplyAfterPreview(t *testing.T) {
	m := newModel(Options{Version: "v1.2.3", Commit: "abcdef1", SyncApply: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = next.(model)
	next, _ = m.open(ScreenSync)
	m = next.(model)
	next, _ = m.startSync(false)
	m = next.(model)
	report := scan.SyncReport{
		PlanSet: true,
		Hash:    "plan-hash",
		Plan: scan.SyncPlan{Actions: []scan.SyncAction{{
			Add:   true,
			Skill: store.SyncSkill{Slug: "beta"},
		}}},
	}
	next, cmd := m.Update(resultMsg{page: ScreenSync, value: syncOutcome{report: report}})
	m = next.(model)
	if !m.loading || cmd == nil || m.opts.SyncApply {
		t.Fatalf("auto apply did not start: loading=%t cmd=%v apply=%t", m.loading, cmd != nil, m.opts.SyncApply)
	}
}
