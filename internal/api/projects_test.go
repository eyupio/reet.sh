package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// projectSentinel is a shared prompt that must never leave the single-Project
// read: not in lists, runs, audit, Activity or logs.
const projectSentinel = "zebra-sentinel shared prompt for every repository"

type projectAPI struct {
	*launchAPI
	logs *bytes.Buffer
}

func projectFixture(t *testing.T) *projectAPI {
	t.Helper()
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	srv, st, agent, _, _ := privateFixture(t, AuthAccounts, "")
	srv.s.debug = true
	uid, _ := st.DefaultOwner()
	return &projectAPI{launchAPI: &launchAPI{srv: srv, st: st, uid: uid, agent: agent, cookie: terminalLogin(t, st, uid)}, logs: &logs}
}

func (f *projectAPI) body(members ...map[string]any) map[string]any {
	if members == nil {
		members = []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/site"}, {"host": f.agent.ID, "dir": "/home/tester/api", "tool": "codex", "model": "gpt-5-codex"}}
	}
	return map[string]any{"name": "Shop", "defaultTool": "claude", "defaultModel": "sonnet", "sharedPrompt": projectSentinel, "useWorktrees": true, "members": members}
}

func (f *projectAPI) create(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", body)
	if code != 201 {
		t.Fatal("create", code, raw)
	}
	return out
}

// raw sends a request with full control over origin, cookie and token.
func (f *projectAPI) raw(t *testing.T, method, path, body, cookie, origin, token string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w.Code, w.Body.String()
}

func projectEvents(t *testing.T, st *store.Store, uid int64, action string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, e := range worktreeEvents(t, st, uid, action) {
		var d map[string]any
		if err := json.Unmarshal([]byte(e.Detail), &d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func keysOf(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestProjectCreateReadUpdateDelete(t *testing.T) {
	f := projectFixture(t)
	created := f.create(t, f.body())
	id, _ := created["id"].(string)
	if len(id) != 32 || created["name"] != "Shop" || created["defaultTool"] != "claude" || created["defaultModel"] != "sonnet" ||
		created["useWorktrees"] != true || created["hasSharedPrompt"] != true {
		t.Fatal("created", created)
	}
	if _, ok := created["sharedPrompt"]; ok {
		t.Fatal("create answered with the shared prompt")
	}
	want := []string{"createdAt", "defaultModel", "defaultTool", "hasSharedPrompt", "id", "members", "name", "updatedAt", "useWorktrees"}
	if got := keysOf(created); !reflect.DeepEqual(got, want) {
		t.Fatal("project keys", got)
	}
	members := created["members"].([]any)
	second := members[1].(map[string]any)
	if len(members) != 2 || second["host"] != f.agent.ID || second["dir"] != "/home/tester/api" || second["tool"] != "codex" || second["model"] != "gpt-5-codex" || second["position"] != float64(1) {
		t.Fatal("members", members)
	}

	code, list, raw := f.do(t, f.cookie, "GET", "/api/v2/projects", nil)
	if code != 200 || len(list["projects"].([]any)) != 1 || strings.Contains(raw, "sharedPrompt\"") || strings.Contains(raw, "zebra") {
		t.Fatal("list", code, raw)
	}
	code, one, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id, nil)
	if code != 200 || one["sharedPrompt"] != projectSentinel || one["hasSharedPrompt"] != true {
		t.Fatal("single read", code, raw)
	}

	// A rename keeps the prompt and the members.
	code, renamed, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"name": "Shop front"})
	if code != 200 || renamed["name"] != "Shop front" || renamed["hasSharedPrompt"] != true || len(renamed["members"].([]any)) != 2 {
		t.Fatal("rename", code, raw)
	}
	// Members, when sent, replace the whole list.
	code, replaced, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"members": []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/docs/"}}, "sharedPrompt": "", "useWorktrees": false})
	if code != 200 || replaced["hasSharedPrompt"] != false || replaced["useWorktrees"] != false {
		t.Fatal("replace", code, raw)
	}
	if ms := replaced["members"].([]any); len(ms) != 1 || ms[0].(map[string]any)["dir"] != "/home/tester/docs" {
		t.Fatal("replaced members", ms)
	}
	saved, err := f.st.ProjectFor(f.uid, id)
	if err != nil || saved.Name != "Shop front" || saved.SharedPrompt != "" || len(saved.Members) != 1 {
		t.Fatal("stored after update", saved, err)
	}

	code, deleted, raw := f.do(t, f.cookie, "DELETE", "/api/v2/projects/"+id, nil)
	if code != 200 || deleted["ok"] != true || deleted["worktrees"] != false || deleted["message"] != "Project deleted. Its runs are kept." {
		t.Fatal("delete", code, raw)
	}
	if code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id, nil); code != 404 || out["code"] != "not_found" || out["error"] != "That Project isn't on this account." {
		t.Fatal("deleted project still readable", code, raw)
	}
	if code, list, _ := f.do(t, f.cookie, "GET", "/api/v2/projects", nil); code != 200 || len(list["projects"].([]any)) != 0 {
		t.Fatal("list after delete", list)
	}
}

