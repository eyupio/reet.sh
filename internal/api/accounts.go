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
// The machine reads its agents' own sign-in files and asks each agent's
// provider, from the machine, with the agent's own sign-in, how much of the
// plan is left. Every account is read this way; there is no switch. No
// credential reaches the controller: an answer is a state, a name, a plan word
// and percentages.

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
// kept answer still comes with those. PlanLimits is always true: plan limits
// are read for every account, and an older web bundle that still reads the
// field shows the figures instead of telling the person to turn them on.
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

// accountsAnswer builds the view from the kept answer, if there is one.
func (s *server) accountsAnswer(uid int64, agent, state string) accountsView {
	view := accountsView{State: state, Sentence: accountsSentence(state), PlanLimits: true, Accounts: []terminalproto.AgentAccount{}}
	kept, ok, err := s.st.MachineAccountsFor(uid, agent)
	if err != nil || !ok {
		return view
	}
	at := kept.CheckedAt
	view.CheckedAt, view.Accounts = &at, kept.Accounts
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

// readMachineAccounts asks the machine once, for the sign-ins and each plan's
// allowance, and keeps what it answers. Refresh asks the machine to read the
// allowance afresh rather than serve its own recent figures. It returns the
// state to report: "read" when a new answer was kept.
func (s *server) readMachineAccounts(ctx context.Context, uid int64, agent string, refresh bool) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, terminalbroker.AccountsCallTimeout)
	defer cancel()
	result, err := s.terminals.Accounts(callCtx, uid, agent, terminalproto.Accounts{Limits: true, Refresh: refresh})
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
	kept, found, err := s.st.MachineAccountsFor(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if found && time.Since(time.UnixMilli(kept.CheckedAt)) < accountsStaleAfter && answeredLimits(kept.Accounts) {
		writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, "read"))
		return
	}
	state, err := s.accountsReadiness(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if state == "" {
		if state, err = s.readMachineAccounts(r.Context(), uid, agent, false); err != nil {
			terminalError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state))
}

// answeredLimits reports whether a kept answer was made with the allowance
// asked for. An answer kept before plan limits were read for every account
// says not_asked for each signed-in plan, and is read again at once.
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
// the machine and that the allowance was asked for, never an account.
func (s *server) machineAccountsRefresh(w http.ResponseWriter, r *http.Request) {
	agent := r.PathValue("agentId")
	if !terminalproto.ValidID(agent) {
		terminalError(w, terminalproto.ErrInvalid)
		return
	}
	uid := s.uid(r)
	state, err := s.accountsReadiness(uid, agent)
	if err != nil {
		terminalError(w, err)
		return
	}
	if state != "" {
		writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state))
		return
	}
	if !s.accountLimitState().take(strconv.FormatInt(uid, 10) + "/" + agent) {
		writeErrCode(w, http.StatusTooManyRequests, "accounts_rate_limited", "", sentenceSlowDown, "")
		return
	}
	if state, err = s.readMachineAccounts(r.Context(), uid, agent, true); err != nil {
		terminalError(w, err)
		return
	}
	s.event(uid, "ui", "machine-accounts-read", "", map[string]any{"machine": agent, "limits": true, "state": state})
	writeJSON(w, http.StatusOK, s.accountsAnswer(uid, agent, state))
}
