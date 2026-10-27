package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

const sentenceProjectsMoved = "Projects now live in the rail on Home."

// Saved Projects moved into folder projects: making, changing and deleting
// one answers 410 projects_moved whatever the request carries, changes
// nothing and audits nothing. Reads keep working, and access is checked first.
func TestOldProjectRoutesAnswer410(t *testing.T) {
	f := projectFixture(t)
	id := f.create(t, f.body())["id"].(string)
	before, _ := f.st.ProjectFor(f.uid, id)
	eventsBefore, _ := f.st.ListEventsFor(f.uid, 1000, "")
	quoted, _ := json.Marshal(projectSentinel)
	S := string(quoted[1 : len(quoted)-1])
	bodies := []string{
		`{"name":"New","defaultTool":"claude","sharedPrompt":"` + S + `"}`,
		`{"name":"","defaultTool":"nope","sharedPrompt":"-` + S + `"}`,
		`{"name":"Strict","bogus":1}`,
		`not json`,
		``,
		`{"sharedPrompt":"` + strings.Repeat("a", 130<<10) + `"}`,
	}
	other, _ := f.st.CreateUser("moved-other@example.test", "Other", "admin", "hash")
	otherCookie := terminalLogin(t, f.st, other.ID)
	for _, cookie := range []string{f.cookie, otherCookie} {
		for _, b := range bodies {
			for _, rt := range []struct{ method, path string }{
				{"POST", "/api/v2/projects"},
				{"PATCH", "/api/v2/projects/" + id},
				{"PATCH", "/api/v2/projects/" + strings.Repeat("0", 32)},
				{"DELETE", "/api/v2/projects/" + id},
			} {
				for _, version := range []string{"", `"1"`, `"9"`, "abc"} {
					res := f.fpDo(t, cookie, rt.method, rt.path, b, version)
					if res.code != 410 || res.out["code"] != "projects_moved" || res.out["error"] != sentenceProjectsMoved ||
						res.header.Get("Cache-Control") != "no-store" || strings.Contains(res.raw, "zebra") || strings.Contains(res.raw, "Shop") {
						t.Fatal(rt.method, rt.path, version, res.code, res.raw[:min(len(res.raw), 200)])
					}
				}
			}
		}
	}
	if after, err := f.st.ProjectFor(f.uid, id); err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("a moved write changed the Project", after, err)
	}
	for _, uid := range []int64{f.uid, other.ID} {
		if projects, _ := f.st.ListProjectsFor(uid); len(projects) != map[int64]int{f.uid: 1, other.ID: 0}[uid] {
			t.Fatal("a moved write made a Project", projects)
		}
	}
	if eventsAfter, _ := f.st.ListEventsFor(f.uid, 1000, ""); len(eventsAfter) != len(eventsBefore) {
		t.Fatal("a moved write was audited", eventsAfter)
	}
	if strings.Contains(f.logs.String(), "zebra") {
		t.Fatal("the prompt reached the log")
	}
	// Reads are unchanged.
	for _, path := range []string{"/api/v2/projects", "/api/v2/projects/" + id, "/api/v2/projects/" + id + "/runs"} {
		if res := f.fpDo(t, f.cookie, "GET", path, nil, ""); res.code != 200 {
			t.Fatal(path, res.code, res.raw)
		}
	}
	// Access is still checked first: a viewer and another origin are refused as before.
	viewer, _ := f.st.CreateUser("moved-viewer@example.test", "Viewer", "viewer", "hash")
	if code, raw := f.raw(t, "DELETE", "/api/v2/projects/"+id, "", terminalLogin(t, f.st, viewer.ID), "https://reet.example", ""); code != 403 || !strings.Contains(raw, "read_only") {
		t.Fatal("viewer", code, raw)
	}
	if code, raw := f.raw(t, "POST", "/api/v2/projects", `{}`, f.cookie, "https://evil.example", ""); code != 403 || !strings.Contains(raw, "origin_refused") {
		t.Fatal("origin", code, raw)
	}
}

