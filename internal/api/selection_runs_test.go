package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/projectrun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const selectionAPIPrompt = "Fix the failing tests\nzebra-sentinel and say what done looks like"

// folders saves folder projects on the fixture's machine, one per folder.
func (f *runMachine) folders(t *testing.T, dirs ...string) []string {
	t.Helper()
	var ids []string
	for _, dir := range dirs {
		p, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: dir, IsRepo: true})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
	}
	return ids
}

func selectionBody(request string, ids []string) map[string]any {
	return map[string]any{"requestId": request, "projectIds": ids, "tool": "claude", "model": "", "effort": "", "prompt": selectionAPIPrompt, "worktree": true}
}

// One request starts a run of the chosen folder projects; every member
// starts in its own worktree as a session of its project. Neither the audit,
// the logs, the run's JSON nor the sessions list carries the prompt.
func TestSelectionRunAuditHasNoPrompt(t *testing.T) {
	f := runMachineFixture(t)
	ids := f.folders(t, "/home/tester/site", "/home/tester/api", "/home/tester/docs")
	request := terminalbroker.ID()
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(request, ids))
	if code != 201 || out["origin"] != "selection" || out["state"] != "running" || out["projectId"] != "" || out["projectName"] != "site, api, docs" || strings.Contains(raw, "zebra") {
		t.Fatal(code, raw)
	}
	members := out["members"].([]any)
	if len(members) != 3 {
		t.Fatal(raw)
	}
	for i, m := range members {
		m := m.(map[string]any)
		if m["state"] != "pending" || m["projectId"] != ids[i] || m["position"] != float64(i) {
			t.Fatal(raw)
		}
	}
	runID := out["id"].(string)
	run := f.settled(t, runID, notRunning)
	if run.State != store.RunComplete {
		t.Fatalf("%+v", run.Members)
	}
	sent := f.creates()
	if len(sent) != 3 {
		t.Fatalf("%d creates", len(sent))
	}
	for _, m := range run.Members {
		session, err := f.st.TerminalSessionFor(f.uid, m.SessionID)
		if err != nil || session.ProjectID == nil || *session.ProjectID != ids[m.Position] {
			t.Fatalf("%v %+v", err, session)
		}
		for _, c := range sent {
			in := c.Body.(terminalproto.Create)
			if in.Dir == m.Dir && (c.RequestID != projectrun.RequestID(runID, m.Position, 1) || in.Worktree != projectrun.WorktreeToken(runID, m.Position) || in.Prompt != selectionAPIPrompt || in.Title != "Fix the failing tests") {
				t.Fatalf("%+v %+v", c, in)
			}
		}
	}
	// The same request again answers with the same run and starts nothing.
	code, again, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(request, ids))
	if code != 200 || again["id"] != runID || strings.Contains(raw, "zebra") {
		t.Fatal(code, raw)
	}
	// The replay registered its driver before answering: once that driver has
	// finished, anything it sent has been counted.
	waitUntil(t, "the replay's pass to finish", func() bool { return !f.srv.s.runs.Driving(f.uid, runID) })
	if len(f.creates()) != 3 {
		t.Fatal("a replay started something")
	}
	runs := projectEvents(t, f.st, f.uid, "project-run")
	if len(runs) != 1 || runs[0]["origin"] != "selection" || runs[0]["run"] != runID || runs[0]["members"] != float64(3) {
		t.Fatal(runs)
	}
	launches := projectEvents(t, f.st, f.uid, "terminal-launch")
	if len(launches) != 3 {
		t.Fatal(launches)
	}
	for _, l := range launches {
		if l["projectId"] == nil || l["projectId"] == "" {
			t.Fatal("a launch names no project", l)
		}
	}
	events, _ := json.Marshal(worktreeEvents(t, f.st, f.uid, "project-run"))
	launched, _ := json.Marshal(worktreeEvents(t, f.st, f.uid, "terminal-launch"))
	_, _, read := f.do(t, f.cookie, "GET", "/api/v2/folder-projects/runs/"+runID, nil)
	_, _, sessions := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
	// The prompt's second line is nowhere; its first line only as the
	// sessions' titles (the title a single launch would give).
	for _, text := range []string{string(events), string(launched), read, sessions, f.logs.String()} {
		if strings.Contains(text, "zebra-sentinel") {
			t.Fatal("the prompt's second line leaked:", text)
		}
	}
	for _, text := range []string{string(events), read, f.logs.String()} {
		if strings.Contains(text, "Fix the failing tests") {
			t.Fatal("the prompt's first line leaked outside a title:", text)
		}
	}
	if n := strings.Count(sessions, `"title":"Fix the failing tests"`); n != 3 {
		t.Fatalf("%d sessions titled from the prompt's first line: %s", n, sessions)
	}
	for name, text := range map[string]string{"sessions": sessions, "launch audit": string(launched)} {
		if n, titled := strings.Count(text, "Fix the failing tests"), strings.Count(text, `"title":"Fix the failing tests"`); n != titled {
			t.Fatalf("the %s carry the prompt's first line %d times, only %d as a title: %s", name, n, titled, text)
		}
	}
}

