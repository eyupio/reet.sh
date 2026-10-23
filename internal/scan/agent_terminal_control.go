//go:build linux || darwin

package scan

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/terminalagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
)

func terminalControl(config string, c agentConfig) agenthealth.TerminalFactory {
	layout, err := terminalhost.NewEnrollmentLayout(config, c.ID)
	if err != nil {
		return nil
	}
	return terminalagent.Factory(terminalhost.ManagerOptions{EnrollmentID: c.ID, Layout: layout, Home: c.Home, Consent: func() error {
		current, err := loadAgentConfig(config)
		if err != nil {
			return err
		}
		if current.ID != c.ID || current.Secret != c.Secret || current.Server != c.Server || !current.TerminalEnabled {
			return errors.New("terminal access disabled locally")
		}
		return nil
	}})
}
