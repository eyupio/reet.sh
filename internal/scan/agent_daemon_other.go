//go:build !linux && !darwin

package scan

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
)

type agentRuntime struct {
	connected              atomic.Bool
	lastCycle              atomic.Int64
	started                int64
	version, servicePrefix string
	supervised             bool
}

func controlAgent(string, string, string, io.Writer) error {
	return errors.New("background agents require Linux or macOS; use reet agent run --foreground")
}
func printAgentStatus(string, io.Writer) error {
	return errors.New("local daemon control requires Linux or macOS; use status --remote for controller status")
}
func serveAgentControl(string, *agentRuntime, context.CancelFunc) (func(), error) {
	return func() {}, nil
}
func notifyAgentReady() error { return nil }
