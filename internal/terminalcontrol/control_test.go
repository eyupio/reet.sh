//go:build linux || darwin

package terminalcontrol

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "reet-ctl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestLiveCommandsPersistAndRestartFences(t *testing.T) {
	dir := shortDir(t)
	c, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var enabled atomic.Bool
	var transitions atomic.Int32
	if err = c.Serve(func(v bool) { enabled.Store(v); transitions.Add(1) }, enabled.Load); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		action  string
		want    bool
		changes int32
	}{{"status", false, 0}, {"start", true, 1}, {"start", true, 1}, {"restart", true, 3}, {"stop", false, 4}, {"stop", false, 4}} {
		status, err := Command(context.Background(), dir, step.action, true)
		if err != nil || !status.Running || status.Enabled != step.want || status.Available != step.want || transitions.Load() != step.changes {
			t.Fatalf("%s: %+v transitions=%d err=%v", step.action, status, transitions.Load(), err)
		}
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.Enabled() {
		t.Fatal("persisted stop was overridden by environment default")
	}
}
func TestOfflineConfigurationAndUnsafeStateFailClosed(t *testing.T) {
	dir := shortDir(t)
	s, err := Command(t.Context(), dir, "start", false)
	if err != nil || s.Running || !s.Enabled || s.Available {
		t.Fatal(s, err)
	}
	if _, err = Command(t.Context(), dir, "restart", false); err == nil {
		t.Fatal("restart claimed a stopped controller was running")
	}
	c, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled() {
		t.Fatal("offline start not loaded")
	}
	c.Close()
	path := filepath.Join(dir, "terminal-control", "state.json")
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, false); err == nil {
		t.Fatal("permissive state accepted")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(dir, "other"), path); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(dir, false); err == nil {
		t.Fatal("symlink state accepted")
	}
}
func TestStoppedControllerDoesNotRemoveForeignSocketFile(t *testing.T) {
	dir := shortDir(t)
	c, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	path := filepath.Join(c.dir, "control.sock")
	if err = os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = c.Serve(func(bool) {}, func() bool { return false }); err == nil {
		t.Fatal("regular file replaced with socket")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatal("foreign file removed", err)
	}
}

func TestFailedPersistenceDoesNotChangeLiveAvailability(t *testing.T) {
	dir := shortDir(t)
	c, err := Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var enabled atomic.Bool
	if err = c.Serve(enabled.Store, enabled.Load); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.dir, "state.json")
	if err = os.WriteFile(path, []byte(`{"enabled":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Command(t.Context(), dir, "start", false); err == nil {
		t.Fatal("unsafe persisted state was overwritten")
	}
	status, err := Command(t.Context(), dir, "status", false)
	if err != nil || status.Enabled || status.Available || enabled.Load() {
		t.Fatal("persistence failure changed runtime", status, err)
	}
}
