//go:build linux

package acpagent

import (
	"context"
	"os"
	"testing"
	"time"
)

// A binary that is still open for writing cannot be run: the kernel answers
// "text file busy". An update that has just written the agent, or any process
// that forked while the file was open, leaves it so for a moment. The probe
// used to report that moment as a missing version and refuse the agent.
func TestAVersionProbeWaitsOutABinaryThatIsStillBeingWritten(t *testing.T) {
	opts, _ := options(t, "ok", testVersion)
	held, err := os.OpenFile(opts.Binary, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		held.Close()
		close(released)
	}()
	found, err := ProbeVersion(context.Background(), opts.Binary, opts.Agent, opts.Dir, opts.Home, opts.BaseEnv)
	<-released
	if err != nil || found != testVersion {
		t.Fatalf("found %q, %v", found, err)
	}
}

// The wait is bounded: a binary that stays busy is still refused.
func TestAVersionProbeGivesUpOnABinaryThatStaysBusy(t *testing.T) {
	opts, _ := options(t, "ok", testVersion)
	held, err := os.OpenFile(opts.Binary, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	started := time.Now()
	if _, err := ProbeVersion(context.Background(), opts.Binary, opts.Agent, opts.Dir, opts.Home, opts.BaseEnv); err == nil {
		t.Fatal("a binary nobody could run was accepted")
	}
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("the probe waited %s for a busy binary", took)
	}
}
