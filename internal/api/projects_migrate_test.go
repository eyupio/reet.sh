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
