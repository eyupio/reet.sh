package cliui

import (
	"bytes"
	"os"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}
func homeKey(m homeModel, s string) (homeModel, tea.Cmd) {
	next, cmd := m.Update(key(s))
	return next.(homeModel), cmd
}

func TestHomeNavigationAndSubmenusUseExistingHandlers(t *testing.T) {
	m := newHome("dev")
	m.colour = false
	if m.Init() != nil {
		t.Fatal("idle home must not schedule animation")
	}
	m, _ = homeKey(m, "j")
	m, _ = homeKey(m, "down")
	m, _ = homeKey(m, "k")
	if m.cursor != 1 {
		t.Fatal(m.cursor)
	}
	m, cmd := homeKey(m, "enter")
	if cmd == nil || !slices.Equal(m.choice, []string{"healthcheck"}) {
		t.Fatalf("choice=%v", m.choice)
	}
	m = newHome("dev")
	m.cursor = 3
	m, cmd = homeKey(m, "enter")
	if cmd != nil || len(m.stack) != 1 || m.title != "Backups" {
		t.Fatal("backup must open its actions")
	}
	m, cmd = homeKey(m, "enter")
	if cmd == nil || !slices.Equal(m.choice, []string{"backup", "list"}) {
		t.Fatal(m.choice)
	}
	m.choice = nil
	m, _ = homeKey(m, "esc")
	if m.title != "Home" || m.cursor != 3 {
		t.Fatal("Escape did not restore the menu selection")
	}
}

func TestHomeNeverSuppliesApprovalAndHelpCannotTakeABackup(t *testing.T) {
	var visit func([]Command)
	visit = func(commands []Command) {
		for _, c := range commands {
			for _, arg := range c.Args {
				if arg == "--yes" || arg == "--apply" {
					t.Fatalf("%s supplies consent", c.Name)
				}
			}
			visit(c.Children)
		}
	}
	visit(Commands())
	m := newHome("dev")
	m.cursor = 2
	m, cmd := homeKey(m, "enter")
	if cmd == nil || !slices.Equal(m.choice, []string{"upgrade"}) {
		t.Fatal("upgrade flags changed")
	}
	m = newHome("dev")
	m.cursor = 3
	m, _ = homeKey(m, "enter")
	m.cursor = 1
	m, cmd = homeKey(m, "?")
	if cmd == nil || !slices.Equal(m.choice, []string{"backup", "--help"}) {
		t.Fatal("help could execute backup run")
	}
}

func TestHomeSearchMatchesRegistryAndTypingQDoesNotQuit(t *testing.T) {
	m := newHome("dev")
	m, _ = homeKey(m, "/")
	for _, r := range "doctor" {
		m, _ = homeKey(m, string(r))
	}
	if len(m.items()) != 1 || m.items()[0].Name != "doctor" {
		t.Fatal(m.items())
	}
	m, cmd := homeKey(m, "enter")
	if cmd == nil || !slices.Equal(m.choice, []string{"doctor"}) {
		t.Fatal(m.choice)
	}
	m = newHome("dev")
	m, _ = homeKey(m, "/")
	m, cmd = homeKey(m, "q")
	if cmd != nil || m.query != "q" {
		t.Fatal("typing q closed search")
	}
	m, cmd = homeKey(m, "enter")
	if cmd != nil || m.choice != nil || !strings.Contains(m.View().Content, "Nowt matches") {
		t.Fatal("empty search ran a command")
	}
	m, _ = homeKey(m, "esc")
	if m.query != "" || m.searching {
		t.Fatal("Escape did not clear search")
	}
}

