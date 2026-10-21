package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestMachineMaintenanceAndLegacyImports(t *testing.T) {
	srv, st := newAccountsSrvStore(t, "")
	owner := &http.Client{Jar: mustJar(t)}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"password-for-fixture"}`); code != 200 {
		t.Fatalf("setup %d %s", code, body)
	}
	for _, host := range []string{"hostname", "PROD"} {
		code, body := doClient(t, owner, srv, "POST", "/api/import", `{"machine":{"host":"`+host+`"},"prune":true,"items":[{"name":"alpha","source":"o/r","host":"`+host+`","agent":"agents"}],"environments":[{"host":"`+host+`","agent":"agents","skills":["alpha"]}]}`)
		if code != 200 {
			t.Fatalf("import %d %s", code, body)
		}
	}
	if code, body := doClient(t, owner, srv, "POST", "/api/v2/machines/merge", `{"from":"hostname","to":"PROD"}`); code != 200 {
		t.Fatalf("merge %d %s", code, body)
	}
	// Old binaries still send their original hostname; pruning and IDs must use the preferred name.
	if code, body := doClient(t, owner, srv, "POST", "/api/import", `{"machine":{"host":"hostname"},"prune":true,"items":[],"environments":[{"host":"hostname","agent":"agents","skills":[]}]}`); code != 200 {
		t.Fatalf("rescan %d %s", code, body)
	}
	user, _, _ := st.OwnerUser()
	envs, _ := st.SyncEnvironmentsFor(user.ID)
	for _, env := range envs {
		if env.Host != "PROD" || env.SkillCount != 0 {
			t.Fatalf("old hostname recreated or failed prune: %+v", env)
		}
	}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/machines/merge", `{"from":"","to":"PROD"}`); code != 400 {
		t.Fatalf("invalid name: %d", code)
	}
	code, body := doClient(t, owner, srv, "POST", "/api/v2/admin/invites", `{"email":"viewer@example.test","role":"viewer"}`)
	var invite struct{ Token string }
	if code != 200 || json.Unmarshal([]byte(body), &invite) != nil {
		t.Fatal(body)
	}
	viewer := &http.Client{Jar: mustJar(t)}
	if code, _ := doClient(t, viewer, srv, "POST", "/api/v2/auth/join", `{"token":"`+invite.Token+`","name":"Viewer","password":"password-for-fixture"}`); code != 200 {
		t.Fatalf("join %d", code)
	}
	for _, method := range []string{"POST", "DELETE"} {
		path, body := "/api/v2/machines/merge", `{"from":"hostname","to":"PROD"}`
		if method == "DELETE" {
			path = "/api/v2/machines?host=PROD"
			body = ""
		}
		if code, _ := doClient(t, viewer, srv, method, path, body); code != 403 {
			t.Fatalf("viewer %s %d", method, code)
		}
		if code, _ := do(t, srv, method, path, body); code != 401 {
			t.Fatalf("anonymous %s %d", method, code)
		}
	}
	if code, _ := doClient(t, owner, srv, "DELETE", "/api/v2/machines?host=hostname", ""); code != 200 {
		t.Fatalf("forget %d", code)
	}
	envs, _ = st.SyncEnvironmentsFor(user.ID)
	if len(envs) != 0 {
		t.Fatal("machine reports remain")
	}
}
