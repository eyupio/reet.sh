package cliui

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRendererFailureCancelsAndJoinsTheOperationWithoutRetry(t *testing.T) {
	var out bytes.Buffer
	var calls atomic.Int32
	cleaned := false
	want := errors.New("terminal disconnected")
	err := runTask(context.Background(), &out, "Backup", true, func(ctx context.Context, observe func(string)) error {
		calls.Add(1)
		observe("Copying committed WAL data")
		<-ctx.Done()
		out.WriteString("cleanup finished\n")
		cleaned = true
		return ctx.Err()
	}, func(taskModel) error { return want })
	if calls.Load() != 1 || !cleaned || !errors.Is(err, want) || !errors.Is(err, context.Canceled) || !strings.Contains(out.String(), "cleanup finished") {
		t.Fatalf("calls=%d cleaned=%v err=%v output=%s", calls.Load(), cleaned, err, out.String())
	}
}

func TestCompletionReturnsTheOriginalFailureWithoutWaitingForATick(t *testing.T) {
	var out bytes.Buffer
	want := errors.New("checksum mismatch")
	err := runTask(context.Background(), &out, "Verify", true, func(context.Context, func(string)) error { return want }, func(m taskModel) error {
		result := <-m.done
		next, cmd := m.Update(result)
		if cmd == nil || next.(taskModel).View().Content != "" {
			t.Fatal("completion did not clear progress immediately")
		}
		return nil
	})
	if !errors.Is(err, want) || strings.Contains(out.String(), "Finished:") {
		t.Fatalf("err=%v output=%s", err, out.String())
	}
}

func TestCancellationKeepsTheRendererUntilWorkHasFinished(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := taskModel{label: "Backup", width: 50, cancel: cancel, notes: &observations{}, start: time.Now(), now: time.Now()}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(taskModel)
	if cmd != nil || !m.cancelling || ctx.Err() == nil || !strings.Contains(m.View().Content, "waiting for cleanup") {
		t.Fatal("cancel quit before cleanup")
	}
}

func TestProgressFitsNarrowTerminalsAndReducedMotionHasNoFrames(t *testing.T) {
	start := time.Unix(100, 0)
	for _, motion := range []bool{true, false} {
		m := taskModel{label: "Checking installation", start: start, now: start.Add(3 * time.Second), width: 28, motion: motion, notes: &observations{latest: "latest observation"}}
		view := m.View().Content
		if !strings.Contains(view, "3s") || !strings.Contains(view, "latest observation") {
			t.Fatal(view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 28 {
				t.Fatal(line)
			}
		}
		next, cmd := m.Update(taskTick(start.Add(4 * time.Second)))
		if !motion && (cmd != nil || next.(taskModel).frame != 0 || strings.Contains(view, "⠋")) {
			t.Fatal("reduced motion animated")
		}
	}
	for width := 1; width < 20; width++ {
		if ansi.StringWidth(fit("日本語 é", width)) > width {
			t.Fatal(width)
		}
	}
}

func TestCancelledTaskDoesNotStartWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Task(ctx, strings.NewReader(""), &bytes.Buffer{}, "Upgrade", TaskOptions{}, func(context.Context, func(string)) error { t.Fatal("started after cancellation"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
