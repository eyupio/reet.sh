package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func accountsCaps() terminalproto.Capabilities {
	caps := launchCaps("claude")
	caps.AgentAccounts = true
	return caps
}

func accountsPath(agent string) string { return "/api/v2/machines/" + agent + "/accounts" }

// signedInAnswer is what a machine answers: Claude signed in to a plan and
// Codex not signed in. With limits, the Claude plan's figures come too.
func signedInAnswer(limits bool) terminalproto.AccountsResult {
	claude := terminalproto.AgentAccount{Tool: "claude", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", Plan: "max", LimitsState: terminalproto.AccountLimitsNotAsked}
	if limits {
		claude.LimitsState, claude.LimitsAt = terminalproto.AccountLimitsRead, time.Now().UnixMilli()
		claude.Limits = []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitSession, UsedPercent: 19, ResetsAt: time.Now().Add(time.Hour).UnixMilli()}, {Kind: terminalproto.AccountLimitWeek, UsedPercent: 60}}
	}
	return terminalproto.AccountsResult{Accounts: []terminalproto.AgentAccount{claude, {Tool: "codex", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}}}
}

// accountsFixture is a controller with one machine that answers a sign-in
// request as a machine would: with figures only when asked for them.
func accountsFixture(t *testing.T, caps terminalproto.Capabilities) *modelsAPI {
	t.Helper()
	f := modelsAPIFixture(t, caps)
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		return signedInAnswer(body.(terminalproto.Accounts).Limits)
	})
	return f
}

func (f *modelsAPI) send(t *testing.T, cookie, method, path, body string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.String()
}

// oldClientPlanLimits sends the setting an older web bundle still sends. The
// controller accepts it and changes nothing: plan limits are always read.
func (f *modelsAPI) oldClientPlanLimits(t *testing.T, on bool) {
	t.Helper()
	body := `{"planLimits":false}`
	if on {
		body = `{"planLimits":true}`
	}
	if code, _, raw := f.send(t, f.cookie, "PATCH", "/api/v2/profile", body); code != 200 {
		t.Fatal("profile", code, raw)
	}
}

func (f *modelsAPI) accountRequests() []terminalproto.Accounts {
	var out []terminalproto.Accounts
	for _, body := range f.sentRequests() {
		if a, ok := body.(terminalproto.Accounts); ok {
			out = append(out, a)
		}
	}
	return out
}

func accountEntry(t *testing.T, out map[string]any, tool string) map[string]any {
	t.Helper()
	for _, raw := range modelsField(t, out, "accounts") {
		entry := raw.(map[string]any)
		if entry["tool"] == tool {
			return entry
		}
	}
	t.Fatalf("no %s in %v", tool, out)
	return nil
}

// Plan limits are always read: the first read asks the machine for the
// sign-ins and each plan's allowance, and what it answers is kept and served.
func TestMachineAccountsFirstReadAsksForTheAllowance(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	code, out, raw := f.get(t, f.cookie, accountsPath(f.agent))
	// planLimits stays in the answer, always true, for an older web bundle that reads it.
	if code != 200 || out["state"] != "read" || out["planLimits"] != true || out["sentence"] != "" || out["checkedAt"] == nil {
		t.Fatal(code, raw)
	}
	claude := accountEntry(t, out, "claude")
	limits, _ := claude["limits"].([]any)
	if claude["state"] != "signed_in" || claude["account"] != "person@example.com" || claude["plan"] != "max" || claude["limitsState"] != "read" || len(limits) != 2 || limits[0].(map[string]any)["usedPercent"] != float64(19) || limits[0].(map[string]any)["kind"] != "session" {
		t.Fatal("claude", claude)
	}
	if codex := accountEntry(t, out, "codex"); codex["state"] != "none" {
		t.Fatal("codex", codex)
	}
	sent := f.accountRequests()
	if len(sent) != 1 || !sent[0].Limits || sent[0].Refresh {
		t.Fatalf("the first read did not ask for the allowance, or asked for fresh figures: %+v", sent)
	}
	// A recent answer is served without asking again.
	if code, out, raw = f.get(t, f.cookie, accountsPath(f.agent)); code != 200 || out["state"] != "read" || len(f.accountRequests()) != 1 {
		t.Fatal("a recent answer was read again", code, raw, len(f.accountRequests()))
	}
}