func TestProjectNewProjectMakesWorktreesUnlessToldOtherwise(t *testing.T) {
	f := projectFixture(t)
	body := f.body()
	delete(body, "useWorktrees")
	if out := f.create(t, body); out["useWorktrees"] != true {
		t.Fatal("default", out)
	}
}

func TestProjectValidationAnswersWithCodesAndSentences(t *testing.T) {
	f := projectFixture(t)
	long := "/" + strings.Repeat("a", 4096)
	cases := []struct {
		name     string
		mutate   func(map[string]any)
		status   int
		code     string
		field    string
		sentence string
	}{
		{"empty name", func(b map[string]any) { b["name"] = "  " }, 422, "invalid_name", "name", "Give the Project a name of 1 to 80 characters, without line breaks or control characters."},
		{"no name", func(b map[string]any) { delete(b, "name") }, 422, "invalid_name", "name", ""},
		{"long name", func(b map[string]any) { b["name"] = strings.Repeat("é", 81) }, 422, "invalid_name", "name", ""},
		{"line break in name", func(b map[string]any) { b["name"] = "Shop\nfront" }, 422, "invalid_name", "name", ""},
		{"no default tool", func(b map[string]any) { delete(b, "defaultTool") }, 422, "invalid_tool", "defaultTool", "Choose one of the agents reet can start."},
		{"unknown tool", func(b map[string]any) { b["defaultTool"] = "bash" }, 422, "invalid_tool", "defaultTool", ""},
		{"bad model", func(b map[string]any) { b["defaultModel"] = "-rm" }, 422, "invalid_model", "defaultModel", ""},
		{"prompt starting with a dash", func(b map[string]any) { b["sharedPrompt"] = "--yolo now" }, 422, "invalid_prompt", "sharedPrompt", "The shared prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash."},
		{"prompt too long", func(b map[string]any) { b["sharedPrompt"] = strings.Repeat("word ", 401) }, 422, "invalid_prompt", "sharedPrompt", ""},
		{"prompt with a control character", func(b map[string]any) { b["sharedPrompt"] = "two\twords" }, 422, "invalid_prompt", "sharedPrompt", ""},
		{"one-word prompt", func(b map[string]any) { b["sharedPrompt"] = "review" }, 422, "prompt_unsupported", "sharedPrompt", "Write the opening prompt as at least two words."},
		{"one-word prompt without members", func(b map[string]any) { b["sharedPrompt"] = "review"; b["members"] = []any{} }, 422, "prompt_unsupported", "sharedPrompt", ""},
		{"relative folder", func(b map[string]any) {
			b["members"] = []map[string]any{{"host": f.agent.ID, "dir": "site"}}
		}, 422, "invalid_dir", "members.0.dir", "Repository 1: the folder must be an absolute path starting with /, at most 4096 bytes, without control characters."},
		{"long folder", func(b map[string]any) { b["members"] = []map[string]any{{"host": f.agent.ID, "dir": long}} }, 422, "invalid_dir", "members.0.dir", ""},
		{"member tool", func(b map[string]any) {
			b["members"] = []map[string]any{{"host": f.agent.ID, "dir": "/a"}, {"host": f.agent.ID, "dir": "/b", "tool": "vim"}}
		}, 422, "invalid_tool", "members.1.tool", "Repository 2: choose one of the agents reet can start."},
		{"member model", func(b map[string]any) {
			b["members"] = []map[string]any{{"host": f.agent.ID, "dir": "/a", "model": "has space"}}
		}, 422, "invalid_model", "members.0.model", ""},
		{"member without machine", func(b map[string]any) { b["members"] = []map[string]any{{"dir": "/a"}} }, 422, "unknown_machine", "members.0.host", "Repository 1: that machine isn't enrolled on this account."},
		{"machine by name, not id", func(b map[string]any) { b["members"] = []map[string]any{{"host": f.agent.Host, "dir": "/a"}} }, 422, "unknown_machine", "members.0.host", ""},
		{"duplicate member", func(b map[string]any) {
			b["members"] = []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/site"}, {"host": f.agent.ID, "dir": "/home/tester/./site/"}}
		}, 422, "duplicate_member", "members", "Each machine and folder can be in a Project only once."},
	}
	for _, c := range cases {
		body := f.body()
		c.mutate(body)
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", body)
		if code != c.status || out["code"] != c.code || out["field"] != c.field || out["error"] != out["message"] || c.sentence != "" && out["error"] != c.sentence {
			t.Fatal(c.name, code, raw)
		}
		if strings.Contains(raw, "zebra") {
			t.Fatal(c.name, "echoed the prompt", raw)
		}
	}
	if projects, _ := f.st.ListProjectsFor(f.uid); len(projects) != 0 {
		t.Fatal("a refused Project was saved", projects)
	}
	if len(projectEvents(t, f.st, f.uid, "project-create")) != 0 {
		t.Fatal("a refused Project was audited")
	}
	// A tool the machine has not reported is still saved: readiness is judged at Start.
	body := f.body(map[string]any{"host": f.agent.ID, "dir": "/a", "tool": "gemini"})
	f.create(t, body)
}

