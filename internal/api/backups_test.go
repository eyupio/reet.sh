package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eyupio/reet.sh/internal/backup"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestBackupAndProfileAPI(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	user, err := st.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	sealer, _ := backup.NewSealer("0123456789abcdef0123456789abcdef", "test")
	svc := backup.New(st, backup.Config{Dir: t.TempDir(), Sealer: sealer, Usable: true, KeySource: "test", Key: "0123456789abcdef0123456789abcdef"})
	handler := New(st, meta.Fetcher{}, "", WithAuthMode(AuthNone), WithBackups(svc))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	get := func(path string) map[string]any {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("GET %s: %s", path, resp.Status)
		}
		var v map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	send := func(method, path string, body any) map[string]any {
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, ts.URL+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s: %s %v", method, path, resp.Status, v)
		}
		return v
	}
	post := func(path string, body any) map[string]any {
		raw, _ := json.Marshal(body)
		resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			var e map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&e)
			t.Fatalf("POST %s: %s %v", path, resp.Status, e)
		}
		var v map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&v)
		return v
	}

	status := get("/api/v2/admin/backups")
	if status["enabled"] != true || status["scope"] != "instance" {
		t.Fatalf("admin status: %+v", status)
	}
	saved := send(http.MethodPatch, "/api/v2/admin/backups", map[string]any{"everySeconds": 7200, "keep": 5, "destination": "remote"})
	if saved["destination"] != "remote" || saved["everySeconds"] != float64(7200) {
		t.Fatalf("settings were not saved: %+v", saved)
	}
	reloaded := get("/api/v2/admin/backups")
	settings, _ := reloaded["settings"].(map[string]any)
	if settings["destination"] != "remote" {
		t.Fatalf("destination did not survive a reload: %+v", settings)
	}
	saved = send(http.MethodPatch, "/api/v2/admin/backups", map[string]any{
		"everySeconds": 7200, "keep": 5, "destination": "both", "compression": "none",
		"password": "a copy password long enough",
	})
	if saved["destination"] != "both" || saved["compression"] != "none" || saved["hasPassword"] != true {
		t.Fatalf("settings were not saved: %+v", saved)
	}
	draft := send(http.MethodPost, "/api/v2/admin/backups/remotes/test", map[string]any{
		"name": "probe", "endpoint": "not a url", "bucket": "b", "accessKey": "k", "secretKey": "s",
	})
	if draft["ok"] != false || draft["error"] == nil || draft["error"] == "" {
		t.Fatalf("a bad draft endpoint was not refused: %+v", draft)
	}
	item := post("/api/v2/admin/backups/run", map[string]any{})
	if item["kind"] != "instance" || item["local"] != true {
		t.Fatalf("instance backup: %+v", item)
	}
	verify := post("/api/v2/admin/backups/items/"+item["id"].(string)+"/verify", map[string]any{})
	if verify["ok"] != true {
		t.Fatalf("verify: %+v", verify)
	}

	// A user's own backup, restored through the API.
	r, _ := ref.Parse("owner/repo/skill")
	if _, err := st.UpsertFor(user.ID, r, store.Patch{}); err != nil {
		t.Fatal(err)
	}
	userItem := post("/api/v2/backups/run", map[string]any{})
	if userItem["kind"] != "user" {
		t.Fatalf("user backup: %+v", userItem)
	}
	if err := st.RemoveFor(user.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	post("/api/v2/backups/items/"+userItem["id"].(string)+"/restore", map[string]any{})
	if _, ok, _ := st.GetFor(user.ID, r.ID); !ok {
		t.Fatal("the user backup did not restore the shelf")
	}

	probe := get("/api/v2/profile")
	if probe["accounts"] != false {
		t.Fatalf("profile: %+v", probe)
	}
	req, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v2/profile", bytes.NewReader([]byte(`{"name":"Owner","theme":"dark","shelfView":"board","cardsExpanded":true}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("save profile: %v %v", err, resp)
	}
	resp.Body.Close()
	profile, err := st.ProfileFor(user.ID)
	if err != nil || profile.ShelfView != "board" || !profile.CardsExpanded {
		t.Fatalf("shelf defaults not saved: %+v %v", profile, err)
	}
	invalid, _ := http.NewRequest(http.MethodPatch, ts.URL+"/api/v2/profile", bytes.NewBufferString(`{"shelfView":"invalid","cardsExpanded":false}`))
	invalid.Header.Set("Content-Type", "application/json")
	bad, err := http.DefaultClient.Do(invalid)
	if err != nil {
		t.Fatal(err)
	}
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid layout: %s", bad.Status)
	}
	profile, _ = st.ProfileFor(user.ID)
	if profile.ShelfView != "board" || !profile.CardsExpanded {
		t.Fatal("invalid request changed defaults")
	}
	if u, ok, _ := st.UserByID(user.ID); !ok || u.Name != "Owner" {
		t.Fatalf("name not saved: %+v", u)
	}
}
