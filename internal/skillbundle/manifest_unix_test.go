//go:build linux || darwin

package skillbundle

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSpecialFileIsNeverOpenedAndReplacementFIFOIsNonblocking(t *testing.T) {
	root := fixture(t, map[string]string{"SKILL.md": "x"})
	fifo := filepath.Join(root, "pipe")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := Scan(root, Limits{})
	assertIncomplete(t, m, err, "special_file")
	// Supply the original regular-file observation, then replace its name with a
	// FIFO. Opening must return to the identity check, not wait for a pipe writer.
	file := filepath.Join(root, "SKILL.md")
	before, err := os.Lstat(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(file, 0o600); err != nil {
		t.Fatal(err)
	}
	bound, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer bound.Close()
	if _, _, code := readFile(bound, "SKILL.md", "SKILL.md", before, MaxBytes, nil); code != "file_changed" {
		t.Fatalf("replacement FIFO accepted: %s", code)
	}
}