func TestProjectNameIsUniquePerAccountIgnoringCase(t *testing.T) {
	f := projectFixture(t)
	f.create(t, f.body())
	body := f.body()
	body["name"] = "  shop "
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", body)
	if code != 409 || out["code"] != "name_taken" || out["field"] != "name" || out["error"] != "Another Project on this account already has that name." {
		t.Fatal(code, raw)
	}
}

func TestProjectLimits(t *testing.T) {
	f := projectFixture(t)
	members := []map[string]any{}
	for i := 0; i < 13; i++ {
		members = append(members, map[string]any{"host": f.agent.ID, "dir": fmt.Sprintf("/home/tester/r%d", i)})
	}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", f.body(members...))
	if code != 422 || out["code"] != "member_limit" || out["error"] != "A Project can have at most 12 repositories." {
		t.Fatal("13 members", code, raw)
	}
	created := f.create(t, f.body(members[:12]...))
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+created["id"].(string), map[string]any{"members": members})
	if code != 422 || out["code"] != "member_limit" {
		t.Fatal("13 members by update", code, raw)
	}
	for i := 1; i < store.MaxProjectsPerAccount; i++ {
		if _, err := f.st.CreateProjectFor(f.uid, store.Project{Name: fmt.Sprintf("P%d", i), DefaultTool: "claude"}); err != nil {
			t.Fatal(err)
		}
	}
	body := f.body()
	body["name"] = "One too many"
	code, out, raw = f.do(t, f.cookie, "POST", "/api/v2/projects", body)
	if code != 409 || out["code"] != "project_limit" || out["error"] != "This account already has 50 Projects, the most it can have. Delete one first." {
		t.Fatal("51st project", code, raw)
	}
}

