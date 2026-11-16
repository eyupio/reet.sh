package agenthealth

import (
	"context"
	"net/http"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestDeskShellFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                             false,
		terminalproto.ShellDirFeature:  false,
		terminalproto.DeskShellFeature: true,
		"shell-dir-v1, desk-shell-v1":  true,
		"desk-shell-v2":                false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.deskShell != want || terminalproto.SupportsDeskShell(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: deskShell %v, features %q", advertised, cfg.deskShell, terminalFeatures(cfg))
		}
	}
}

// DeskShell is reported only with the negotiated desk-shell-v1 and terminal
// control, and is valid for the features the machine claims.
func TestDeskShellCapability(t *testing.T) {
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
		{"not negotiated", func(c *AgentConfig) { c.deskShell = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
	} {
		cfg := AgentConfig{TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready, deskShell: true}
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.DeskShell != tc.want {
			t.Fatal(tc.name, caps.DeskShell)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}
