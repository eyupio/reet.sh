package agenthealth

import (
	"context"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"net/http"
	"testing"
)

func TestAgentsV2NegotiationFiltersNewTools(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		cfg := AgentConfig{TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
			return nil, nil
		}, LaunchTools: func() []string { return []string{"cline", "claude", "cline", "unknown"} }}
		features := terminalproto.LaunchFeature
		if v2 {
			features += "," + terminalproto.AgentsFeature
		}
		negotiateFeatures(&cfg, features)
		caps := capabilitiesFor(cfg, true, terminalproto.Ready)
		if caps.AgentsV2 != v2 {
			t.Fatalf("agentsV2=%v want %v", caps.AgentsV2, v2)
		}
		if len(caps.Tools) != 1+btoi(v2) {
			t.Fatalf("v2=%v tools=%v", v2, caps.Tools)
		}
		if !v2 && caps.OSVersion != "" {
			t.Fatal("old controller received OSVersion")
		}
		if err := caps.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
