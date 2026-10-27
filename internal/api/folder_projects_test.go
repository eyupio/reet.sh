package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

// folderSentinel is a starter prompt that must never leave the single read:
// not in lists, write answers, refusals, audit, Activity or logs.
const folderSentinel = "okapi-sentinel starter prompt for this folder"

const folderChangedSentence = "Someone changed this project. Reload it, then try again."

// fpResult is one folder-project request's answer.
type fpResult struct {
	code   int
	out    map[string]any
	raw    string
	header http.Header
}

// fpDo sends a request as cookie from the canonical origin, with If-Match
// when version is not empty.
func (f *projectAPI) fpDo(t *testing.T, cookie, method, path string, body any, version string) fpResult {
	t.Helper()
	var data string
	switch b := body.(type) {
	case nil:
	case string:
		data = b
	default:
		raw, _ := json.Marshal(b)
		data = string(raw)
	}
	r := httptest.NewRequest(method, path, strings.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://reet.example")
	if version != "" {
		r.Header.Set("If-Match", version)
	}
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	}
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return fpResult{w.Code, out, w.Body.String(), w.Header()}
}

func (f *projectAPI) fpBody(over map[string]any) map[string]any {
	body := map[string]any{"host": f.agent.ID, "dir": "/home/tester/site", "name": "Site", "isRepo": true, "baseBranch": "main",
		"worktreeDefault": "yes", "defaultTool": "claude", "defaultModel": "sonnet", "starterPrompt": folderSentinel}
	for k, v := range over {
		if v == nil {
			delete(body, k)
		} else {
			body[k] = v
		}
	}
	return body
}

func (f *projectAPI) fpCreate(t *testing.T, over map[string]any) map[string]any {
	t.Helper()
	res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(over), "")
	if res.code != 201 {
		t.Fatal("create", res.code, res.raw)
	}
	return res.out
}

// resetFolderWrites gives the account a fresh write allowance, so a test that
// is not about the rate limit can make more than 30 writes.
func (f *projectAPI) resetFolderWrites() {
	f.srv.s.folderWrites = auth.NewLimiter(folderProjectWriteLimit, time.Minute)
}

func folderRows(t *testing.T, f *projectAPI, uid int64, action string) []map[string]any {
	t.Helper()
	return projectEvents(t, f.st, uid, action)
}

func TestFolderProjectCreateReadUpdateDelete(t *testing.T) {
	f := projectFixture(t)
	created := f.fpCreate(t, nil)
	id, _ := created["id"].(string)
	want := []string{"baseBranch", "createdAt", "defaultModel", "defaultTool", "dir", "hasStarterPrompt", "host", "id", "isRepo", "machine", "name", "position", "updatedAt", "version", "worktreeDefault"}
	if got := keysOf(created); !reflect.DeepEqual(got, want) {
		t.Fatal("project keys", got)
	}
	if id == "" || created["host"] != f.agent.ID || created["machine"] != "laptop" || created["dir"] != "/home/tester/site" || created["name"] != "Site" || created["isRepo"] != true ||
		created["baseBranch"] != "main" || created["worktreeDefault"] != "yes" || created["defaultTool"] != "claude" || created["defaultModel"] != "sonnet" ||
		created["hasStarterPrompt"] != true || created["version"] != float64(1) || created["position"] != float64(0) {
		t.Fatal("created", created)
	}
	list := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, "")
	if list.code != 200 || len(list.out["projects"].([]any)) != 1 || strings.Contains(list.raw, "okapi") || strings.Contains(list.raw, `"starterPrompt"`) {
		t.Fatal("list", list.code, list.raw)
	}
	one := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects/"+id, nil, "")
	if one.code != 200 || one.out["starterPrompt"] != folderSentinel || one.header.Get("ETag") != `"1"` {
		t.Fatal("single read", one.code, one.header, one.raw)
	}
	// Absent keeps a field; "" clears a text field.
	res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"name": "Site front"}, `"1"`)
	if res.code != 200 || res.out["name"] != "Site front" || res.out["hasStarterPrompt"] != true || res.out["baseBranch"] != "main" || res.header.Get("ETag") != `"2"` {
		t.Fatal("rename", res.code, res.raw)
	}
	res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"starterPrompt": "", "baseBranch": "", "worktreeDefault": "inherit", "dir": "/home/tester/site/web/"}, `"2"`)
	if res.code != 200 || res.out["hasStarterPrompt"] != false || res.out["baseBranch"] != "" || res.out["worktreeDefault"] != "inherit" || res.out["dir"] != "/home/tester/site/web" {
		t.Fatal("clear", res.code, res.raw)
	}
	saved, err := f.st.FolderProjectFor(f.uid, id)
	if err != nil || saved.StarterPrompt != "" || saved.Name != "Site front" || saved.Version != 3 {
		t.Fatal("stored", saved, err)
	}
	res = f.fpDo(t, f.cookie, "DELETE", "/api/v2/folder-projects/"+id, nil, `"3"`)
	if res.code != 200 || res.out["message"] != "Project removed. Its sessions and any worktrees stay on laptop." {
		t.Fatal("delete", res.code, res.raw)
	}
	if res := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects/"+id, nil, ""); res.code != 404 || res.out["code"] != "project_not_found" || res.out["error"] != "That project isn't on this account." {
		t.Fatal("deleted project still readable", res.code, res.raw)
	}
}

