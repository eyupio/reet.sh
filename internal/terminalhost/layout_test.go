package terminalhost

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "reet-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func privateLayout(t *testing.T) Layout {
	t.Helper()
	config := filepath.Join(shortTempDir(t), "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	layout, err := NewLayout(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestEnsureCreatesPrivateLayout(t *testing.T) {
	layout := privateLayout(t)
	for _, dir := range []string{layout.Dir, layout.Logs} {
		info, err := os.Stat(dir)
		if err != nil || info.Mode().Perm() != 0700 {
			t.Fatalf("%s: %v %v", dir, info, err)
		}
	}
	info, err := os.Stat(layout.Config)
	if err != nil || info.Mode().Perm() != 0600 || info.Size() == 0 {
		t.Fatalf("config: %v %v", info, err)
	}
	if !strings.Contains(readFile(t, layout.Config), "history-limit 5000") || strings.Contains(readFile(t, layout.Config), "source-file") {
		t.Fatal("tmux config is not minimal")
	}
	if err := Check(layout); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureRefusesUnsafePaths(t *testing.T) {
	config := filepath.Join(shortTempDir(t), "agent.json")
	if err := os.WriteFile(config, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	layout, err := NewLayout(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(layout.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Test the on-disk mode regardless of the invoking shell's umask.
	if err := os.Chmod(layout.Dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("permissive directory: %v", err)
	}
	if err := os.Chmod(layout.Dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(layout.Config, layout.Dir+"/link"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(layout.Config); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(layout.Socket, layout.Config); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink config: %v", err)
	}
}

func TestNewLayoutRefusesLongSocketPaths(t *testing.T) {
	dir := filepath.Join(shortTempDir(t), strings.Repeat("x", 100))
	if _, err := NewLayout(filepath.Join(dir, "agent.json")); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("long socket path: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
