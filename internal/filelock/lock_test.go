//go:build linux || darwin

package filelock

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExclusiveLockReleasedByProcessDeath(t *testing.T) {
	if path := os.Getenv("REET_TEST_LOCK_HELPER"); path != "" {
		f, err := Acquire(path)
		if err != nil {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(2)
		}
		defer f.Close()
		fmt.Fprintln(os.Stdout, "locked")
		select {}
	}
	path := filepath.Join(t.TempDir(), "agent.lock")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command(executable, "-test.run=^TestExclusiveLockReleasedByProcessDeath$")
	child.Env = append(os.Environ(), "REET_TEST_LOCK_HELPER="+path)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	reader := bufio.NewScanner(output)
	if !reader.Scan() || reader.Text() != "locked" {
		t.Fatal("child did not acquire lock")
	}
	if f, err := Acquire(path); !errors.Is(err, ErrLocked) {
		if f != nil {
			f.Close()
		}
		t.Fatalf("simultaneous owner accepted: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	f, err := Acquire(path)
	if err != nil {
		t.Fatal("crash left an unrecoverable lock", err)
	}
	defer f.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("lock inode must remain persistent", err)
	}
}

func TestLockRejectsSymlinksAndPermissiveFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unsafe")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if f, err := Acquire(path); err == nil {
		f.Close()
		t.Fatal("permissive lock accepted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if f, err := Acquire(link); err == nil {
		f.Close()
		t.Fatal("symlink lock accepted")
	}
}