// After the migration the old Project still starts runs, retries them and
// removes their worktrees: that is how "start all" works until runs move.
func TestOldProjectRunStillStartsAfterMigration(t *testing.T) {
	f := runMachineFixture(t)
	id := f.project(t)
	rep, err := f.st.MigrateProjectsToFolders()
	if err != nil || rep.Created != 3 {
		t.Fatal(rep, err)
	}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/projects/"+id+"/runs", map[string]any{"requestId": terminalbroker.ID(), "positions": []int{0, 1}})
	if code != 201 {
		t.Fatal("start", code, raw)
	}
	runID := out["id"].(string)
	run := f.settled(t, runID, notRunning)
	if run.State != store.RunComplete {
		t.Fatalf("%+v", run.Members)
	}
	// Retry answers as it always did (nothing failed here).
	if code, out, _ := f.do(t, f.cookie, "POST", "/api/v2/project-runs/"+runID+"/members/1/retry", map[string]any{}); code != 409 || out["code"] != "not_failed" {
		t.Fatal("retry", code, out)
	}
	_, view, _ := f.do(t, f.cookie, "GET", "/api/v2/project-runs/"+runID, nil)
	m := view["members"].([]any)[0].(map[string]any)
	target := map[string]any{"agentId": m["host"], "project": m["worktreeProject"], "repo": m["worktreeRepo"], "run": m["worktreeRun"], "changed": 0, "unpushed": 0}
	if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", target); code != 200 {
		t.Fatal("remove", code, raw)
	}
	if run, _ := f.st.RunFor(f.uid, runID); run.Members[0].State != store.RunMemberRemoved {
		t.Fatal(run.Members)
	}
	// The folder projects it became are in the rail.
	_, list, raw := f.do(t, f.cookie, "GET", "/api/v2/folder-projects", nil)
	if projects := list["projects"].([]any); len(projects) != 3 || strings.Contains(raw, "zebra") {
		t.Fatal(raw)
	}
}

// The migration's note is in every read and can only be cleared.
func TestFolderProjectMigrationNote(t *testing.T) {
	f := projectFixture(t)
	storeProject(t, f.st, f.uid, map[string]any{"name": "Older", "defaultTool": "claude", "sharedPrompt": "fix the build",
		"members": []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/site"}}})
	storeProject(t, f.st, f.uid, map[string]any{"name": "Newer", "defaultTool": "claude", "sharedPrompt": projectSentinel,
		"members": []map[string]any{{"host": f.agent.ID, "dir": "/home/tester/site"}}})
	if _, err := f.st.MigrateProjectsToFolders(); err != nil {
		t.Fatal(err)
	}
	_, list, raw := f.do(t, f.cookie, "GET", "/api/v2/folder-projects", nil)
	projects := list["projects"].([]any)
	if len(projects) != 1 || strings.Contains(raw, "zebra") {
		t.Fatal(raw)
	}
	p := projects[0].(map[string]any)
	id := p["id"].(string)
	if p["note"] != "Newer" || p["name"] != "Older" || p["hasStarterPrompt"] != true {
		t.Fatal(p)
	}
	one := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects/"+id, nil, "")
	if one.out["note"] != "Newer" || one.out["starterPrompt"] != "fix the build" {
		t.Fatal(one.raw)
	}
	version := one.header.Get("ETag")
	// An edit without the note keeps it; only "" is taken.
	if res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"note": "something else"}, version); res.code != 422 || res.out["code"] != "invalid_note" || res.out["field"] != "note" {
		t.Fatal(res.code, res.raw)
	}
	if res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", f.fpBody(map[string]any{"dir": "/home/tester/other", "name": "Other", "note": "x"}), ""); res.code != 422 || res.out["code"] != "invalid_note" {
		t.Fatal(res.code, res.raw)
	}
	res := f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"name": "Site"}, version)
	if res.code != 200 || res.out["note"] != "Newer" {
		t.Fatal(res.code, res.raw)
	}
	res = f.fpDo(t, f.cookie, "PATCH", "/api/v2/folder-projects/"+id, map[string]any{"note": ""}, res.header.Get("ETag"))
	if res.code != 200 || res.out["note"] != "" {
		t.Fatal(res.code, res.raw)
	}
	if saved, _ := f.st.FolderProjectFor(f.uid, id); saved.Note != "" || saved.StarterPrompt != "fix the build" {
		t.Fatalf("%+v", saved)
	}
	// The migration's audit row names the Project and its folder projects only.
	rows := projectEvents(t, f.st, f.uid, "projects-migrated")
	if len(rows) != 2 || !reflect.DeepEqual(rows[1], map[string]any{"project": "Older", "folderProjects": []any{id}}) {
		t.Fatal(rows)
	}
}