func TestFolderProjectRoutesAccess(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	viewer, err := f.st.CreateUser("folder-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, f.st, viewer.ID)
	personal := "folder-personal-token"
	if _, err := f.st.CreateAPIToken(f.uid, "test", auth.HashToken(personal)); err != nil {
		t.Fatal(err)
	}
	add := `{"host":"` + f.agent.ID + `","dir":"/home/tester/other"}`
	routes := []struct{ method, path, body string }{
		{"GET", "/api/v2/folder-projects", ""},
		{"POST", "/api/v2/folder-projects", add},
		{"GET", "/api/v2/folder-projects/" + id, ""},
		{"PATCH", "/api/v2/folder-projects/" + id, `{"name":"Taken"}`},
		{"DELETE", "/api/v2/folder-projects/" + id, ""},
		{"POST", "/api/v2/folder-projects/reorder", `{"ids":["` + id + `"],"versions":[1]}`},
	}
	origin := "https://reet.example"
	for _, rt := range routes {
		f.resetFolderWrites()
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, viewerCookie, origin, ""); code != 403 || !strings.Contains(raw, "read_only") || !strings.Contains(raw, "This account is read-only, so it can't use projects.") {
			t.Fatal("viewer", rt.method, rt.path, code, raw)
		}
		for _, token := range []string{personal, "fixture-owner"} {
			if code, raw := f.raw(t, rt.method, rt.path, rt.body, "", origin, token); code != 403 || !strings.Contains(raw, "accounts_required") || !strings.Contains(raw, "Using projects needs an account signed in through the browser.") {
				t.Fatal("API token", rt.method, rt.path, code, raw)
			}
			if code, raw := f.raw(t, rt.method, rt.path, rt.body, f.cookie, origin, token); code != 403 {
				t.Fatal("API token beside a cookie", rt.method, rt.path, code, raw)
			}
		}
		// An MCP credential never reaches the routes (the auth layer answers
		// first), alone or beside a cookie.
		for _, cookie := range []string{"", f.cookie} {
			if code, raw := f.raw(t, rt.method, rt.path, rt.body, cookie, origin, "reet_mcp_0123456789abcdef"); code != 401 && code != 403 {
				t.Fatal("MCP token", rt.method, rt.path, code, raw)
			}
		}
		// The token in its other header is refused too.
		r := httptest.NewRequest(rt.method, rt.path, strings.NewReader(rt.body))
		r.Header.Set("Origin", origin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Reet-Token", personal)
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != 403 || !strings.Contains(w.Body.String(), "accounts_required") {
			t.Fatal("X-Reet-Token", rt.method, rt.path, w.Code, w.Body.String())
		}
		if code, raw := f.raw(t, rt.method, rt.path, rt.body, "", origin, ""); code != 401 {
			t.Fatal("signed out", rt.method, rt.path, code, raw)
		}
		if rt.method != "GET" {
			for _, bad := range []string{"", "https://evil.example", "null", "http://reet.example"} {
				if code, raw := f.raw(t, rt.method, rt.path, rt.body, f.cookie, bad, ""); code != 403 || !strings.Contains(raw, "origin_refused") {
					t.Fatal("origin", bad, rt.method, rt.path, code, raw)
				}
			}
		} else if code, raw := f.raw(t, rt.method, rt.path, "", f.cookie, "", ""); code != 200 {
			t.Fatal("read without origin", rt.path, code, raw)
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
	projects, _ := f.st.ListFolderProjectsFor(f.uid)
	if len(projects) != 1 || projects[0].Name != "Site" || projects[0].Version != 1 {
		t.Fatal("a refused caller changed the projects", projects)
	}
	if theirs, _ := f.st.ListFolderProjectsFor(viewer.ID); len(theirs) != 0 {
		t.Fatal("a viewer created a project", theirs)
	}
	if rows := folderRows(t, f, f.uid, "project-update"); len(rows) != 0 {
		t.Fatal("a refused caller was audited", rows)
	}
}

func TestFolderProjectOtherAccountIsNotFound(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	other, err := f.st.CreateUser("folder-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.st.EnrollSyncAgent(other.ID, "desktop", "test"); err != nil {
		t.Fatal(err)
	}
	b := terminalLogin(t, f.st, other.ID)
	unknown := strings.Repeat("0", 32)
	for _, c := range []struct {
		method  string
		body    any
		version string
	}{
		{"GET", nil, ""},
		{"PATCH", map[string]any{"name": "Taken over"}, ""},
		{"PATCH", map[string]any{"name": "Taken over"}, `"1"`},
		{"DELETE", nil, ""},
		{"DELETE", nil, `"1"`},
	} {
		theirs := f.fpDo(t, b, c.method, "/api/v2/folder-projects/"+id, c.body, c.version)
		missing := f.fpDo(t, b, c.method, "/api/v2/folder-projects/"+unknown, c.body, c.version)
		if theirs.code != 404 || theirs.raw != missing.raw || theirs.out["code"] != "project_not_found" || strings.Contains(theirs.raw, "Site") {
			t.Fatal("another account reached", c.method, theirs.code, theirs.raw, missing.raw)
		}
	}
	theirs := f.fpDo(t, b, "POST", "/api/v2/folder-projects/reorder", map[string]any{"ids": []string{id}, "versions": []int{1}}, "")
	missing := f.fpDo(t, b, "POST", "/api/v2/folder-projects/reorder", map[string]any{"ids": []string{unknown}, "versions": []int{1}}, "")
	if theirs.code != 404 || theirs.raw != missing.raw {
		t.Fatal("another account reordered", theirs.code, theirs.raw, missing.raw)
	}
	if list := f.fpDo(t, b, "GET", "/api/v2/folder-projects", nil, ""); list.code != 200 || len(list.out["projects"].([]any)) != 0 {
		t.Fatal("another account listed", list.raw)
	}
	after, err := f.st.FolderProjectFor(f.uid, id)
	if err != nil || after.Name != "Site" || after.Version != 1 || after.StarterPrompt != folderSentinel {
		t.Fatal("another account changed the project", after, err)
	}
}

func TestFolderProjectUnknownHostRefused(t *testing.T) {
	f := projectFixture(t)
	other, _ := f.st.CreateUser("folder-host@example.test", "Other", "admin", "hash")
	theirs, _, err := f.st.EnrollSyncAgent(other.ID, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	for name, host := range map[string]any{
		"never enrolled":     "nowhere",
		"another account's":  theirs.ID,
		"the machine's name": f.agent.Host,
		"id in upper case":   strings.ToUpper(f.agent.ID),
		"blank":              "",
		"missing":            nil,
		"with a space":       " " + f.agent.ID,
		"trailing space":     f.agent.ID + " ",
	} {
		body := f.fpBody(map[string]any{"host": host})
		if host == nil {
			delete(body, "host")
		}
		res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", body, "")
		if res.code != 422 || res.out["code"] != "unknown_machine" || res.out["field"] != "host" || res.out["error"] != "That machine isn't enrolled on this account." {
			t.Fatal(name, res.code, res.raw)
		}
	}
	if out := f.fpCreate(t, nil); out["host"] != f.agent.ID || out["machine"] != "laptop" {
		t.Fatal("by id", out)
	}
	if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 1 || projects[0].Host != f.agent.ID {
		t.Fatal("a refused host was saved", projects)
	}
	// A rename keeps the id: the project follows the machine's new name.
	if err := f.st.MergeMachinesFor(f.uid, "laptop", "work-laptop"); err != nil {
		t.Fatal(err)
	}
	if list := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, ""); list.code != 200 || list.out["projects"].([]any)[0].(map[string]any)["machine"] != "work-laptop" {
		t.Fatal("after a rename", list.raw)
	}
	// An update may keep a machine that has since been forgotten, but may not
	// move the project to an unknown one or add a new folder there.
	spare, _, err := f.st.EnrollSyncAgent(f.uid, "spare", "test")
	if err != nil {
		t.Fatal(err)
	}
	created := f.fpCreate(t, map[string]any{"host": spare.ID, "dir": "/srv/a", "name": "Spare"})
	if err := f.st.ControlSyncAgent(f.uid, spare.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)
	res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"name": "Spare kept", "host": spare.ID, "dir": "/srv/a/"}, "")
	if res.code != 200 || res.out["host"] != spare.ID || res.out["machine"] != "" {
		t.Fatal("saved machine refused", res.code, res.raw)
	}
	for name, change := range map[string]map[string]any{
		"unknown machine":         {"host": "nowhere"},
		"new folder on forgotten": {"dir": "/srv/b"},
	} {
		res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, change, "")
		if res.code != 422 || res.out["code"] != "unknown_machine" || res.out["field"] != "host" {
			t.Fatal(name, res.code, res.raw)
		}
	}
	if saved, _ := f.st.FolderProjectFor(f.uid, id); saved.Host != spare.ID || saved.Dir != "/srv/a" {
		t.Fatal("a refused update changed the project", saved)
	}
	// The forgotten machine has no name to show.
	res = f.fpDo(t, f.cookie, "DELETE", "/api/v2/folder-projects/"+id, nil, "")
	if res.code != 200 || res.out["message"] != "Project removed. Its sessions and any worktrees stay on that machine." {
		t.Fatal("delete on a forgotten machine", res.code, res.raw)
	}
	if rows := folderRows(t, f, f.uid, "project-remove"); len(rows) != 1 || rows[0]["machine"] != spare.ID || rows[0]["host"] != "" {
		t.Fatal("remove row", rows)
	}
}

