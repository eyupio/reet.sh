package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestGoalSetupIsOneAtomicOwnedReviewAndResumePreservesBudget(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("goal-setup-test", 8), "test")
	ts := httptest.NewUnstartedServer(nil)
	h := New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL("http://"+ts.Listener.Addr().String()), WithRemoteTerminals(true), WithGafferProviders(seal))
	ts.Config.Handler = h
	ts.Start()
	t.Cleanup(func() { ts.Close(); h.Close(); st.Close() })
	h.s.gafferProviders.SetWorker(&workerAPIFake{st: st}, h.s.terminalEnabled)
	browser := &http.Client{Jar: mustJar(t)}
	contextAPIJSON(t, browser, ts, "POST", "/api/v2/auth/setup", map[string]string{"name": "Owner", "email": "setup-goal@example.test", "password": "long setup goal password"}, 200)
	owner, _, _ := st.OwnerUser()
	session := contextAPISession(t, st, owner.ID)
	caps, _ := st.RemoteCapabilitiesFor(owner.ID, session.AgentID)
	caps.Capabilities.ACPLiveControls = true
	record, err := st.BeginRemoteConnectionFor(owner.ID, session.AgentID, strings.Repeat("3", 32), caps.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(owner.ID, session.AgentID, session.ID, record.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	provider, err := h.s.gafferProviders.Save(gaffer.Principal{UserID: owner.ID, CanUseLocal: true}, gaffer.Input{Name: "One reviewed setup", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1", LocalOnly: true, Enabled: true, Model: "fake-model"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.s.gafferProviders.CreateConversation(owner.ID, gaffer.ConversationInput{SessionID: session.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v2/gaffer/conversations/" + c.Conversation.ID
	view, err := h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	if err != nil || !view.SetupAllowed || view.Allowed || !view.RequiresProviderAccess {
		t.Fatal(view, err)
	}
	in := gaffer.GoalSetupInput{GoalInput: gaffer.GoalInput{GoalPins: view.Pins, Text: "PRIVATE_SETUP_GOAL: raise a PR, ask about scope, do not merge", Hours: 24, MaxReviews: 12}, ConversationRevision: view.ConversationRevision, Action: "start"}
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/setup", in, 422)
	in.Confirmed = true
	in.Revision = 99
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/setup", in, 409)
	// The goal CAS fails after access updates inside the transaction. None of
	// those updates may remain when the user's single action fails.
	p, _ := st.GafferProviderFor(owner.ID, provider.ID)
	sharing, _ := st.ContextGrantFor(owner.ID, "gaffer", c.Conversation.ID, session.ID)
	worker, _ := st.WorkerGrantFor(owner.ID, "gaffer", c.Conversation.ID, session.ID)
	if p.Revision != provider.Revision || p.AllowSessions || p.AllowCode || sharing.Any() || sharing.Revision != 0 || worker.Enabled || worker.Revision != 0 {
		t.Fatal("failed setup left partial permission", p, sharing, worker)
	}
	in.Revision = 0
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/setup", in, 200)
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/setup", in, 409)
	view, err = h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	if err != nil || !view.Allowed || view.Goal == nil || view.Goal.State != "active" || view.Goal.MaxReviews != 12 || view.Goal.UsedReviews != 0 || view.Goal.ProviderRevision != provider.Revision+1 {
		t.Fatal(view, err)
	}
	p, _ = st.GafferProviderFor(owner.ID, provider.ID)
	if !p.AllowSessions || !p.AllowCode || p.AllowStatus {
		t.Fatal("setup changed unrelated provider maxima", p)
	}
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/control", map[string]any{"revision": view.Goal.Revision, "action": "pause"}, 200)
	paused, _ := h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	// Explicit review can renew revoked access and resume the same goal/budget.
	if _, err := h.s.gafferProviders.ConfigureWorker(owner.ID, "gaffer", c.Conversation.ID, session.ID, store.WorkerGrant{Revision: paused.Pins.WorkerRevision, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	paused, _ = h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	in.GoalPins = paused.Pins
	in.ConversationRevision = paused.ConversationRevision
	in.Revision = paused.Goal.Revision
	in.Action = "resume"
	contextAPIJSON(t, browser, ts, "POST", base+"/goal/setup", in, 200)
	resumed, _ := h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	if resumed.Goal.ID != paused.Goal.ID || resumed.Goal.ExpiresAt != paused.Goal.ExpiresAt || resumed.Goal.UsedReviews != paused.Goal.UsedReviews || resumed.Goal.MaxReviews != paused.Goal.MaxReviews || resumed.Goal.State != "active" {
		t.Fatal("resume replaced the budget or goal", paused, resumed)
	}
	// A separate owned session has no grant just because provider maxima changed.
	other, _, err := st.ReserveTerminalCreateFor(owner.ID, session.AgentID, strings.Repeat("4", 32), record.Generation, terminalproto.Create{Title: "Other selected session", Tool: "opencode", Structured: true, Dir: "/work", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ObserveTerminalStateFor(owner.ID, session.AgentID, other.ID, record.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	otherChat, err := h.s.gafferProviders.CreateConversation(owner.ID, gaffer.ConversationInput{SessionID: other.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	otherView, err := h.s.gafferProviders.Goal(owner.ID, otherChat.Conversation.ID)
	if err != nil || otherView.Allowed || otherView.Goal != nil {
		t.Fatal("setup enabled another session", otherView, err)
	}
	code, _ := doClient(t, browser, ts, "POST", base+"/goal/setup", `{}`, "Origin", "https://elsewhere.test", "Content-Type", "application/json")
	if code != 403 {
		t.Fatal("cross-origin setup", code)
	}
	token, _ := mcpManual(t, browser, ts, "write")
	code, _ = do(t, ts, "POST", base+"/goal/setup", `{}`, "Authorization", "Bearer "+token, "Origin", ts.URL, "Content-Type", "application/json")
	if code != 403 && code != 401 {
		t.Fatal("MCP self-enabled setup", code)
	}
}