func TestProjectsAreScopedToTheirOwner(t *testing.T) {
	f := projectFixture(t)
	a := f.create(t, f.body())
	aID := a["id"].(string)
	run, err := f.st.CreateRunFor(f.uid, aID)
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.st.CreateUser("projects-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	otherAgent, _, err := f.st.EnrollSyncAgent(other.ID, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	b := terminalLogin(t, f.st, other.ID)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/v2/projects/" + aID},
		{"PATCH", "/api/v2/projects/" + aID},
		{"DELETE", "/api/v2/projects/" + aID},
		{"GET", "/api/v2/projects/" + aID + "/runs"},
		{"GET", "/api/v2/projects/runs/" + run.ID},
	} {
		var body any
		if c.method == "PATCH" {
			body = map[string]any{"name": "Taken over", "members": []map[string]any{{"host": otherAgent.ID, "dir": "/x"}}}
		}
		code, out, raw := f.do(t, b, c.method, c.path, body)
		if code != 404 || (out["code"] != "not_found" && out["code"] != "run_not_found") || strings.Contains(raw, "Shop") {
			t.Fatal("another account reached", c.method, c.path, code, raw)
		}
	}
	if code, list, raw := f.do(t, b, "GET", "/api/v2/projects", nil); code != 200 || len(list["projects"].([]any)) != 0 {
		t.Fatal("another account listed", raw)
	}
	after, err := f.st.ProjectFor(f.uid, aID)
	if err != nil || after.Name != "Shop" || len(after.Members) != 2 || after.SharedPrompt != projectSentinel {
		t.Fatal("another account changed the Project", after, err)
	}
	if _, err := f.st.RunFor(f.uid, run.ID); err != nil {
		t.Fatal(err)
	}
	// A machine of another account is refused, on create and on update.
	body := f.body(map[string]any{"host": f.agent.ID, "dir": "/x"})
	code, out, raw := f.do(t, b, "POST", "/api/v2/projects", body)
	if code != 422 || out["code"] != "unknown_machine" {
		t.Fatal("another account's machine accepted", code, raw)
	}
	bProject, err := f.st.CreateProjectFor(other.ID, store.Project{Name: "Mine", DefaultTool: "claude", Members: []store.ProjectMember{{Host: otherAgent.ID, Dir: "/z"}}})
	if err != nil {
		t.Fatal(err)
	}
	code, out, raw = f.do(t, b, "PATCH", "/api/v2/projects/"+bProject.ID, map[string]any{"members": []map[string]any{{"host": otherAgent.ID, "dir": "/z"}, {"host": f.agent.ID, "dir": "/z"}}})
	if code != 422 || out["code"] != "unknown_machine" || out["field"] != "members.1.host" {
		t.Fatal("another account's machine accepted on update", code, raw)
	}
	if code, _, raw := f.do(t, b, "PATCH", "/api/v2/projects/"+bProject.ID, map[string]any{"members": []map[string]any{{"host": otherAgent.ID, "dir": "/w"}}}); code != 200 {
		t.Fatal("own machine refused", code, raw)
	}
}

func TestProjectKeepsAForgottenMachineButAcceptsNoNewUnknownOne(t *testing.T) {
	f := projectFixture(t)
	spare, _, err := f.st.EnrollSyncAgent(f.uid, "spare", "test")
	if err != nil {
		t.Fatal(err)
	}
	created := f.create(t, f.body(map[string]any{"host": spare.ID, "dir": "/a"}))
	if err := f.st.ControlSyncAgent(f.uid, spare.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if agents, _ := f.st.SyncAgentsFor(f.uid); len(agents) != 1 {
		t.Fatal("machine not forgotten", agents)
	}
	id := created["id"].(string)
	// The edit form sends the saved members back unchanged.
	members := []map[string]any{{"host": spare.ID, "dir": "/a"}, {"host": f.agent.ID, "dir": "/b"}}
	if code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"members": members}); code != 200 {
		t.Fatal("saved machine refused", code, raw)
	}
	body := f.body(map[string]any{"host": spare.ID, "dir": "/a"})
	body["name"] = "Elsewhere"
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects", body); code != 422 || out["code"] != "unknown_machine" {
		t.Fatal("forgotten machine accepted in a new Project", code, raw)
	}
}