func TestFolderProjectCreateValidatesAndAudits(t *testing.T) {
	f := projectFixture(t)
	cases := []struct {
		name     string
		over     map[string]any
		status   int
		code     string
		field    string
		sentence string
	}{
		{"relative folder", map[string]any{"dir": "site"}, 422, "invalid_dir", "dir", "The folder must be an absolute path starting with /, at most 4096 bytes, without control characters or line separators."},
		{"no folder", map[string]any{"dir": nil}, 422, "invalid_dir", "dir", ""},
		{"long folder", map[string]any{"dir": "/" + strings.Repeat("a", 4096)}, 422, "invalid_dir", "dir", ""},
		{"line break in folder", map[string]any{"dir": "/a\nb"}, 422, "invalid_dir", "dir", ""},
		{"long name", map[string]any{"name": strings.Repeat("é", 81)}, 422, "invalid_name", "name", "Give the project a name of 1 to 80 characters, without line breaks or control characters."},
		{"line break in name", map[string]any{"name": "Site\nfront"}, 422, "invalid_name", "name", ""},
		{"bad base branch", map[string]any{"baseBranch": "bad..branch"}, 422, "invalid_branch", "baseBranch", "The base branch must be blank or a valid git branch name of at most 255 bytes."},
		{"base branch with a space", map[string]any{"baseBranch": "my branch"}, 422, "invalid_branch", "baseBranch", ""},
		{"worktree maybe", map[string]any{"worktreeDefault": "maybe"}, 422, "invalid_worktree_default", "worktreeDefault", "Choose whether each session gets a worktree: inherit, yes or no."},
		{"unknown tool", map[string]any{"defaultTool": "bash"}, 422, "invalid_tool", "defaultTool", "Choose one of the agents reet can start."},
		{"bad model", map[string]any{"defaultModel": "-rm"}, 422, "invalid_model", "defaultModel", sentenceModel},
		{"prompt starting with a dash", map[string]any{"starterPrompt": "--yolo " + folderSentinel}, 422, "invalid_prompt", "starterPrompt", "The starter prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash."},
		{"prompt too long", map[string]any{"starterPrompt": folderSentinel + strings.Repeat(" word", 400)}, 422, "invalid_prompt", "starterPrompt", ""},
		{"prompt with a control character", map[string]any{"starterPrompt": folderSentinel + "\t"}, 422, "invalid_prompt", "starterPrompt", ""},
		{"one-word prompt", map[string]any{"starterPrompt": "okapi-sentinel"}, 422, "prompt_unsupported", "starterPrompt", "Write the starter prompt as at least two words."},
	}
	for _, c := range cases {
		f.resetFolderWrites()
		res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(c.over), "")
		if res.code != c.status || res.out["code"] != c.code || res.out["field"] != c.field || res.out["error"] != res.out["message"] || c.sentence != "" && res.out["error"] != c.sentence {
			t.Fatal(c.name, res.code, res.raw)
		}
		if strings.Contains(res.raw, "okapi") || res.header.Get("ETag") != "" {
			t.Fatal(c.name, "echoed the prompt or a version", res.raw)
		}
	}
	if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 0 {
		t.Fatal("a refused project was saved", projects)
	}
	if rows := folderRows(t, f, f.uid, "project-add"); len(rows) != 0 {
		t.Fatal("a refused project was audited", rows)
	}

	created := f.fpCreate(t, nil)
	id := created["id"].(string)
	if strings.Contains(fmt.Sprint(created), "okapi") {
		t.Fatal("create answered with the prompt", created)
	}
	rows := folderRows(t, f, f.uid, "project-add")
	if len(rows) != 1 || !reflect.DeepEqual(rows[0], map[string]any{"project": id, "machine": f.agent.ID, "host": "laptop", "folder": "/home/tester/site", "name": "Site"}) {
		t.Fatal("add row", rows)
	}
	// A blank name is the folder's name; the store adds (2) on a clash.
	if out := f.fpCreate(t, map[string]any{"dir": "/srv/Site", "name": nil}); out["name"] != "Site (2)" {
		t.Fatal("default name", out)
	}
	// The same machine and folder, however it is spelled.
	res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/home/tester/./site/", "name": "Again"}), "")
	if res.code != 409 || res.out["code"] != "project_exists" || res.out["field"] != "dir" || res.out["error"] != "laptop already has a project for that folder." {
		t.Fatal("exists", res.code, res.raw)
	}
	res = f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/srv/other", "name": " site "}), "")
	if res.code != 409 || res.out["code"] != "name_taken" || res.out["field"] != "name" || res.out["error"] != "Another project on this account already has that name." {
		t.Fatal("name taken", res.code, res.raw)
	}
	for i := 2; i < store.MaxFolderProjects; i++ {
		if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: fmt.Sprintf("/fill/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	res = f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/one/too/many", "name": "Too many"}), "")
	if res.code != 409 || res.out["code"] != "project_limit" || res.out["error"] != "You have 100 projects, the most reet keeps. Remove one first." {
		t.Fatal("limit", res.code, res.raw)
	}
	if rows := folderRows(t, f, f.uid, "project-add"); len(rows) != 2 {
		t.Fatal("refused adds were audited", len(rows))
	}
}

