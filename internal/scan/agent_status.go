package scan

import (
	"fmt"
	"io"
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
	return nil
}
