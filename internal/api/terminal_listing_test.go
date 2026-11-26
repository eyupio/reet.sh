package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Both Home and Sessions read this endpoint, so a Factory stage session belongs
// here only while an unarchived outcome still references it.
func TestTerminalSessionsHidesArchivedAndDeletedFactorySessions(t *testing.T) {
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	cookie := terminalLogin(t, st, uid)
	caps, err := st.BeginRemoteConnectionFor(uid, agent.ID, terminalbroker.ID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	next := func() string { n++; return fmt.Sprintf("%032x", n) }
	session := func() string {
		t.Helper()
		s, created, err := st.ReserveTerminalCreateFor(uid, agent.ID, next(), caps.Generation, terminalproto.Create{Title: "Session", Tool: "opencode", Structured: true, Cols: 80, Rows: 24})
		if err != nil || !created {
			t.Fatal(created, err)
		}
		return s.ID
	}
	goal := func(sid, text string) {
		t.Helper()
		provider, err := st.SaveGafferProviderFor(uid, store.GafferProvider{ID: next(), Name: "listing " + next(), Kind: "openai", BaseURL: "http://127.0.0.1:1/v1", Model: "fake", Enabled: true, AllowSessions: true, AllowCode: true})
		if err != nil {
			t.Fatal(err)
		}
		conversation, err := st.CreateGafferConversationFor(uid, store.GafferConversation{ID: next(), SessionID: sid, ProviderID: provider.ID})
		if err != nil {
			t.Fatal(err)
		}
		worker, err := st.SetWorkerGrantFor(uid, "gaffer", conversation.ID, 0, store.WorkerGrant{SessionID: sid, Enabled: true, ProviderID: provider.ID, ProviderRevision: provider.Revision, Model: provider.Model})
		if err != nil {
			t.Fatal(err)
		}
		context, err := st.SetContextGrantFor(uid, "gaffer", conversation.ID, 0, store.ContextGrant{SessionID: sid, ContextFlags: store.ContextFlags{History: true, Text: true}, ProviderID: provider.ID, ProviderRevision: provider.Revision, Model: provider.Model})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.SaveGafferGoal(uid, 0, store.GafferGoal{ID: next(), ConversationID: conversation.ID, Text: text, MaxReviews: 12, ExpiresAt: time.Now().Add(time.Hour).UnixMilli(), ProviderID: provider.ID, ProviderRevision: provider.Revision, Model: provider.Model, ContextRevision: context.Revision, WorkerRevision: worker.Revision}); err != nil {
			t.Fatal(err)
		}
	}
	outcome := func(sid string) store.GafferOutcome {
		t.Helper()
		o, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: next(), Objective: "Ship it", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", SessionID: sid}}})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	ids := func(include ...string) map[string]bool {
		t.Helper()
		target := "/api/v2/terminal/sessions"
		if len(include) > 0 {
			target += "?session=" + include[0]
		}
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("GET sessions: %d %s", w.Code, w.Body.String())
		}
		var body struct {
			Sessions []struct {
				ID string `json:"id"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, s := range body.Sessions {
			seen[s.ID] = true
		}
		return seen
	}

	live := session()
	goal(live, "Outcome: Ship it")
	outcome(live)

	archived := session()
	goal(archived, "Outcome: Ship it")
	archivedOutcome, err := st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: next(), Objective: "Ship it", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", SessionID: archived}}})
	if err != nil {
		t.Fatal(err)
	}
	archivedOutcome, err = st.UpdateGafferOutcomeFor(uid, archivedOutcome.ID, archivedOutcome.Revision, func(o *store.GafferOutcome) bool {
		o.ArchivedAt = 1_800_000_000_000
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	deleted := session()
	goal(deleted, "Outcome: Ship it")
	gone := outcome(deleted)
	if _, err = st.UpdateGafferOutcomeFor(uid, gone.ID, gone.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State = "settled", "done"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	gone, err = st.GafferOutcomeFor(uid, gone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = st.DeleteGafferOutcomeFor(uid, gone.ID, gone.Revision); err != nil {
		t.Fatal(err)
	}

	ordinary := session()
	plainGoal := session()
	goal(plainGoal, "Refactor: tidy it")

	seen := ids()
	for _, id := range []string{live, ordinary, plainGoal} {
		if !seen[id] {
			t.Fatalf("session %s hidden but should be listed", id)
		}
	}
	for _, id := range []string{archived, deleted} {
		if seen[id] {
			t.Fatalf("session %s listed after its outcome was archived or deleted", id)
		}
	}
	for _, id := range []string{archived, deleted} {
		if !ids(id)[id] {
			t.Fatalf("session %s not kept for a direct link", id)
		}
	}

	if _, err = st.UpdateGafferOutcomeFor(uid, archivedOutcome.ID, archivedOutcome.Revision, func(o *store.GafferOutcome) bool {
		o.ArchivedAt = 0
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if !ids()[archived] {
		t.Fatal("restored outcome session still hidden")
	}
}
