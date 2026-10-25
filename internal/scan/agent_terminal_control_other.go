//go:build !linux && !darwin

package scan

import "github.com/eyupio/reet.sh/internal/agenthealth"

func terminalControl(string, agentConfig) agenthealth.TerminalFactory { return nil }

// launchTools is nil where the connector cannot start terminal sessions.
var launchTools func() []string
