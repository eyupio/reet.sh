package terminalproto

import (
	"bytes"
	"strings"
	"testing"
)

func TestAgentAccountsFeatureToken(t *testing.T) {
	for _, value := range []string{"", "models-v1", "agent-accounts-v10", "prefix-agent-accounts-v1"} {
		if SupportsAgentAccounts(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsAgentAccounts("models-v1, agent-accounts-v1") || AgentAccountsFeature != "agent-accounts-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestAccountsRequestValid(t *testing.T) {
	for name, c := range map[string]struct {
		in   Accounts
		want bool
	}{
		"sign-in only":      {Accounts{}, true},
		"with limits":       {Accounts{Limits: true}, true},
		"fresh limits":      {Accounts{Limits: true, Refresh: true}, true},
		"refresh of no one": {Accounts{Refresh: true}, false},
	} {
		if got := c.in.valid(); got != c.want {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.want)
		}
	}
}

func signedIn() AgentAccount {
	return AgentAccount{Tool: "claude", State: AccountSignedIn, Method: AccountSubscription, Account: "person@example.com", Plan: "max", LimitsState: AccountLimitsNotAsked}
}

func TestAgentAccountValid(t *testing.T) {
	limits := []AccountLimit{{Kind: AccountLimitSession, UsedPercent: 19, ResetsAt: 1790000000000}, {Kind: AccountLimitWeek, Label: "Opus", UsedPercent: 100}}
	with := func(change func(*AgentAccount)) AgentAccount { a := signedIn(); change(&a); return a }
	cases := map[string]struct {
		a    AgentAccount
		want bool
	}{
		"signed in, not asked":      {signedIn(), true},
		"signed in, read":           {with(func(a *AgentAccount) { a.LimitsState, a.Limits, a.LimitsAt = AccountLimitsRead, limits, 5 }), true},
		"last figures after a fail": {with(func(a *AgentAccount) { a.LimitsState, a.Limits, a.LimitsAt = AccountLimitsFailed, limits, 5 }), true},
		"renew with no figures":     {with(func(a *AgentAccount) { a.LimitsState = AccountLimitsRenew }), true},
		"api key":                   {AgentAccount{Tool: "codex", State: AccountSignedIn, Method: AccountAPIKey, LimitsState: AccountLimitsUnsupported}, true},
		"none":                      {AgentAccount{Tool: "codex", State: AccountNone, LimitsState: AccountLimitsNotAsked}, true},
		"unreadable":                {AgentAccount{Tool: "claude", State: AccountUnreadable, LimitsState: AccountLimitsNotAsked}, true},

		"unknown tool":             {with(func(a *AgentAccount) { a.Tool = "nope" }), false},
		"unknown state":            {with(func(a *AgentAccount) { a.State = "maybe" }), false},
		"unknown method":           {with(func(a *AgentAccount) { a.Method = "magic" }), false},
		"signed in with no method": {with(func(a *AgentAccount) { a.Method = "" }), false},
		"no limits state":          {with(func(a *AgentAccount) { a.LimitsState = "" }), false},
		"unknown limits state":     {with(func(a *AgentAccount) { a.LimitsState = "soon" }), false},
		"control in the name":      {with(func(a *AgentAccount) { a.Account = "a\nb" }), false},
		"long name":                {with(func(a *AgentAccount) { a.Account = strings.Repeat("a", 129) }), false},
		"long plan":                {with(func(a *AgentAccount) { a.Plan = strings.Repeat("p", 33) }), false},
		"read with no figures":     {with(func(a *AgentAccount) { a.LimitsState = AccountLimitsRead }), false},
		"figures with no time":     {with(func(a *AgentAccount) { a.LimitsState, a.Limits = AccountLimitsRead, limits }), false},
		"time with no figures":     {with(func(a *AgentAccount) { a.LimitsState, a.LimitsAt = AccountLimitsFailed, 5 }), false},
		"figures nobody asked for": {with(func(a *AgentAccount) { a.Limits, a.LimitsAt = limits, 5 }), false},
		"figures for an api key":   {AgentAccount{Tool: "codex", State: AccountSignedIn, Method: AccountAPIKey, LimitsState: AccountLimitsUnsupported, Limits: limits, LimitsAt: 5}, false},
		"over 100 percent": {with(func(a *AgentAccount) {
			a.LimitsState, a.LimitsAt, a.Limits = AccountLimitsRead, 5, []AccountLimit{{Kind: AccountLimitWeek, UsedPercent: 101}}
		}), false},
		"below nothing": {with(func(a *AgentAccount) {
			a.LimitsState, a.LimitsAt, a.Limits = AccountLimitsRead, 5, []AccountLimit{{Kind: AccountLimitWeek, UsedPercent: -1}}
		}), false},
		"unknown window": {with(func(a *AgentAccount) {
			a.LimitsState, a.LimitsAt, a.Limits = AccountLimitsRead, 5, []AccountLimit{{Kind: "year", UsedPercent: 1}}
		}), false},
		"too many windows": {with(func(a *AgentAccount) {
			a.LimitsState, a.LimitsAt, a.Limits = AccountLimitsRead, 5, make([]AccountLimit, MaxAccountLimits+1)
		}), false},
		"none with a name":        {AgentAccount{Tool: "codex", State: AccountNone, Account: "person@example.com", LimitsState: AccountLimitsNotAsked}, false},
		"none with a method":      {AgentAccount{Tool: "codex", State: AccountNone, Method: AccountSubscription, LimitsState: AccountLimitsNotAsked}, false},
		"none that was read":      {AgentAccount{Tool: "codex", State: AccountNone, LimitsState: AccountLimitsFailed}, false},
		"unreadable with figures": {AgentAccount{Tool: "claude", State: AccountUnreadable, LimitsState: AccountLimitsNotAsked, Limits: limits, LimitsAt: 5}, false},
	}
	for name, c := range cases {
		if got := c.a.valid(); got != c.want {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.want)
		}
	}
}

