//go:build linux || darwin

package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func probeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "reet")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAnOlderBuildDoesNotAnswerTheMarkerProbe(t *testing.T) {
	current := probeScript(t, `if [ "$REET_AGENT_UPDATE_PROTOCOL" = "1" ]; then echo `+AgentUpdateProtocol+`; else echo "reet v1.1.0 (abc)"; fi`)
	if !agentUpdateProtocolSupported(context.Background(), current) {
		t.Fatal("the build that carries the marker was not recognised")
	}
	older := probeScript(t, `echo "reet v1.0.0 (abc)"`)
	if agentUpdateProtocolSupported(context.Background(), older) {
		t.Fatal("an older build claimed the marker protocol")
	}
	if agentUpdateProtocolSupported(context.Background(), filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("a missing binary claimed the marker protocol")
	}
}