// Repositories that waited for room come in when the person asks, never by
// themselves; the list says how many wait.
func TestFolderProjectImportSaved(t *testing.T) {
	f := projectFixture(t)
	for i := 0; i < store.MaxFolderProjects-1; i++ {
		if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/full/" + strings.Repeat("d", i+1)}); err != nil {
			t.Fatal(err)
		}
	}
	storeProject(t, f.st, f.uid, map[string]any{"name": "Shop", "defaultTool": "claude", "sharedPrompt": projectSentinel, "members": []map[string]any{
		{"host": f.agent.ID, "dir": "/home/tester/a"}, {"host": f.agent.ID, "dir": "/home/tester/b"}, {"host": f.agent.ID, "dir": "/home/tester/c"}}})
	if rep, err := f.st.MigrateProjectsToFolders(); err != nil || rep.Created != 1 || rep.Waiting != 2 {
		t.Fatal(rep, err)
	}
	list := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, "")
	if list.code != 200 || list.out["savedWaiting"] != float64(2) {
		t.Fatal(list.code, list.raw[:min(len(list.raw), 300)])
	}
	// Still no room: nothing comes in, and the answer says why.
	res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/import-saved", nil, "")
	if res.code != 200 || res.out["created"] != float64(0) || res.out["savedWaiting"] != float64(2) ||
		res.out["message"] != "2 saved repositories are waiting for room. Remove a project, then try again." || strings.Contains(res.raw, "zebra") {
		t.Fatal(res.code, res.raw[:min(len(res.raw), 300)])
	}
	// Making room does not bring them in by itself.
	projects := list.out["projects"].([]any)
	for _, p := range projects[:2] {
		p := p.(map[string]any)
		if res := f.fpDo(t, f.cookie, "DELETE", "/api/v2/folder-projects/"+p["id"].(string), nil, ""); res.code != 200 {
			t.Fatal(res.code, res.raw)
		}
	}
	if list := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects", nil, ""); list.out["savedWaiting"] != float64(2) || len(list.out["projects"].([]any)) != 98 {
		t.Fatal("brought in without being asked", list.out["savedWaiting"])
	}
	before := len(projectEvents(t, f.st, f.uid, "projects-migrated"))
	res = f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/import-saved", "{}", "")
	if res.code != 200 || res.out["created"] != float64(2) || res.out["savedWaiting"] != float64(0) || len(res.out["projects"].([]any)) != 100 ||
		res.out["message"] != "Added 2 saved repositories as projects." || strings.Contains(res.raw, "zebra") {
		t.Fatal(res.code, res.raw[:min(len(res.raw), 300)])
	}
	rows := projectEvents(t, f.st, f.uid, "projects-migrated")
	if len(rows) != before+1 || rows[0]["project"] != "Shop" || len(rows[0]["folderProjects"].([]any)) != 3 {
		t.Fatal(rows)
	}
	for _, e := range worktreeEvents(t, f.st, f.uid, "projects-migrated")[:1] {
		if e.Source != "ui" {
			t.Fatal(e.Source)
		}
	}
	// Again: nothing waits, nothing changes.
	res = f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/import-saved", nil, "")
	if res.code != 200 || res.out["created"] != float64(0) || res.out["message"] != "Nothing is waiting to be added: every saved folder is already in reet, or can't be used as a project." || len(projectEvents(t, f.st, f.uid, "projects-migrated")) != before+1 {
		t.Fatal(res.code, res.raw[:min(len(res.raw), 300)])
	}
	// Strict body, and the access rules of every folder-project write.
	if res := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects/import-saved", `{"all":true}`, ""); res.code != 400 || res.out["code"] != "invalid_request" {
		t.Fatal(res.code, res.raw)
	}
	viewer, _ := f.st.CreateUser("import-viewer@example.test", "Viewer", "viewer", "hash")
	if code, raw := f.raw(t, "POST", "/api/v2/folder-projects/import-saved", "", terminalLogin(t, f.st, viewer.ID), "https://reet.example", ""); code != 403 || !strings.Contains(raw, "read_only") {
		t.Fatal("viewer", code, raw)
	}
	if code, raw := f.raw(t, "POST", "/api/v2/folder-projects/import-saved", "", f.cookie, "https://evil.example", ""); code != 403 || !strings.Contains(raw, "origin_refused") {
		t.Fatal("origin", code, raw)
	}
	// Another account's saved Projects are its own.
	other, _ := f.st.CreateUser("import-other@example.test", "Other", "admin", "hash")
	if res := f.fpDo(t, terminalLogin(t, f.st, other.ID), "POST", "/api/v2/folder-projects/import-saved", nil, ""); res.code != 200 || res.out["created"] != float64(0) || len(res.out["projects"].([]any)) != 0 {
		t.Fatal(res.code, res.raw)
	}
}

