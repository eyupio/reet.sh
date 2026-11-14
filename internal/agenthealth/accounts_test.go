package agenthealth

import (
	"context"
	"net/http"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func accountsConfig(t *testing.T) AgentConfig {
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	return AgentConfig{
		TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		agentAccounts: true,
		Consent: func() (bool, terminalproto.Readiness, error) {
			t.Error("the readiness probe ran for a sign-in request")
			return true, terminalproto.Ready, nil
		},
		TerminalAllowed: func() bool { return true },
	}
}

// AgentAccounts is reported only with the negotiated agent-accounts-v1,
// terminal control and terminal access on.
func TestAgentAccountsCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.agentAccounts = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
		{"terminal access off", func(c *AgentConfig) {
			c.TerminalEnabled, c.TerminalReadiness = false, terminalproto.DisabledLocally
		}, false},
	} {
		cfg := accountsConfig(t)
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.AgentAccounts != tc.want {
			t.Fatal(tc.name, caps.AgentAccounts)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

func TestAgentAccountsFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                                  false,
		terminalproto.ModelsFeature:         false,
		terminalproto.AgentAccountsFeature:  true,
		"models-v1, " + "agent-accounts-v1": true,
		"agent-accounts-v2":                 false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.agentAccounts != want || terminalproto.SupportsAgentAccounts(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: agentAccounts %v, features %q", advertised, cfg.agentAccounts, terminalFeatures(cfg))
		}
	}
}

// A sign-in request the connector would not advertise now is answered off
// here, from the terminal opt-in read again; one it would, or one that was
// never negotiated (a protocol error there), goes to the terminal handler.
func TestAccountsRequestsFollowTheAdvertisedCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	ask := terminalproto.Message{Version: terminalproto.Version, RequestID: "accounts-request-0001", Generation: 1, Body: &terminalproto.Accounts{Limits: true}}
	for _, tc := range []struct {
		name      string
		edit      func(*AgentConfig)
		forwarded bool
	}{
		{"everything on", func(*AgentConfig) {}, true},
		{"terminal access switched off since", func(c *AgentConfig) { c.TerminalAllowed = func() bool { return false } }, false},
		{"not negotiated", func(c *AgentConfig) { c.agentAccounts = false }, true},
	} {
		cfg := accountsConfig(t)
		tc.edit(&cfg)
		var forwarded int
		var sent []terminalproto.Message
		gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
			func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
		if err := gate(t.Context(), ask); err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.forwarded {
			if forwarded != 1 || len(sent) != 0 {
				t.Fatal(tc.name, forwarded, sent)
			}
			continue
		}
		if forwarded != 0 || len(sent) != 1 || sent[0].RequestID != ask.RequestID {
			t.Fatal(tc.name, forwarded, sent)
		}
		body, ok := sent[0].Body.(terminalproto.AccountsResult)
		if !ok || body.Reason != terminalproto.AccountsOff || len(body.Accounts) != 0 {
			t.Fatal(tc.name, sent[0].Body)
		}
	}
}

// A sign-in request the dispatcher cannot take is answered busy, only when
// it was negotiated.
func TestAccountsUnavailableIsBusy(t *testing.T) {
	ask := terminalproto.Message{Version: terminalproto.Version, RequestID: "accounts-request-0001", Generation: 1, Body: &terminalproto.Accounts{}}
	m, ok := unavailable(ask, terminalproto.AgentAccountsFeature)
	body, isResult := m.Body.(terminalproto.AccountsResult)
	if !ok || !isResult || body.Reason != terminalproto.AccountsConflict || m.RequestID != ask.RequestID {
		t.Fatal(ok, m)
	}
	if _, err := terminalproto.Encode(m); err != nil {
		t.Fatal(err)
	}
	if _, ok := unavailable(ask, terminalproto.ModelsFeature); ok {
		t.Fatal("answered a sign-in request that was not negotiated")
	}
}