func TestProjectRoutesRefuseViewersLegacyAuthTokensAndSignedOut(t *testing.T) {
	f := projectFixture(t)
	created := f.create(t, f.body())
	id := created["id"].(string)
	run, err := f.st.CreateRunFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := f.st.CreateUser("projects-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, f.st, viewer.ID)
	routes := []struct{ method, path, body string }{
		{"GET", "/api/v2/projects", ""},
		{"POST", "/api/v2/projects", `{"name":"Viewer","defaultTool":"claude"}`},
		{"GET", "/api/v2/projects/" + id, ""},
		{"PATCH", "/api/v2/projects/" + id, `{"name":"Viewer"}`},
		{"DELETE", "/api/v2/projects/" + id, ""},
		{"GET", "/api/v2/projects/" + id + "/runs", ""},
		{"GET", "/api/v2/projects/runs/" + run.ID, ""},
	}
	origin := "https://reet.example"
	for _, rt := range routes {
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, viewerCookie, origin, ""); code != 403 || !strings.Contains(raw, "read_only") {
			t.Fatal("viewer", rt.method, rt.path, code, raw)
		}
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, "", origin, "fixture-owner"); code != 403 || !strings.Contains(raw, "accounts_required") {
			t.Fatal("API token", rt.method, rt.path, code, raw)
		}
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, f.cookie, origin, "fixture-owner"); code != 403 {
			t.Fatal("API token beside a cookie", rt.method, rt.path, code, raw)
		}
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, "", origin, ""); code != 401 {
			t.Fatal("signed out", rt.method, rt.path, code, raw)
		}
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode = mode
		f.srv.s.token = "single-token"
		for _, rt := range routes {
			for _, token := range []string{"", "single-token"} {
				if code, raw := f.raw(t, rt.method, rt.path, rt.body, f.cookie, origin, token); code != 403 && code != 401 {
					t.Fatal("legacy auth", mode, rt.method, rt.path, code, raw)
				}
			}
		}
	}
	f.srv.s.authMode, f.srv.s.token = AuthAccounts, ""
	after, err := f.st.ProjectFor(f.uid, id)
	if err != nil || after.Name != "Shop" {
		t.Fatal("a refused caller changed the Project", after, err)
	}
	if projects, _ := f.st.ListProjectsFor(f.uid); len(projects) != 1 {
		t.Fatal("a refused caller created a Project", projects)
	}
	if projects, _ := f.st.ListProjectsFor(viewer.ID); len(projects) != 0 {
		t.Fatal("a viewer created a Project", projects)
	}
}

func TestProjectChangesNeedTheCanonicalOrigin(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	for _, origin := range []string{"", "https://evil.example", "null", "https://reet.example.evil", "http://reet.example"} {
		for _, rt := range []struct{ method, path, body string }{
			{"POST", "/api/v2/projects", `{"name":"Other","defaultTool":"claude"}`},
			{"PATCH", "/api/v2/projects/" + id, `{"name":"Renamed"}`},
			{"DELETE", "/api/v2/projects/" + id, ""},
		} {
			if code, raw := f.raw(t, rt.method, rt.path, rt.body, f.cookie, origin, ""); code != 403 || !strings.Contains(raw, "origin_refused") {
				t.Fatal("origin", origin, rt.method, code, raw)
			}
		}
	}
	if projects, _ := f.st.ListProjectsFor(f.uid); len(projects) != 1 || projects[0].Name != "Shop" {
		t.Fatal("a request from another origin changed Projects", projects)
	}
	// Reads need no Origin header.
	if code, raw := f.raw(t, "GET", "/api/v2/projects/"+id, "", f.cookie, "", ""); code != 200 {
		t.Fatal("read without origin", code, raw)
	}
}

