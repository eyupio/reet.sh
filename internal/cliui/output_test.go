package cliui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestOutputLinesWrapStripAndIndent(t *testing.T) {
	lines := outputLines("\x1b[31mone\x1b[0m\r\ntwo\tthree\n", 8)
	for _, line := range lines {
		if ansi.StringWidth(line) > 8 {
			t.Fatalf("%q is wider than 8", line)
		}
		if strings.ContainsAny(line, "\x1b\r") {
			t.Fatalf("escape or carriage return survived: %q", line)
		}
	}
	if !strings.Contains(strings.Join(lines, "\n"), "one") {
		t.Fatal(lines)
	}
}

func TestOutputModelShowsStreamedOutputAndFinalStatus(t *testing.T) {
	buf := newOutputBuffer()
	buf.Write([]byte("checking binary\n"))
	m := outputModel{label: "reet doctor", width: 60, height: 12, running: true, follow: true, buf: buf}
	m, _ = updateOutput(m, tea.WindowSizeMsg{Width: 60, Height: 12})
	view := m.View().Content
	if !strings.Contains(view, "reet doctor") || !strings.Contains(view, "Working") || !strings.Contains(view, "checking binary") {
		t.Fatal(view)
	}
	m, _ = updateOutput(m, outputResult{code: 3})
	view = m.View().Content
	if !strings.Contains(view, "Failed (exit 3)") || !strings.Contains(view, "Enter / Esc home") {
		t.Fatal(view)
	}
	m, _ = updateOutput(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if !m.stop || m.quit {
		t.Fatalf("finished output did not return home on Escape: stop=%v quit=%v", m.stop, m.quit)
	}
	m, _ = updateOutput(m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !m.stop || !m.quit {
		t.Fatal("q did not ask to exit home")
	}
}

func TestOutputScrollKeepsEarlierLinesVisible(t *testing.T) {
	buf := newOutputBuffer()
	for i := 0; i < 30; i++ {
		buf.Write([]byte("line\n"))
	}
	m := outputModel{label: "x", width: 30, height: 8, running: true, follow: true, buf: buf}
	m, _ = updateOutput(m, tea.WindowSizeMsg{Width: 30, Height: 8})
	if !strings.Contains(m.View().Content, "line") {
		t.Fatal("no output")
	}
	m, _ = updateOutput(m, tea.KeyPressMsg{Code: tea.KeyUp})
	m, _ = updateOutput(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.follow || m.offset != 2 {
		t.Fatalf("offset=%d follow=%v", m.offset, m.follow)
	}
	m, _ = updateOutput(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if !m.follow || m.offset != 0 {
		t.Fatalf("end did not follow output: offset=%d follow=%v", m.offset, m.follow)
	}
}

func updateOutput(m outputModel, msg tea.Msg) (outputModel, tea.Cmd) {
	next, cmd := m.Update(msg)
	return next.(outputModel), cmd
}