func TestFolderProjectAuditNeverHasStarterPrompt(t *testing.T) {
	f := projectFixture(t)
	created := f.fpCreate(t, nil)
	id := created["id"].(string)
	res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"name": "Site front", "starterPrompt": folderSentinel + " again"}, `"1"`)
	if res.code != 200 || strings.Contains(res.raw, "okapi") {
		t.Fatal("update", res.code, res.raw)
	}
	// A refused update with the prompt in it.
	res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"starterPrompt": "-" + folderSentinel}, "")
	if res.code != 422 || strings.Contains(res.raw, "okapi") {
		t.Fatal("refused update", res.code, res.raw)
	}
	reads := []string{}
	for _, path := range []string{"/api/v2/folder-projects", "/api/events?limit=100"} {
		r := f.fpDo(t, f.cookie, "GET", path, nil, "")
		if r.code != 200 {
			t.Fatal(path, r.code, r.raw)
		}
		reads = append(reads, r.raw)
	}
	res = f.fpDo(t, f.cookie, "DELETE", "/api/v2/folder-projects/"+id, nil, `"2"`)
	if res.code != 200 || strings.Contains(res.raw, "okapi") {
		t.Fatal("delete", res.code, res.raw)
	}
	update := folderRows(t, f, f.uid, "project-update")
	remove := folderRows(t, f, f.uid, "project-remove")
	if len(update) != 1 || !reflect.DeepEqual(update[0], map[string]any{"project": id, "machine": f.agent.ID, "host": "laptop", "folder": "/home/tester/site", "name": "Site front"}) {
		t.Fatal("update row", update)
	}
	if len(remove) != 1 || !reflect.DeepEqual(remove[0], map[string]any{"project": id, "machine": f.agent.ID, "host": "laptop", "folder": "/home/tester/site", "name": "Site front"}) {
		t.Fatal("remove row", remove)
	}
	events, err := f.st.ListEventsFor(f.uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if strings.Contains(e.Detail, "okapi") {
			t.Fatal("audit has the starter prompt", e.Action, e.Detail)
		}
	}
	for _, raw := range reads {
		if strings.Contains(raw, "okapi") {
			t.Fatal("a read has the starter prompt", raw)
		}
	}
	if strings.Contains(f.logs.String(), "okapi") {
		t.Fatal("logs have the starter prompt", f.logs.String())
	}
}

