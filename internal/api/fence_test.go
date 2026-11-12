package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
)

// fenceCall sends one request to the target with an optional JSON body.
func (tg target) fenceCall(t *testing.T, method, path, token, body string) reply {
	t.Helper()
	if body == "" && method == "POST" {
		body = "{}" // the API requires a JSON body on every POST; the address is optional
	}
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, tg.url+path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doImport(t, req)
}

func wantFence(t *testing.T, r reply, fenced bool, movedTo string) {
	t.Helper()
	got, _ := r.v["fenced"].(bool)
	since, _ := r.v["since"].(float64)
	to, _ := r.v["movedTo"].(string)
	if r.code != 200 || got != fenced || to != movedTo || (fenced && since <= 0) || (!fenced && since != 0) {
		t.Fatalf("want fenced=%v movedTo=%q, got %d %s", fenced, movedTo, r.code, r.text)
	}
}

func TestTheFenceRoutesRaiseReadAndLiftTheState(t *testing.T) {
	t.Parallel()
	tg := newTarget(t, false)
	wantFence(t, tg.fenceCall(t, "GET", fencePath, "owner-token", ""), false, "")
	raised := tg.fenceCall(t, "POST", fencePath, "owner-token", `{"movedTo":"https://new.example.com"}`)
	wantFence(t, raised, true, "https://new.example.com")
	since := raised.v["since"]

	// A second POST answers with the existing state and changes nothing.
	again := tg.fenceCall(t, "POST", fencePath, "owner-token", `{"movedTo":"https://elsewhere.example.com"}`)
	wantFence(t, again, true, "https://new.example.com")
	if again.v["since"] != since {
		t.Fatalf("a second POST moved the date: %v then %v", since, again.v["since"])
	}
	wantFence(t, tg.fenceCall(t, "GET", fencePath, "owner-token", ""), true, "https://new.example.com")

	wantFence(t, tg.fenceCall(t, "DELETE", fencePath, "owner-token", ""), false, "")
	// DELETE when not fenced answers the same way and succeeds.
	wantFence(t, tg.fenceCall(t, "DELETE", fencePath, "owner-token", ""), false, "")
	wantFence(t, tg.fenceCall(t, "GET", fencePath, "owner-token", ""), false, "")
}

func TestTheAddressIsOptionalAndMustBeHTTPOrHTTPS(t *testing.T) {
	tg := newTarget(t, false)
	for _, bad := range []string{`{"movedTo":"ftp://new.example.com"}`, `{"movedTo":"not a url"}`, `{"movedTo":"https://user:pw@new.example.com"}`, `{"movedTo":"//new.example.com"}`} {
		if r := tg.fenceCall(t, "POST", fencePath, "owner-token", bad); r.code != 400 {
			t.Fatalf("%s: %d %s", bad, r.code, r.text)
		}
		wantFence(t, tg.fenceCall(t, "GET", fencePath, "owner-token", ""), false, "")
	}
	wantFence(t, tg.fenceCall(t, "POST", fencePath, "owner-token", ""), true, "")
}

func TestTheFenceRoutesRefuseANonOwner(t *testing.T) {
	t.Parallel()
	tg := newTarget(t, true)
	for _, m := range []string{"GET", "POST", "DELETE"} {
		if r := tg.fenceCall(t, m, fencePath, "member-token", ""); r.code != 403 {
			t.Fatalf("%s as a member: %d %s", m, r.code, r.text)
		}
	}
	tg.fenceCall(t, "POST", fencePath, "owner-token", "")
	// Fenced, a member's lift attempt is still a 403 and not an unlock.
	if r := tg.fenceCall(t, "DELETE", fencePath, "member-token", ""); r.code != 403 {
		t.Fatalf("a member lifted the fence: %d %s", r.code, r.text)
	}
	if !tg.st.Fenced() {
		t.Fatal("the fence was lifted by a member")
	}
}

