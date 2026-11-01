//go:build linux

package acpagent

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A group member that outlives its leader (the SC2 probe's survivor) must be
// found alive, killed and confirmed gone.
func TestRetireGroupKillsSurvivorOfExitedLeader(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 300 & echo $!; exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, _ := out.Read(buf)
	child, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
	if err != nil {
		t.Fatalf("child pid %q: %v", buf[:n], err)
	}
	_ = cmd.Wait() // the leader is gone; the sleeper keeps the group alive
	pgid := cmd.Process.Pid
	if !groupAlive(pgid) {
		t.Fatalf("survivor %d not seen in group %d", child, pgid)
	}
	if !retireGroup(pgid, 3*time.Second) {
		t.Fatal("group not confirmed retired")
	}
	if groupAlive(pgid) {
		t.Fatal("group still alive")
	}
}

func TestRetireGroupEmptyGroup(t *testing.T) {
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !retireGroup(cmd.Process.Pid, time.Second) {
		t.Fatal("empty group reported alive")
	}
}

// A group that ignores SIGTERM is killed by the escalation and confirmed gone.
func TestTerminateGroupKillsAGroupThatIgnoresTerm(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM HUP; sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	pgid := cmd.Process.Pid
	time.Sleep(200 * time.Millisecond) // let the trap install
	if !groupAlive(pgid) {
		t.Fatal("fixture: the group is not alive")
	}
	if !TerminateGroup(pgid, 200*time.Millisecond, 3*time.Second) {
		t.Fatal("group not confirmed gone")
	}
	if groupAlive(pgid) {
		t.Fatal("group still alive")
	}
}

// A group that exits on SIGTERM is not sent SIGKILL: the TERM-only wait is
// enough, so a TUI that flushes on TERM is left to finish.
func TestTerminateGroupStopsAtTermWhenTheGroupExits(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap 'exit 0' TERM; while :; do sleep 1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	pgid := cmd.Process.Pid
	time.Sleep(200 * time.Millisecond)
	killed := false
	old := killGroup
	killGroup = func(pgid int) { killed = true; old(pgid) }
	t.Cleanup(func() { killGroup = old })
	if !TerminateGroup(pgid, 3*time.Second, 3*time.Second) {
		t.Fatal("group not confirmed gone")
	}
	if killed {
		t.Fatal("SIGKILL was sent although the group exited on SIGTERM")
	}
}

func TestTerminateGroupEmptyGroup(t *testing.T) {
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !TerminateGroup(cmd.Process.Pid, time.Second, time.Second) {
		t.Fatal("empty group reported alive")
	}
}

// retireGroup on a group that is already gone must not signal: the group id may
// have been reused by an unrelated group since it was seen alive.
func TestRetireGroupDoesNotSignalAGoneGroup(t *testing.T) {
	cmd := exec.Command("true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	var calls []int
	old := killGroup
	killGroup = func(pgid int) { calls = append(calls, pgid); old(pgid) }
	t.Cleanup(func() { killGroup = old })
	if !retireGroup(cmd.Process.Pid, time.Second) {
		t.Fatal("a gone group was reported alive")
	}
	if len(calls) != 0 {
		t.Fatalf("killGroup called for a gone group: %v", calls)
	}
}

func TestRetireGroupKillsALiveGroup(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	pgid := cmd.Process.Pid
	time.Sleep(100 * time.Millisecond)
	var calls []int
	old := killGroup
	killGroup = func(pgid int) { calls = append(calls, pgid); old(pgid) }
	t.Cleanup(func() { killGroup = old })
	if !retireGroup(pgid, 3*time.Second) {
		t.Fatal("live group not retired")
	}
	if len(calls) == 0 || calls[0] != pgid {
		t.Fatalf("killGroup calls %v, want the live group", calls)
	}
}
