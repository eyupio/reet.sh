//go:build !linux && !darwin

package scan

import (
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func terminalControl(string, agentConfig) agenthealth.TerminalFactory { return nil }

// launchTools is nil where the connector cannot start terminal sessions.
var launchTools func() []string

// softwareInventory is nil where the connector cannot inspect agent software.
var softwareInventory func(string) []terminalproto.SoftwareEntry

// gitAvailable is nil where the connector cannot make run worktrees.
var gitAvailable func() bool

// browseAllowed is nil where the connector cannot browse folders.
func browseAllowed(string, agentConfig) func() bool { return nil }

// terminalAllowed is nil where the connector has no terminal access.
func terminalAllowed(string, agentConfig) func() bool { return nil }
