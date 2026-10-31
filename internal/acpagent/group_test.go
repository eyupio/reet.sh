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
