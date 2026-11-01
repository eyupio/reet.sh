package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
)

func TestBranchPruneSettingDefaultsOff(t *testing.T) {
	srv, _ := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	code, body := doClient(t, owner, srv, "GET", "/api/v2/settings/branch-prune", "")
	var got branchPruneSettingView
	if code != 200 || json.Unmarshal([]byte(body), &got) != nil || got.AfterWorktree {
		t.Fatalf("%d %s", code, body)
	}
}

func TestBranchPruneSettingOnlyOwnersAndAdmins(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "bootstrap")
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.com","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup: %d", code)
	}
	hash, _ := auth.HashPassword("long-enough-password")
	if _, err := st.CreateUser("member@example.com", "Member", "member", hash); err != nil {
		t.Fatal(err)
	}
	adminUser, err := st.CreateUser("admin@example.com", "Admin", "admin", hash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("viewer@example.com", "Viewer", "viewer", hash); err != nil {
		t.Fatal(err)
	}
	member, admin, viewer := &http.Client{Jar: mustJar(t)}, &http.Client{Jar: mustJar(t)}, &http.Client{Jar: mustJar(t)}
	for client, email := range map[*http.Client]string{member: "member@example.com", admin: "admin@example.com", viewer: "viewer@example.com"} {
		if code, _ := doClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatalf("sign in %s: %d", email, code)
		}
	}
	if code, _ := doClient(t, member, srv, "GET", "/api/v2/settings/branch-prune", ""); code != 200 {
		t.Fatalf("member read: %d", code)
	}
	if code, _ := doClient(t, member, srv, "PUT", "/api/v2/settings/branch-prune", `{"afterWorktree":true}`); code != 403 {
		t.Fatalf("member write: %d", code)
	}
	if code, _ := doClient(t, viewer, srv, "PUT", "/api/v2/settings/branch-prune", `{"afterWorktree":true}`); code != 403 {
		t.Fatalf("viewer write: %d", code)
	}
	code, body := doClient(t, admin, srv, "PUT", "/api/v2/settings/branch-prune", `{"afterWorktree":true}`)
	if code != 200 || !strings.Contains(body, `"afterWorktree":true`) {
		t.Fatalf("admin write: %d %s", code, body)
	}
	if code, body := doClient(t, member, srv, "GET", "/api/v2/settings/branch-prune", ""); code != 200 || !strings.Contains(body, `"afterWorktree":true`) {
		t.Fatalf("member read after: %d %s", code, body)
	}
	if code, body := doClient(t, owner, srv, "PUT", "/api/v2/settings/branch-prune", `{"afterWorktree":false}`); code != 200 || !strings.Contains(body, `"afterWorktree":false`) {
		t.Fatalf("owner write: %d %s", code, body)
	}
	on, off := false, false
	for _, uid := range []int64{1, adminUser.ID} {
		events, err := st.ListEventsFor(uid, 50, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range events {
			on = on || e.Action == "branch-prune-automation-on"
			off = off || e.Action == "branch-prune-automation-off"
		}
	}
	if !on || !off {
		t.Fatalf("automation changes were not audited (on=%v off=%v)", on, off)
	}
}