func TestFolderProjectStaleIfMatch(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	path := "/api/v2/folder-projects/" + id
	// Two editors both read version 1.
	first := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "First editor"}, `"1"`)
	if first.code != 200 || first.out["version"] != float64(2) || first.header.Get("ETag") != `"2"` {
		t.Fatal("first edit", first.code, first.raw)
	}
	second := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "Second editor", "dir": "/srv/x"}, "1")
	if second.code != 409 || second.out["code"] != "project_changed" || second.out["error"] != folderChangedSentence {
		t.Fatal("stale edit", second.code, second.raw)
	}
	saved, _ := f.st.FolderProjectFor(f.uid, id)
	if saved.Name != "First editor" || saved.Dir != "/home/tester/site" || saved.Version != 2 {
		t.Fatal("the stale edit reverted the first", saved)
	}
	if rows := folderRows(t, f, f.uid, "project-update"); len(rows) != 1 {
		t.Fatal("a stale edit was audited", rows)
	}
	// A stale delete removes nothing.
	if res := f.fpDo(t, f.cookie, "DELETE", path, nil, `"1"`); res.code != 409 || res.out["code"] != "project_changed" || res.out["error"] != folderChangedSentence {
		t.Fatal("stale delete", res.code, res.raw)
	}
	if _, err := f.st.FolderProjectFor(f.uid, id); err != nil {
		t.Fatal("a stale delete removed the project", err)
	}
	read := f.fpDo(t, f.cookie, "GET", path, nil, "")
	if read.header.Get("ETag") != `"2"` || read.out["version"] != float64(2) {
		t.Fatal("read", read.header, read.raw)
	}
	for _, bad := range []string{"abc", "0", "-1", `W/"2"`, "1.5"} {
		for _, method := range []string{"PATCH", "DELETE"} {
			if res := f.fpDo(t, f.cookie, method, path, map[string]any{"name": "Bad"}, bad); res.code != 400 || res.out["code"] != "invalid_version" {
				t.Fatal("bad If-Match", method, bad, res.code, res.raw)
			}
		}
	}
	// Without If-Match the last write wins, as with Projects.
	if res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "No header"}, ""); res.code != 200 || res.out["version"] != float64(3) {
		t.Fatal("no header", res.code, res.raw)
	}
	if res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "Old"}, `"2"`); res.code != 409 {
		t.Fatal("stale after a header-less edit", res.code, res.raw)
	}
}