func TestArgumentEditingPreservesQuotedFlagsWithoutShellExpansion(t *testing.T) {
	want := []string{"--state-dir", "/tmp/reet state", "--version", "dev", "$(not-a-command)", "", "it's literal"}
	got, err := SplitArgs(QuoteArgs(want))
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("%q %v", got, err)
	}
	for _, invalid := range []string{"'unfinished", "trailing\\", "line\nbreak"} {
		if _, err := SplitArgs(invalid); err == nil {
			t.Fatal(invalid)
		}
	}
	m := newHome("dev")
	m.cursor = 2
	m, _ = homeKey(m, "e")
	m.input.SetValue("--version dev --state-dir '/tmp/reet state'")
	m, cmd := homeKey(m, "enter")
	if cmd == nil || !slices.Equal(m.choice, []string{"upgrade", "--version", "dev", "--state-dir", "/tmp/reet state"}) {
		t.Fatal(m.choice)
	}
	m = newHome("dev")
	m, _ = homeKey(m, "e")
	m.input.SetValue("'unfinished")
	m, cmd = homeKey(m, "enter")
	if cmd != nil || m.problem == "" {
		t.Fatal("invalid quoting ran a command")
	}
	m, _ = homeKey(m, "esc")
	if m.editing || m.choice != nil {
		t.Fatal("cancelled edit ran a command")
	}
}

func TestHomeResizesWithoutHidingTheSelectionOrQuit(t *testing.T) {
	for _, size := range [][2]int{{12, 4}, {24, 8}, {40, 18}, {64, 24}, {80, 30}, {140, 45}} {
		m := newHome("dev")
		m.colour = false
		m.cursor = len(m.entries) - 1
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(homeModel)
		view := m.View().Content
		rows := strings.Split(view, "\n")
		if len(rows) > size[1] {
			t.Fatalf("%v too tall: %d", size, len(rows))
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > size[0] {
				t.Fatalf("%v wide row %q", size, row)
			}
		}
		if strings.Count(view, Tagline) > 1 {
			t.Fatal("repeated tagline")
		}
		if size[0] >= 24 && size[1] >= 8 && !strings.Contains(view, "› Help") {
			t.Fatalf("%v missing selected command: %s", size, view)
		}
		if size[0] < 24 {
			m, cmd := homeKey(m, "enter")
			if cmd != nil || m.choice != nil {
				t.Fatal("hidden selection ran")
			}
		}
	}
}

func TestCommandReferenceAndHelpComeFromTheCatalog(t *testing.T) {
	var help, reference bytes.Buffer
	Help(&help)
	Reference(&reference)
	for _, c := range Commands() {
		if !strings.Contains(help.String(), c.Name) || !strings.Contains(reference.String(), "## reet "+c.Name) {
			t.Fatal(c.Name)
		}
	}
	data, err := os.ReadFile("../../docs/cli-reference.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != reference.String() {
		t.Fatal("run go run ./cmd/command-reference > docs/cli-reference.md")
	}
}

func TestArgumentEditorKeepsItsControlsVisibleAfterNarrowResize(t *testing.T) {
	m := newHome("dev")
	m.cursor = 2
	m, _ = homeKey(m, "e")
	m.input.SetValue("--state-dir '/tmp/a much longer path' --version dev")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 28, Height: 8})
	m = next.(homeModel)
	view := m.View().Content
	if !strings.Contains(view, "Enter run   Esc cancel") {
		t.Fatal(view)
	}
	if m.input.Width()+len(m.input.Prompt) > 28 {
		t.Fatal("editor cursor could extend outside the terminal")
	}
}

func TestOutputWaitsForAnExplicitReturnOrQuit(t *testing.T) {
	m := pauseModel{code: 7, width: 40}
	if !strings.Contains(m.View().Content, "exit 7") {
		t.Fatal("failure outcome lost")
	}
	next, cmd := m.Update(key("j"))
	if cmd != nil || next.(pauseModel).quit {
		t.Fatal("output disappeared on an unrelated key")
	}
	next, cmd = m.Update(key("esc"))
	if cmd == nil || next.(pauseModel).quit {
		t.Fatal("Escape must return home")
	}
	next, cmd = m.Update(key("q"))
	if cmd == nil || !next.(pauseModel).quit {
		t.Fatal("q must exit")
	}
}
