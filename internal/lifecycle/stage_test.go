package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStageDirSitsBesideTheBinary(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "bin", "reet")
	file(t, target, "old binary")
	dir, err := StageDir(target)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if filepath.Dir(dir) != filepath.Dir(target) {
		t.Fatalf("staged in %s, want beside %s", dir, target)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("stage folder %v %v", info, err)
	}
}

func TestStageDirFallsBackWhenTheBinaryFolderCannotBeUsed(t *testing.T) {
	root := t.TempDir()
	fallback := filepath.Join(root, "tmp")
	if err := os.Mkdir(fallback, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", fallback)
	second := filepath.Join(root, "bin", "reet")
	file(t, second, "running binary")
	// The first folder does not exist, so the next one is used.
	dir, err := StageDir(filepath.Join(root, "missing", "reet"), "", second)
	if err != nil || filepath.Dir(dir) != filepath.Dir(second) {
		t.Fatalf("staged in %s (%v), want beside %s", dir, err, second)
	}
	dir, err = StageDir(filepath.Join(root, "missing", "reet"))
	if err != nil || filepath.Dir(dir) != fallback {
		t.Fatalf("staged in %s (%v), want the temporary folder %s", dir, err, fallback)
	}
}

func TestUpgradeRunsTheCandidateBesideTheBinaryAndCleansUp(t *testing.T) {
	for _, mode := range []string{"--dry-run", "--yes"} {
		t.Run(mode, func(t *testing.T) {
			a, state, binary := testApp(t)
			ran := ""
			a.run = func(_ context.Context, name string, args ...string) (string, error) {
				if strings.Contains(name, ".reet-download-") {
					ran = name
					return "reet v2.0.0 (new)\n", nil
				}
				return "", os.ErrNotExist
			}
			if err := a.Run("upgrade", []string{"--state-dir", state, mode}); err != nil {
				t.Fatal(err)
			}
			if filepath.Dir(filepath.Dir(ran)) != filepath.Dir(binary) {
				t.Fatalf("candidate ran from %s, want a folder beside %s", ran, binary)
			}
			entries, err := os.ReadDir(filepath.Dir(binary))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".reet-") {
					t.Fatalf("left %s beside the binary", entry.Name())
				}
			}
		})
	}
}
