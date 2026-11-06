package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestGoalHTTPNeedsOwningBrowserExplicitPinsAndBothGrants(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-goal-test", 8), "test")
	ts := httptest.NewUnstartedServer(nil)
	h := New(st, meta.Fetcher{}, "", WithAuthMode(AuthAccounts), WithPublicURL("http://"+ts.Listener.Addr().String()), WithRemoteTerminals(true), WithGafferProviders(seal))
	ts.Config.Handler = h
	ts.Start()
	t.Cleanup(func() { ts.Close(); h.Close(); st.Close() })
	h.s.gafferProviders.SetWorker(&workerAPIFake{st: st}, h.s.terminalEnabled)
	client := &http.Client{Jar: mustJar(t)}
	contextAPIJSON(t, client, ts, "POST", "/api/v2/auth/setup", map[string]any{"name": "Owner", "email": "goal-owner@example.test", "password": "long goal password"}, 200)
	owner, _, _ := st.OwnerUser()
	session := contextAPISession(t, st, owner.ID)
	provider, err := h.s.gafferProviders.Save(gaffer.Principal{UserID: owner.ID, CanUseLocal: true}, gaffer.Input{Name: "Fake goal", Kind: "openai_compatible", BaseURL: "http://127.0.0.1:1", LocalOnly: true, Enabled: true, Model: "fake-goal", AllowSessions: true, AllowCode: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := h.s.gafferProviders.CreateConversation(owner.ID, gaffer.ConversationInput{SessionID: session.ID, ProviderID: provider.ID})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/v2/gaffer/conversations/" + c.Conversation.ID
	path := base + "/goal"
	v, err := h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	if err != nil || v.Allowed {
		t.Fatal(v, err)
	}
	payload := gaffer.GoalInput{GoalPins: v.Pins, Text: "GOAL_PRIVATE_TEXT: raise PR and fix checks, do not merge", MaxReviews: 12, Hours: 24}
	contextAPIJSON(t, client, ts, "PUT", path, payload, 422)
	worker, err := h.s.gafferProviders.WorkerSettings(owner.ID, "gaffer", c.Conversation.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.s.gafferProviders.ConfigureWorker(owner.ID, "gaffer", c.Conversation.ID, session.ID, store.WorkerGrant{Enabled: true, ProviderID: worker.ProviderID, ProviderRevision: worker.ProviderRevision, Model: worker.Model}); err != nil {
		t.Fatal(err)
	}
	sharing, err := h.s.gafferProviders.ContextSettings(owner.ID, "gaffer", c.Conversation.ID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.s.gafferProviders.ConfigureContext(owner.ID, "gaffer", c.Conversation.ID, session.ID, gaffer.ContextInput{ProviderID: sharing.ProviderID, ProviderRevision: sharing.ProviderRevision, Model: sharing.Model, ContextFlags: store.ContextFlags{History: true, Text: true}}); err != nil {
		t.Fatal(err)
	}
	v, err = h.s.gafferProviders.Goal(owner.ID, c.Conversation.ID)
	if err != nil || !v.Allowed {
		t.Fatal(v, err)
	}
	payload.GoalPins = v.Pins
	body, _ := json.Marshal(payload)
	code, _ := doClient(t, client, ts, "PUT", path, string(body), "Content-Type", "application/json", "Origin", "https://elsewhere.test")
	if code != 403 {
		t.Fatal("cross-site goal", code)
	}
	token, _ := mcpManual(t, client, ts, "write")
	code, _ = do(t, ts, "PUT", path, string(body), "Content-Type", "application/json", "Authorization", "Bearer "+token, "Origin", ts.URL)
	if code != 403 && code != 401 {
		t.Fatal("MCP credential started goal", code)
	}
	contextAPIJSON(t, client, ts, "PUT", path, payload, 200)
	overview := contextAPIJSON(t, client, ts, "GET", "/api/v2/gaffer/goals", nil, 200)
	goals := overview["goals"].([]any)
	if len(goals) != 1 || goals[0].(map[string]any)["goal"].(map[string]any)["text"] != payload.Text {
		t.Fatal("overview omitted the owned goal", overview)
	}
	code, _ = do(t, ts, "GET", "/api/v2/gaffer/goals", "", "Authorization", "Bearer "+token)
	if code != 403 && code != 401 {
		t.Fatal("MCP credential read browser overview", code)
	}
	contextAPIJSON(t, client, ts, "PUT", path, payload, 409)
	raw := contextAPIJSON(t, client, ts, "GET", path, nil, 200)
	goal := raw["goal"].(map[string]any)
	if goal["maxReviews"] != float64(12) || goal["usedReviews"] != float64(0) {
		t.Fatal(goal)
	}
	revision := int64(goal["revision"].(float64))
	control := map[string]any{"revision": revision, "action": "pause", "pins": v.Pins}
	contextAPIJSON(t, client, ts, "POST", path+"/control", control, 200)
	control["revision"] = revision + 1
	control["action"] = "resume"
	contextAPIJSON(t, client, ts, "POST", path+"/control", control, 422)
	control["confirmed"] = true
	contextAPIJSON(t, client, ts, "POST", path+"/control", control, 200)
	hashed, _ := auth.HashPassword("long other password")
	for _, role := range []string{"member", "viewer"} {
		u, err := st.CreateUser(role+"-goal@example.test", role, role, hashed)
		if err != nil {
			t.Fatal(err)
		}
		other := &http.Client{Jar: mustJar(t)}
		contextAPIJSON(t, other, ts, "POST", "/api/v2/auth/sign-in", map[string]any{"email": u.Email, "password": "long other password"}, 200)
		want := 404
		if role == "viewer" {
			want = 403
		}
		contextAPIJSON(t, other, ts, "GET", path, nil, want)
		listWant := 200
		if role == "viewer" {
			listWant = 403
		}
		list := contextAPIJSON(t, other, ts, "GET", "/api/v2/gaffer/goals", nil, listWant)
		if role == "member" && len(list["goals"].([]any)) != 0 {
			t.Fatal("overview leaked another account's goal", list)
		}
	}
	events := contextAPIJSON(t, client, ts, "GET", "/api/events?limit=100", nil, 200)
	audit, _ := json.Marshal(events)
	if strings.Contains(string(audit), "GOAL_PRIVATE_TEXT") {
		t.Fatal("goal text entered audit")
	}
	// Internal scheduler fields cannot be supplied through an ordinary chat send.
	code, _ = doClient(t, client, ts, "POST", base+"/turns", fmt.Sprintf(`{"id":%q,"revision":3,"text":"A normal question","goalId":%q}`, strings.Repeat("a", 32), goal["id"]), "Content-Type", "application/json", "Origin", ts.URL)
	if code != 400 {
		t.Fatal("browser supplied scheduler authority", code)
	}
}
