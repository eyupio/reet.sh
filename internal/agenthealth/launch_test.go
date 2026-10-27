package agenthealth

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestLaunchCapabilityRequiresNegotiationAndTerminalControl(t *testing.T) {
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	tools := func() []string { return []string{"claude", "gemini"} }
	for _, tc := range []struct {
		name      string
		cfg       AgentConfig
		launch    bool
		wantTools []string
	}{
		{"not negotiated", AgentConfig{TerminalSession: factory, LaunchTools: tools}, false, nil},
		{"no terminal control", AgentConfig{launch: true, LaunchTools: tools}, false, nil},
		{"no launch helper", AgentConfig{TerminalSession: factory, launch: true}, false, nil},
		{"negotiated", AgentConfig{TerminalSession: factory, launch: true, LaunchTools: tools}, true, []string{"claude", "gemini"}},
		{"nothing installed", AgentConfig{TerminalSession: factory, launch: true, LaunchTools: func() []string { return []string{} }}, true, nil},
		{"unknown ids dropped", AgentConfig{TerminalSession: factory, launch: true, LaunchTools: func() []string { return []string{"vim", "codex"} }}, true, []string{"codex"}},
	} {
		caps := capabilities(tc.cfg)
		if err := caps.Validate(); err != nil {
			t.Fatal(tc.name, err)
		}
		data, _ := json.Marshal(caps)
		if caps.Launch != tc.launch || !slices.Equal(caps.Tools, tc.wantTools) || strings.Contains(string(data), `"launch"`) != tc.launch {
			t.Fatal(tc.name, string(data))
		}
		if len(tc.wantTools) == 0 && strings.Contains(string(data), `"tools"`) {
			t.Fatal(tc.name, "empty tools serialised", string(data))
		}
	}
}

func TestTerminalFeaturesFollowTheControllerAdvertisement(t *testing.T) {
	for _, tc := range []struct {
		advertised string
		history    bool
		launch     bool
	}{
		{"", false, false},
		{terminalproto.CommandHistoryFeature, true, false},
		{terminalproto.LaunchFeature, false, true},
		{terminalproto.CommandHistoryFeature + ", " + terminalproto.LaunchFeature, true, true},
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, tc.advertised)
		if cfg.commandHistory != tc.history || cfg.launch != tc.launch {
			t.Fatal("negotiated", tc.advertised, cfg.commandHistory, cfg.launch)
		}
		headers := http.Header{}
		setTerminalFeatures(headers, cfg)
		value := headers.Get(terminalproto.FeatureHeader)
		if terminalproto.SupportsCommandHistory(value) != tc.history || terminalproto.SupportsLaunch(value) != tc.launch || (value == "") != (!tc.history && !tc.launch) {
			t.Fatal("terminal features header", tc.advertised, value)
		}
	}
}