func TestFolderProjectDeleteThenStalePatch(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	path := "/api/v2/folder-projects/" + id
	res := f.fpDo(t, f.cookie, "DELETE", path, nil, `"1"`)
	if res.code != 200 || res.out["message"] != "Project removed. Its sessions and any worktrees stay on laptop." {
		t.Fatal("delete", res.code, res.raw)
	}
	for _, version := range []string{`"1"`, ""} {
		res := f.fpDo(t, f.cookie, "PATCH", path, f.fpBody(map[string]any{"name": "Back again"}), version)
		if res.code != 404 || res.out["code"] != "project_not_found" || res.out["error"] != "That project isn't on this account." {
			t.Fatal("patch after delete", version, res.code, res.raw)
		}
	}
	if res := f.fpDo(t, f.cookie, "DELETE", path, nil, ""); res.code != 404 {
		t.Fatal("second delete", res.code, res.raw)
	}
	if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 0 {
		t.Fatal("a patch recreated the project", projects)
	}
	if rows := folderRows(t, f, f.uid, "project-update"); len(rows) != 0 {
		t.Fatal("a refused patch was audited", rows)
	}
	if rows := folderRows(t, f, f.uid, "project-remove"); len(rows) != 1 {
		t.Fatal("remove rows", rows)
	}
}

