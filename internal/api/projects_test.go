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

// create saves a Project through the store (the API no longer makes them:
// they moved into folder projects) and answers with its single read.
func (f *projectAPI) create(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	p := storeProject(t, f.st, f.uid, body)
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+p.ID, nil)
	if code != 200 {
		t.Fatal("read", code, raw)
	}
	return out
}

// storeProject saves the Project a request body describes through the store,
// as POST /api/v2/projects once did (useWorktrees defaults to true).
func storeProject(t *testing.T, st *store.Store, uid int64, body map[string]any) store.Project {
	t.Helper()
	var in struct {
		Name, DefaultTool, DefaultModel, SharedPrompt string
		UseWorktrees                                  *bool
		Members                                       []store.ProjectMember
	}
	data, _ := json.Marshal(body)
	if err := json.Unmarshal(data, &in); err != nil {
		t.Fatal(err)
	}
	p := store.Project{Name: in.Name, DefaultTool: in.DefaultTool, DefaultModel: in.DefaultModel, SharedPrompt: in.SharedPrompt, UseWorktrees: in.UseWorktrees == nil || *in.UseWorktrees, Members: in.Members}
	saved, err := st.CreateProjectFor(uid, p)
	if err != nil {
		t.Fatal(err)
	}
	return saved
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

func TestProjectReadsKeepWorkingAndWritesAreGone(t *testing.T) {
	f := projectFixture(t)
	created := f.create(t, f.body())
	id, _ := created["id"].(string)
	if len(id) != 32 || created["name"] != "Shop" || created["defaultTool"] != "claude" || created["defaultModel"] != "sonnet" ||
		created["useWorktrees"] != true || created["hasSharedPrompt"] != true || created["sharedPrompt"] != projectSentinel {
		t.Fatal("single read", created)
	}
	want := []string{"createdAt", "defaultModel", "defaultTool", "hasSharedPrompt", "id", "members", "name", "sharedPrompt", "updatedAt", "useWorktrees", "version"}
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
	before, _ := f.st.ProjectFor(f.uid, id)
	// Making, changing and deleting a Project moved to the rail on Home.
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/v2/projects", f.body()},
		{"PATCH", "/api/v2/projects/" + id, map[string]any{"name": "Shop front"}},
		{"DELETE", "/api/v2/projects/" + id, nil},
	} {
		code, out, raw := f.do(t, f.cookie, c.method, c.path, c.body)
		if code != 410 || out["code"] != "projects_moved" || out["error"] != "Projects now live in the rail on Home." {
			t.Fatal(c.method, code, raw)
		}
	}
	if after, err := f.st.ProjectFor(f.uid, id); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("a moved write changed the Project", after, err)
	}
	if projects, _ := f.st.ListProjectsFor(f.uid); len(projects) != 1 {
		t.Fatal("a moved write made a Project", projects)
	}
	if code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/projects/"+strings.Repeat("0", 32), nil); code != 404 || out["code"] != "not_found" || out["error"] != "That Project isn't on this account." {
		t.Fatal("unknown project", code, raw)
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
	for _, c := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/api/v2/projects/" + aID, 404},
		// Changes are gone for every Project: the same answer whoever owns it.
		{"PATCH", "/api/v2/projects/" + aID, 410},
		{"DELETE", "/api/v2/projects/" + aID, 410},
		{"GET", "/api/v2/projects/" + aID + "/runs", 404},
		{"GET", "/api/v2/project-runs/" + run.ID, 404},
	} {
		var body any
		if c.method == "PATCH" {
			body = map[string]any{"name": "Taken over", "members": []map[string]any{{"host": otherAgent.ID, "dir": "/x"}}}
		}
		code, out, raw := f.do(t, b, c.method, c.path, body)
		if code != c.code || (out["code"] != "not_found" && out["code"] != "run_not_found" && out["code"] != "projects_moved") || strings.Contains(raw, "Shop") {
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
		{"GET", "/api/v2/project-runs/" + run.ID, ""},
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

func TestProjectSharedPromptOnlyInTheSingleRead(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	if code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/projects/"+id, map[string]any{"name": "Shop two", "sharedPrompt": projectSentinel + " again"}); code != 410 || strings.Contains(raw, "zebra") {
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
	for _, path := range []string{"/api/v2/projects", "/api/v2/projects/" + id + "/runs", "/api/v2/project-runs/" + run.ID, "/api/events?limit=100"} {
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
	if _, err := f.st.ProjectFor(f.uid, id); err != nil {
		t.Fatal("the moved delete removed the Project", err)
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
	// The API no longer deletes Projects; one deleted (as its account's
	// data can be) keeps its runs readable.
	if err := f.st.DeleteProjectFor(f.uid, id); err != nil {
		t.Fatal(err)
	}
	code, got, raw := f.do(t, f.cookie, "GET", "/api/v2/project-runs/"+run.ID, nil)
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
	wantRun := []string{"id", "members", "projectId", "projectName", "startedAt", "state", "useWorktrees", "worktreeProject"}
	if keys := keysOf(got); !reflect.DeepEqual(keys, wantRun) {
		t.Fatal("run keys", keys)
	}
	wantMember := []string{"attempt", "code", "dir", "host", "model", "position", "sentence", "sessionId", "sharesCheckout", "state", "tool", "worktreeBranch", "worktreePath", "worktreeProject", "worktreeRepo", "worktreeRun"}
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
	for _, path := range []string{"/api/v2/projects/" + id + "/other", "/api/v2/project-runs/" + strings.Repeat("0", 32), "/api/v2/projects/runs/" + ids[0], "/api/v2/projects/" + strings.Repeat("0", 32) + "/runs"} {
		if code, _, raw := f.do(t, f.cookie, "GET", path, nil); code != 404 {
			t.Fatal(path, code, raw)
		}
	}
}

func TestProjectStoreErrorsHaveStableCodes(t *testing.T) {
	// A folder the store refuses names its repository, not the whole Project.
	dirErr := fmt.Errorf("saving: %w", &store.ProjectMemberError{Index: 2, Field: "dir", Err: store.ErrProjectInvalid})
	if r := storeRefusal(dirErr); r.code != "invalid_dir" || r.field != "members.2.dir" || r.status != 422 || !strings.HasPrefix(r.sentence, "Repository 3: the folder") {
		t.Fatal(r)
	}
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
		store.ErrProjectStale:        "project_changed",
		fmt.Errorf("disk on fire"):   "server_error",
	} {
		r := storeRefusal(fmt.Errorf("wrapped: %w", err))
		if r.code != code || r.sentence == "" || strings.Contains(strings.ToLower(r.sentence), "verified") || strings.Contains(strings.ToLower(r.sentence), "healthy") {
			t.Fatal(err, r)
		}
	}
}
