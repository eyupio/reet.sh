//go:build linux || darwin

package skillbundle

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReadContentsPreservesBytesModesAndRejectsChanges(t *testing.T) {
	root := fixture(t, map[string]string{"SKILL.md": "x\r\ny\r\n", "bin/run": "hello"})
	if err := os.Chmod(filepath.Join(root, "bin/run"), 0700); err != nil {
		t.Fatal(err)
	}
	m, err := Scan(root, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	got, files, err := ReadContents(root, m.Digest)
	if err != nil || got.Digest != m.Digest || len(files) != 2 {
		t.Fatal(got, files, err)
	}
	if !bytes.Equal(files[0].Data, []byte("x\r\ny\r\n")) || !files[1].Executable {
		t.Fatal("byte/mode loss", files)
	}
	if _, files, err = ReadContents(root, "wrong"); err == nil || files != nil {
		t.Fatal("unexpected digest exported", err)
	}
	changed := false
	_, files, err = readContents(root, m.Digest, func(relative string) {
		if !changed {
			changed = true
			if err := os.Rename(filepath.Join(root, relative), filepath.Join(root, relative+".moved")); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err == nil || files != nil {
		t.Fatal("changed read leaked contents", err)
	}
}
func TestReadContentsUnsafeRootNeverReturnsPayload(t *testing.T) {
	root := fixture(t, map[string]string{"SKILL.md": "private"})
	m, err := Scan(root, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "linked")
	if err = os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if _, files, err := ReadContents(link, m.Digest); err == nil || files != nil {
		t.Fatal(files, err)
	}
}
