package agenthealth

import (
	"context"
	"net/http"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func repoCloneConfig(t *testing.T) AgentConfig {
	factory := func(context.Context, *http.Client, http.Header, int64, Sender) (func(context.Context, terminalproto.Message) error, func()) {
		return nil, func() {}
	}
	return AgentConfig{
		TerminalSession: factory, TerminalEnabled: true, TerminalReadiness: terminalproto.Ready,
		launch: true, worktree: true, repoClone: true,
		LaunchTools:  func() []string { return []string{"claude"} },
		GitAvailable: func() bool { return true },
		GHAvailable:  func() bool { return true },
		Consent: func() (bool, terminalproto.Readiness, error) {
			t.Error("the readiness probe ran for a clone request")
			return true, terminalproto.Ready, nil
		},
		TerminalAllowed: func() bool { return true },
	}
}

// RepoClone is reported only with launch and worktrees, the negotiated
// repo-clone-v1, and both git and gh found.
func TestRepoCloneCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.repoClone = false }, false},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"no gh lookup", func(c *AgentConfig) { c.GHAvailable = nil }, false},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
		{"worktree not negotiated", func(c *AgentConfig) { c.worktree = false }, false},
		{"launch not negotiated", func(c *AgentConfig) { c.launch = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
	} {
		cfg := repoCloneConfig(t)
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.RepoClone != tc.want {
			t.Fatal(tc.name, caps.RepoClone)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

func TestRepoCloneFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                                  false,
		terminalproto.WorktreeFeature:       false,
		terminalproto.RepoCloneFeature:      true,
		"base-fresh-v1, " + "repo-clone-v1": true,
		"repo-clone-v2":                     false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.repoClone != want || terminalproto.SupportsRepoClone(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: repoClone %v, features %q", advertised, cfg.repoClone, terminalFeatures(cfg))
		}
	}
}

// A clone the connector would not advertise now is answered denied here;
// one it would, or one that was never negotiated (a protocol error there),
// goes to the terminal handler.
func TestRepoCloneRequestsFollowTheAdvertisedCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	clone := terminalproto.Message{Version: terminalproto.Version, RequestID: "clone-request-000001", Generation: 1, Body: &terminalproto.RepoClone{Owner: "o", Repo: "r"}}
	for _, tc := range []struct {
		name      string
		edit      func(*AgentConfig)
		forwarded bool
	}{
		{"everything on", func(*AgentConfig) {}, true},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
		{"not negotiated", func(c *AgentConfig) { c.repoClone = false }, true},
	} {
		cfg := repoCloneConfig(t)
		tc.edit(&cfg)
		var forwarded int
		var sent []terminalproto.Message
		gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
			func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
		if err := gate(t.Context(), clone); err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.forwarded {
			if forwarded != 1 || len(sent) != 0 {
				t.Fatal(tc.name, forwarded, sent)
			}
			continue
		}
		if forwarded != 0 || len(sent) != 1 || sent[0].RequestID != clone.RequestID ||
			sent[0].Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneDenied}) {
			t.Fatal(tc.name, forwarded, sent)
		}
	}
}

// A clone the dispatcher cannot take is answered busy, only when it was
// negotiated.
func TestRepoCloneUnavailableIsBusy(t *testing.T) {
	clone := terminalproto.Message{Version: terminalproto.Version, RequestID: "clone-request-000001", Generation: 1, Body: &terminalproto.RepoClone{Owner: "o", Repo: "r"}}
	m, ok := unavailable(clone, terminalproto.RepoCloneFeature)
	if !ok || m.Body != (terminalproto.RepoCloneResult{Reason: terminalproto.RepoCloneBusy}) || m.RequestID != clone.RequestID {
		t.Fatal(ok, m)
	}
	if _, err := terminalproto.Encode(m); err != nil {
		t.Fatal(err)
	}
	if _, ok := unavailable(clone, terminalproto.BaseFreshFeature); ok {
		t.Fatal("answered a clone that was not negotiated")
	}
}