func TestProjectRequestsAreStrictJSONWithASizeLimit(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	good, _ := json.Marshal(map[string]any{"name": "Strict", "defaultTool": "claude"})
	for name, body := range map[string]string{
		"unknown field":         `{"name":"Strict","defaultTool":"claude","prompt":"two words"}`,
		"member position":       `{"name":"Strict","defaultTool":"claude","members":[{"host":"` + f.agent.ID + `","dir":"/a","position":0}]}`,
		"member unknown field":  `{"name":"Strict","defaultTool":"claude","members":[{"host":"` + f.agent.ID + `","dir":"/a","branch":"main"}]}`,
		"two values":            string(good) + string(good),
		"trailing garbage":      string(good) + "x",
		"wrong type":            `{"name":1,"defaultTool":"claude"}`,
		"not an object":         `["Strict"]`,
		"not JSON":              `name=Strict`,
		"empty":                 ``,
		"read-only id":          `{"id":"abc","name":"Strict","defaultTool":"claude"}`,
		"read-only flag":        `{"hasSharedPrompt":true,"name":"Strict","defaultTool":"claude"}`,
		"useWorktrees a string": `{"name":"Strict","defaultTool":"claude","useWorktrees":"yes"}`,
	} {
		for _, rt := range []struct{ method, path string }{{"POST", "/api/v2/projects"}, {"PATCH", "/api/v2/projects/" + id}} {
			code, out, raw := f.do(t, f.cookie, rt.method, rt.path, body)
			if code != 400 || out["code"] != "invalid_request" || out["error"] != "That request isn't a Project reet can read." {
				t.Fatal(name, rt.method, code, raw)
			}
		}
	}
	huge := `{"name":"Strict","defaultTool":"claude","sharedPrompt":"` + strings.Repeat("a", 130<<10) + `"}`
	for _, rt := range []struct{ method, path string }{{"POST", "/api/v2/projects"}, {"PATCH", "/api/v2/projects/" + id}} {
		code, out, raw := f.do(t, f.cookie, rt.method, rt.path, huge)
		if code != 413 || out["code"] != "too_large" {
			t.Fatal("size limit", rt.method, code, raw[:min(len(raw), 200)])
		}
	}
	// The largest real Project fits: twelve longest folders and a full prompt.
	members := []map[string]any{}
	for i := 0; i < 12; i++ {
		members = append(members, map[string]any{"host": f.agent.ID, "dir": fmt.Sprintf("/%02d", i) + strings.Repeat("d", 4093), "model": strings.Repeat("m", 100)})
	}
	body := f.body(members...)
	body["name"] = strings.Repeat("n", 80)
	body["sharedPrompt"] = strings.Repeat("ab ", 666) + "ab"
	f.create(t, body)
	if projects, _ := f.st.ListProjectsFor(f.uid); len(projects) != 2 {
		t.Fatal("a refused request saved something", len(projects))
	}
}

func TestProjectSharedPromptOnlyInTheSingleRead(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	if code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"name": "Shop two", "sharedPrompt": projectSentinel + " again"}); code != 200 || strings.Contains(raw, "zebra") {
		t.Fatal("update", code, raw)
	}
	run, err := f.st.CreateRunFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if !run.HasPrompt() {
		t.Fatal("run did not snapshot the prompt")
	}
	reads := map[string]string{}
	for _, path := range []string{"/api/v2/projects", "/api/v2/projects/" + id + "/runs", "/api/v2/projects/runs/" + run.ID, "/api/events?limit=100"} {
		code, _, raw := f.do(t, f.cookie, "GET", path, nil)
		if code != 200 {
			t.Fatal(path, code, raw)
		}
		reads[path] = raw
	}
	// Delete last, so its answer and audit row are checked too.
	_, _, deleteRaw := f.do(t, f.cookie, "DELETE", "/api/v2/projects/"+id, nil)
	_, _, events := f.do(t, f.cookie, "GET", "/api/events?limit=100", nil)
	reads["delete"], reads["events after delete"] = deleteRaw, events
	all, err := f.st.ListEventsFor(f.uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := json.Marshal(all)
	reads["audit rows"], reads["logs"] = string(rows), f.logs.String()
	for where, text := range reads {
		if strings.Contains(text, "zebra") || strings.Contains(text, "sharedPrompt\"") || strings.Contains(text, `"prompt"`) {
			t.Fatal("the shared prompt reached", where, text)
		}
	}
	for _, action := range []string{"project-create", "project-update", "project-delete"} {
		if !strings.Contains(reads["events after delete"], action) {
			t.Fatal("Activity does not list", action)
		}
	}
}

