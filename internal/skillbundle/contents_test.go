package skillbundle

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestContentsIdentityMatchesExactFilesystemObservation(t *testing.T) {
	if !supportedModes {
		t.Skip("filesystem executable semantics unavailable")
	}
	files := []ContentFile{{Path: "SKILL.md", Data: []byte("---\r\nname: sample\r\ndescription: exact bytes\r\n---\r\nInstructions\r\n")}, {Path: "scripts/run.sh", Data: []byte("#!/bin/sh\nprintf hello\n"), Executable: true}, {Path: "assets/data.bin", Data: []byte{0, 1, 255}}}
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0600)
		if f.Executable {
			mode = 0700
		}
		if err := os.WriteFile(p, f.Data, mode); err != nil {
			t.Fatal(err)
		}
	}
	observed, err := Scan(root, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, err := FromContents(files)
	if err != nil || !reflect.DeepEqual(uploaded, observed) {
		t.Fatalf("manifest parity: %+v %+v %v", uploaded, observed, err)
	}
	files[1].Executable = false
	changed, err := FromContents(files)
	if err != nil || changed.Digest == uploaded.Digest {
		t.Fatal("executable semantics lost", err)
	}
}
func TestContentsRejectsAmbiguousAndUnboundedPayloads(t *testing.T) {
	for _, paths := range [][]string{{"SKILL.md", "SKILL.md"}, {"SKILL.md", "a", "a/b"}, {"SKILL.md", "A/a", "a/b"}, {"SKILL.md", "../escape"}, {"SKILL.md", "CON"}, {"SKILL.md", "a\\b"}, {"SKILL.md", "x/../b"}, {"skill.md"}, {"SKILL.md", strings.Repeat("a/", MaxDepth+1) + "x"}} {
		var files []ContentFile
		for _, p := range paths {
			files = append(files, ContentFile{Path: p})
		}
		if _, err := FromContents(files); err == nil {
			t.Fatal("accepted", paths)
		}
	}
	if _, err := FromContents([]ContentFile{{Path: "SKILL.md", Data: make([]byte, MaxBytes+1)}}); err == nil {
		t.Fatal("accepted oversized upload")
	}
}
