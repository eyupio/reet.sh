package agenthealth

import (
	"context"
	"net/http"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalQuestionFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                                    false,
		terminalproto.AttentionFeature:        false,
		terminalproto.TerminalQuestionFeature: true,
		"attention-v1, terminal-question-v1":  true,
		"terminal-question-v2":                false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.terminalQuestion != want || terminalproto.SupportsTerminalQuestion(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: terminalQuestion %v, features %q", advertised, cfg.terminalQuestion, terminalFeatures(cfg))
		}
	}
}

// TerminalQuestion is reported only with the negotiated terminal-question-v1 and terminal
// control, and is valid for the features the machine claims.
func TestTerminalQuestionCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.terminalQuestion = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
	} {
		cfg := AgentConfig{TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready, terminalQuestion: true}
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.TerminalQuestion != tc.want {
			t.Fatal(tc.name, caps.TerminalQuestion)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}