// A saved starter prompt that today's rules refuse does not block clearing
// the note or editing other fields; a new prompt is checked.
func TestFolderProjectEditKeepsAnOldOneWordPrompt(t *testing.T) {
	f := projectFixture(t)
	saved, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/site", DefaultTool: "claude", StarterPrompt: "update"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/folder-projects/" + saved.ID
	for _, body := range []map[string]any{{"note": ""}, {"name": "Renamed"}, {"defaultModel": "sonnet"}} {
		if res := f.fpDo(t, f.cookie, "PATCH", path, body, ""); res.code != 200 {
			t.Fatal(body, res.code, res.raw)
		}
	}
	if res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"starterPrompt": "review"}, ""); res.code != 422 || res.out["code"] != "prompt_unsupported" {
		t.Fatal(res.code, res.raw)
	}
	if res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"starterPrompt": "review the change"}, ""); res.code != 200 {
		t.Fatal(res.code, res.raw)
	}
}

// A one-word starter prompt is fine for an agent that does not read it as a
// subcommand; switching the default agent to one that would is checked again,
// even though the prompt itself did not change.
func TestFolderProjectToolChangeRechecksAOneWordPrompt(t *testing.T) {
	f := projectFixture(t)
	saved := f.fpDo(t, f.cookie, "POST", "/api/v2/folder-projects", map[string]any{"host": f.agent.ID, "dir": "/home/tester/site", "defaultTool": "gemini", "starterPrompt": "update"}, "")
	if saved.code != 201 {
		t.Fatal(saved.code, saved.raw)
	}
	path := "/api/v2/folder-projects/" + saved.out["id"].(string)
	for _, tool := range []string{"claude", "codex"} {
		if res := f.fpDo(t, f.cookie, "PATCH", path, map[string]any{"defaultTool": tool}, ""); res.code != 422 || res.out["code"] != "prompt_unsupported" || res.out["field"] != "starterPrompt" {
			t.Fatal(tool, res.code, res.raw)
		}
	}
	if got, _ := f.st.FolderProjectFor(f.uid, saved.out["id"].(string)); got.DefaultTool != "gemini" {
		t.Fatal("refused edit saved", got.DefaultTool)
	}
	for _, body := range []map[string]any{{"defaultTool": "opencode"}, {"name": "Renamed"}, {"defaultTool": "claude", "starterPrompt": "update the docs"}} {
		if res := f.fpDo(t, f.cookie, "PATCH", path, body, ""); res.code != 200 {
			t.Fatal(body, res.code, res.raw)
		}
	}
}

// /projects/<id> resolves through ?legacy=<id> to the folder project made
// from that saved Project; another account's id, an unknown one and a
// removed folder project are all not found, as the same answer.
func TestFolderProjectLegacyLookup(t *testing.T) {
	f := projectFixture(t)
	p := storeProject(t, f.st, f.uid, map[string]any{"name": "Shop", "defaultTool": "claude", "members": []map[string]any{
		{"host": f.agent.ID, "dir": "/home/tester/a"}, {"host": f.agent.ID, "dir": "/home/tester/b"}}})
	if rep, err := f.st.MigrateProjectsToFolders(); err != nil || rep.Created != 2 {
		t.Fatal(rep, err)
	}
	first, err := f.st.FolderProjectForLegacyFor(f.uid, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	res := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects?legacy="+p.ID, nil, "")
	if res.code != 200 || res.out["id"] != first.ID || len(res.out) != 1 || res.header.Get("Cache-Control") != "no-store" {
		t.Fatal(res.code, res.raw, res.header)
	}
	other, _ := f.st.CreateUser("legacy-other@example.test", "Other", "admin", "hash")
	for _, c := range []struct{ cookie, id string }{{f.cookie, "nope"}, {f.cookie, ""}, {terminalLogin(t, f.st, other.ID), p.ID}} {
		if res := f.fpDo(t, c.cookie, "GET", "/api/v2/folder-projects?legacy="+c.id, nil, ""); res.code != 404 || res.out["code"] != "project_not_found" || strings.Contains(res.raw, "Shop") {
			t.Fatal(c.id, res.code, res.raw)
		}
	}
	// Removed: the next folder made from it answers; with none left, not found.
	if err := f.st.DeleteFolderProjectFor(f.uid, first.ID, 0); err != nil {
		t.Fatal(err)
	}
	if res := f.fpDo(t, f.cookie, "GET", "/api/v2/folder-projects?legacy="+p.ID, nil, ""); res.code != 200 || res.out["id"] == first.ID || res.out["id"] == "" {
		t.Fatal(res.code, res.raw)
	}
	viewer, _ := f.st.CreateUser("legacy-viewer@example.test", "Viewer", "viewer", "hash")
	if code, raw := f.raw(t, "GET", "/api/v2/folder-projects?legacy="+p.ID, "", terminalLogin(t, f.st, viewer.ID), "https://reet.example", ""); code != 403 || strings.Contains(raw, first.ID) {
		t.Fatal("viewer", code, raw)
	}
}
