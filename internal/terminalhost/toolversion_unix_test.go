//go:build linux || darwin

package terminalhost

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A version command that hangs is stopped, with everything it started, and costs no more than the timeout; a cancelled request stops it at once.
func TestReadToolVersionStopsAHungWrapperAndItsChild(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	script := filepath.Join(dir, "codex")
	body := "#!/bin/sh\nsleep 30 &\necho $! > " + pidFile + "\nwait\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	old := toolVersionTimeout
	toolVersionTimeout = 300 * time.Millisecond
	defer func() { toolVersionTimeout = old }()
	start := time.Now()
	if got := readToolVersion(context.Background(), script); got != "" {
		t.Fatalf("a hung command read as %q", got)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("a hung version command took %v", took)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if syscall.Kill(pid, 0) == nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
		t.Fatal("the version command's child outlived it")
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	toolVersionTimeout = 10 * time.Second
	start = time.Now()
	if got := readToolVersion(cancelled, script); got != "" || time.Since(start) > 3*time.Second {
		t.Fatalf("a cancelled request still waited: %q after %v", got, time.Since(start))
	}
}
