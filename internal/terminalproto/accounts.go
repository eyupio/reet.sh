package terminalproto

import (
	"encoding/json"
	"strings"
)

// AgentAccountsFeature lets a connector say, for the agents it can read, whether
// an account is signed in on the machine and, when asked, how much of the
// account's plan allowance is left. A credential never crosses the connection:
// the answer carries a name, a plan word and numbers.
const AgentAccountsFeature = "agent-accounts-v1"

// SupportsAgentAccounts reports whether the peer advertised agent-accounts-v1.
func SupportsAgentAccounts(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == AgentAccountsFeature {
			return true
		}
	}
	return false
}

const (
	// MaxAgentAccounts bounds the agents one answer describes.
	MaxAgentAccounts = 16
	// MaxAccountLimits bounds the allowance windows of one account.
	MaxAccountLimits = 8
	// MaxAccountsReplyBytes keeps an answer well inside the control frame.
	MaxAccountsReplyBytes = 8 << 10
)

// Accounts asks the connector what it can see of each agent's sign-in. Limits
// also asks it to read the plan allowance from the agent's provider, with the
// agent's own sign-in, from the machine; without it nothing leaves the
// machine but this answer. Refresh is a person asking now: the connector reads
// the allowance again instead of answering its short cache.
type Accounts struct {
	Limits  bool `json:"limits,omitempty"`
	Refresh bool `json:"refresh,omitempty"`
}

func (Accounts) kind() string { return "accounts" }
func (a Accounts) valid() bool {
	// A refresh is of the allowance; there is nothing else to refresh.
	return !a.Refresh || a.Limits
}

// AccountState is what the machine found of an agent's sign-in.
type AccountState string

const (
	// AccountSignedIn means the agent's own sign-in was found on the machine.
	AccountSignedIn AccountState = "signed_in"
	// AccountNone means the places the agent keeps a sign-in were read and
	// held none. An agent signed in some other way is not ruled out.
	AccountNone AccountState = "none"
	// AccountUnreadable means a sign-in may be there but could not be read:
	// a file that is not a plain file, is too large or does not parse, or a
	// sign-in kept somewhere the connector does not open (a keychain).
	AccountUnreadable AccountState = "unreadable"
)

func (s AccountState) Valid() bool {
	switch s {
	case AccountSignedIn, AccountNone, AccountUnreadable:
		return true
	}
	return false
}

// AccountMethod is how the agent is signed in.
type AccountMethod string

const (
	// AccountSubscription is a sign-in to a plan (Claude, ChatGPT).
	AccountSubscription AccountMethod = "subscription"
	// AccountAPIKey is a provider API key; it has no plan allowance to read.
	AccountAPIKey AccountMethod = "api_key"
)

func (m AccountMethod) Valid() bool {
	return m == "" || m == AccountSubscription || m == AccountAPIKey
}

// AccountLimitsState says what became of reading the plan allowance. Limits
// may accompany any state but not_asked and unsupported: after a failure they
// are the last figures read, with LimitsAt saying when.
type AccountLimitsState string

const (
	// AccountLimitsRead means the figures were read just now or from the
	// connector's short cache.
	AccountLimitsRead AccountLimitsState = "read"
	// AccountLimitsNotAsked means the request did not ask for the allowance.
	AccountLimitsNotAsked AccountLimitsState = "not_asked"
	// AccountLimitsUnsupported means there is no allowance to read: the
	// sign-in has no plan (an API key).
	AccountLimitsUnsupported AccountLimitsState = "unsupported"
	// AccountLimitsRenew means the sign-in's short-lived token has run out.
	// The agent renews it when it next runs; reet never renews it, because
	// doing so can sign the agent out.
	AccountLimitsRenew AccountLimitsState = "renew"
	// AccountLimitsRefused means the provider refused the sign-in.
	AccountLimitsRefused AccountLimitsState = "refused"
	// AccountLimitsRateLimited means the provider asked for fewer requests.
	AccountLimitsRateLimited AccountLimitsState = "rate_limited"
	// AccountLimitsFailed is any other failure: no network, an answer in
	// another shape, a timeout.
	AccountLimitsFailed AccountLimitsState = "failed"
)

func (s AccountLimitsState) Valid() bool {
	switch s {
	case AccountLimitsRead, AccountLimitsNotAsked, AccountLimitsUnsupported, AccountLimitsRenew, AccountLimitsRefused, AccountLimitsRateLimited, AccountLimitsFailed:
		return true
	}
	return false
}

// AccountLimitKind is the length of an allowance window.
type AccountLimitKind string