func TestFolderProjectReorderStaleVersions(t *testing.T) {
	f := projectFixture(t)
	a := f.fpCreate(t, map[string]any{"dir": "/w/a", "name": "A"})["id"].(string)
	b := f.fpCreate(t, map[string]any{"dir": "/w/b", "name": "B"})["id"].(string)
	c := f.fpCreate(t, map[string]any{"dir": "/w/c", "name": "C"})["id"].(string)
	reorder := func(ids []string, versions []int) fpResult {
		return f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/reorder", map[string]any{"ids": ids, "versions": versions}, "")
	}
	res := reorder([]string{c, a, b}, []int{1, 1, 1})
	if res.code != 200 {
		t.Fatal("reorder", res.code, res.raw)
	}
	order := []string{}
	for _, p := range res.out["projects"].([]any) {
		order = append(order, p.(map[string]any)["name"].(string))
	}
	if !reflect.DeepEqual(order, []string{"C", "A", "B"}) || strings.Contains(res.raw, "okapi") {
		t.Fatal("order", order, res.raw)
	}
	// Another tab still holds version 1 of every project.
	res = reorder([]string{a, b, c}, []int{1, 1, 1})
	if res.code != 409 || res.out["code"] != "project_changed" || res.out["error"] != folderChangedSentence {
		t.Fatal("stale reorder", res.code, res.raw)
	}
	// Another tab removed a project: a stale view, not "not found".
	gone := f.fpCreate(t, map[string]any{"dir": "/w/gone", "name": "Gone"})
	if res := f.fpDo(t, f.cookie, "DELETE", "/api/v2/folder-projects/"+gone["id"].(string), nil, ""); res.code != 200 {
		t.Fatal("delete", res.code, res.raw)
	}
	fresh := map[string]int{}
	for _, p := range f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, "").out["projects"].([]any) {
		fresh[p.(map[string]any)["id"].(string)] = int(p.(map[string]any)["version"].(float64))
	}
	if res := reorder([]string{gone["id"].(string), c, a, b}, []int{1, fresh[c], fresh[a], fresh[b]}); res.code != 409 || res.out["code"] != "project_changed" || res.out["error"] != folderChangedSentence {
		t.Fatal("reorder after another tab's delete", res.code, res.raw)
	}
	// Leaving a project out is a stale view too.
	pc, _ := f.st.FolderProjectFor(f.uid, c)
	pa, _ := f.st.FolderProjectFor(f.uid, a)
	if res := reorder([]string{a, c}, []int{int(pa.Version), int(pc.Version)}); res.code != 409 || res.out["code"] != "project_changed" {
		t.Fatal("partial reorder", res.code, res.raw)
	}
	for name, body := range map[string]any{
		"lengths differ": map[string]any{"ids": []string{a, b, c}, "versions": []int{1, 1}},
		"twice":          map[string]any{"ids": []string{a, a, c}, "versions": []int{1, 1, 1}},
		"no ids":         map[string]any{"versions": []int{1}},
	} {
		if res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/reorder", body, ""); res.code != 422 || res.out["code"] != "invalid_order" || res.out["field"] != "ids" {
			t.Fatal(name, res.code, res.raw)
		}
	}
	projects, _ := f.st.ListFolderProjectsFor(f.uid)
	if len(projects) != 3 || projects[0].ID != c || projects[1].ID != a || projects[2].ID != b {
		t.Fatal("a refused reorder moved something", projects)
	}
}

func TestFolderProjectWriteRateLimit(t *testing.T) {
	f := projectFixture(t)
	f.resetFolderWrites()
	id := f.fpCreate(t, map[string]any{"dir": "/w/0", "name": "Zero"})["id"].(string)
	for i := 1; i < 30; i++ {
		var res fpResult
		if i%2 == 0 {
			res = f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": fmt.Sprintf("/w/%d", i), "name": fmt.Sprintf("P%d", i)}), "")
		} else {
			res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"name": fmt.Sprintf("Zero %d", i)}, "")
		}
		if res.code != 200 && res.code != 201 {
			t.Fatal("write", i, res.code, res.raw)
		}
	}
	before, _ := f.st.ListFolderProjectsFor(f.uid)
	for _, w := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/w/over", "name": "Over"})},
		{"PATCH", "/api/v2/folder-projects/" + id, map[string]any{"name": "Over"}},
		{"DELETE", "/api/v2/folder-projects/" + id, nil},
		{"POST", "/api/v2/folder-projects/reorder", map[string]any{"ids": []string{id}, "versions": []int{1}}},
	} {
		res := f.fpDo(t, f.cookie, w.method, w.path, w.body, "")
		if res.code != 429 || res.out["code"] != "project_rate_limited" || res.out["error"] != "Slow down a moment, then try again." || res.header.Get("Cache-Control") != "no-store" {
			t.Fatal("over the limit", w.method, w.path, res.code, res.raw)
		}
	}
	after, _ := f.st.ListFolderProjectsFor(f.uid)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("a limited write changed something")
	}
	// Reads are not limited, and the limit is per account.
	if res := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, ""); res.code != 200 {
		t.Fatal("read", res.code, res.raw)
	}
	other, _ := f.st.CreateUser("folder-rate@example.test", "Other", "admin", "hash")
	desk, _, err := f.st.EnrollSyncAgent(other.ID, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	res := f.fpDo(t, terminalLogin(t, f.st, other.ID), "POST", "/api/v2/folder-projects", map[string]any{"host": desk.ID, "dir": "/d"}, "")
	if res.code != 201 {
		t.Fatal("another account limited", res.code, res.raw)
	}
}

