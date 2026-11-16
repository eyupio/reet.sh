package agenthealth

import (
	"context"
	"runtime"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func repoCloneFetchConfig(t *testing.T) AgentConfig {
	cfg := repoCloneRemoveConfig(t)
	cfg.repoCloneFetch = true
	return cfg
}

// RepoCloneFetch is reported where a clone can be reported on and the machine
// can reach github.com as a clone does: gh found, terminal access on, and the
// negotiated repo-clone-fetch-v1.
func TestRepoCloneFetchCapability(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	for _, tc := range []struct {
		name string
		edit func(*AgentConfig)
		want bool
	}{
		{"everything", func(*AgentConfig) {}, true},
		{"not negotiated", func(c *AgentConfig) { c.repoCloneFetch = false }, false},
		{"removal not negotiated", func(c *AgentConfig) { c.repoCloneRemove = false }, false},
		{"gh not found", func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"git not found", func(c *AgentConfig) { c.GitAvailable = func() bool { return false } }, false},
		{"terminal access off", func(c *AgentConfig) {
			c.TerminalEnabled, c.TerminalReadiness = false, terminalproto.DisabledLocally
		}, false},
	} {
		cfg := repoCloneFetchConfig(t)
		tc.edit(&cfg)
		caps := capabilities(cfg)
		if caps.RepoCloneFetch != tc.want {
			t.Fatal(tc.name, caps.RepoCloneFetch)
		}
		if err := caps.ValidateFor(terminalFeatures(cfg)); err != nil {
			t.Fatal(tc.name, err)
		}
	}
}

func TestRepoCloneFetchFeatureFollowsTheControllerAdvertisement(t *testing.T) {
	for advertised, want := range map[string]bool{
		"":                                   false,
		terminalproto.RepoCloneRemoveFeature: false,
		terminalproto.RepoCloneFetchFeature:  true,
		"repo-clone-remove-v1, repo-clone-fetch-v1": true,
		"repo-clone-fetch-v2":                       false,
	} {
		cfg := AgentConfig{}
		negotiateFeatures(&cfg, advertised)
		if cfg.repoCloneFetch != want || terminalproto.SupportsRepoCloneFetch(terminalFeatures(cfg)) != want {
			t.Fatalf("%q: repoCloneFetch %v, features %q", advertised, cfg.repoCloneFetch, terminalFeatures(cfg))
		}
	}
}

// A status that asks to fetch is answered denied when the machine would not
// advertise the capability now (gh gone), and is never taken without the
// negotiated feature; a status that does not ask is unaffected.
func TestRepoCloneFetchStatusIsGated(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("terminal control is Linux and macOS only")
	}
	dir := "/home/u/reet/repos/o/r"
	status := func(fetch bool) terminalproto.Message {
		return terminalproto.Message{Version: terminalproto.Version, RequestID: "clone-status-0000001", Generation: 1, Body: &terminalproto.RepoCloneStatus{Dir: dir, Origin: "o/r", Fetch: fetch}}
	}
	for _, tc := range []struct {
		name      string
		fetch     bool
		edit      func(*AgentConfig)
		forwarded bool
	}{
		{"fetch, everything on", true, func(*AgentConfig) {}, true},
		{"fetch, gh not found", true, func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, false},
		{"no fetch, gh not found", false, func(c *AgentConfig) { c.GHAvailable = func() bool { return false } }, true},
	} {
		cfg := repoCloneFetchConfig(t)
		tc.edit(&cfg)
		var forwarded int
		var sent []terminalproto.Message
		gate := gateMachineRequests(cfg, func(context.Context, terminalproto.Message) error { forwarded++; return nil },
			func(_ context.Context, m terminalproto.Message) error { sent = append(sent, m); return nil })
		if err := gate(t.Context(), status(tc.fetch)); err != nil {
			t.Fatal(tc.name, err)
		}
		if tc.forwarded != (forwarded == 1) {
			t.Fatal(tc.name, forwarded, sent)
		}
		if !tc.forwarded && (len(sent) != 1 || sent[0].Body != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneDenied})) {
			t.Fatal(tc.name, sent)
		}
	}
	// Dispatcher busy: a fetching status needs the fetch feature to be answered.
	if _, ok := unavailable(status(true), terminalproto.RepoCloneRemoveFeature); ok {
		t.Fatal("a fetching status was answered without the fetch feature")
	}
	if m, ok := unavailable(status(true), terminalproto.RepoCloneRemoveFeature+","+terminalproto.RepoCloneFetchFeature); !ok || m.Body != (terminalproto.RepoCloneState{Reason: terminalproto.RepoCloneBusy}) {
		t.Fatal("busy not answered", ok, m)
	}
}
