package cliui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

const outputLimit = 4 << 20

// RunOutput runs work that cannot prompt and keeps its output in the terminal
// interface. The caller writes both stdout and stderr to the writer; the view
// streams it, then stays open with the final exit status until the user
// returns home or quits.
func RunOutput(ctx context.Context, in io.Reader, out io.Writer, label string, work func(context.Context, io.Writer) (int, error)) (int, bool, error) {
	// The child has its own cancellable context. Cancelling it must keep the
	// renderer alive so the view can show the cancelled result and still let
	// the user return home.
	workCtx, cancelWork := context.WithCancel(ctx)
	defer cancelWork()
	buf := newOutputBuffer()
	done := make(chan outputResult, 1)
	finished := make(chan struct{})
	go func() {
		code, err := work(workCtx, buf)
		buf.close()
		close(finished)
		done <- outputResult{code: code, err: err}
	}()
	model := outputModel{label: label, width: 80, height: 24, follow: true, running: true, buf: buf, done: done, cancel: cancelWork}
	final, uiErr := runOutputProgram(ctx, in, out, model)
	select {
	case <-finished:
	default:
		cancelWork()
		if uiErr == nil {
			uiErr = errors.New("output renderer stopped before the command finished")
		}
	}
	<-finished
	return final.code, final.quit, uiErr
}

// runOutputProgram is replaceable so model tests do not need a terminal.
var runOutputProgram = func(ctx context.Context, in io.Reader, out io.Writer, m outputModel) (outputModel, error) {
	final, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out), tea.WithoutSignalHandler()).Run()
	if err != nil {
		return m, err
	}
	return final.(outputModel), nil
}

type outputResult struct {
	code int
	err  error
}

type outputUpdate struct{}
type outputDone struct{}

type outputBuffer struct {
	mu        sync.Mutex
	data      []byte
	truncated bool
	closed    bool
	changed   chan struct{}
	finished  chan struct{}
}

func newOutputBuffer() *outputBuffer {
	return &outputBuffer{changed: make(chan struct{}, 1), finished: make(chan struct{})}
}

func (b *outputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	if !b.closed {
		b.data = append(b.data, p...)
		if len(b.data) > outputLimit {
			b.data = b.data[len(b.data)-outputLimit:]
			if i := bytes.IndexByte(b.data, '\n'); i >= 0 {
				b.data = b.data[i+1:]
			}
			b.truncated = true
		}
	}
	b.mu.Unlock()
	select {
	case b.changed <- struct{}{}:
	default:
	}
	return len(p), nil
}

func (b *outputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(b.data)
	if b.truncated {
		text = "[earlier output truncated]\n" + text
	}
	return text
}

func (b *outputBuffer) close() {
	b.mu.Lock()
	if !b.closed {
		b.closed = true
		close(b.finished)
	}
	b.mu.Unlock()
}

type outputModel struct {
	label               string
	width, height       int
	offset              int
	follow              bool
	running, cancelling bool
	cancelled           bool
	quit, stop          bool
	code                int
	buf                 *outputBuffer
	done                <-chan outputResult
	cancel              context.CancelFunc
}

func (m outputModel) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return <-m.done },
		m.observe(),
	)
}

func (m outputModel) observe() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.buf.changed:
			return outputUpdate{}
		case <-m.buf.finished:
			return outputDone{}
		}
	}
}

func (m outputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = max(msg.Width, 1), max(msg.Height, 1)
	case outputUpdate:
		return m, m.observe()
	case outputDone:
		return m, nil
	case outputResult:
		m.running = false
		m.cancelling = false
		m.code = msg.code
		if msg.err != nil && !m.cancelled {
			m.code = 1
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q":
			if m.running {
				m.cancel()
			}
			m.quit, m.stop = true, true
			return m, tea.Quit
		case "ctrl+c":
			if m.running && !m.cancelling {
				m.cancel()
				m.cancelling = true
				m.cancelled = true
				return m, nil
			}
			m.quit, m.stop = true, true
			return m, tea.Quit
		case "esc", "enter":
			if m.running {
				if !m.cancelling {
					m.cancel()
					m.cancelling = true
					m.cancelled = true
				}
				return m, nil
			}
			m.stop = true
			return m, tea.Quit
		case "up", "k":
			m.follow = false
			m.offset++
		case "down", "j":
			m.offset--
		case "pgup":
			m.follow = false
			m.offset += m.bodyHeight()
		case "pgdown":
			m.offset -= m.bodyHeight()
		case "home", "g":
			m.follow = false
			m.offset = len(outputLines(m.buf.String(), m.textWidth()))
		case "end", "G":
			m.follow = true
			m.offset = 0
		}
	}
	if m.offset <= 0 {
		m.offset = 0
		m.follow = true
	}
	return m, nil
}

func (m outputModel) bodyHeight() int { return max(m.height-2, 1) }
func (m outputModel) textWidth() int  { return max(m.width-2, 1) }

func (m outputModel) View() tea.View {
	status, role := "Working…", "accent"
	if m.cancelling {
		status, role = "Cancelling…", "warn"
	} else if !m.running {
		switch {
		case m.cancelled:
			status, role = "Cancelled", "warn"
		case m.code != 0:
			status, role = "Failed (exit "+strconv.Itoa(m.code)+")", "fail"
		default:
			status, role = "Finished", "muted"
		}
	}
	statusWidth := ansi.StringWidth(status)
	header := "  " + fit(m.label, max(m.width-2-statusWidth-2, 1))
	header += strings.Repeat(" ", max(m.width-ansi.StringWidth(header)-statusWidth-1, 0)) + styled(status, role, Colour())

	lines := outputLines(m.buf.String(), m.textWidth())
	body := m.bodyHeight()
	maxOffset := max(len(lines)-body, 0)
	offset := min(max(m.offset, 0), maxOffset)
	end := len(lines) - offset
	start := max(end-body, 0)
	rows := make([]string, 0, body+1)
	for _, line := range lines[start:end] {
		rows = append(rows, fit("  "+line, m.width))
	}
	for len(rows) < body {
		rows = append(rows, "")
	}

	footer := "↑/↓ scroll   Enter / Esc home   q quit"
	if m.running {
		footer = "↑/↓ scroll   Esc / Ctrl+C cancel"
	}
	rows = append(rows, styled(fit("  "+footer, m.width), "muted", Colour()))

	view := tea.NewView(fit(header, m.width) + "\n" + strings.Join(rows, "\n"))
	view.AltScreen = true
	return view
}

// outputLines wraps plain output to the view width. Captured output comes from
// a non-terminal child, so escape sequences are removed rather than replayed.
func outputLines(s string, width int) []string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.ReplaceAll(s, "\t", "    ")
	if width < 1 || s == "" {
		return strings.Split(s, "\n")
	}
	return strings.Split(ansi.Wordwrap(s, width, ""), "\n")
}
