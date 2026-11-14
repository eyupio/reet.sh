package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// What a machine says of its agents' sign-ins, kept per account and machine.
// The machine reads its agents' own sign-in files; when the account has plan
// limits on it also asks each agent's provider, from the machine, with the
// agent's own sign-in. No credential reaches the controller: an answer is a
// state, a name, a plan word and percentages.

const (
	// accountsStaleAfter is how old a kept answer may be before an ordinary
	// read asks the machine again.
	accountsStaleAfter  = 5 * time.Minute
	accountsRefreshMax  = 6
	accountsRefreshSpan = time.Minute
)

const (
	sentenceAccountsUpdate = "Update reet on that machine to see which agents are signed in."
	sentenceAccountsOff    = "Terminal access is off on that machine, so reet doesn't read its agents' sign-ins."
	sentenceAccountsBusy   = "That machine is already reading its agents' sign-ins. Try again in a moment."
	sentenceAccountsFailed = "That machine couldn't read its agents' sign-ins."
)

func (s *server) registerAccountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v2/machines/{agentId}/accounts", s.terminal(s.sameOriginRead(s.machineAccounts)))
	mux.HandleFunc("POST /api/v2/machines/{agentId}/accounts/refresh", s.terminal(s.machineAccountsRefresh))
}

func (s *server) accountLimitState() *modelLimits {
	s.accountsOnce.Do(func() {
		s.accountsLimits = &modelLimits{refresh: auth.NewLimiter(accountsRefreshMax, accountsRefreshSpan)}
	})
	return s.accountsLimits
}

// accountsView is what the browser is given. State is "read" for an answer,
// or why there is none new: offline, update, off, busy or failed; the last
// kept answer still comes with those. PlanLimits is the account's setting.
type accountsView struct {
	State      string                       `json:"state"`
	Sentence   string                       `json:"sentence"`
	PlanLimits bool                         `json:"planLimits"`
	CheckedAt  *int64                       `json:"checkedAt"`
	Accounts   []terminalproto.AgentAccount `json:"accounts"`
}

func accountsSentence(state string) string {
	switch state {
	case "offline":
		return sentenceMachineOffline
	case "update":
		return sentenceAccountsUpdate
	case "off":
		return sentenceAccountsOff
	case "busy":
		return sentenceAccountsBusy
	case "failed":
		return sentenceAccountsFailed
	}
	return ""
}

// withoutLimits is an answer as an account with plan limits off is shown it:
// figures kept from when the setting was on are not shown once it is off.
func withoutLimits(accounts []terminalproto.AgentAccount) []terminalproto.AgentAccount {
	out := make([]terminalproto.AgentAccount, len(accounts))
	for i, a := range accounts {
		a.Limits, a.LimitsAt = nil, 0
		if a.LimitsState != terminalproto.AccountLimitsUnsupported {
			a.LimitsState = terminalproto.AccountLimitsNotAsked
		}
		out[i] = a
	}
	return out
}

// accountsAnswer builds the view from the kept answer, if there is one.
func (s *server) accountsAnswer(uid int64, agent, state string, limits bool) accountsView {
	view := accountsView{State: state, Sentence: accountsSentence(state), PlanLimits: limits, Accounts: []terminalproto.AgentAccount{}}
	kept, ok, err := s.st.MachineAccountsFor(uid, agent)
	if err != nil || !ok {
		return view
	}
	at := kept.CheckedAt
	view.CheckedAt, view.Accounts = &at, kept.Accounts
	if !limits {
		view.Accounts = withoutLimits(kept.Accounts)
	}
	return view
}

// accountsReadiness says why a machine cannot be asked now, or "" when it can.
func (s *server) accountsReadiness(uid int64, agent string) (string, error) {
	if s.machineHost(uid, agent) == "" {
		// Another account's id is answered as an offline machine.
		return "offline", nil
	}
	caps, err := s.st.RemoteCapabilitiesFor(uid, agent)
	if errors.Is(err, store.ErrRemoteNotFound) {
		return "update", nil
	}
	if err != nil {
		return "", err
	}
	switch {
	case caps.Capabilities.AgentAccounts:
	case caps.Capabilities.TerminalControl && !caps.Capabilities.TerminalEnabled:
		// A current machine with terminal access off does not report the
		// capability; that is the switch, not an old reet.
		return "off", nil
	default:
		return "update", nil
	}
	if !s.terminals.Ready(uid, agent) {
		return "offline", nil
	}
	return "", nil
}

