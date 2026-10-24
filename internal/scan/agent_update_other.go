//go:build !linux && !darwin

package scan

import (
	"context"
	"errors"
)

func updateAgentBinary(context.Context, string, string, string, string, func(string, string)) (string, string, error) {
	return "", "", errors.New("updating reet from the controller requires Linux or macOS")
}

func checkPendingUpdate(agentConfig, string, string) (bool, error) {
	return false, nil
}

func reportAgentStage(agentConfig, string, string, string, string) {}

func rollbackPendingUpdate(string, pendingUpdate, string) error {
	return errors.New("rolling back a reet update requires Linux or macOS")
}

func restartAgent(string) error {
	return errors.New("restarting into an updated reet binary requires Linux or macOS")
}
