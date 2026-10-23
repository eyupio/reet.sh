//go:build !linux && !darwin

package scan

import "github.com/eyupio/reet.sh/internal/agenthealth"

func terminalControl(string, agentConfig) agenthealth.TerminalFactory { return nil }
