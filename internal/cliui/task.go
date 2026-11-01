package cliui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
)

type TaskOptions struct{ Disabled, NoAnimation bool }
type Work func(context.Context, func(string)) error

// Task owns the terminal only for work that cannot prompt or start a child.
// Its operation starts once. A renderer failure cancels and joins that same
// operation, allowing deferred cleanup and recovery to finish before returning.
func Task(ctx context.Context, in io.Reader, out io.Writer, label string, opts TaskOptions, work Work) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Disabled || !Interactive(in, out) {
		return work(ctx, func(string) {})
	}
	return runTask(ctx, out, label, !opts.NoAnimation && Motion(), work, func(m taskModel) error {
		_, err := tea.NewProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithoutSignalHandler(), tea.WithFPS(20)).Run()
		return err
	})
}

// Step reports stages that must run with the terminal released: child programs
// and binary replacement. It never starts a renderer or consumes input.
func Step(ctx context.Context, in io.Reader, out io.Writer, label string, opts TaskOptions, work func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if opts.Disabled || !Interactive(in, out) {
		return work(ctx)
	}
	start := time.Now()
	if _, err := fmt.Fprintf(out, "  · %s\n", label); err != nil {
		return err
	}
	err := work(ctx)
	return errors.Join(err, taskOutcome(out, label, time.Since(start), err))
}

func taskOutcome(out io.Writer, label string, elapsed time.Duration, err error) error {
	status, role := "Finished", "muted"
	if err != nil {
		status, role = "Failed", "fail"
	}
	if errors.Is(err, context.Canceled) {
		status, role = "Cancelled", "warn"
	}
	_, writeErr := fmt.Fprintf(out, "  %s: %s (%s)\n", styled(status, role, Colour()), label, elapsed.Round(100*time.Millisecond))
	return writeErr
}

type taskResult struct{ err error }
type taskObservation struct{}
type taskCancelled struct{}
type taskTick time.Time

type observations struct {
	sync.Mutex
	latest  string
	changed chan struct{}
}

func (o *observations) add(s string) {
	o.Lock()
	o.latest = plain(strings.TrimSpace(s))
	o.Unlock()
	select {
	case o.changed <- struct{}{}:
	default:
	}
}
func (o *observations) read() string { o.Lock(); defer o.Unlock(); return o.latest }

func runTask(ctx context.Context, out io.Writer, label string, motion bool, work Work, render func(taskModel) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan taskResult, 1)
	finished := make(chan struct{})
	notes := &observations{changed: make(chan struct{}, 1)}
	start := time.Now()
	var result taskResult
	go func() {
		result.err = work(ctx, notes.add)
		close(finished)
		done <- result
	}()
	uiErr := render(taskModel{label: label, start: start, now: start, width: 80, motion: motion, colour: Colour(), done: done, notes: notes, cancel: cancel, cancelled: ctx.Done()})
	select {
	case <-finished:
	default:
		cancel()
		if uiErr == nil {
			uiErr = errors.New("progress renderer stopped before the operation finished")
		}
	}
	<-finished
	err := errors.Join(result.err, uiErr)
	return errors.Join(err, taskOutcome(out, label, time.Since(start), err))
}

type taskModel struct {
	label                                string
	start, now                           time.Time
	frame, width                         int
	motion, colour, finished, cancelling bool
	done                                 <-chan taskResult
	notes                                *observations
	cancel                               context.CancelFunc
	cancelled                            <-chan struct{}
}

func tickTask() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return taskTick(t) })
}
func (m taskModel) observe() tea.Cmd {
	return func() tea.Msg {
		select {
		case <-m.notes.changed:
			return taskObservation{}
		case <-m.cancelled:
			return taskCancelled{}
		}
	}
}
func (m taskModel) Init() tea.Cmd {
	cmds := []tea.Cmd{func() tea.Msg { return <-m.done }, m.observe()}
	if m.motion {
		cmds = append(cmds, tickTask())
	}
	return tea.Batch(cmds...)
}
func (m taskModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, 1)
	case taskTick:
		if m.motion && !m.finished {
			m.frame++
			m.now = time.Time(msg)
			return m, tickTask()
		}
	case taskObservation:
		m.now = time.Now()
		return m, m.observe()
	case taskCancelled:
		m.cancelling = true
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" || msg.String() == "esc" {
			m.cancel()
			m.cancelling = true
		}
	case taskResult:
		m.finished = true
		return m, tea.Quit
	}
	return m, nil
}
func (m taskModel) View() tea.View {
	if m.finished {
		return tea.NewView("")
	}
	marker := "·"
	if m.motion && !m.cancelling {
		marker = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}[m.frame%10]
	}
	label := m.label
	if m.cancelling {
		label = "Cancelling; waiting for cleanup"
	}
	elapsed := m.now.Sub(m.start).Truncate(time.Second).String()
	left := "  " + styled(marker, "accent", m.colour) + " " + styled(label, "bold", m.colour)
	row := pad(fit(left, max(m.width-len(elapsed)-2, 1)), max(m.width-len(elapsed)-1, 0)) + " " + elapsed
	text := fit(row, m.width)
	if last := m.notes.read(); last != "" {
		text += "\n" + fit("    "+styled(last, "muted", m.colour), m.width)
	}
	return tea.NewView(text)
}
