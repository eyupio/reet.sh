package agenthealth

import (
	"context"
	"reflect"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func repoListConfig(t *testing.T) AgentConfig {
	cfg := repoCloneConfig(t)
	cfg.repoList = true
	return cfg
}

func repoListRequest() terminalproto.Message {
	return terminalproto.Message{Version: terminalproto.Version, RequestID: "repo-list-000000001", Generation: 1, Body: &terminalproto.RepoList{}}
}

// RepoList is reported exactly where RepoClone is: launch and worktrees, gh
// on the machine, the negotiated repo-list-v1 and terminal access on.
func TestRepoListCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.repoList = false }, false},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"no gh check", func(c *AgentConfig) { c.GHAvailable = nil }, false},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
		{"worktree not negotiated", func(c *AgentConfig) { c.worktree = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
		{"terminal access off", func(c *AgentConfig) {
			c.TerminalEnabled, c.TerminalReadiness = false, terminalproto.DisabledLocally
		}, false},
	} {
		cfg := repoListConfig(t)
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.RepoList != tc.want {
			t.Fatal(tc.name, caps.RepoList)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

func TestRepoListFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                             false,
		terminalproto.RepoCloneFeature: false,
		terminalproto.RepoListFeature:  true,
		"repo-clone-v1, repo-list-v1":  true,
		"repo-list-v2":                 false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.repoList != want || terminalproto.SupportsRepoList(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: repoList %v, features %q", advertised, cfg.repoList, terminalFeatures(cfg))
		}
	}
}

// A listing the connector would not advertise now is answered denied here;
// one it would, or one that was never negotiated (a protocol error there),
// goes to the terminal handler.
func TestRepoListRequestsFollowTheAdvertisedCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	request := repoListRequest()
	for _, tc := range []struct {
		name      string
		edit      func(*AgentConfig)
		forwarded bool
	}{
		{"everything on", func(*AgentConfig) {}, true},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"terminal access off", func(c *AgentConfig) { c.TerminalAllowed = func() bool { return false } }, false},
		{"not negotiated", func(c *AgentConfig) { c.repoList = false }, true},
	} {
		cfg := repoListConfig(t)
		tc.edit(&cfg)
		var forwarded int
		var sent []terminalproto.Message
		gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
			func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
		if err := gate(t.Context(), request); err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.forwarded {
			if forwarded != 1 || len(sent) != 0 {
				t.Fatal(tc.name, forwarded, sent)
			}
			continue
		}
		want := terminalproto.RepoListResult{Reason: terminalproto.RepoListDenied}
		if forwarded != 0 || len(sent) != 1 || sent[0].RequestID != request.RequestID || !reflect.DeepEqual(sent[0].Body, want) {
			t.Fatal(tc.name, forwarded, sent)
		}
		if _, err := terminalproto.Encode(sent[0]); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

// A listing the dispatcher cannot take is answered busy, only when it was
// negotiated.
func TestRepoListUnavailableIsBusy(t *testing.T) {
	request := repoListRequest()
	m, ok := unavailable(request, terminalproto.RepoListFeature)
	if !ok || !reflect.DeepEqual(m.Body, terminalproto.RepoListResult{Reason: terminalproto.RepoListBusy}) || m.RequestID != request.RequestID {
		t.Fatal(ok, m)
	}
	if _, err := terminalproto.Encode(m); err != nil {
		t.Fatal(err)
	}
	if _, ok := unavailable(request, terminalproto.RepoCloneFeature); ok {
		t.Fatal("answered without the negotiated feature")
	}
}
