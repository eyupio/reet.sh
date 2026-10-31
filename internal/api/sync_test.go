package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestSyncAPIAndIncompleteScan(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok"))
	defer srv.Close()
	client := srv.Client()
	scan := func(host, items, skills string) string {
		t.Helper()
		body := `{"machine":{"host":"` + host + `","os":"linux","arch":"amd64","version":"vtest"},"prune":true,"items":` + items + `,"environments":[{"host":"` + host + `","agent":"agents","skills":` + skills + `}]}`
		code, res := doClient(t, client, srv, "POST", "/api/import", body, "X-Reet-Token", "tok", "X-Reet-Source", "scan")
		if code != 200 {
			t.Fatalf("scan %d %s", code, res)
		}
		return res
	}
	scan("laptop", `[{"name":"alpha","source":"o/r","host":"laptop","agent":"agents"}]`, `["alpha"]`)
	scan("server", `[]`, `[]`)
	var state struct {
		Environments []store.SyncEnvironment
		Targets      []store.SyncTarget
	}
	code, body := doClient(t, client, srv, "GET", "/api/v2/sync", "", "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatal(body)
	}
	json.Unmarshal([]byte(body), &state)
	source, target := "", ""
	for _, env := range state.Environments {
		if env.Agent == "agents" {
			if env.Host == "laptop" {
				source = env.ID
			} else {
				target = env.ID
			}
		}
	}
	code, body = doClient(t, client, srv, "PUT", "/api/v2/sync", `{"sourceId":"`+source+`","targetId":"`+target+`"}`, "X-Reet-Token", "tok")
	if code != 200 {
		t.Fatalf("save %d %s", code, body)
	}
	json.Unmarshal([]byte(body), &state)
	if len(state.Targets) != 1 || len(state.Targets[0].Missing) != 1 {
		t.Fatalf("missing drift: %s", body)
	}
	res := scan("laptop", `[{"name":"invalid/name"}]`, `[]`)
	if !strings.Contains(res, `"errors":[`) {
		t.Fatal(res)
	}
	owner, _ := st.DefaultOwner()
	skills, err := st.EnvironmentSkillsFor(owner, source)
	if err != nil || len(skills) != 1 {
		t.Fatalf("incomplete scan pruned valid inventory: %+v %v", skills, err)
	}
	envs, _ := st.SyncEnvironmentsFor(owner)
	for _, env := range envs {
		if env.ID == source && env.Complete {
			t.Fatal("partial scan marked authoritative")
		}
	}
	code, _ = doClient(t, client, srv, "PUT", "/api/v2/sync", `{"sourceId":"`+source+`","targetId":"`+target+`"}`, "X-Reet-Token", "tok")
	if code != 400 {
		t.Fatalf("incomplete reference accepted: %d", code)
	}
}

func TestLocalScanErrorPreservesInventoryAndShelfIntent(t *testing.T) {
	srv := newSrv(t, deadFetcher(t), "")
	good := `{"prune":true,"machine":{"host":"laptop"},"items":[{"name":"one","source":"a/b","host":"laptop","agent":"agents"}],"environments":[{"host":"laptop","agent":"agents","skills":["one"]}]}`
	if code, b := do(t, srv, "POST", "/api/import", good); code != 200 {
		t.Fatal(b)
	}
	do(t, srv, "PATCH", "/api/skill", `{"id":"a/b/one","used":false,"notes":"keep"}`)
	do(t, srv, "POST", "/api/import", good)
	sk := list(t, srv, "").Skills[0]
	if sk.Used || sk.Notes != "keep" {
		t.Fatal("scan overwrote user intent")
	}
	bad := `{"prune":true,"machine":{"host":"laptop"},"items":[],"environments":[{"host":"laptop","agent":"agents","skills":[],"errors":["permission denied"]}]}`
	code, body := do(t, srv, "POST", "/api/import", bad)
	if code != 200 || !strings.Contains(body, "permission denied") {
		t.Fatal(body)
	}
	_, body = do(t, srv, "GET", "/api/v2/sync", "")
	var state struct{ Environments []store.SyncEnvironment }
	json.Unmarshal([]byte(body), &state)
	for _, env := range state.Environments {
		if env.Agent == "agents" && (env.Complete || len(env.Skills) != 1 || env.Error == "") {
			t.Fatalf("bad scan pruned or confirmed inventory: %+v", env)
		}
	}
}