func TestAFencedControllerRefusesWritesWith423AndKeepsReading(t *testing.T) {
	tg := newTarget(t, false)
	write := func() reply { return tg.fenceCall(t, "PATCH", "/api/admin/settings", "owner-token", `{}`) }
	if r := write(); r.code == http.StatusLocked {
		t.Fatalf("a write was refused before the fence: %s", r.text)
	}
	tg.fenceCall(t, "POST", fencePath, "owner-token", `{"movedTo":"https://new.example.com"}`)
	r := write()
	msg, _ := r.v["error"].(string)
	if r.code != http.StatusLocked || !strings.Contains(msg, "moved") || !strings.Contains(msg, "https://new.example.com") || !strings.Contains(msg, "UTC") {
		t.Fatalf("a fenced write: %d %s", r.code, r.text)
	}
	// Every kind of write is refused, not only PATCH, and not only under /api/v2.
	for _, c := range [][2]string{{"POST", "/api/v2/lists"}, {"PUT", "/api/v2/maintenance/worktrees"}, {"DELETE", "/api/v2/admin/backups/items/x"}, {"GET", "/quick?action=save"}} {
		if r := tg.fenceCall(t, c[0], c[1], "owner-token", ""); r.code != http.StatusLocked {
			t.Fatalf("%s %s: %d %s", c[0], c[1], r.code, r.text)
		}
	}
	// Reads keep working.
	for _, p := range []string{"/api/version", "/api/v2/admin/backups", fencePath} {
		if r := tg.fenceCall(t, "GET", p, "owner-token", ""); r.code != 200 {
			t.Fatalf("GET %s while fenced: %d %s", p, r.code, r.text)
		}
	}
	// And the lift returns the controller to what it was.
	tg.fenceCall(t, "DELETE", fencePath, "owner-token", "")
	if r := write(); r.code == http.StatusLocked {
		t.Fatalf("still refusing after the lift: %s", r.text)
	}
}

func TestTheOwnersBackupRoutesAndTheLiftStillWorkWhileFenced(t *testing.T) {
	t.Parallel()
	tg := newTarget(t, false)
	tg.fenceCall(t, "POST", fencePath, "owner-token", "")
	for _, c := range [][3]string{
		{"PATCH", "/api/v2/admin/backups", `{"everySeconds":0}`},
		{"POST", "/api/v2/admin/backups/run", `{}`},
		{"POST", "/api/v2/admin/backups/items/instance-nope/verify", `{}`},
		{"POST", "/api/v2/auth/sign-in", `{"email":"a@b.c","password":"x"}`},
	} {
		if r := tg.fenceCall(t, c[0], c[1], "owner-token", c[2]); r.code == http.StatusLocked {
			t.Fatalf("%s %s was refused as moved: %s", c[0], c[1], r.text)
		}
	}
	wantFence(t, tg.fenceCall(t, "DELETE", fencePath, "owner-token", ""), false, "")
}

// The state is in the database, so a restarted controller comes up still
// fenced, and quiet.
func TestTheFenceSurvivesARestart(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnsureLocalOwner(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetFence("https://new.example.com"); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	h := New(reopened, meta.Fetcher{}, "", WithAuthMode(AuthNone))
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	req, _ := http.NewRequest("PATCH", ts.URL+"/api/admin/settings", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r := doImport(t, req)
	if r.code != http.StatusLocked || !strings.Contains(r.text, "https://new.example.com") {
		t.Fatalf("a restarted controller took a write: %d %s", r.code, r.text)
	}
}

// Topic sync is a background fetch that writes; it stands still while fenced.
func TestAFencedControllerSyncsNoTopicsUntilTheLift(t *testing.T) {
	srv := newServer(mustTestStore(t), deadFetcher(t), "")
	t.Cleanup(func() { srv.cancel() })
	if _, err := srv.st.SetFence(""); err != nil {
		t.Fatal(err)
	}
	srv.syncTopics()
	if v, _ := srv.st.GetSetting("topics_last_error"); v != "" {
		t.Fatalf("topics were fetched while fenced: %q", v)
	}
	if _, err := srv.st.LiftFence(); err != nil {
		t.Fatal(err)
	}
	srv.syncTopics()
	if v, _ := srv.st.GetSetting("topics_last_error"); v == "" {
		t.Fatal("topics were not fetched after the lift")
	}
}

func mustTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
