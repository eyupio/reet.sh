//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestCommandHistoryParsesSavedBashAndZshWithoutChangingFiles(t *testing.T) {
	home := t.TempDir()
	bash := "#1700000000\nprintf 'first'\nprintf 'second'\n#1700000001\n  df -h  \n"
	zsh := ": 1700000002:0;git status\n: 1700000003:1;printf first\\\nprintf second\n"
	for name, text := range map[string]string{".bash_history": bash, ".zsh_history": zsh} {
		if err := os.WriteFile(filepath.Join(home, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	history := readShellHistory(context.Background(), home)
	if history.Validate() != nil || len(history.Entries) != 4 {
		t.Fatal(history)
	}
	want := []string{"printf first\nprintf second", "git status", "  df -h  ", "printf 'first'\nprintf 'second'"}
	for i, command := range want {
		if history.Entries[i].Command != command {
			t.Fatalf("entry %d: %q", i, history.Entries[i].Command)
		}
	}
	if history.Entries[0].Timestamp != 1700000003000 {
		t.Fatal(history.Entries[0])
	}
	for name, text := range map[string]string{".bash_history": bash, ".zsh_history": zsh} {
		got, err := os.ReadFile(filepath.Join(home, name))
		if err != nil || string(got) != text {
			t.Fatal("history was modified", err)
		}
	}
}

func TestCommandHistoryRejectsLinkedWritableAndNonregularFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable", "directory", "foreign"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			path := filepath.Join(home, ".bash_history")
			target := filepath.Join(t.TempDir(), "other-file")
			if err := os.WriteFile(target, []byte("must-not-read"), 0600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(target, path); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.WriteFile(path, []byte("unsafe"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			case "foreign":
				if os.Geteuid() != 0 {
					t.Skip("changing ownership requires root")
				}
				if err := os.WriteFile(path, []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chown(path, 65534, 65534); err != nil {
					t.Fatal(err)
				}
			}
			got := readShellHistory(context.Background(), home)
			if len(got.Entries) != 0 || len(got.Sources) != 2 || got.Sources[0].Available {
				t.Fatal("unsafe history was read", got)
			}
		})
	}
}

func TestCommandHistoryBoundsAndUnsupportedText(t *testing.T) {
	home := t.TempDir()
	text := strings.Repeat("echo earlier\n", 30000) + "echo latest\n"
	if err := os.WriteFile(filepath.Join(home, ".bash_history"), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	got := readShellHistory(context.Background(), home)
	encoded, err := terminalproto.EncodeCommandHistory(got)
	if err != nil || !got.Truncated || len(got.Entries) != 500 || len(encoded) > terminalproto.MaxCommandHistoryBytes || got.Entries[0].Command != "echo latest" {
		t.Fatal(len(got.Entries), len(encoded), err)
	}
	if err = os.WriteFile(filepath.Join(home, ".bash_history"), []byte("valid command\nbad\x1b[2J\n"+strings.Repeat("x", 16385)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got = readShellHistory(context.Background(), home)
	if len(got.Entries) != 1 || got.Skipped != 2 {
		t.Fatal(got)
	}
}

func TestCommandHistoryRequiresLocalConsentAndAnOwnedRunningRecord(t *testing.T) {
	m, opts := managerFixture(t)
	defer m.Close()
	session, err := m.Create(context.Background(), testRequestID(1), testSessionID(1), terminalproto.Create{Title: "History", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(opts.Home, ".bash_history"), []byte("df -h\n"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := m.CommandHistory(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	history, err := terminalproto.DecodeCommandHistory(data)
	if err != nil || len(history.Entries) != 1 {
		t.Fatal(history, err)
	}
	if _, err = m.CommandHistory(context.Background(), testSessionID(2)); !errors.Is(err, ErrSessionMissing) {
		t.Fatal("unknown session read history", err)
	}
	m.mu.Lock()
	m.consent = func() error { return ErrTerminalDisabled }
	m.mu.Unlock()
	if _, err = m.CommandHistory(context.Background(), session.ID); !errors.Is(err, ErrTerminalDisabled) {
		t.Fatal("revoked consent read history", err)
	}
}
