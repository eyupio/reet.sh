package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const testDeskPane = "0123456789abcdef"

// A desk pane's folder may lie outside home: any real directory this user can
// enter, with links followed.
func TestResolveDeskDirOutsideHome(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(root, "srv", "app")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(outside)
	for _, dir := range []string{outside, outside + "/", link} {
		got, err := ResolveDeskDir(dir)
		if err != nil || got != real {
			t.Fatalf("%q: %q %v", dir, got, err)
		}
	}
	if got, err := ResolveDeskDir("/"); err != nil || got != "/" {
		t.Fatalf("root: %q %v", got, err)
	}
}

func TestResolveDeskDirRefusesFile(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(root, "to-file")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{file, filepath.Join(root, "to-file"), "relative", ""} {
		if got, err := ResolveDeskDir(dir); !errors.Is(err, ErrFolderGone) || !errors.Is(err, ErrLaunchUnavailable) {
			t.Fatalf("%q: %q %v", dir, got, err)
		}
	}
}

func TestResolveDeskDirMissing(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "gone"), filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(root, "gone"), filepath.Join(root, "dangling")} {
		if _, err := ResolveDeskDir(dir); !errors.Is(err, ErrFolderGone) {
			t.Fatalf("%q: %v", dir, err)
		}
	}
	if os.Geteuid() == 0 {
		return // root enters every folder
	}
	locked := filepath.Join(root, "locked")
	if err := os.Mkdir(locked, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDeskDir(locked); !errors.Is(err, ErrFolderGone) {
		t.Fatalf("a folder this user cannot enter: %v", err)
	}
}

// A desk create with Desk.Dir starts the shell in that folder even outside
// home; the same folder as plain Dir keeps the home rule. A folder that is
// gone is refused as launch_failed with ErrFolderGone, leaving nothing.
func TestManagerDeskCreateOutsideHome(t *testing.T) {
	m, _ := managerFixture(t)
	calls := recordingTmux(t, m)
	outside := shortTempDir(t)
	real, _ := filepath.EvalSymlinks(outside)
	plain := terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Dir: outside, Desk: terminalproto.DeskRef{Pane: testDeskPane}}
	if _, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), plain); !errors.Is(err, ErrLaunchUnavailable) || errors.Is(err, ErrFolderGone) {
		t.Fatalf("plain folder outside home: %v", err)
	}
	gone := terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: testDeskPane, Dir: filepath.Join(outside, "gone")}}
	if _, err := m.Create(context.Background(), testRequestID(2), testSessionID(2), gone); !errors.Is(err, ErrFolderGone) || LaunchResultCode(err) != terminalproto.LaunchFailed {
		t.Fatalf("missing desk folder: %v", err)
	}
	if created := newSessionCalls(calls()); len(created) != 0 {
		t.Fatalf("a refused create reached tmux: %q", created)
	}
	in := terminalproto.Create{Title: "a", Cols: 80, Rows: 24, Desk: terminalproto.DeskRef{Pane: testDeskPane, Dir: outside}}
	s, err := m.Create(context.Background(), testRequestID(3), testSessionID(3), in)
	if err != nil || s.State != terminalproto.Running {
		t.Fatal(s, err)
	}
	created := newSessionCalls(calls())
	if len(created) != 1 || !containsPair(created[0], "-c", real) {
		t.Fatalf("new-session: %q", created)
	}
	// A retry of the same create is a lookup, not a second shell.
	if again, err := m.Create(context.Background(), testRequestID(3), testSessionID(3), in); err != nil || again.ID != s.ID || len(newSessionCalls(calls())) != 1 {
		t.Fatal("retry", again, err)
	}
}

func containsPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
