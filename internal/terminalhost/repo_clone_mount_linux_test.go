//go:build linux

package terminalhost

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A real filesystem mounted on an ignored folder of the clone: the clone is
// kept and what is on the mount survives a remove. Mounting needs root.
func TestRemoveCloneKeepsAFilesystemMountedInside(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("mounting a filesystem needs root")
	}
	f := newCloneFixture(t)
	dir := f.madeClone(t)
	writeFile(t, filepath.Join(dir, ".git", "info", "exclude"), "data/\n")
	data := filepath.Join(dir, "data")
	if err := os.Mkdir(data, 0o700); err != nil {
		t.Fatal(err)
	}
	shown := f.status(t, dir, "o/r")
	wantCounts(t, shown, 0, 0, 0, 0)
	if err := syscall.Mount("tmpfs", data, "tmpfs", 0, "size=1m"); err != nil {
		t.Skip("cannot mount a tmpfs here:", err)
	}
	t.Cleanup(func() {
		if err := syscall.Unmount(data, syscall.MNT_DETACH); err != nil {
			t.Error("unmount:", err)
		}
	})
	precious := filepath.Join(data, "precious")
	writeFile(t, precious, "on the mount\n")
	before := snapshot(t, f.home)
	if s := f.status(t, dir, "o/r"); s != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneNotRemovable}) {
		t.Fatalf("state = %+v, want not_removable", s)
	}
	wantKept(t, f.remove(t, dir, "o/r", shown.Fingerprint), terminalproto.RepoCloneNotRemovable, f.home, before)
	if content, err := os.ReadFile(precious); err != nil || string(content) != "on the mount\n" {
		t.Fatalf("the file on the mount: %q %v", content, err)
	}
}