func TestFolderProjectStrictJSON(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	good, _ := json.Marshal(map[string]any{"host": f.agent.ID, "dir": "/strict"})
	for name, body := range map[string]string{
		"unknown field":      `{"host":"` + f.agent.ID + `","dir":"/strict","prompt":"two words"}`,
		"two values":         string(good) + string(good),
		"trailing garbage":   string(good) + "x",
		"wrong type":         `{"host":"` + f.agent.ID + `","dir":"/strict","name":1}`,
		"isRepo a string":    `{"host":"` + f.agent.ID + `","dir":"/strict","isRepo":"yes"}`,
		"not an object":      `["laptop"]`,
		"not JSON":           `host=laptop`,
		"empty":              ``,
		"read-only id":       `{"id":"abc","host":"` + f.agent.ID + `","dir":"/strict"}`,
		"read-only version":  `{"version":3,"host":"` + f.agent.ID + `","dir":"/strict"}`,
		"read-only position": `{"position":3,"host":"` + f.agent.ID + `","dir":"/strict"}`,
		"read-only flag":     `{"hasStarterPrompt":true,"host":"` + f.agent.ID + `","dir":"/strict"}`,
		"legacy id":          `{"legacyProjectId":"x:0","host":"` + f.agent.ID + `","dir":"/strict"}`,
	} {
		for _, rt := range []struct{ method, path string }{{"POST", "/api/v2/folder-projects"}, {"PATCH", "/api/v2/folder-projects/" + id}} {
			f.resetFolderWrites()
			res := f.fpDo(t, f.cookie, rt.method, rt.path, body, "")
			if res.code != 400 || res.out["code"] != "invalid_request" || res.out["error"] != "That request isn't a project reet can read." {
				t.Fatal(name, rt.method, res.code, res.raw)
			}
		}
	}
	for name, body := range map[string]string{
		"unknown field":   `{"ids":["` + id + `"],"versions":[1],"positions":[0]}`,
		"versions string": `{"ids":["` + id + `"],"versions":["1"]}`,
		"two values":      `{"ids":[],"versions":[]}{}`,
		"empty":           ``,
	} {
		f.resetFolderWrites()
		res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/reorder", body, "")
		if res.code != 400 || res.out["code"] != "invalid_request" {
			t.Fatal("reorder", name, res.code, res.raw)
		}
	}
	huge := `{"host":"` + f.agent.ID + `","dir":"/strict","starterPrompt":"` + strings.Repeat("a", 70<<10) + `"}`
	for _, rt := range []struct{ method, path string }{{"POST", "/api/v2/folder-projects"}, {"PATCH", "/api/v2/folder-projects/" + id}, {"POST", "/api/v2/folder-projects/reorder"}} {
		f.resetFolderWrites()
		res := f.fpDo(t, f.cookie, rt.method, rt.path, huge, "")
		if res.code != 413 || res.out["code"] != "too_large" {
			t.Fatal("size limit", rt.method, rt.path, res.code, res.raw[:min(len(res.raw), 200)])
		}
	}
	// The largest real project fits: the longest folder, name, branch and
	// prompt, all escaped.
	f.resetFolderWrites()
	f.fpCreate(t, map[string]any{"dir": "/" + strings.Repeat("é", 2047) + "d", "name": strings.Repeat("é", 80), "baseBranch": strings.Repeat("b", 255),
		"defaultModel": strings.Repeat("m", 100), "starterPrompt": strings.Repeat("<> ", 666) + "ab"})
	if projects, _ := f.st.ListFolderProjectsFor(f.uid); len(projects) != 2 || projects[0].Name != "Site" || projects[0].Version != 1 {
		t.Fatal("a refused request saved something", projects)
	}
}

func TestFolderProjectNoStore(t *testing.T) {
	f := projectFixture(t)
	id := f.fpCreate(t, nil)["id"].(string)
	check := func(name string, res fpResult, want int) {
		t.Helper()
		if res.code != want || res.header.Get("Cache-Control") != "no-store" || res.header.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal(name, res.code, res.header, res.raw)
		}
	}
	path := "/api/v2/folder-projects/" + id
	check("list", f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, ""), 200)
	check("read", f.fpDo(t, f.cookie, "GET", path, nil, ""), 200)
	check("create", f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/w/two", "name": "Two"}), ""), 201)
	check("update", f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "Renamed"}, ""), 200)
	check("reorder", f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/reorder", map[string]any{"ids": []string{id}, "versions": []int{2}}, ""), 409)
	check("unknown", f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects/"+strings.Repeat("0", 32), nil, ""), 404)
	check("refused", f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "relative"}), ""), 422)
	check("over the limit", func() fpResult {
		var last fpResult
		for i := 0; i < folderProjectWriteLimit+1; i++ {
			last = f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"name": "Again"}, "")
		}
		return last
	}(), 429)
	f.resetFolderWrites()
	check("delete", f.fpDo(t, f.cookie, "DELETE", path, nil, ""), 200)
}