// A start in several projects names 2 to 12 projects of the account and a
// launch reet would send; each refusal is one sentence that never repeats the
// prompt, and nothing is made or sent.
func TestSelectionRunRefusals(t *testing.T) {
	f := runMachineFixture(t)
	dirs := []string{}
	for i := 0; i < 13; i++ {
		dirs = append(dirs, "/home/tester/p"+string(rune('a'+i)))
	}
	ids := f.folders(t, dirs...)
	with := func(change func(map[string]any)) map[string]any {
		b := selectionBody(terminalbroker.ID(), ids[:2])
		change(b)
		return b
	}
	for name, tc := range map[string]struct {
		body any
		code int
		want string
	}{
		"one":           {with(func(b map[string]any) { b["projectIds"] = ids[:1] }), 422, "invalid_selection"},
		"thirteen":      {with(func(b map[string]any) { b["projectIds"] = ids }), 422, "invalid_selection"},
		"twice":         {with(func(b map[string]any) { b["projectIds"] = []string{ids[0], ids[0]} }), 422, "invalid_selection"},
		"none":          {with(func(b map[string]any) { delete(b, "projectIds") }), 422, "invalid_selection"},
		"unknown":       {with(func(b map[string]any) { b["projectIds"] = []string{ids[0], "0123456789abcdef0123456789abcdef"} }), 404, "project_not_found"},
		"request":       {with(func(b map[string]any) { b["requestId"] = "zebra-sentinel" }), 422, "request_id_required"},
		"tool":          {with(func(b map[string]any) { b["tool"] = "pi" }), 422, "tool_unavailable"},
		"model":         {with(func(b map[string]any) { b["model"] = "bad model" }), 422, "invalid_model"},
		"effort":        {with(func(b map[string]any) { b["effort"] = "high" }), 409, "launch_option_unsupported"},
		"effort?":       {with(func(b map[string]any) { b["effort"] = "max" }), 422, "invalid_effort"},
		"dash":          {with(func(b map[string]any) { b["prompt"] = "-" + selectionAPIPrompt }), 422, "invalid_prompt"},
		"long":          {with(func(b map[string]any) { b["prompt"] = strings.Repeat(selectionAPIPrompt, 40) }), 422, "invalid_prompt"},
		"one word":      {with(func(b map[string]any) { b["prompt"] = "zebra-sentinel" }), 422, "prompt_unsupported"},
		"yolo":          {with(func(b map[string]any) { b["yolo"] = true }), 409, "yolo_refused"},
		"unknown field": {with(func(b map[string]any) { b["sharedPrompt"] = selectionAPIPrompt }), 400, "invalid_request"},
		"not json":      {`{"prompt":"zebra-sentinel"`, 400, "invalid_request"},
	} {
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", tc.body)
		if code != tc.code || out["code"] != tc.want {
			t.Fatal(name, code, raw)
		}
		if strings.Contains(raw, "zebra") {
			t.Fatal(name, "the prompt was echoed", raw)
		}
	}
	if pending, _ := f.st.PendingRuns(); len(pending) != 0 || len(f.creates()) != 0 {
		t.Fatal("a refused start made a run", pending)
	}
	if strings.Contains(f.logs.String(), "zebra") {
		t.Fatal("the prompt reached the log")
	}
	// A project still starting in an earlier run waits.
	f.hold = make(chan struct{})
	code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(terminalbroker.ID(), ids[:2]))
	if code != 201 {
		t.Fatal(code, raw)
	}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(terminalbroker.ID(), ids[1:3]))
	close(f.hold)
	if code != 409 || out["code"] != "run_in_progress" {
		t.Fatal(code, raw)
	}
}

