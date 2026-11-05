package terminalhost

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
)

// InstallAgent installs monitoring and skill sync independently of terminal
// access. It neither needs tmux nor creates, restarts or stops its supervisor.
func InstallAgent(ctx context.Context, plan ServicePlan) error {
	if data, err := os.ReadFile(plan.AgentPath); err == nil {
		if err = CheckEnrollmentConfig(plan.AgentPath); err != nil {
			return err
		}
		binding := systemdExec([]string{"--config", plan.ConfigPath})
		if runtime.GOOS == "darwin" {
			binding = "<string>--config</string><string>" + xmlEscape(plan.ConfigPath) + "</string>"
		}
		if !strings.Contains(string(data), binding) {
			return errors.New("installed agent service belongs to another enrollment; inspect it locally")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	// Fail before writing a unit when this login has no native service manager.
	if runtime.GOOS == "darwin" {
		if err := runServiceCommand(ctx, "launchctl", "print", macServiceDomain()); err != nil {
			return err
		}
	} else {
		if err := runServiceCommand(ctx, "systemctl", "--user", "show-environment"); err != nil {
			return err
		}
	}
	if err := Ensure(plan.Layout); err != nil {
		return err
	}
	if err := os.MkdirAll(plan.UnitDir, 0700); err != nil {
		return err
	}
	if err := writePrivateFile(plan.AgentPath, []byte(plan.AgentText())); err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		if err := bootoutMac(ctx, plan.AgentLabel); err != nil {
			return err
		}
		if err := runServiceCommand(ctx, "launchctl", "bootstrap", macServiceDomain(), plan.AgentPath); err != nil {
			return err
		}
		return runServiceCommand(ctx, "launchctl", "enable", macServiceDomain()+"/"+plan.AgentLabel)
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	if err := runServiceCommand(ctx, "systemctl", "--user", "enable", plan.AgentName); err != nil {
		return err
	}
	return runServiceCommand(ctx, "systemctl", "--user", "restart", plan.AgentName)
}
