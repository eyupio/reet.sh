package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// pathwayStart is an account with a default provider, a project and a saved
// pathway whose custom stage has a skill, behind a server that has remote
// sessions on.
func pathwayStart(t *testing.T) (*store.Store, *httptest.Server, *http.Client, string, string) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal), WithRemoteTerminals(true))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	if _, err := h.s.gafferProviders.Save(gaffer.Principal{UserID: 1, CanUseLocal: true}, gaffer.Input{Name: "Brains", Kind: "openai_compatible", BaseURL: srv.URL, LocalOnly: true, Enabled: true, MakeDefault: true, Model: "fake-model"}); err != nil {
		t.Fatal(err)
	}
	agent, _, err := st.EnrollSyncAgent(1, "machine", "test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateFolderProjectFor(1, store.FolderProject{Host: agent.ID, Dir: "/work/billing", Name: "billing", IsRepo: true})
	if err != nil {
		t.Fatal(err)
	}
	p, err := h.s.gafferProviders.SavePathway(1, store.GafferPathway{Name: "Careful", Enabled: true, Stages: []store.GafferPathwayStage{
		{Kind: "code"}, {Kind: "custom", Name: "Audit", Prompt: "Audit the logging.", Skills: []string{"builtin:review"}}, {Kind: "pr"},
	}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return st, srv, owner, project.ID, p.ID
}

func TestStartOutcomeOnAPathway(t *testing.T) {
	_, srv, owner, project, pathway := pathwayStart(t)
	body := `{"objective":"Tidy the logs.","projectId":"` + project + `","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Logs","prompt":"Tidy the logs. Do not merge."}]}`
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", body)
	if code != 201 {
		t.Fatalf("start: %d %s", code, got)
	}
	var o struct {
		Sessions []struct {
			PathwayName string `json:"pathwayName"`
			Stages      []struct{ Kind, Name string }
		}
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, st := range o.Sessions[0].Stages {
		if st.Name != "" {
			labels = append(labels, st.Name)
		} else {
			labels = append(labels, st.Kind)
		}
	}
	if strings.Join(labels, ",") != "code,Audit,pr" || o.Sessions[0].PathwayName != "Careful" {
		t.Fatalf("stages %v pathway %q: %s", labels, o.Sessions[0].PathwayName, got)
	}
}

func TestStartRefusesStagesWithAPathway(t *testing.T) {
	_, srv, owner, project, pathway := pathwayStart(t)
	body := `{"objective":"Tidy the logs.","projectId":"` + project + `","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Logs","prompt":"Tidy the logs. Do not merge.","stages":[{"kind":"code"}]}]}`
	if code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", body); code != 422 || !strings.Contains(got, "not both") {
		t.Fatalf("stages and a pathway: %d %s", code, got)
	}
}

func pathwayBody(name string, extra string) string {
	return `{"name":"` + name + `","description":"","enabled":true,"stages":[{"kind":"code"},{"kind":"pr"}]` + extra + `}`
}

func TestPathwayCreateListAndStalePut(t *testing.T) {
	_, srv, owner, _, _ := pathwayStart(t)
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", `{"id":"ignored","createdAt":5,"updatedAt":9,"name":"Quick","description":"","enabled":false,"stages":[{"kind":"code"}]}`)
	if code != 201 {
		t.Fatalf("create: %d %s", code, got)
	}
	var made struct {
		ID        string   `json:"id"`
		Enabled   bool     `json:"enabled"`
		UpdatedAt int64    `json:"updatedAt"`
		Problems  []string `json:"problems"`
	}
	if err := json.Unmarshal([]byte(got), &made); err != nil || made.ID == "ignored" || made.Enabled || made.Problems == nil || len(made.Problems) != 0 {
		t.Fatalf("created %v %s", err, got)
	}
	_, list := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/pathways", "")
	if !strings.Contains(list, `"Quick"`) || !strings.Contains(list, `"Careful"`) || !strings.Contains(list, `"problems":[]`) {
		t.Fatalf("list: %s", list)
	}
	path := "/api/v2/gaffer/pathways/" + made.ID
	first := `{"id":"other","name":"First","enabled":true,"stages":[{"kind":"code"}],"updatedAt":` + strconv.FormatInt(made.UpdatedAt, 10) + `}`
	if code, got := gafferClient(t, owner, srv, "PUT", path, first); code != 200 || !strings.Contains(got, `"problems":[]`) {
		t.Fatalf("first put: %d %s", code, got)
	}
	second := strings.Replace(first, `"First"`, `"Second"`, 1)
	if code, got := gafferClient(t, owner, srv, "PUT", path, second); code != 409 || !strings.Contains(got, `"pathway_changed"`) {
		t.Fatalf("stale put: %d %s", code, got)
	}
	if _, got := gafferClient(t, owner, srv, "GET", path, ""); !strings.Contains(got, `"First"`) {
		t.Fatalf("get after stale put: %s", got)
	}
	if code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", pathwayBody("first", "")); code != 409 || !strings.Contains(got, `"pathway_name"`) {
		t.Fatalf("duplicate name: %d %s", code, got)
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", path, ""); code != 204 {
		t.Fatalf("delete: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "GET", path, ""); code != 404 {
		t.Fatalf("get deleted: %d", code)
	}
}

func TestPathwayInvalidStagesAndProblems(t *testing.T) {
	_, srv, owner, _, _ := pathwayStart(t)
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", `{"name":"Bad","enabled":true,"stages":[{"kind":"pr"},{"kind":"code"},{"kind":"pr"}]}`)
	if code != 422 || !strings.Contains(got, `"stages"`) {
		t.Fatalf("invalid stages: %d %s", code, got)
	}
	code, got = gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", `{"name":"Skilled","enabled":true,"stages":[{"kind":"code","skills":["no-such-skill"]}]}`)
	if code != 201 {
		t.Fatalf("skilled: %d %s", code, got)
	}
	var made struct{ Problems []string }
	if err := json.Unmarshal([]byte(got), &made); err != nil {
		t.Fatal(err)
	}
	if len(made.Problems) != 1 || !strings.Contains(made.Problems[0], "no-such-skill") {
		t.Fatalf("problems: %s", got)
	}
}

func TestPathwayRoutesAreTheOwningAccountsOnly(t *testing.T) {
	st, srv, owner := templateServer(t)
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", pathwayBody("Mine", ""))
	if code != 201 {
		t.Fatalf("create: %d %s", code, got)
	}
	var made struct {
		ID        string
		UpdatedAt int64
	}
	if err := json.Unmarshal([]byte(got), &made); err != nil {
		t.Fatal(err)
	}
	stamp := `,"updatedAt":` + strconv.FormatInt(made.UpdatedAt, 10)
	path := "/api/v2/gaffer/pathways/" + made.ID
	password, _ := auth.HashPassword("long-enough-password")
	clients := map[string]*http.Client{}
	for _, role := range []string{"member", "viewer"} {
		user, err := st.CreateUser(role+"@example.test", role, role, password)
		if err != nil {
			t.Fatal(err)
		}
		clients[role] = &http.Client{Jar: mustJar(t)}
		if code, _ := gafferClient(t, clients[role], srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatal(code)
		}
	}
	for _, route := range []struct{ method, path, body string }{
		{"GET", "/api/v2/gaffer/pathways", ""}, {"POST", "/api/v2/gaffer/pathways", pathwayBody("Other", "")},
		{"GET", path, ""}, {"PUT", path, pathwayBody("Mine", stamp)}, {"DELETE", path, ""},
	} {
		if code, _ := gafferClient(t, &http.Client{}, srv, route.method, route.path, route.body); code != 401 {
			t.Fatalf("signed out %s %s: %d", route.method, route.path, code)
		}
		if code, _ := gafferClient(t, clients["viewer"], srv, route.method, route.path, route.body); code != 403 {
			t.Fatalf("read-only %s %s: %d", route.method, route.path, code)
		}
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		body := ""
		if method == "PUT" {
			body = pathwayBody("Mine", stamp)
		}
		if code, got := gafferClient(t, clients["member"], srv, method, path, body); code != 404 || strings.Contains(got, "Mine") {
			t.Fatalf("other account %s: %d %s", method, code, got)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "PUT", path, pathwayBody("Mine", "")); code != 409 {
		t.Fatalf("put without a read time: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "GET", path, ""); code != 200 {
		t.Fatalf("owner after: %d", code)
	}
}

// stageSkillShown says whether the Skills panel of a pipeline's stage holds a
// selected receipt for the skill.
func stageSkillShown(t *testing.T, st *store.Store, outcome, key string, stage int, skill string) bool {
	t.Helper()
	rows, err := st.SkillBuddyRecordsFor(1, "factory:"+outcome+":"+key+":"+strconv.Itoa(stage))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Skill.ID == skill && r.Skill.State == "selected" {
			return true
		}
	}
	return false
}

func TestPathwaySkillsShowInTheStagesSkillsPanel(t *testing.T) {
	st, srv, owner, project, pathway := pathwayStart(t)
	body := `{"objective":"Tidy the logs.","projectId":"` + project + `","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Logs","prompt":"Tidy the logs. Do not merge."}]}`
	code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", body)
	if code != 201 {
		t.Fatalf("start: %d %s", code, got)
	}
	var o struct {
		ID       string
		Revision int64
		Sessions []struct{ Key string }
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil {
		t.Fatal(err)
	}
	if !stageSkillShown(t, st, o.ID, o.Sessions[0].Key, 1, "builtin:review") {
		t.Fatalf("start: the custom stage's skill is not in its Skills panel")
	}
	add := `{"revision":` + strconv.FormatInt(o.Revision, 10) + `,"confirmed":true,"pathway":"` + pathway + `","sessions":[{"title":"Metrics","prompt":"Tidy the metrics. Do not merge."}]}`
	code, got = gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+o.ID+"/sessions", add)
	if code != 200 {
		t.Fatalf("add: %d %s", code, got)
	}
	if err := json.Unmarshal([]byte(got), &o); err != nil {
		t.Fatal(err)
	}
	if len(o.Sessions) != 2 || !stageSkillShown(t, st, o.ID, o.Sessions[1].Key, 1, "builtin:review") {
		t.Fatalf("add: the custom stage's skill is not in its Skills panel: %s", got)
	}
}

func TestAFullSizePathwayIsAccepted(t *testing.T) {
	_, srv, owner, _, _ := pathwayStart(t)
	// Quotes and newlines take two bytes each once JSON escapes them.
	prompt := strings.Repeat("\"q\"\n", 1000)
	stages := make([]map[string]string, 12)
	for i := range stages {
		stages[i] = map[string]string{"kind": "custom", "name": "Stage " + strconv.Itoa(i), "prompt": prompt}
	}
	body, err := json.Marshal(map[string]any{"name": "Big", "enabled": true, "stages": stages})
	if err != nil {
		t.Fatal(err)
	}
	if code, got := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/pathways", string(body)); code != 201 {
		t.Fatalf("full-size pathway: %d %s", code, got)
	}
}
