package store

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func gatedStoreCaps() terminalproto.Capabilities {
	entries := []terminalproto.ACPAgent{
		{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1"},
		{Tool: "codex", Adapter: "codex-acp", State: "available", MinimumVersion: "1.13.1"},
		{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true},
	}
	return terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, HealthEnabled: true, TerminalControl: true,
		Readiness: terminalproto.Ready, Launch: true, AgentsV2: true, Tools: []string{"claude", "codex", "opencode"}, ACP: true, ACPHistory: true, ACPAgents: &entries}
}

func descriptorStates(c terminalproto.Capabilities) map[string]string {
	out := map[string]string{}
	if c.ACPAgents != nil {
		for _, a := range *c.ACPAgents {
			out[a.Tool] = a.State
		}
	}
	return out
}

// An account that has made no choice has every gated chat off. A saved choice reads back and survives a reopen.
func TestChatAgentsRoundTripAndDefaultToOff(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.ProfileFor(user.ID)
	if err != nil || p.ChatEnabled("claude") || p.ChatEnabled("codex") || !p.ChatEnabled("opencode") || len(p.ChatAgents) != 0 {
		t.Fatalf("a fresh profile: %+v %v", p, err)
	}
	p.ChatAgents = map[string]bool{"claude": true}
	if err := st.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	st.Close()
	st, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if p, err = st.ProfileFor(user.ID); err != nil || !p.ChatEnabled("claude") || p.ChatEnabled("codex") || !reflect.DeepEqual(p.ChatAgents, map[string]bool{"claude": true}) {
		t.Fatalf("the choice did not survive a reopen: %+v %v", p, err)
	}
}

// A database from before the setting existed gains the column, keeps the profile, and a second open changes nothing.
func TestChatAgentsColumnMigratesAnOldDatabase(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProfileFor(user.ID, Profile{Theme: "dark", ShelfView: "board", HostedPrefix: DefaultHostedPrefix}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`ALTER TABLE user_profiles DROP COLUMN chat_agents`); err != nil {
		t.Fatal(err)
	}
	st.Close()
	for i := 0; i < 2; i++ {
		st, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		p, err := st.ProfileFor(user.ID)
		if err != nil || p.Theme != "dark" || p.ShelfView != "board" || p.ChatEnabled("claude") || len(p.ChatAgents) != 0 {
			t.Fatalf("open %d: %+v %v", i, p, err)
		}
		st.Close()
	}
}

// There is no separate profile export: the profile is a table that backups copy with the database, and its JSON form
// (what the API and any export carry) keeps the explicit choices.
func TestChatAgentsKeepTheirExplicitChoicesInTheProfileJSON(t *testing.T) {
	raw, err := json.Marshal(Profile{ChatAgents: map[string]bool{"claude": true, "codex": false}})
	if err != nil {
		t.Fatal(err)
	}
	var back Profile
	if err := json.Unmarshal(raw, &back); err != nil || !reflect.DeepEqual(back.ChatAgents, map[string]bool{"claude": true, "codex": false}) {
		t.Fatalf("round trip: %s %+v %v", raw, back, err)
	}
	if !back.ChatEnabled("claude") || back.ChatEnabled("codex") {
		t.Fatalf("effective values: %+v", back)
	}
}

// The controller hides a gated agent from an account that has it off, applying the setting when the record is read for
// use. The stored record keeps what the connector reported, so turning a chat on needs no new report.
func TestRemoteCapabilitiesAreMaskedByTheAccountSetting(t *testing.T) {
	s, uid, agent, _ := terminalFixture(t)
	if _, err := s.BeginRemoteConnectionFor(uid, agent.ID, syncID(), gatedStoreCaps()); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]string {
		c, err := s.RemoteCapabilitiesFor(uid, agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		return descriptorStates(c.Capabilities)
	}
	if got := read(); got["claude"] != "unverified" || got["codex"] != "unverified" || got["opencode"] != "available" {
		t.Fatalf("with no choice: %v", got)
	}
	p, _ := s.ProfileFor(uid)
	p.ChatAgents = map[string]bool{"claude": true}
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	if got := read(); got["claude"] != "available" || got["codex"] != "unverified" {
		t.Fatalf("claude turned on: %v", got)
	}
	raw, err := s.RemoteCapabilitiesRawFor(uid, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := descriptorStates(raw.Capabilities); got["claude"] != "available" || got["codex"] != "available" {
		t.Fatalf("the raw read is masked: %v", got)
	}
	p.ChatAgents = nil
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	if got := read(); got["claude"] != "unverified" {
		t.Fatalf("claude turned off again: %v", got)
	}
}