// A machine that reads more agents is kept and served whole: the controller
// keeps no list of its own. A plan sign-in its provider said has nothing to
// read is shown as that while plan limits are on, and as not asked once they
// are off; an API key has no plan either way.
func TestMachineAccountsServeEveryAgentTheMachineReads(t *testing.T) {
	f := modelsAPIFixture(t, accountsCaps())
	f.answering(func(body terminalproto.Body) terminalproto.Body {
		answer := signedInAnswer(body.(terminalproto.Accounts).Limits)
		copilot := terminalproto.AgentAccount{Tool: "copilot", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person", LimitsState: terminalproto.AccountLimitsNotAsked}
		opencode := terminalproto.AgentAccount{Tool: "opencode", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, LimitsState: terminalproto.AccountLimitsNotAsked}
		if body.(terminalproto.Accounts).Limits {
			copilot.LimitsState = terminalproto.AccountLimitsUnsupported
			opencode.LimitsState, opencode.Plan, opencode.LimitsAt = terminalproto.AccountLimitsRead, "go", time.Now().UnixMilli()
			opencode.Limits = []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitMonth, UsedPercent: 3}}
		}
		answer.Accounts[1] = terminalproto.AgentAccount{Tool: "codex", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountAPIKey, LimitsState: terminalproto.AccountLimitsUnsupported}
		answer.Accounts = append(answer.Accounts, opencode, terminalproto.AgentAccount{Tool: "antigravity", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}, copilot)
		return answer
	})
	code, out, raw := f.get(t, f.cookie, accountsPath(f.agent))
	if code != 200 || len(modelsField(t, out, "accounts")) != 5 {
		t.Fatal(code, raw)
	}
	if copilot := accountEntry(t, out, "copilot"); copilot["limitsState"] != "unsupported" || copilot["limits"] != nil {
		t.Fatal("copilot", copilot)
	}
	if opencode := accountEntry(t, out, "opencode"); opencode["limitsState"] != "read" || opencode["plan"] != "go" {
		t.Fatal("opencode", opencode)
	}
	// An older web bundle that still sends the switch off changes nothing: what the provider said is shown.
	f.oldClientPlanLimits(t, false)
	_, out, _ = f.get(t, f.cookie, accountsPath(f.agent))
	if copilot := accountEntry(t, out, "copilot"); copilot["limitsState"] != "unsupported" {
		t.Fatal("the retired switch hid what a provider said", copilot)
	}
	if opencode := accountEntry(t, out, "opencode"); opencode["limitsState"] != "read" {
		t.Fatal("the retired switch hid a plan's figures", opencode)
	}
	if codex := accountEntry(t, out, "codex"); codex["limitsState"] != "unsupported" {
		t.Fatal("an API key", codex)
	}
}

func TestMachineAccountsAnOldAnswerIsReadAgain(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	old := time.Now().Add(-accountsStaleAfter - time.Minute).UnixMilli()
	stale := []terminalproto.AgentAccount{{Tool: "claude", State: terminalproto.AccountNone, LimitsState: terminalproto.AccountLimitsNotAsked}}
	if err := f.st.SaveMachineAccountsFor(f.uid, f.agent, stale, old); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.get(t, f.cookie, accountsPath(f.agent))
	if code != 200 || out["state"] != "read" || len(f.accountRequests()) != 1 || accountEntry(t, out, "claude")["state"] != "signed_in" {
		t.Fatal("an answer past its time was served as it was", code, raw)
	}
	if at := int64(out["checkedAt"].(float64)); at <= old {
		t.Fatal("the answer's time did not move", at)
	}
}