func TestProjectAuditRows(t *testing.T) {
	f := projectFixture(t)
	created := f.create(t, f.body())
	id := created["id"].(string)
	f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"name": "Shop front", "members": []map[string]any{{"host": f.agent.ID, "dir": "/a"}}})
	f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"useWorktrees": false})
	f.do(t, f.cookie, "DELETE", "/api/v2/projects/"+id, nil)
	createRows := projectEvents(t, f.st, f.uid, "project-create")
	updateRows := projectEvents(t, f.st, f.uid, "project-update")
	deleteRows := projectEvents(t, f.st, f.uid, "project-delete")
	if len(createRows) != 1 || !reflect.DeepEqual(createRows[0], map[string]any{"project": id, "name": "Shop", "members": float64(2)}) {
		t.Fatal("create row", createRows)
	}
	// Newest first.
	if len(updateRows) != 2 || !reflect.DeepEqual(updateRows[1], map[string]any{"project": id, "name": "Shop front", "members": float64(1), "previousName": "Shop"}) ||
		!reflect.DeepEqual(updateRows[0], map[string]any{"project": id, "name": "Shop front", "members": float64(1)}) {
		t.Fatal("update rows", updateRows)
	}
	if len(deleteRows) != 1 || !reflect.DeepEqual(deleteRows[0], map[string]any{"project": id, "name": "Shop front", "members": float64(1)}) {
		t.Fatal("delete row", deleteRows)
	}
	for _, e := range worktreeEvents(t, f.st, f.uid, "project-delete") {
		if e.Source != "ui" {
			t.Fatal("source", e.Source)
		}
	}
	// A refused change is not audited.
	f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"name": "Gone"})
	if len(projectEvents(t, f.st, f.uid, "project-update")) != 2 {
		t.Fatal("a refused update was audited")
	}
}

func TestProjectFailedUpdateLeavesTheProjectIntact(t *testing.T) {
	f := projectFixture(t)
	first := f.create(t, f.body())
	second := f.body(map[string]any{"host": f.agent.ID, "dir": "/other"})
	second["name"] = "Second"
	id := f.create(t, second)["id"].(string)
	before, err := f.st.ProjectFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	for name, patch := range map[string]map[string]any{
		// The store replaces members, then the rename hits the unique name: one transaction.
		"name taken after member replace": {"name": first["name"], "members": []map[string]any{{"host": f.agent.ID, "dir": "/new1"}, {"host": f.agent.ID, "dir": "/new2"}}},
		"duplicate members":               {"members": []map[string]any{{"host": f.agent.ID, "dir": "/new1"}, {"host": f.agent.ID, "dir": "/new1/"}}},
		"one bad member":                  {"members": []map[string]any{{"host": f.agent.ID, "dir": "/new1"}, {"host": "nope", "dir": "/new2"}}},
		"one-word prompt":                 {"sharedPrompt": "review", "members": []map[string]any{{"host": f.agent.ID, "dir": "/new1"}}},
	} {
		code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, patch)
		if code < 400 {
			t.Fatal(name, code, raw)
		}
		after, err := f.st.ProjectFor(f.uid, id)
		if err != nil || !reflect.DeepEqual(after, before) {
			t.Fatal(name, "changed the Project", after, before, err)
		}
	}
}

