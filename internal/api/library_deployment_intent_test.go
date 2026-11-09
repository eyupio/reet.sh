package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestLibraryDeploymentIntentConnectorFence(t *testing.T) {
	srv, st, agent, secret, token := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	if err := st.ClaimPublishingUsername(uid, "writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveHostedSkill(uid, store.HostedSkill{Slug: "managed", Content: "---\nname: managed\ndescription: Managed example\n---\nPrivate instructions\n"}); err != nil {
		t.Fatal(err)
	}
	items, err := st.LibraryFor(uid)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	item := items[0]
	st.TouchEnvironmentFor(uid, agent.Host, "agents", "", "")
	target := store.EnvironmentIDFor(uid, agent.Host, "agents", "")
	if _, err = st.SaveLibrarySelectionFor(uid, store.LibrarySelection{TargetID: target, ItemIDs: []string{item.ID}}); err != nil {
		t.Fatal(err)
	}
	if err = st.SetLibraryStatusFor(uid, item.ID, store.LibraryInUse, item.Version); err != nil {
		t.Fatal(err)
	}
	in := store.LibraryDeploymentCheck{TargetID: target, LibraryBase: store.LibraryBase{BindingID: strings.Repeat("a", 64), ItemID: item.ID, RevisionID: item.RevisionID}}
	if err = st.RecordLibraryBaseFor(uid, agent.ID, in.LibraryBase); err != nil {
		t.Fatal(err)
	}
	call := func(payload, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://reet.example/api/v2/library/deployment-intent", strings.NewReader(payload))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Reet-Agent", agent.ID)
		r.Header.Set("X-Reet-Agent-Key", key)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		return w
	}
	raw, _ := json.Marshal(in)
	if w := call(string(raw), "wrong"); w.Code != 403 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call(strings.TrimSuffix(string(raw), "}")+`,"unknown":true}`, secret); w.Code != 400 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := call(string(raw), secret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var intent store.LibraryDeploymentIntent
	if err = json.Unmarshal(w.Body.Bytes(), &intent); err != nil || intent.Action != "maintain" || intent.Generation == "" {
		t.Fatal(intent, err)
	}
	item, _ = st.LibraryItemFor(uid, item.ID)
	if err = st.SetLibraryStatusFor(uid, item.ID, store.LibraryToTest, item.Version); err != nil {
		t.Fatal(err)
	}
	in.Generation = intent.Generation
	raw, _ = json.Marshal(in)
	if w = call(string(raw), secret); w.Code != 409 {
		t.Fatal("late work accepted", w.Code, w.Body.String())
	}
	in.Generation = ""
	raw, _ = json.Marshal(in)
	w = call(string(raw), secret)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &intent); err != nil || intent.Action != "remove" || intent.Reason != "inactive" {
		t.Fatal(intent, err)
	}
	if err = st.ControlSyncAgent(uid, agent.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if w = call(string(raw), secret); w.Code != 409 {
		t.Fatal("paused machine received intent", w.Code, w.Body.String())
	}
}

func TestLibraryProtocolAdvertisesManagedRemovalsOnlyWhenEnabled(t *testing.T) {
	srv, st, agent, secret, token := privateFixture(t, AuthAccounts, "")
	call := func() map[string]any {
		r := httptest.NewRequest("POST", "https://reet.example/api/v2/library/protocol", strings.NewReader("{}"))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Reet-Agent", agent.ID)
		r.Header.Set("X-Reet-Agent-Key", secret)
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := call(); out["schemaVersion"] != float64(1) || out["managedRemovals"] != nil {
		t.Fatalf("capability advertised by default: %v", out)
	}
	st.SetManagedRemovals(true)
	if out := call(); out["managedRemovals"] != float64(1) {
		t.Fatalf("capability not advertised when enabled: %v", out)
	}
	st.SetManagedRemovals(false)
	if out := call(); out["managedRemovals"] != nil {
		t.Fatalf("capability retained after disabling: %v", out)
	}
}
