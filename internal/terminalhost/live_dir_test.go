//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	liveA = "reet-0123456789abcdef0123456789abcdef"
	liveB = "reet-fedcba9876543210fedcba9876543210"
)

func TestParseLiveDirsFirstPaneAndValidity(t *testing.T) {
	out := strings.Join([]string{
		liveA + "|/home/me/first",
		liveA + "|/home/me/second pane",
		liveB + "|relative/path",
		"other-session|/home/me/elsewhere",
		"",
	}, "\n")
	got := parseLiveDirs([]byte(out))
	want := map[string]string{liveA: "/home/me/first"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A tmux that prints a newline in a folder as it is splits the row: the row
// is skipped, and so is the line that continues it, while the rows around it
// are kept.
func TestLiveDirSkipsNewlinePath(t *testing.T) {
	out := liveA + "|/home/me/a\nb\n" + liveB + "|/home/me/fine\n"
	got := parseLiveDirs([]byte(out))
	want := map[string]string{liveB: "/home/me/fine"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A continuation that names a session is still read as a row of its own.
	got = parseLiveDirs([]byte(liveA + "|/home/me/x\n" + liveB + "|/home/me/y\n"))
	if got[liveA] != "/home/me/x" || got[liveB] != "/home/me/y" {
		t.Fatalf("plain rows: %v", got)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// typeInShell types a line into the session's shell and presses Enter.
func typeInShell(t *testing.T, m *Manager, id, line string) {
	t.Helper()
	if _, err := m.command(context.Background(), 0, "send-keys", "-t", "="+m.name(id)+":", "-l", line); err != nil {
		t.Fatal(err)
	}
	if _, err := m.command(context.Background(), 0, "send-keys", "-t", "="+m.name(id)+":", "Enter"); err != nil {
		t.Fatal(err)
	}
}

func waitLiveDir(t *testing.T, m *Manager, id, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		if err := m.Reconcile(context.Background()); err != nil {
			t.Fatal(err)
		}
		sessions, err := m.List(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range sessions {
			if s.ID == id {
				got = s.LiveDir
			}
		}
		if got == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("live folder = %q, want %q", got, want)
}

func TestLiveDirKeepsSpacesAndQuotes(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	dir := filepath.Join(opts.Home, `it's a "dir" | here`)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed := m.Changed()
	typeInShell(t, m, s.ID, "cd "+shellQuote(dir))
	waitLiveDir(t, m, s.ID, real)
	select {
	case <-changed:
	default:
		t.Fatal("a folder change did not wake Changed")
	}
}

// A failed folder read leaves the sweep, and the folder last read, alone.
func TestLiveDirFailureLeavesSweep(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	home, err := filepath.EvalSymlinks(opts.Home)
	if err != nil {
		t.Fatal(err)
	}
	waitLiveDir(t, m, s.ID, home)
	m.mu.Lock()
	m.readLiveDirs = func(context.Context) (map[string]string, error) {
		return map[string]string{}, errors.New("tmux refused")
	}
	m.mu.Unlock()
	if err := m.Reconcile(context.Background()); err != nil {
		t.Fatalf("sweep failed with the folder read: %v", err)
	}
	sessions, err := m.List(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].State != terminalproto.Running || sessions[0].LiveDir != home {
		t.Fatalf("after a failed read: %+v %v", sessions, err)
	}
}

// The folder a shell was in when it was ended is kept with the ended session.
func TestLiveDirReadWhenSessionEnds(t *testing.T) {
	m, opts := managerFixture(t)
	s := createManaged(t, m, 1)
	dir := filepath.Join(opts.Home, "last")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Change folder without a sweep: End must read it itself.
	m.mu.Lock()
	m.readLiveDirs = func(context.Context) (map[string]string, error) { return map[string]string{}, errors.New("not yet") }
	m.mu.Unlock()
	typeInShell(t, m, s.ID, "cd "+shellQuote(dir))
	deadline := time.Now().Add(5 * time.Second)
	for {
		dirs, err := m.liveDirs(context.Background())
		if err == nil && dirs[m.name(s.ID)] == real {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("shell did not change folder: %v %v", dirs, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.mu.Lock()
	m.readLiveDirs = nil
	m.mu.Unlock()
	ended, err := m.End(context.Background(), testRequestID(2), s.ID)
	if err != nil || ended.State.ReservesQuota() {
		t.Fatalf("end: %+v %v", ended, err)
	}
	sessions, err := m.List(context.Background())
	if err != nil || len(sessions) != 1 || sessions[0].LiveDir != real {
		t.Fatalf("ended session's folder: %+v %v", sessions, err)
	}
}