func TestProjectDeleteKeepsRunsReadable(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	run, err := f.st.CreateRunFor(f.uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.RunMemberStart(f.uid, run.ID, 0); err != nil {
		t.Fatal(err)
	}
	path := "/home/tester/.reet/worktrees/shop/site/" + worktreeTestRun
	if err := f.st.SetRunMemberState(f.uid, run.ID, 0, store.RunMemberStarted, "", "session-1", path, "site", "reet/shop-"+worktreeTestRun, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.st.RunMemberStart(f.uid, run.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := f.st.SetRunMemberState(f.uid, run.ID, 1, store.RunMemberFailed, "tool_not_installed", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.do(t, f.cookie, "DELETE", "/api/v2/projects/"+id, nil)
	if code != 200 || out["worktrees"] != true || out["message"] != "Project deleted. Its runs are kept, and the worktrees they made stay on their machines." {
		t.Fatal("delete", code, raw)
	}
	code, got, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/runs/"+run.ID, nil)
	if code != 200 || got["projectName"] != "Shop" || got["projectId"] != "" || got["state"] != "partial" || got["id"] != run.ID {
		t.Fatal("run after delete", code, raw)
	}
	members := got["members"].([]any)
	started, failed := members[0].(map[string]any), members[1].(map[string]any)
	if started["state"] != "started" || started["sessionId"] != "session-1" || started["worktreePath"] != path || started["worktreeRepo"] != "site" ||
		started["worktreeBranch"] != "reet/shop-"+worktreeTestRun || started["sentence"] != "" || started["attempt"] != float64(1) || started["tool"] != "claude" || started["model"] != "sonnet" {
		t.Fatal("started member", started)
	}
	if failed["state"] != "failed" || failed["code"] != "tool_not_installed" || failed["sentence"] != "That agent isn't installed on that machine." || failed["tool"] != "codex" || failed["model"] != "gpt-5-codex" {
		t.Fatal("failed member", failed)
	}
	wantRun := []string{"id", "members", "projectId", "projectName", "startedAt", "state", "useWorktrees"}
	if keys := keysOf(got); !reflect.DeepEqual(keys, wantRun) {
		t.Fatal("run keys", keys)
	}
	wantMember := []string{"attempt", "code", "dir", "host", "model", "position", "sentence", "sessionId", "state", "tool", "worktreeBranch", "worktreePath", "worktreeRepo"}
	if keys := keysOf(started); !reflect.DeepEqual(keys, wantMember) {
		t.Fatal("run member keys", keys)
	}
	// The deleted Project's runs list is gone with it; the run itself stays.
	if code, out, _ := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id+"/runs", nil); code != 404 || out["code"] != "not_found" {
		t.Fatal("runs of a deleted Project", code)
	}
}

func TestProjectRunsListTheNewestTwenty(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	var ids []string
	for i := 0; i < 21; i++ {
		run, err := f.st.CreateRunFor(f.uid, id, 1)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, run.ID)
	}
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+id+"/runs", nil)
	if code != 200 {
		t.Fatal(code, raw)
	}
	runs := out["runs"].([]any)
	if len(runs) != 20 || runs[0].(map[string]any)["id"] != ids[20] || runs[19].(map[string]any)["id"] != ids[1] {
		t.Fatal("newest twenty", len(runs))
	}
	r := runs[0].(map[string]any)
	if r["state"] != "running" || r["projectName"] != "Shop" || len(r["members"].([]any)) != 1 || r["members"].([]any)[0].(map[string]any)["position"] != float64(1) {
		t.Fatal("run", r)
	}
	code, list, raw := f.do(t, f.cookie, "GET", "/api/v2/projects", nil)
	last := list["projects"].([]any)[0].(map[string]any)["lastRun"].(map[string]any)
	if code != 200 || last["id"] != ids[20] || last["state"] != "running" {
		t.Fatal("last run", raw)
	}
	for _, path := range []string{"/api/v2/projects/" + id + "/other", "/api/v2/projects/runs/" + strings.Repeat("0", 32), "/api/v2/projects/runs/runs", "/api/v2/projects/" + strings.Repeat("0", 32) + "/runs"} {
		if code, _, raw := f.do(t, f.cookie, "GET", path, nil); code != 404 {
			t.Fatal(path, code, raw)
		}
	}
}

func TestProjectStoreErrorsHaveStableCodes(t *testing.T) {
	for err, code := range map[error]string{
		store.ErrProjectNotFound:     "not_found",
		store.ErrProjectNameTaken:    "name_taken",
		store.ErrProjectName:         "invalid_name",
		store.ErrProjectLimit:        "project_limit",
		store.ErrMemberLimit:         "member_limit",
		store.ErrDuplicateMember:     "duplicate_member",
		store.ErrProjectInvalid:      "invalid_project",
		store.ErrRunNotFound:         "run_not_found",
		store.ErrRunNoMembers:        "no_members",
		store.ErrRunMemberNotPending: "member_not_pending",
		store.ErrRunMemberTransition: "member_state",
		store.ErrRunMemberStale:      "member_stale",
		fmt.Errorf("disk on fire"):   "server_error",
	} {
		r := storeRefusal(fmt.Errorf("wrapped: %w", err))
		if r.code != code || r.sentence == "" || strings.Contains(strings.ToLower(r.sentence), "verified") || strings.Contains(strings.ToLower(r.sentence), "healthy") {
			t.Fatal(err, r)
		}
	}
}
