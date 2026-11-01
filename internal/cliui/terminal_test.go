//go:build linux || darwin

package cliui

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func terminalFixture(t *testing.T) (*os.File, *os.File, <-chan string) {
	t.Helper()
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	t.Setenv("REET_NON_INTERACTIVE", "")
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	if err = pty.Setsize(slave, &pty.Winsize{Rows: 30, Cols: 80}); err != nil {
		t.Fatal(err)
	}
	chunks := make(chan string, 128)
	go func() {
		defer close(chunks)
		buffer := make([]byte, 8192)
		for {
			n, err := master.Read(buffer)
			if n > 0 {
				chunks <- string(buffer[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	return master, slave, chunks
}
func terminalUntil(t *testing.T, chunks <-chan string, want string) {
	t.Helper()
	var text strings.Builder
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case part, ok := <-chunks:
			if !ok {
				t.Fatalf("terminal closed before %q: %s", want, text.String())
			}
			text.WriteString(part)
			if strings.Contains(text.String(), want) {
				return
			}
		case <-timeout.C:
			t.Fatalf("terminal did not show %q: %s", want, text.String())
		}
	}
}

func TestRealTerminalCancellationWaitsForCleanupAndRestoresMode(t *testing.T) {
	master, slave, chunks := terminalFixture(t)
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	cleanup := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- Task(context.Background(), slave, slave, "Checking fixture", TaskOptions{}, func(ctx context.Context, observe func(string)) error {
			observe("Waiting for cancellation")
			<-ctx.Done()
			<-cleanup
			return ctx.Err()
		})
	}()
	terminalUntil(t, chunks, "Checking fixture")
	if _, err = master.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}
	terminalUntil(t, chunks, "waiting for cleanup")
	select {
	case err := <-result:
		t.Fatal("returned before cleanup", err)
	default:
	}
	close(cleanup)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cleanup did not finish")
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("terminal mode not restored", err)
	}
}

func TestRealHomeSearchReturnsSelectionWithTerminalReleased(t *testing.T) {
	master, slave, chunks := terminalFixture(t)
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		args []string
		err  error
	}
	done := make(chan result, 1)
	go func() { args, err := Choose(context.Background(), slave, slave, "fixture"); done <- result{args, err} }()
	terminalUntil(t, chunks, "Doctor")
	if _, err = master.Write([]byte("/version\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if r.err != nil || !reflect.DeepEqual(r.args, []string{"version"}) {
			t.Fatal(r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("home did not accept input")
	}
	after, err := term.GetState(int(slave.Fd()))
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("home retained terminal mode", err)
	}
}