// The run is read and its members retried under the folder projects' routes,
// by its owner only; another account is told nothing.
func TestSelectionRunReadAndRetry(t *testing.T) {
	f := runMachineFixture(t)
	ids := f.folders(t, "/home/tester/site", "/home/tester/api")
	f.decide = func(in terminalproto.Create) terminalproto.Body {
		if strings.HasSuffix(in.Dir, "/api") {
			return terminalproto.Result{Code: terminalproto.Capacity}
		}
		return nil
	}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(terminalbroker.ID(), ids))
	if code != 201 {
		t.Fatal(code, raw)
	}
	runID := out["id"].(string)
	run := f.settled(t, runID, notRunning)
	if run.State != store.RunPartial || run.Members[1].Code != projectrun.CodeSessionLimit {
		t.Fatalf("%+v", run.Members)
	}
	code, out, raw = f.do(t, f.cookie, "GET", "/api/v2/folder-projects/runs/"+runID, nil)
	members, _ := out["members"].([]any)
	if code != 200 || out["state"] != "partial" || len(members) != 2 || members[1].(map[string]any)["sentence"] == "" || members[1].(map[string]any)["name"] != "api" {
		t.Fatal(code, raw)
	}
	other, err := f.st.CreateUser("selection-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	b := terminalLogin(t, f.st, other.ID)
	if code, out, raw := f.do(t, b, "GET", "/api/v2/folder-projects/runs/"+runID, nil); code != 404 || out["code"] != "run_not_found" {
		t.Fatal(code, raw)
	}
	if code, out, raw := f.do(t, b, "POST", "/api/v2/folder-projects/runs/"+runID+"/members/1/retry", map[string]any{}); code != 404 || out["code"] != "run_not_found" {
		t.Fatal(code, raw)
	}
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs/"+runID+"/members/0/retry", map[string]any{}); code != 409 || out["code"] != "not_failed" {
		t.Fatal("a started member retried", code, raw)
	}
	f.mu.Lock()
	f.decide = nil
	f.mu.Unlock()
	code, out, raw = f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs/"+runID+"/members/1/retry", map[string]any{})
	if code != 200 || out["id"] != runID {
		t.Fatal(code, raw)
	}
	run = f.settled(t, runID, notRunning)
	if run.State != store.RunComplete || run.Members[1].Attempt != 2 {
		t.Fatalf("%+v", run.Members)
	}
	if n := len(f.creates()); n != 3 {
		t.Fatalf("%d creates", n)
	}
}

// A session is put in a folder project only in that project's folder.
func TestCreateWithProjectIDInAnotherFolderRefused(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	here, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/srv/site"})
	if err != nil {
		t.Fatal(err)
	}
	for _, extra := range []map[string]any{
		{"projectId": here.ID},
		{"tool": "claude", "dir": "/srv/api", "projectId": here.ID},
		{"tool": "claude", "dir": "/srv/site/web", "projectId": here.ID},
	} {
		code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), extra))
		if code != 422 || out["code"] != "project_folder_mismatch" || out["field"] != "dir" {
			t.Fatal(extra, code, raw)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatal("a refused create reached the machine")
	}
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "dir": "/srv/site/", "projectId": here.ID}))
	if code != 200 || out["projectId"] != here.ID {
		t.Fatal(code, raw)
	}
}

// A selection member's refusals read as a single launch's: an option an old
// machine can't take asks for an update.
func TestSelectionRunMemberSentences(t *testing.T) {
	for code, want := range map[string]string{
		projectrun.CodeOptionNeedsUpdate:       "Update reet on that machine to use that option.",
		projectrun.CodeLaunchOptionUnsupported: "That agent doesn't take that option.",
		projectrun.CodeYoloRefused:             "reet doesn't start YOLO sessions yet.",
		projectrun.CodeToolUnavailable:         "reet doesn't start this agent yet.",
		projectrun.CodeProjectMoved:            "That project was removed, or moved to another machine or folder, since this start. Start it again from the rail.",
		"base_missing":                         "That branch isn't in the repository on that machine.",
	} {
		if got := runMemberSentence(store.RunMemberFailed, code); got != want {
			t.Fatal(code, got)
		}
	}
}

// A lost reply sent again is never refused for speed: a replay of a start
// already made answers with its run whatever the write limit says, while a
// new start is still counted and refused.
func TestSelectionRunReplayIsNotRateLimited(t *testing.T) {
	f := runMachineFixture(t)
	ids := f.folders(t, "/home/tester/site", "/home/tester/api")
	request := terminalbroker.ID()
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(request, ids))
	if code != 201 {
		t.Fatal(code, raw)
	}
	runID := out["id"]
	for i := 1; i < folderProjectWriteLimit; i++ {
		bad := selectionBody(terminalbroker.ID(), ids[:1])
		if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", bad); code != 422 {
			t.Fatal(i, code, raw)
		}
	}
	for i := 0; i < 3; i++ {
		if code, again, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(request, ids)); code != 200 || again["id"] != runID {
			t.Fatal("replay", i, code, raw)
		}
	}
	if code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/folder-projects/runs", selectionBody(terminalbroker.ID(), ids)); code != 429 || out["code"] != "project_rate_limited" {
		t.Fatal("a new start past the limit", code, raw)
	}
}

// waitUntil polls cond until it holds, failing the test after 20 seconds.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for " + what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}