func TestAccountsResultValid(t *testing.T) {
	other := signedIn()
	other.Tool = "codex"
	cases := map[string]struct {
		r    AccountsResult
		want bool
	}{
		"nothing readable":  {AccountsResult{}, true},
		"two agents":        {AccountsResult{Accounts: []AgentAccount{signedIn(), other}}, true},
		"the same agent":    {AccountsResult{Accounts: []AgentAccount{signedIn(), signedIn()}}, false},
		"an invalid agent":  {AccountsResult{Accounts: []AgentAccount{{Tool: "claude"}}}, false},
		"reason and agents": {AccountsResult{Reason: AccountsOff, Accounts: []AgentAccount{signedIn()}}, false},
		"unknown reason":    {AccountsResult{Reason: "boom"}, false},
		"too many agents":   {AccountsResult{Accounts: make([]AgentAccount, MaxAgentAccounts+1)}, false},
	}
	for _, reason := range []AccountsReason{AccountsOff, AccountsConflict, AccountsTimedOut} {
		cases["reason "+string(reason)] = struct {
			r    AccountsResult
			want bool
		}{AccountsResult{Reason: reason}, true}
	}
	for name, c := range cases {
		if got := c.r.valid(); got != c.want {
			t.Errorf("%s: valid() = %v, want %v", name, got, c.want)
		}
	}
}

func TestAccountsRoundTripAndLimits(t *testing.T) {
	read := signedIn()
	read.LimitsState, read.LimitsAt = AccountLimitsRead, 1790000000000
	read.Limits = []AccountLimit{{Kind: AccountLimitSession, UsedPercent: 19, ResetsAt: 1790000900000}}
	for _, body := range []Body{
		Accounts{},
		Accounts{Limits: true, Refresh: true},
		AccountsResult{Accounts: []AgentAccount{read}},
		AccountsResult{Reason: AccountsOff},
	} {
		encoded, err := Encode(testMessage(body))
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatal(body.kind(), err)
		}
		if _, err = Encode(decoded); err != nil {
			t.Fatal(err)
		}
	}
	// A credential has no field to travel in: an answer that names one is refused.
	encoded, _ := Encode(testMessage(AccountsResult{Accounts: []AgentAccount{signedIn()}}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"tool":`), []byte(`"token":"secret","tool":`), 1)); err == nil {
		t.Fatal("unknown account field accepted")
	}
	encoded, _ = Encode(testMessage(Accounts{Limits: true}))
	if _, err := Decode(bytes.Replace(encoded, []byte(`"limits":`), []byte(`"tool":"claude","limits":`), 1)); err == nil {
		t.Fatal("unknown request field accepted")
	}
	if _, err := Encode(testMessage(Accounts{Refresh: true})); err == nil {
		t.Fatal("invalid request encoded")
	}
	if _, err := Encode(testMessage(AccountsResult{Accounts: []AgentAccount{{Tool: "claude"}}})); err == nil {
		t.Fatal("invalid result encoded")
	}
}

func TestAgentAccountsCapability(t *testing.T) {
	base := Capabilities{ProtocolVersion: Version, TerminalControl: true, OS: "linux", OSUser: "person", TerminalEnabled: true, Readiness: Ready}
	on := base
	on.AgentAccounts = true
	if err := on.ValidateFor(AgentAccountsFeature); err != nil {
		t.Fatal("negotiated capability refused:", err)
	}
	if err := on.ValidateFor("models-v1"); err == nil {
		t.Fatal("capability accepted without its feature")
	}
	// Terminal access is the gate: with it off, the capability is not valid.
	off := on
	off.TerminalEnabled = false
	if err := off.ValidateFor(AgentAccountsFeature); err == nil {
		t.Fatal("capability accepted with terminal access off")
	}
	plain := on
	plain.TerminalControl = false
	if err := plain.ValidateFor(AgentAccountsFeature); err == nil {
		t.Fatal("capability accepted without terminal control")
	}
	if err := on.Validate(); err != nil {
		t.Fatal("the capability's own features do not validate it:", err)
	}
}
