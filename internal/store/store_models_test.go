package store

import (
	"errors"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func listedModelsRow(agent, tool string) MachineModels {
	return MachineModels{
		AgentID: agent, Tool: tool, State: MachineModelsListed,
		Models: []terminalproto.ModelEntry{
			{ID: "anthropic/claude-sonnet-4-5", Provider: "anthropic"},
			{ID: "openai/gpt-5", Provider: "openai"},
		},
		More: true, Source: "opencode models", AgentVersion: "1.19.5", FetchedAt: 1000, CheckedAt: 1000,
	}
}

func TestMachineModelsMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	a, _ := terminalAgent(t, s, uid, "laptop")
	row := listedModelsRow(a.ID, "opencode")
	if err := s.UpsertMachineModelsFor(uid, row); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal("reopen", err)
	}
	defer s.Close()
	got, ok, err := s.MachineModelsFor(uid, a.ID, "opencode")
	if err != nil || !ok {
		t.Fatal("row did not survive reopen", got, ok, err)
	}
	var columns int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('machine_agent_models')`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 11 {
		t.Fatal("machine_agent_models columns", columns)
	}
}

func TestMachineModelsListsRoundTripAndAllRows(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	b, _ := terminalAgent(t, s, uid, "desktop")
	c, _ := terminalAgent(t, s, uid, "tablet")
	first := listedModelsRow(a.ID, "opencode")
	second := listedModelsRow(b.ID, "opencode")
	second.Models = []terminalproto.ModelEntry{{ID: "orphan-model", Provider: ""}}
	second.More, second.FetchedAt, second.CheckedAt = false, 2000, 2100
	if err := s.UpsertMachineModelsFor(uid, first); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMachineModelsFor(uid, second); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertMachineModelsFor(uid, listedModelsRow(c.ID, "claude")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.MachineModelsFor(uid, a.ID, "opencode")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if got.State != MachineModelsListed || got.Reason != "" || got.Source != "opencode models" || got.AgentVersion != "1.19.5" || !got.More || got.FetchedAt != 1000 || got.CheckedAt != 1000 {
		t.Fatal(got)
	}
	if len(got.Models) != 2 || got.Models[0] != first.Models[0] || got.Models[1] != first.Models[1] {
		t.Fatal("models round trip", got.Models)
	}
	all, err := s.MachineModelsAllFor(uid, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatal("all rows", all)
	}
	byAgent := map[string]MachineModels{}
	for _, row := range all {
		byAgent[row.AgentID] = row
	}
	if byAgent[a.ID].State != MachineModelsListed || byAgent[b.ID].State != MachineModelsListed || len(byAgent[a.ID].Models) != 2 || len(byAgent[b.ID].Models) != 1 {
		t.Fatal("all rows", all)
	}
	if _, ok, err := s.MachineModelsFor(uid, c.ID, "opencode"); err != nil || ok {
		t.Fatal("a row for another tool was returned", ok, err)
	}
}

func TestMachineModelsFailedRefreshKeepsTheLastGoodList(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	good := listedModelsRow(a.ID, "opencode")
	if err := s.UpsertMachineModelsFor(uid, good); err != nil {
		t.Fatal(err)
	}
	failed := MachineModels{
		AgentID: a.ID, Tool: "opencode", State: MachineModelsFailed, Reason: terminalproto.ModelsTimedOut,
		Models: []terminalproto.ModelEntry{{ID: "must-not-replace", Provider: ""}}, More: false,
		Source: "other", AgentVersion: "9.9.9", FetchedAt: 9999, CheckedAt: 2000,
	}
	if err := s.UpsertMachineModelsFor(uid, failed); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.MachineModelsFor(uid, a.ID, "opencode")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if got.State != MachineModelsFailed || got.Reason != terminalproto.ModelsTimedOut || got.CheckedAt != 2000 {
		t.Fatal("state not updated", got)
	}
	if len(got.Models) != 2 || got.Models[0] != good.Models[0] || got.Models[1] != good.Models[1] {
		t.Fatal("failed refresh replaced the last good models", got.Models)
	}
	if !got.More || got.Source != good.Source || got.AgentVersion != good.AgentVersion || got.FetchedAt != good.FetchedAt {
		t.Fatal("failed refresh replaced last good metadata", got)
	}
}

func TestMachineModelsUnsupportedAndOffAreStoredWithoutAModelsList(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	for name, tc := range map[string]struct {
		tool   string
		state  string
		reason terminalproto.ModelsReason
	}{
		"unsupported": {"claude", MachineModelsUnsupported, terminalproto.ModelsUnsupported},
		"off":         {"opencode", MachineModelsOff, terminalproto.ModelsOff},
	} {
		row := MachineModels{AgentID: a.ID, Tool: tc.tool, State: tc.state, Reason: tc.reason, CheckedAt: 3000}
		if err := s.UpsertMachineModelsFor(uid, row); err != nil {
			t.Fatal(name, err)
		}
		got, ok, err := s.MachineModelsFor(uid, a.ID, tc.tool)
		if err != nil || !ok {
			t.Fatal(name, ok, err)
		}
		if got.State != tc.state || got.Reason != tc.reason || got.CheckedAt != 3000 || len(got.Models) != 0 || got.FetchedAt != 0 {
			t.Fatal(name, got)
		}
	}
}

func TestMachineModelsBoundsTheListAtFiveHundred(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	makeModels := func(n int) []terminalproto.ModelEntry {
		out := make([]terminalproto.ModelEntry, n)
		for i := range out {
			out[i] = terminalproto.ModelEntry{ID: fmt.Sprintf("model-%d", i)}
		}
		return out
	}
	row := MachineModels{AgentID: a.ID, Tool: "opencode", State: MachineModelsListed, Models: makeModels(terminalproto.MaxModels), FetchedAt: 1, CheckedAt: 1}
	if err := s.UpsertMachineModelsFor(uid, row); err != nil {
		t.Fatal("a list at the cap", err)
	}
	got, ok, err := s.MachineModelsFor(uid, a.ID, "opencode")
	if err != nil || !ok || len(got.Models) != terminalproto.MaxModels {
		t.Fatal("a list at the cap was not stored", len(got.Models), err)
	}
	row.Models = makeModels(terminalproto.MaxModels + 1)
	if err := s.UpsertMachineModelsFor(uid, row); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("a list over the cap", err)
	}
}

func TestMachineModelsRejectsBadInput(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	for name, row := range map[string]MachineModels{
		"tool":   {AgentID: a.ID, Tool: "pi", State: MachineModelsListed},
		"state":  {AgentID: a.ID, Tool: "opencode", State: "live"},
		"reason": {AgentID: a.ID, Tool: "opencode", State: MachineModelsFailed, Reason: "because"},
		"agent":  {AgentID: "short", Tool: "opencode", State: MachineModelsListed},
	} {
		if err := s.UpsertMachineModelsFor(uid, row); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatal(name, err)
		}
	}
}

func TestForgetMachineDeletesItsModelListsOnly(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	b, _ := terminalAgent(t, s, uid, "desktop")
	other, err := s.CreateUser("models-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, _ := terminalAgent(t, s, other.ID, "laptop")
	for _, in := range []struct {
		uid int64
		row MachineModels
	}{
		{uid, listedModelsRow(a.ID, "opencode")},
		{uid, listedModelsRow(b.ID, "opencode")},
		{other.ID, listedModelsRow(otherAgent.ID, "opencode")},
	} {
		if err := s.UpsertMachineModelsFor(in.uid, in.row); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ForgetMachineFor(uid, "laptop"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.MachineModelsFor(uid, a.ID, "opencode"); err != nil || ok {
		t.Fatal("the forgotten machine kept its model list", ok, err)
	}
	if _, ok, err := s.MachineModelsFor(uid, b.ID, "opencode"); err != nil || !ok {
		t.Fatal("forget removed another machine's list", ok, err)
	}
	if _, ok, err := s.MachineModelsFor(other.ID, otherAgent.ID, "opencode"); err != nil || !ok {
		t.Fatal("forget removed another account's list", ok, err)
	}
}

func TestRevokingAnEnrollmentDeletesItsModelLists(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	if err := s.UpsertMachineModelsFor(uid, listedModelsRow(a.ID, "opencode")); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	all, err := s.MachineModelsAllFor(uid, "opencode")
	if err != nil || len(all) != 0 {
		t.Fatal("revoked enrollment kept its model list", all, err)
	}
}
