//go:build unix

package agentaccounts

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A pipe at a sign-in path would hold a read open until something wrote to
// it. Every reader takes it as a file it cannot read, at once, and so does a
// link to one; a link to a plain file is still read.
func TestAPipeAtASignInPathIsUnreadableAndDoesNotBlock(t *testing.T) {
	f := newFixture(t)
	paths := map[string]string{
		"claude":      ".claude/.credentials.json",
		"codex":       ".codex/auth.json",
		"opencode":    ".local/share/opencode/auth.json",
		"antigravity": antigravityFile,
		"copilot":     ".config/github-copilot/apps.json",
	}
	for _, rel := range paths {
		path := filepath.Join(f.home, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Skipf("no named pipes here: %v", err)
		}
	}
	done := make(chan map[string]terminalproto.AgentAccount, 1)
	go func() { done <- f.all(false, false) }()
	select {
	case accounts := <-done:
		for tool := range paths {
			if accounts[tool].State != terminalproto.AccountUnreadable {
				t.Errorf("%s with a pipe for its sign-in: %+v", tool, accounts[tool])
			}
		}
	case <-time.After(5 * time.Second):
		// Unblock the stuck reads so the test can end.
		for _, rel := range paths {
			if w, err := os.OpenFile(filepath.Join(f.home, rel), os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				w.Close()
			}
		}
		t.Fatal("reading the sign-ins blocked on a pipe")
	}

	// A link to a pipe is no better; a link to a plain file is read.
	dir := t.TempDir()
	pipe, plain := filepath.Join(dir, "pipe"), filepath.Join(dir, "plain")
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(plain, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	toPipe, toPlain := filepath.Join(dir, "to-pipe"), filepath.Join(dir, "to-plain")
	if os.Symlink(pipe, toPipe) != nil || os.Symlink(plain, toPlain) != nil {
		t.Fatal("cannot make links")
	}
	read := make(chan error, 1)
	go func() { _, err := readFile(toPipe, maxCredentialBytes); read <- err }()
	select {
	case err := <-read:
		if err != errUnreadable {
			t.Fatalf("a link to a pipe: %v", err)
		}
	case <-time.After(5 * time.Second):
		if w, err := os.OpenFile(pipe, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			w.Close()
		}
		t.Fatal("reading a link to a pipe blocked")
	}
	if data, err := readFile(toPlain, maxCredentialBytes); err != nil || string(data) != "{}" {
		t.Fatalf("a link to a plain file: %q %v", data, err)
	}
}
