package store

import (
	"errors"
	"reflect"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func machineAccountsRow() []terminalproto.AgentAccount {
	return []terminalproto.AgentAccount{
		{Tool: "claude", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", Plan: "max",
			LimitsState: terminalproto.AccountLimitsRead, LimitsAt: 900,
			Limits: []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitSession, UsedPercent: 19, ResetsAt: 5000}, {Kind: terminalproto.AccountLimitWeek, Label: "Opus", UsedPercent: 60}}},
		{Tool: "codex", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked},
	}
}

func TestMachineAccountsMigrationIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	a, _ := terminalAgent(t, s, uid, "laptop")
	if err := s.SaveMachineAccountsFor(uid, a.ID, machineAccountsRow(), 1000); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal("reopen", err)
	}
	defer s.Close()
	got, ok, err := s.MachineAccountsFor(uid, a.ID)
	if err != nil || !ok || got.CheckedAt != 1000 || !reflect.DeepEqual(got.Accounts, machineAccountsRow()) {
		t.Fatal("row did not survive reopen", got, ok, err)
	}
}

func TestMachineAccountsAreReplacedWholeAndReadPerMachine(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	b, _ := terminalAgent(t, s, uid, "desktop")
	if _, ok, err := s.MachineAccountsFor(uid, a.ID); err != nil || ok {
		t.Fatal("a machine never read has an answer", ok, err)
	}
	if err := s.SaveMachineAccountsFor(uid, a.ID, machineAccountsRow(), 1000); err != nil {
		t.Fatal(err)
	}
	// A later answer replaces the earlier one whole: an account that signed
	// out must not keep its name from the answer before.
	later := []terminalproto.AgentAccount{{Tool: "claude", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}}
	if err := s.SaveMachineAccountsFor(uid, a.ID, later, 2000); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.MachineAccountsFor(uid, a.ID)
	if err != nil || !ok || got.CheckedAt != 2000 || !reflect.DeepEqual(got.Accounts, later) || got.AgentID != a.ID {
		t.Fatal(got, ok, err)
	}
	if _, ok, err := s.MachineAccountsFor(uid, b.ID); err != nil || ok {
		t.Fatal("another machine read this one's answer", ok, err)
	}
	// An answer with nothing readable is still an answer.
	if err := s.SaveMachineAccountsFor(uid, b.ID, nil, 3000); err != nil {
		t.Fatal(err)
	}
	got, ok, err = s.MachineAccountsFor(uid, b.ID)
	if err != nil || !ok || len(got.Accounts) != 0 || got.Accounts == nil {
		t.Fatal("an empty answer", got, ok, err)
	}
}

func TestMachineAccountsRefuseWhatAConnectorMayNotAnswer(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	for name, accounts := range map[string][]terminalproto.AgentAccount{
		"unknown tool":   {{Tool: "nope", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}},
		"the same agent": {machineAccountsRow()[1], machineAccountsRow()[1]},
		"no state":       {{Tool: "claude"}},
	} {
		if err := s.SaveMachineAccountsFor(uid, a.ID, accounts, 1000); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := s.SaveMachineAccountsFor(uid, "not an id", machineAccountsRow(), 1000); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("a malformed machine id:", err)
	}
	if err := s.SaveMachineAccountsFor(uid, a.ID, machineAccountsRow(), 0); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal("an answer with no time:", err)
	}
	if _, ok, _ := s.MachineAccountsFor(uid, a.ID); ok {
		t.Fatal("a refused answer was kept")
	}
	// A machine of another account cannot be written through this one.
	other, err := s.CreateUser("accounts-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveMachineAccountsFor(other.ID, a.ID, machineAccountsRow(), 1000); err == nil {
		t.Fatal("an answer was kept for a machine the account does not own")
	}
	if _, ok, _ := s.MachineAccountsFor(other.ID, a.ID); ok {
		t.Fatal("another account read this machine's answer")
	}
}

func TestForgetMachineDeletesItsAccountsOnly(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	b, _ := terminalAgent(t, s, uid, "desktop")
	other, err := s.CreateUser("accounts-forget@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, _ := terminalAgent(t, s, other.ID, "laptop")
	for _, in := range []struct {
		uid   int64
		agent string
	}{{uid, a.ID}, {uid, b.ID}, {other.ID, otherAgent.ID}} {
		if err := s.SaveMachineAccountsFor(in.uid, in.agent, machineAccountsRow(), 1000); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.ForgetMachineFor(uid, "laptop"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.MachineAccountsFor(uid, a.ID); err != nil || ok {
		t.Fatal("the forgotten machine kept its accounts", ok, err)
	}
	if _, ok, err := s.MachineAccountsFor(uid, b.ID); err != nil || !ok {
		t.Fatal("forget removed another machine's accounts", ok, err)
	}
	if _, ok, err := s.MachineAccountsFor(other.ID, otherAgent.ID); err != nil || !ok {
		t.Fatal("forget removed another account's answer", ok, err)
	}
}

func TestRevokingAnEnrollmentDeletesItsAccounts(t *testing.T) {
	s, uid, a, _ := terminalFixture(t)
	if err := s.SaveMachineAccountsFor(uid, a.ID, machineAccountsRow(), 1000); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := s.MachineAccountsFor(uid, a.ID); err != nil || ok {
		t.Fatal("a revoked enrollment kept its accounts", ok, err)
	}
}
