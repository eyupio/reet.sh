package agenthealth

import (
	"context"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const removeFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func repoCloneRemoveConfig(t *testing.T) AgentConfig {
	cfg := repoCloneConfig(t)
	cfg.repoCloneRemove = true
	return cfg
}

func cloneRemoveRequests() map[string]terminalproto.Message {
	dir := "/home/u/reet/repos/o/r"
	return map[string]terminalproto.Message{
		"status": {Version: terminalproto.Version, RequestID: "clone-status-0000001", Generation: 1, Body: &terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r"}},
		"remove": {Version: terminalproto.Version, RequestID: "clone-remove-0000001", Generation: 1, Body: &terminalproto.RepoCloneRemove{Dir: dir, Origin: "o/r", Fingerprint: removeFingerprint}},
	}
}

// RepoCloneRemove is reported only with launch and worktrees (so git), the
// negotiated repo-clone-remove-v1 and terminal access on. It does not need
// gh or the clone feature: removing reaches no network.
func TestRepoCloneRemoveCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.repoCloneRemove = false }, false},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, true},
		{"clone not negotiated", func(c *AgentConfig) { c.repoClone = false }, true},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
		{"worktree not negotiated", func(c *AgentConfig) { c.worktree = false }, false},
		{"launch not negotiated", func(c *AgentConfig) { c.launch = false }, false},
		{"no terminal control", func(c *AgentConfig) { c.TerminalSession = nil }, false},
		{"terminal access off", func(c *AgentConfig) {
			c.TerminalEnabled, c.TerminalReadiness = false, terminalproto.DisabledLocally
		}, false},
	} {
		cfg := repoCloneRemoveConfig(t)
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.RepoCloneRemove != tc.want {
			t.Fatal(tc.name, caps.RepoCloneRemove)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

func TestRepoCloneRemoveFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                                   false,
		terminalproto.RepoCloneFeature:       false,
		terminalproto.RepoCloneRemoveFeature: true,
		"repo-clone-v1, " + "repo-clone-remove-v1": true,
		"repo-clone-remove-v2":                     false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.repoCloneRemove != want || terminalproto.SupportsRepoCloneRemove(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: repoCloneRemove %v, features %q", advertised, cfg.repoCloneRemove, terminalFeatures(cfg))
		}
	}
}

// A status or remove the connector would not advertise now is answered
// denied here; one it would, or one that was never negotiated (a protocol
// error there), goes to the terminal handler.
func TestRepoCloneRemoveRequestsFollowTheAdvertisedCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	denied := map[string]terminalproto.Body{
		"status": terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied},
		"remove": terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneDenied},
	}
	for kind, request := range cloneRemoveRequests() {
		for _, tc := range []struct {
			name      string
			edit      func(*AgentConfig)
			forwarded bool
		}{
			{"everything on", func(*AgentConfig) {}, true},
			{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, true},
			{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
			{"terminal access off", func(c *AgentConfig) { c.TerminalAllowed = func() bool { return false } }, false},
			{"not negotiated", func(c *AgentConfig) { c.repoCloneRemove = false }, true},
		} {
			cfg := repoCloneRemoveConfig(t)
			tc.edit(&cfg)
			var forwarded int
			var sent []terminalproto.Message
			gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
				func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
			if err := gate(t.Context(), request); err != nil {
				t.Fatal(kind, tc.name, err)
			}
			if tc.forwarded {
				if forwarded != 1 || len(sent) != 0 {
					t.Fatal(kind, tc.name, forwarded, sent)
				}
				continue
			}
			if forwarded != 0 || len(sent) != 1 || sent[0].RequestID != request.RequestID || sent[0].Body != denied[kind] {
				t.Fatal(kind, tc.name, forwarded, sent)
			}
			if _, err := terminalproto.Encode(sent[0]); err != nil {
				t.Fatal(kind, tc.name, err)
			}
		}
	}
}

// A status or remove the dispatcher cannot take is answered busy, only when
// it was negotiated.
func TestRepoCloneRemoveUnavailableIsBusy(t *testing.T) {
	busy := map[string]terminalproto.Body{
		"status": terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneBusy},
		"remove": terminalproto.RepoCloneRemoved{Reason: terminalproto.RepoCloneBusy},
	}
	for kind, request := range cloneRemoveRequests() {
		m, ok := unavailable(request, terminalproto.RepoCloneRemoveFeature)
		if !ok || m.Body != busy[kind] || m.RequestID != request.RequestID {
			t.Fatal(kind, ok, m)
		}
		if _, err := terminalproto.Encode(m); err != nil {
			t.Fatal(kind, err)
		}
		if _, ok := unavailable(request, terminalproto.RepoCloneFeature); ok {
			t.Fatal(kind, "answered without the negotiated feature")
		}
	}
}
