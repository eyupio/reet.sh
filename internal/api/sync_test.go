package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestSyncAPIAndIncompleteScan(t *testing.T) {
	st, err := store.Open(t.TempDir())
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
