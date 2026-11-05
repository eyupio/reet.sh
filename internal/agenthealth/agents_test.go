package agenthealth

import (
	"context"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"net/http"
	"slices"
	"testing"
)

func TestAgentsV2NegotiationFiltersNewTools(t *testing.T) {
	for _, tc := range []struct {
		features string
		want     []string
	}{
		{terminalproto.LaunchFeature, []string{"claude"}},
		{terminalproto.LaunchFeature + "," + terminalproto.AgentsFeature, []string{"claude", "cline"}},
		{terminalproto.LaunchFeature + "," + terminalproto.AntigravityFeature, []string{"claude"}},
		{terminalproto.LaunchFeature + "," + terminalproto.AgentsFeature + "," + terminalproto.AntigravityFeature, []string{"antigravity", "claude", "cline"}},
	} {
		cfg := AgentConfig{TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
			return nil, nil
		}, LaunchTools: func() []string { return []string{"cline", "claude", "antigravity", "cline", "antigravity", "unknown"} }}
		negotiateFeatures(&cfg, tc.features)
		caps := capabilitiesFor(cfg, true, terminalproto.Ready)
		if !slices.Equal(caps.Tools, tc.want) {
			t.Fatalf("features=%s tools=%v want=%v", tc.features, caps.Tools, tc.want)
		}
		if err := caps.ValidateFor(tc.features); err != nil {
			t.Fatal(err)
		}
		if terminalproto.SupportsAntigravity(terminalFeatures(cfg)) != slices.Contains(tc.want, "antigravity") {
			t.Fatal("dispatcher received unnegotiated AGY feature")
		}
	}
}

func TestYoloCapabilityNeedsAgentsV2AndTheLocalSwitch(t *testing.T) {
	cfg := AgentConfig{TerminalSession: func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, nil
	}, LaunchTools: func() []string { return []string{"cline"} }, YoloAllowed: func() bool { return true }}
	negotiateFeatures(&cfg, terminalproto.LaunchFeature+","+terminalproto.AgentsFeature)
	cfg.yoloAllowed = readYoloAllowed(cfg)
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); !caps.Yolo {
		t.Fatal("YOLO not reported with agents-v2 and the switch on")
	}
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.Validate() != nil {
		t.Fatal(caps.Validate())
	}
	cfg.yoloAllowed = false
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.Yolo {
		t.Fatal("YOLO reported with the switch off")
	}
	cfg.yoloAllowed = true
	cfg.agentsV2 = false
	if caps := capabilitiesFor(cfg, true, terminalproto.Ready); caps.Yolo {
		t.Fatal("YOLO reported without agents-v2")
	}
}