// readMachineAccounts asks the machine once and keeps what it answers. It
// returns the state to report: "read" when a new answer was kept.
func (s *server) readMachineAccounts(ctx context.Context, uid int64, agent string, limits, refresh bool) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, terminalbroker.AccountsCallTimeout)
	defer cancel()
	result, err := s.terminals.Accounts(callCtx, uid, agent, terminalproto.Accounts{Limits: limits, Refresh: limits && refresh})
	switch {
	case errors.Is(err, context.Canceled):
		// The browser went away; nothing is concluded from that.
		return "", err
	case errors.Is(err, terminalbroker.ErrAccountsUnsupported):
		return "update", nil
	case errors.Is(err, terminalbroker.ErrOffline):
		return "offline", nil
	case errors.Is(err, terminalbroker.ErrConflict):
		return "busy", nil
	case err != nil:
		return "failed", nil
	}
	switch result.Reason {
	case "":
	case terminalproto.AccountsOff:
		return "off", nil
	case terminalproto.AccountsConflict:
		return "busy", nil
	default:
		return "failed", nil
	}
	if err := s.st.SaveMachineAccountsFor(uid, agent, result.Accounts, time.Now().UnixMilli()); err != nil {
		return "", err
	}
	return "read", nil
}

// machineAccounts serves the kept answer while it is recent. An older one,
// or none, has a machine that is online asked once, synchronously; a machine
// that cannot be asked answers with what was kept and why it is not newer.
func (s *server) machineAccounts(w http.ResponseWriter, r *http.Request) {
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	profile, err := s.st.ProfileFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	limits := profile.PlanLimits
	kept, found, err := s.st.MachineAccountsFor(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if found && time.Since(time.UnixMilli(kept.CheckedAt)) < accountsStaleAfter && (!limits || answeredLimits(kept.Accounts)) {
		writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, "read", limits))
		return
	}
	state, err := s.accountsReadiness(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if state == "" {
		if state, err = s.readMachineAccounts(r.Context(), uid, agent, limits, false); err != nil {
			terminalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state, limits))
}

// answeredLimits reports whether a kept answer was made with the allowance
// asked for: an answer kept while plan limits were off says not_asked for
// every signed-in plan, and is no answer to an account that has since turned
// them on.
func answeredLimits(accounts []terminalproto.AgentAccount) bool {
	for _, a := range accounts {
		if a.State == terminalproto.AccountSignedIn && a.LimitsState == terminalproto.AccountLimitsNotAsked {
			return false
		}
	}
	return true
}

// machineAccountsRefresh is the person-pressed read. It is rate limited per
// account and machine and is the only path that audits. The audit row names
// the machine and whether the allowance was asked for, never an account.
func (s *server) machineAccountsRefresh(w http.ResponseWriter, r *http.Request) {
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	profile, err := s.st.ProfileFor(uid)
	if err != nil {
		terminalError(w, err)
		return
	}
	limits := profile.PlanLimits
	state, err := s.accountsReadiness(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if state != "" {
		writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state, limits))
		return
	}
	if !s.accountLimitState().take(strconv.FormatInt(uid, 10) + "/" + agent) {
		writeErrCode(w, http.StatusTooManyRequests, "accounts_rate_limited", "", sentenceSlowDown, "")
		return
	}
	if state, err = s.readMachineAccounts(r.Context(), uid, agent, limits, true); err != nil {
		terminalError(w, err)
		return
	}
	s.event(uid, "ui", "machine-accounts-read", "", map[string]any{"machine": agent, "limits": limits, "state": state})
	writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state, limits))
}
