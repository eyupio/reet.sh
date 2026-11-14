package terminalbroker

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func accountsCapabilities() terminalproto.Capabilities {
	caps := browseCapabilities()
	caps.AgentAccounts = true
	return caps
}

func TestAccountsAreForwardedExactlyAndNeedTheCapability(t *testing.T) {
	f := browseFixture(t, accountsCapabilities())
	want := terminalproto.AccountsResult{Accounts: []terminalproto.AgentAccount{
		{Tool: "claude", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", Plan: "max",
			LimitsState: terminalproto.AccountLimitsRead, LimitsAt: 5, Limits: []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitSession, UsedPercent: 19}}},
		{Tool: "codex", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked},
	}}
	f.answering(func(terminalproto.Body) terminalproto.Body { return want })
	req := terminalproto.Accounts{Limits: true, Refresh: true}
	got, err := f.b.Accounts(t.Context(), f.uid, f.agent, req)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal(got, err)
	}
	if sent := f.sentRequests(); len(sent) != 1 || !reflect.DeepEqual(sent[0], req) {
		t.Fatalf("request not forwarded exactly: %+v", sent)
	}

	old := browseFixture(t, browseCapabilities()) // no agent-accounts-v1, or terminal access off
	if _, err := old.b.Accounts(t.Context(), old.uid, old.agent, req); !errors.Is(err, ErrAccountsUnsupported) {
		t.Fatalf("err = %v, want unsupported", err)
	}
	if len(old.sentRequests()) != 0 {
		t.Fatal("a sign-in request was sent to a machine that did not report the capability")
	}
}

// Every reason the machine gives comes back as its answer for the caller to
// word; none is an error.
func TestAccountsReturnEachReason(t *testing.T) {
	f := browseFixture(t, accountsCapabilities())
	for _, reason := range []terminalproto.AccountsReason{terminalproto.AccountsOff, terminalproto.AccountsConflict, terminalproto.AccountsTimedOut} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.AccountsResult{Reason: reason} })
		got, err := f.b.Accounts(t.Context(), f.uid, f.agent, terminalproto.Accounts{})
		if err != nil || got.Reason != reason {
			t.Fatalf("%s: %+v %v", reason, got, err)
		}
	}
}

func TestAccountsRefuseAMalformedRequestBeforeSending(t *testing.T) {
	f := browseFixture(t, accountsCapabilities())
	if _, err := f.b.Accounts(t.Context(), f.uid, f.agent, terminalproto.Accounts{Refresh: true}); !errors.Is(err, terminalproto.ErrInvalid) {
		t.Fatal(err)
	}
	if len(f.sentRequests()) != 0 {
		t.Fatal("a malformed request was sent")
	}
}

func TestAccountsAnswerOfAnotherKindIsRefused(t *testing.T) {
	f := browseFixture(t, accountsCapabilities())
	f.answering(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.ModelsResult{Tool: "opencode", Reason: terminalproto.ModelsOff}
	})
	if _, err := f.b.Accounts(t.Context(), f.uid, f.agent, terminalproto.Accounts{}); !errors.Is(err, ErrMachineAnswer) {
		t.Fatalf("err = %v, want an unexpected answer", err)
	}
}

func TestAccountsCallTimeoutOutlastsTheMachinesBound(t *testing.T) {
	if AccountsCallTimeout != 14*time.Second {
		t.Fatal(AccountsCallTimeout)
	}
}

// An answer that arrives after its request gave up is counted and dropped;
// it must not end the machine's connection.
func TestALateAccountsAnswerIsDroppedAndTheConnectionKept(t *testing.T) {
	ordinary := AccountsCallTimeout
	AccountsCallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { AccountsCallTimeout = ordinary })
	f := browseFixture(t, accountsCapabilities())
	if _, err := f.b.Accounts(t.Context(), f.uid, f.agent, terminalproto.Accounts{}); !errors.Is(err, ErrMachineTimeout) {
		t.Fatal("timeout", err)
	}
	held := f.heldReplies()
	if len(held) != 1 {
		t.Fatal("held", held)
	}
	before := f.b.LateReplies()
	held[0].Body = terminalproto.AccountsResult{Reason: terminalproto.AccountsTimedOut}
	if err := f.deliver(held[0]); err != nil {
		t.Fatal("a late answer was refused:", err)
	}
	if f.b.LateReplies() != before+1 {
		t.Fatal("late replies counted", f.b.LateReplies()-before)
	}
	AccountsCallTimeout = 2 * time.Second
	f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.AccountsResult{} })
	if _, err := f.b.Accounts(t.Context(), f.uid, f.agent, terminalproto.Accounts{}); err != nil {
		t.Fatal("a late answer ended the connection:", err)
	}
}