const (
	AccountLimitSession AccountLimitKind = "session" // a few hours
	AccountLimitDay     AccountLimitKind = "day"
	AccountLimitWeek    AccountLimitKind = "week"
	AccountLimitMonth   AccountLimitKind = "month"
)

func (k AccountLimitKind) Valid() bool {
	switch k {
	case AccountLimitSession, AccountLimitDay, AccountLimitWeek, AccountLimitMonth:
		return true
	}
	return false
}

// AccountLimit is one allowance window: how much of it is used, as a whole
// percentage, and when it starts again. Label names a window that covers only
// part of the plan (one model); it is empty for the plan-wide window. ResetsAt
// is in milliseconds, zero when the provider did not say.
type AccountLimit struct {
	Kind        AccountLimitKind `json:"kind"`
	Label       string           `json:"label,omitempty"`
	UsedPercent int              `json:"usedPercent"`
	ResetsAt    int64            `json:"resetsAt,omitempty"`
}

func (l AccountLimit) valid() bool {
	return l.Kind.Valid() && plainText(l.Label, 32) && l.UsedPercent >= 0 && l.UsedPercent <= 100 && l.ResetsAt >= 0
}

// AgentAccount is what the machine found for one agent. It never carries a
// token, a key or any part of one.
type AgentAccount struct {
	Tool    string        `json:"tool"`
	State   AccountState  `json:"state"`
	Method  AccountMethod `json:"method,omitempty"`
	Account string        `json:"account,omitempty"`
	Plan    string        `json:"plan,omitempty"`
	// LimitsState is always set. LimitsAt is when Limits were read, in
	// milliseconds; both are absent when no figures were ever read.
	LimitsState AccountLimitsState `json:"limitsState"`
	Limits      []AccountLimit     `json:"limits,omitempty"`
	LimitsAt    int64              `json:"limitsAt,omitempty"`
}

func (a AgentAccount) valid() bool {
	if !ValidToolV2(a.Tool) || !a.State.Valid() || !a.Method.Valid() || !a.LimitsState.Valid() {
		return false
	}
	if !plainText(a.Account, 128) || !plainText(a.Plan, 32) || a.LimitsAt < 0 || len(a.Limits) > MaxAccountLimits {
		return false
	}
	// Only a sign-in that was found has a method, a name, a plan or figures.
	if a.State != AccountSignedIn {
		bare := a.Method == "" && a.Account == "" && a.Plan == "" && len(a.Limits) == 0 && a.LimitsAt == 0
		quiet := a.LimitsState == AccountLimitsNotAsked || a.LimitsState == AccountLimitsUnsupported
		return bare && quiet
	}
	if a.Method == "" {
		return false
	}
	noFigures := a.LimitsState == AccountLimitsNotAsked || a.LimitsState == AccountLimitsUnsupported
	if noFigures && (len(a.Limits) > 0 || a.LimitsAt != 0) {
		return false
	}
	// Figures and the time they were read come together.
	if (len(a.Limits) > 0) != (a.LimitsAt > 0) {
		return false
	}
	if a.LimitsState == AccountLimitsRead && len(a.Limits) == 0 {
		return false
	}
	for _, limit := range a.Limits {
		if !limit.valid() {
			return false
		}
	}
	return true
}

// AccountsReason is a closed reason the connector could not answer at all.
type AccountsReason string

const (
	// AccountsOff means terminal access is off on the machine: its consent
	// gates reading a sign-in as it gates a shell.
	AccountsOff AccountsReason = "off"
	// AccountsConflict means the machine is already reading.
	AccountsConflict AccountsReason = "conflict"
	// AccountsTimedOut means the read did not finish in time.
	AccountsTimedOut AccountsReason = "timed_out"
)

func (r AccountsReason) Valid() bool {
	return r == AccountsOff || r == AccountsConflict || r == AccountsTimedOut
}

// AccountsResult is the connector's answer: one entry per agent it has a
// reviewed reader for, each agent once, or the reason it read nothing. An
// agent that is absent is one reet cannot tell about.
type AccountsResult struct {
	Accounts []AgentAccount `json:"accounts,omitempty"`
	Reason   AccountsReason `json:"reason,omitempty"`
}

func (AccountsResult) kind() string { return "accounts_result" }

func (r AccountsResult) valid() bool {
	if r.Reason != "" {
		return r.Reason.Valid() && len(r.Accounts) == 0
	}
	if len(r.Accounts) > MaxAgentAccounts {
		return false
	}
	seen := make(map[string]bool, len(r.Accounts))
	for _, account := range r.Accounts {
		if seen[account.Tool] || !account.valid() {
			return false
		}
		seen[account.Tool] = true
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxAccountsReplyBytes
}
