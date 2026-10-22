//go:build unix

package setup

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestConfigReadRejectsFIFOReplacementWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfig(t, path, `{}`)
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if data, issue := readConfig(path, before); issue != "file_changed" || data != nil {
		t.Fatalf("FIFO accepted: %q", issue)
	}
}
