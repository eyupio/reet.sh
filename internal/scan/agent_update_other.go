//go:build !linux && !darwin

package scan

import (
	"context"
	"errors"
)

func updateAgentBinary(context.Context, string, string) (string, string, error) {
	return "", "", errors.New("updating reet from the controller requires Linux or macOS")
}

func restartAgent(string) error {
	return errors.New("restarting into an updated reet binary requires Linux or macOS")
}
