package scan

import (
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"
)

// AgentStatus is this machine's enrollment and local daemon state. It is the
// value reet agent status prints and the terminal UI renders.
type AgentStatus struct {
	ConfigPath   string
	Running      bool
	LocalControl bool
	PID          int
	Supervised   bool
	Connected    bool
	StartedAt    time.Time
	Version      string
	LastCycle    time.Time
	// Why the sync cycle and the private connection last failed; empty when working.
	CycleError, PrivateError string
}

func printAgentStatus(config string, out io.Writer) error {
	s, err := ReadAgentStatus(config)
	if err != nil {
		return err
	}
	if !s.Running {
		fmt.Fprintln(out, "Agent: stopped\nPrivate connection: disconnected")
		return nil
	}
	if !s.LocalControl {
		fmt.Fprintln(out, "Agent: running (local control unavailable; older process)")
		return nil
	}
	connection := "reconnecting"
	if s.Connected {
		connection = "connected"
	}
	mode := "background/foreground process"
	if s.Supervised {
		mode = "native service"
	}
	fmt.Fprintf(out, "Agent: running (PID %d, %s)\nPrivate connection: %s\nStarted: %s\nVersion: %s\n", s.PID, mode, connection, s.StartedAt.Format(time.RFC3339), s.Version)
	if !s.LastCycle.IsZero() {
		fmt.Fprintf(out, "Last successful inventory/job cycle: %s\n", s.LastCycle.Format(time.RFC3339))
	}
	for _, line := range s.Problems() {
		fmt.Fprintln(out, line)
	}
	return nil
}

// Problems says, in plain words, why this machine is not reporting or not
// connected, and what to do about it. It is empty when both channels work.
func (s AgentStatus) Problems() []string {
	var lines []string
	if s.CycleError != "" {
		lines = append(lines, "Sync is failing: "+s.CycleError)
	}
	if s.PrivateError != "" && !s.Connected {
		lines = append(lines, "Private connection is failing: "+s.PrivateError)
	}
	if len(lines) == 0 {
		return nil
	}
	both := s.CycleError + " " + s.PrivateError
	switch {
	case strings.Contains(both, "401"), strings.Contains(both, "403"), strings.Contains(both, "revoked"):
		lines = append(lines, "The controller is refusing this machine. Its token or enrolment was probably removed or replaced: check Machines in the web app, then connect again with reet agent enroll.")
	case strings.Contains(both, "no such host"), strings.Contains(both, "connection refused"), strings.Contains(both, "timeout"), strings.Contains(both, "deadline"):
		lines = append(lines, "This machine cannot reach the controller. Check the network and the controller URL in "+s.ConfigPath+".")
	}
	if s.Supervised && runtime.GOOS == "linux" {
		lines = append(lines, "Full log: journalctl --user -u reet-agent.service -n 50")
	}
	return lines
}
