//go:build linux || darwin

package terminalhost

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Opening a FIFO for reading blocks until a writer appears, so a browse that
// ever opened one would hang: a FIFO entry is omitted from its type alone, and
// the machine's ReadDir refuses to open anything but a real directory.
func TestBrowseSkipsSpecialFiles(t *testing.T) {
	home := browseHome(t)
	mkdirs(t, home, "folder")
	fifo := filepath.Join(home, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skip("no FIFOs here:", err)
	}
	symlink(t, filepath.Join(home, "folder"), filepath.Join(home, "folder-link"))

	r := browseOS(t, home, terminalproto.Browse{})
	requireNames(t, r, "folder", "folder-link")
	if r.Omitted != 1 {
		t.Fatalf("omitted %d, want the FIFO", r.Omitted)
	}
	for _, path := range []string{fifo, filepath.Join(home, "folder-link")} {
		done := make(chan error, 1)
		go func() { _, err := OSFS.ReadDir(path); done <- err }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("ReadDir(%q) opened something that is not a real directory", path)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("ReadDir(%q) blocked opening a FIFO", path)
		}
	}
}
