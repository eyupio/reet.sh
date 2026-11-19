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
	// A scan only joins skills the library has.
	if code, res := doClient(t, client, srv, "POST", "/api/skills", `{"input":"o/r/alpha"}`, "X-Reet-Token", "tok"); code != 200 {
		t.Fatalf("add %d %s", code, res)
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
	// A scan only joins skills the library has.
	if code, b := do(t, srv, "POST", "/api/skills", `{"input":"a/b/one","used":true}`); code != 200 {
		t.Fatal(b)
	}
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

// The skills.sh catalogue, the skills CLI and a repository can each name one
// skill differently. "UI/UX Design Review" is uiux-design-review in the
// catalogue and is installed into ui-ux-design-review. A scan that reports the
// installed folder must be read as the catalogued skill, or the folder stays
// "missing" for ever and a second, unlinked skill appears beside it.
func TestScanReadsAnInstalledFolderAsItsCataloguedSkill(t *testing.T) {
	st, err := storetest.Open(t)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := httptest.NewServer(New(st, deadFetcher(t), "tok"))
	defer srv.Close()
	client := srv.Client()
	const catalogued = "rknall/claude-skills/uiux-design-review"
	if code, body := doClient(t, client, srv, "POST", "/api/skills", `{"input":"`+catalogued+`"}`, "X-Reet-Token", "tok"); code >= 300 {
		t.Fatalf("add %d %s", code, body)
	}
	// A scan only joins skills the library has.
	if code, body := doClient(t, client, srv, "POST", "/api/skills", `{"input":"rknall/claude-skills/alpha"}`, "X-Reet-Token", "tok"); code >= 300 {
		t.Fatalf("add %d %s", code, body)
	}
	if err := st.SetMeta(catalogued, "UI/UX Design Review", "", 0, 0, nil); err != nil {
		t.Fatal(err)
	}
	scan := `{"machine":{"host":"laptop","os":"linux","arch":"amd64","version":"vtest"},"prune":true,` +
		`"items":[{"name":"ui-ux-design-review","source":"rknall/claude-skills","host":"laptop","agent":"agents"},` +
		`{"name":"alpha","source":"rknall/claude-skills","host":"laptop","agent":"agents"}],` +
		`"environments":[{"host":"laptop","agent":"agents","skills":["alpha","ui-ux-design-review"]}]}`
	for range 2 {
		if code, body := doClient(t, client, srv, "POST", "/api/import", scan, "X-Reet-Token", "tok", "X-Reet-Source", "scan"); code != 200 {
			t.Fatalf("scan %d %s", code, body)
		}
	}
	var state struct{ Environments []store.SyncEnvironment }
	_, body := doClient(t, client, srv, "GET", "/api/v2/sync", "", "X-Reet-Token", "tok")
	if err := json.Unmarshal([]byte(body), &state); err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, env := range state.Environments {
		if env.Agent == "agents" {
			for _, sk := range env.Skills {
				ids = append(ids, sk.ID)
			}
		}
	}
	if strings.Join(ids, ",") != "rknall/claude-skills/alpha,"+catalogued {
		t.Fatalf("installed folder was not read as the catalogued skill: %v", ids)
	}
	_, body = doClient(t, client, srv, "GET", "/api/skills", "", "X-Reet-Token", "tok")
	if strings.Contains(body, "rknall/claude-skills/ui-ux-design-review") {
		t.Fatalf("a second skill was made for the installed folder: %s", body)
	}
}