// An answer kept before plan limits were always read says not_asked for a
// signed-in plan: it is no answer now, so it is read again at once though it
// is recent. An older web bundle turning the old switch off changes nothing.
func TestMachineAccountsAnAnswerWithoutTheAllowanceIsReadAgain(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	if err := f.st.SaveMachineAccountsFor(f.uid, f.agent, signedInAnswer(false).Accounts, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.get(t, f.cookie, accountsPath(f.agent))
	if code != 200 || accountEntry(t, out, "claude")["limitsState"] != "read" {
		t.Fatal("an answer without the allowance was served as it was", code, raw)
	}
	if sent := f.accountRequests(); len(sent) != 1 || !sent[0].Limits {
		t.Fatalf("the machine was not asked for the allowance: %+v", sent)
	}
	f.oldClientPlanLimits(t, false)
	code, out, raw = f.get(t, f.cookie, accountsPath(f.agent))
	claude := accountEntry(t, out, "claude")
	if code != 200 || out["planLimits"] != true || claude["limitsState"] != "read" || claude["limits"] == nil {
		t.Fatal("the old switch hid the figures", raw)
	}
	if len(f.accountRequests()) != 1 {
		t.Fatal("the old switch made the machine be asked again")
	}
}

func TestMachineAccountsStatesWithoutAnAnswer(t *testing.T) {
	// An older reet: no capability, terminal access on.
	old := accountsFixture(t, launchCaps("claude"))
	code, out, raw := old.get(t, old.cookie, accountsPath(old.agent))
	if code != 200 || out["state"] != "update" || out["sentence"] != sentenceAccountsUpdate || out["checkedAt"] != nil || len(modelsField(t, out, "accounts")) != 0 {
		t.Fatal("update", code, raw)
	}
	// A current reet with terminal access off: that is the switch, not an old reet.
	caps := launchCaps()
	caps.TerminalEnabled, caps.Readiness = false, terminalproto.DisabledLocally
	off := accountsFixture(t, caps)
	code, out, raw = off.get(t, off.cookie, accountsPath(off.agent))
	if code != 200 || out["state"] != "off" || out["sentence"] != sentenceAccountsOff {
		t.Fatal("off", code, raw)
	}
	// A machine that is not connected, with an answer kept from before.
	f := accountsFixture(t, accountsCaps())
	if code, _, raw := f.get(t, f.cookie, accountsPath(f.agent)); code != 200 {
		t.Fatal(code, raw)
	}
	kept, _, _ := f.st.MachineAccountsFor(f.uid, f.agent)
	if err := f.st.SaveMachineAccountsFor(f.uid, f.agent, kept.Accounts, time.Now().Add(-time.Hour).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	f.peers[f.agent].Close()
	code, out, raw = f.get(t, f.cookie, accountsPath(f.agent))
	if code != 200 || out["state"] != "offline" || out["sentence"] != sentenceMachineOffline || accountEntry(t, out, "claude")["account"] != "person@example.com" {
		t.Fatal("offline did not keep the last answer", code, raw)
	}
	for _, o := range []*modelsAPI{old, off} {
		if len(o.accountRequests()) != 0 {
			t.Fatal("a machine that cannot answer was asked")
		}
	}
	if code, _, _ := f.get(t, "", accountsPath(f.agent)); code != http.StatusUnauthorized {
		t.Fatal("anonymous", code)
	}
	if code, _, _ := f.get(t, f.cookie, accountsPath("not-an-id")); code != http.StatusBadRequest {
		t.Fatal("malformed id", code)
	}
}

func TestMachineAccountsAnotherAccountSeesNothing(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	if code, _, raw := f.get(t, f.cookie, accountsPath(f.agent)); code != 200 {
		t.Fatal(code, raw)
	}
	other, err := f.st.CreateUser("accounts-api-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cookie := terminalLogin(t, f.st, other.ID)
	code, out, raw := f.get(t, cookie, accountsPath(f.agent))
	if code != 200 || out["state"] != "offline" || len(modelsField(t, out, "accounts")) != 0 || strings.Contains(raw, "person@example.com") {
		t.Fatal("another account read this machine's sign-ins", code, raw)
	}
	if code, out, raw = f.send(t, cookie, "POST", accountsPath(f.agent)+"/refresh", ""); code != 200 || out["state"] != "offline" || strings.Contains(raw, "person@example.com") {
		t.Fatal("another account refreshed this machine", code, raw)
	}
	if len(f.accountRequests()) != 1 {
		t.Fatal("another account's request reached the machine")
	}
}

// What the machine could not do is said, and the last answer stays.
func TestMachineAccountsKeepTheLastAnswerWhenTheMachineCannotRead(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	if code, _, raw := f.get(t, f.cookie, accountsPath(f.agent)); code != 200 {
		t.Fatal(code, raw)
	}
	for reason, want := range map[terminalproto.AccountsReason]string{terminalproto.AccountsConflict: "busy", terminalproto.AccountsTimedOut: "failed", terminalproto.AccountsOff: "off"} {
		f.answering(func(terminalproto.Body) terminalproto.Body { return terminalproto.AccountsResult{Reason: reason} })
		code, out, raw := f.send(t, f.cookie, "POST", accountsPath(f.agent)+"/refresh", "")
		if code != 200 || out["state"] != want || out["sentence"] != accountsSentence(want) || accountEntry(t, out, "claude")["account"] != "person@example.com" {
			t.Fatalf("%s: %d %s", reason, code, raw)
		}
	}
}

// A person-pressed read always asks for fresh figures, is rate limited, and
// is the only read that is audited. The audit row names no account.
func TestMachineAccountsRefresh(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	code, out, raw := f.send(t, f.cookie, "POST", accountsPath(f.agent)+"/refresh", "")
	if code != 200 || out["state"] != "read" || accountEntry(t, out, "claude")["limitsState"] != "read" {
		t.Fatal(code, raw)
	}
	if sent := f.accountRequests(); len(sent) != 1 || !sent[0].Limits || !sent[0].Refresh {
		t.Fatalf("a refresh did not ask for fresh figures: %+v", sent)
	}
	if code, _, raw = f.send(t, f.cookie, "POST", accountsPath(f.agent)+"/refresh", ""); code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.accountRequests(); len(sent) != 2 || !sent[1].Limits || !sent[1].Refresh {
		t.Fatalf("a second refresh did not ask for fresh figures: %+v", sent)
	}
	events := worktreeEvents(t, f.st, f.uid, "machine-accounts-read")
	if len(events) != 2 {
		t.Fatal("refresh audit", events)
	}
	for _, event := range events {
		if strings.Contains(event.Detail, "person@example.com") || strings.Contains(event.Detail, "max") {
			t.Fatal("the audit row names an account or plan", event.Detail)
		}
	}
	detail := map[string]any{}
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if len(detail) != 3 || detail["machine"] != f.agent || detail["state"] != "read" || detail["limits"] != true {
		t.Fatal("audit detail", detail)
	}
	if code, _, raw := f.get(t, f.cookie, accountsPath(f.agent)); code != 200 || len(worktreeEvents(t, f.st, f.uid, "machine-accounts-read")) != 2 {
		t.Fatal("an ordinary read was audited", code, raw)
	}
	for i := len(f.accountRequests()); i < accountsRefreshMax; i++ {
		if code, _, raw := f.send(t, f.cookie, "POST", accountsPath(f.agent)+"/refresh", ""); code != 200 {
			t.Fatal(i, code, raw)
		}
	}
	code, out, raw = f.send(t, f.cookie, "POST", accountsPath(f.agent)+"/refresh", "")
	if code != http.StatusTooManyRequests || out["code"] != "accounts_rate_limited" {
		t.Fatal("rate limit", code, raw)
	}
	if code, _, _ := f.send(t, "", "POST", accountsPath(f.agent)+"/refresh", ""); code != http.StatusUnauthorized {
		t.Fatal("anonymous refresh", code)
	}
}

// The old per-account switch is gone: the profile says plan limits are on for
// every account, so an older web bundle shows the figures, and the setting it
// may still send is accepted and changes nothing.
func TestProfilePlanLimitsIsAlwaysOn(t *testing.T) {
	f := accountsFixture(t, accountsCaps())
	code, out, raw := f.get(t, f.cookie, "/api/v2/profile")
	profile, _ := out["profile"].(map[string]any)
	if code != 200 || profile["planLimits"] != true {
		t.Fatal("plan limits are not on for a new account", code, raw)
	}
	f.oldClientPlanLimits(t, false)
	_, out, raw = f.get(t, f.cookie, "/api/v2/profile")
	if profile, _ = out["profile"].(map[string]any); profile["planLimits"] != true {
		t.Fatal("an older client turned plan limits off", raw)
	}
	for _, event := range worktreeEvents(t, f.st, f.uid, "profile") {
		if strings.Contains(event.Detail, "planLimits") {
			t.Fatal("a setting that changes nothing was audited", event.Detail)
		}
	}
	// Another setting saved later leaves it on.
	if code, _, raw := f.send(t, f.cookie, "PATCH", "/api/v2/profile", `{"worktreesByDefault":false}`); code != 200 {
		t.Fatal(code, raw)
	}
	_, out, raw = f.get(t, f.cookie, "/api/v2/profile")
	if profile, _ = out["profile"].(map[string]any); profile["planLimits"] != true {
		t.Fatal("another setting turned plan limits off", raw)
	}
}
