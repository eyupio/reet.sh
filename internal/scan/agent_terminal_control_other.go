//go:build !linux && !darwin

package scan

import "github.com/eyupio/reet.sh/internal/agenthealth"

func terminalControl(string, agentConfig) agenthealth.TerminalFactory { return nil }

// launchTools is nil where the connector cannot start terminal sessions.
var launchTools func() []string

// gitAvailable is nil where the connector cannot make run worktrees.
var gitAvailable func() bool

// browseAllowed is nil where the connector cannot browse folders.
func browseAllowed(string, agentConfig) func() bool { return nil }
