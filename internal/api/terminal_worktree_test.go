package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	worktreeTestRun     = "0123456789abcdef01234567"
	worktreeTestProject = "my-project"
	worktreeTestPath    = "/home/tester/.reet/worktrees/my-project/site/" + worktreeTestRun
	worktreeTestBranch  = "reet/my-project-" + worktreeTestRun
)

func worktreeCaps(tools ...string) terminalproto.Capabilities {
	caps := launchCaps(tools...)
	caps.Worktree = true
	return caps
}

func (f *launchAPI) worktreeSent() []terminalproto.Body {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]terminalproto.Body(nil), f.worktrees...)
}

func (f *launchAPI) answer(fn func(terminalproto.Body) terminalproto.Body) {
	f.mu.Lock()
	f.worktree = fn
	f.mu.Unlock()
}

func worktreeLaunchBody(agent, request string) map[string]any {
	return launchBody(agent, request, map[string]any{"tool": "claude", "model": "sonnet", "dir": "/home/tester/site", "prompt": launchTestPrompt, "worktree": worktreeTestRun, "project": worktreeTestProject})
}

func worktreeRequest(agent string, extra map[string]any) map[string]any {
	body := map[string]any{"agentId": agent, "project": worktreeTestProject, "repo": "site", "run": worktreeTestRun}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

func freshState() terminalproto.WorktreeState {
	return terminalproto.WorktreeState{Path: worktreeTestPath, Branch: worktreeTestBranch}
}

func worktreeEvents(t *testing.T, st *store.Store, uid int64, action string) []store.Event {
	t.Helper()
	events, err := st.ListEventsFor(uid, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Event
	for _, e := range events {
		if e.Action == action {
			out = append(out, e)
		}
	}
	return out
}

func TestTerminalWorktreeCreateIsStrictAndValidated(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	for name, mutate := range map[string]func(map[string]any){
		"worktree as a switch":    func(b map[string]any) { b["worktree"] = true },
		"worktree without tool":   func(b map[string]any) { delete(b, "tool"); delete(b, "model"); delete(b, "prompt") },
		"worktree without folder": func(b map[string]any) { delete(b, "dir") },
		"worktree without project": func(b map[string]any) {
			delete(b, "project")
		},
		"project without worktree": func(b map[string]any) { delete(b, "worktree") },
		"upper-case run token":     func(b map[string]any) { b["worktree"] = strings.ToUpper(worktreeTestRun) },
		"short run token":          func(b map[string]any) { b["worktree"] = "abc123" },
		"project with a dot":       func(b map[string]any) { b["project"] = "my.project" },
		"project path":             func(b map[string]any) { b["project"] = "../../etc" },
		"branch named":             func(b map[string]any) { b["branch"] = "main" },
	} {
		body := worktreeLaunchBody(f.agent.ID, terminalbroker.ID())
		mutate(body)
		if code, _, raw := f.create(t, body); code != 400 {
			t.Fatal(name, code, raw)
		}
	}
	if len(f.sent()) != 0 {
		t.Fatal("an invalid worktree create reached the connector", f.sent())
	}
}

func TestTerminalWorktreeOnAPeerWithoutTheCapabilityAsksForAnUpdate(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.create(t, worktreeLaunchBody(f.agent.ID, terminalbroker.ID()))
	if code != 409 || out["code"] != "worktree_unsupported" || out["error"] != "Update reet on that machine to start agents with worktrees." {
		t.Fatal(code, raw)
	}
	for _, path := range []string{"/api/v2/terminal/worktrees/status", "/api/v2/terminal/worktrees/remove"} {
		code, out, raw := f.do(t, f.cookie, "POST", path, worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}))
		if path == "/api/v2/terminal/worktrees/status" {
			code, out, raw = f.do(t, f.cookie, "POST", path, worktreeRequest(f.agent.ID, nil))
		}
		if code != 409 || out["code"] != "worktree_unsupported" {
			t.Fatal(path, code, raw)
		}
	}
	if len(f.sent()) != 0 || len(f.worktreeSent()) != 0 {
		t.Fatal("worktree fields were sent to an old peer", f.sent(), f.worktreeSent())
	}
	// The same machine still starts an agent without a worktree.
	body := worktreeLaunchBody(f.agent.ID, terminalbroker.ID())
	delete(body, "worktree")
	delete(body, "project")
	if code, out, raw := f.create(t, body); code != 200 || out["state"] != "running" {
		t.Fatal("launch on a launch-only peer", code, raw)
	}
}

func TestTerminalWorktreeCreateForwardsAndAuditsWithoutThePrompt(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.reply = func(in terminalproto.Create) terminalproto.Body {
		if in.Worktree != "" {
			return terminalproto.SessionChanged{State: terminalproto.Running, WorktreePath: worktreeTestPath, WorktreeRepo: "site"}
		}
		return terminalproto.SessionChanged{State: terminalproto.Running}
	}
	request := terminalbroker.ID()
	body := worktreeLaunchBody(f.agent.ID, request)
	code, first, raw := f.create(t, body)
	if code != 200 || first["state"] != "running" || first["worktreePath"] != worktreeTestPath || first["worktreeRepo"] != "site" || first["worktreeBranch"] != worktreeTestBranch {
		t.Fatal(code, raw)
	}
	want := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Model: "sonnet", Dir: "/home/tester/site", Prompt: launchTestPrompt, Worktree: worktreeTestRun, Project: worktreeTestProject}
	if sent := f.sent(); len(sent) != 1 || sent[0] != want {
		t.Fatal("worktree create not forwarded exactly", sent)
	}
	if code, again, raw := f.create(t, body); code != 200 || again["id"] != first["id"] || len(f.sent()) != 1 {
		t.Fatal("idempotent retry", code, raw, len(f.sent()))
	}
	changed := worktreeLaunchBody(f.agent.ID, request)
	changed["worktree"] = "fedcba9876543210fedcba98"
	if code, _, raw := f.create(t, changed); code != 409 {
		t.Fatal("changed run token on the same request", code, raw)
	}
	events := launchEvents(t, f.st, f.uid)
	if len(events) != 1 {
		t.Fatal("launch audited", len(events), "times")
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["tool"] != "claude" || detail["dir"] != "/home/tester/site" || detail["worktreeRun"] != worktreeTestRun || detail["project"] != worktreeTestProject || detail["branch"] != worktreeTestBranch || detail["worktree"] != worktreeTestPath {
		t.Fatal("launch audit detail", events[0].Detail)
	}
	all, _ := f.st.ListEventsFor(f.uid, 100, "")
	everything, _ := json.Marshal(all)
	if strings.Contains(string(everything), "flaky login") {
		t.Fatal("the opening prompt was written to audit")
	}
	// A launch without a worktree keeps its old audit shape.
	plain := worktreeLaunchBody(f.agent.ID, terminalbroker.ID())
	delete(plain, "worktree")
	delete(plain, "project")
	if code, _, raw := f.create(t, plain); code != 200 {
		t.Fatal(code, raw)
	}
	for _, e := range launchEvents(t, f.st, f.uid) {
		if !strings.Contains(e.Detail, "worktreeRun") && (strings.Contains(e.Detail, "branch") || strings.Contains(e.Detail, "project")) {
			t.Fatal("plain launch audit gained worktree fields", e.Detail)
		}
	}
}

func TestTerminalWorktreeCreateRefusalsMapToSentences(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	for _, tc := range []struct {
		reason   terminalproto.WorktreeReason
		detail   string
		code     string
		sentence string
	}{
		{terminalproto.WorktreeNotRepoRoot, "", "not_repo_root", "That folder isn't the top of a git repository."},
		{terminalproto.WorktreeRepoBusy, "", "repo_busy", "That repository is in the middle of a merge, rebase or similar. Finish or abort it first."},
		{terminalproto.WorktreeExists, "", "worktree_exists", "A worktree or branch with this run's name is already on that machine, so nothing was started."},
		{terminalproto.WorktreeGitFailed, "fatal: invalid reference: HEAD", "git_failed", "Git couldn't make a worktree there."},
		{terminalproto.WorktreeOutsideHome, "", "outside_home", "The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again."},
	} {
		f.reply = func(terminalproto.Create) terminalproto.Body {
			return terminalproto.Result{Code: terminalproto.WorktreeRefused, Reason: tc.reason, Detail: tc.detail}
		}
		request := terminalbroker.ID()
		code, out, raw := f.create(t, worktreeLaunchBody(f.agent.ID, request))
		if code != 422 || out["code"] != tc.code || out["error"] != tc.sentence {
			t.Fatal(tc.reason, code, raw)
		}
		if tc.detail != "" && out["detail"] != tc.detail {
			t.Fatal(tc.reason, "detail", raw)
		}
		if tc.detail == "" && out["detail"] != nil {
			t.Fatal(tc.reason, "unexpected detail", raw)
		}
		// A retry of the same request reports the same refusal without resending it.
		sent := len(f.sent())
		code, out, raw = f.create(t, worktreeLaunchBody(f.agent.ID, request))
		if code != 422 || out["code"] != tc.code || out["error"] != tc.sentence || tc.detail != "" && out["detail"] != tc.detail || len(f.sent()) != sent {
			t.Fatal(tc.reason, "replay", code, raw)
		}
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("a refused worktree create was audited")
	}
}

func TestTerminalWorktreeStatusReportsEveryField(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	state := terminalproto.WorktreeState{Path: worktreeTestPath, Branch: worktreeTestBranch, Changed: 3, Unpushed: 2, Paths: []string{"a.go", "docs/b.md", "c"}, Unreferenced: 1, Hidden: 4, Nested: true, Incomplete: true}
	f.answer(func(terminalproto.Body) terminalproto.Body { return state })
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", worktreeRequest(f.agent.ID, nil))
	if code != 200 {
		t.Fatal(code, raw)
	}
	for k, v := range map[string]any{"path": worktreeTestPath, "branch": worktreeTestBranch, "changed": 3.0, "unpushed": 2.0, "unreferenced": 1.0, "hidden": 4.0, "nested": true, "incomplete": true} {
		if out[k] != v {
			t.Fatal(k, raw)
		}
	}
	if paths, ok := out["paths"].([]any); !ok || len(paths) != 3 || paths[1] != "docs/b.md" {
		t.Fatal("paths", raw)
	}
	// A fresh worktree reports every field, zero and empty included.
	f.answer(func(terminalproto.Body) terminalproto.Body { return freshState() })
	_, _, raw = f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", worktreeRequest(f.agent.ID, nil))
	for _, field := range []string{`"changed":0`, `"unpushed":0`, `"unreferenced":0`, `"hidden":0`, `"nested":false`, `"incomplete":false`, `"paths":[]`} {
		if !strings.Contains(raw, field) {
			t.Fatal(field, raw)
		}
	}
	if sent := f.worktreeSent(); len(sent) != 2 || sent[0] != (terminalproto.WorktreeStatus{Run: worktreeTestRun, Project: worktreeTestProject, Repo: "site"}) {
		t.Fatal("status request", sent)
	}
	// Status never writes audit.
	if len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) != 0 {
		t.Fatal("status was audited")
	}
}

func TestTerminalWorktreeRequestsAreStrict(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(terminalproto.Body) terminalproto.Body { return freshState() })
	for name, body := range map[string]any{
		"unknown field":     worktreeRequest(f.agent.ID, map[string]any{"path": "/etc"}),
		"bad run":           worktreeRequest(f.agent.ID, map[string]any{"run": "zz"}),
		"bad project":       worktreeRequest(f.agent.ID, map[string]any{"project": "My Project"}),
		"repo path":         worktreeRequest(f.agent.ID, map[string]any{"repo": "../site"}),
		"repo flag":         worktreeRequest(f.agent.ID, map[string]any{"repo": "--force"}),
		"no repo":           worktreeRequest(f.agent.ID, map[string]any{"repo": ""}),
		"counts for status": worktreeRequest(f.agent.ID, map[string]any{"changed": 0}),
		"trailing data":     `{"agentId":"x","project":"p","repo":"r","run":"` + worktreeTestRun + `"} {}`,
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", body); code != 400 {
			t.Fatal("status", name, code, raw)
		}
	}
	for name, body := range map[string]any{
		"no counts":      worktreeRequest(f.agent.ID, nil),
		"no unpushed":    worktreeRequest(f.agent.ID, map[string]any{"changed": 0}),
		"negative":       worktreeRequest(f.agent.ID, map[string]any{"changed": -1, "unpushed": 0}),
		"fraction":       worktreeRequest(f.agent.ID, map[string]any{"changed": 0.5, "unpushed": 0}),
		"force":          worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0, "force": true}),
		"bad run":        worktreeRequest(f.agent.ID, map[string]any{"run": "zz", "changed": 0, "unpushed": 0}),
		"deletes branch": worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0, "deleteBranch": true}),
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", body); code != 400 {
			t.Fatal("remove", name, code, raw)
		}
	}
	if len(f.worktreeSent()) != 0 {
		t.Fatal("an invalid worktree request reached the connector", f.worktreeSent())
	}
}

func TestTerminalWorktreeRoutesAreBlockedForViewersLegacyAuthAndOtherOrigins(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(terminalproto.Body) terminalproto.Body { return freshState() })
	viewer, err := f.st.CreateUser("worktree-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	viewerCookie := terminalLogin(t, f.st, viewer.ID)
	routes := map[string]map[string]any{
		"/api/v2/terminal/sessions":         worktreeLaunchBody(f.agent.ID, terminalbroker.ID()),
		"/api/v2/terminal/worktrees/status": worktreeRequest(f.agent.ID, nil),
		"/api/v2/terminal/worktrees/remove": worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}),
	}
	for path, body := range routes {
		if code, _, raw := f.do(t, viewerCookie, "POST", path, body); code != 403 {
			t.Fatal("viewer", path, code, raw)
		}
		if code, _, raw := f.do(t, "", "POST", path, body); code != 401 && code != 403 {
			t.Fatal("signed out", path, code, raw)
		}
		data, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", path, strings.NewReader(string(data)))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://elsewhere.example")
		r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
		w := httptest.NewRecorder()
		f.srv.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("other origin", path, w.Code, w.Body.String())
		}
	}
	for _, mode := range []AuthMode{AuthNone, AuthToken} {
		f.srv.s.authMode = mode
		for path, body := range routes {
			if code, _, raw := f.do(t, f.cookie, "POST", path, body); code != 403 {
				t.Fatal("legacy auth", mode, path, code, raw)
			}
		}
	}
	if len(f.sent()) != 0 || len(f.worktreeSent()) != 0 {
		t.Fatal("a blocked request reached the connector", f.sent(), f.worktreeSent())
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 || len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) != 0 {
		t.Fatal("a blocked request was audited")
	}
}

func TestTerminalWorktreeRemoveRefusalsMapToSentences(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	for _, tc := range []struct {
		name     string
		reply    terminalproto.WorktreeResult
		status   int
		code     string
		sentence string
	}{
		{"changed", terminalproto.WorktreeResult{Code: terminalproto.WorktreeChanged}, 409, "worktree_changed", "That worktree changed since you looked. Check again."},
		{"has changes", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHasChanges}, 422, "has_changes", "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again."},
		{"unreferenced", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeUnreferencedCommits}, 422, "unreferenced_commits", "Some commits in this worktree are not on any branch. Put them on a branch first, then check again."},
		{"nested", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeNestedRepository}, 422, "nested_repository", "This worktree contains another git repository. Move or remove it first."},
		{"hidden", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeHiddenChanges}, 422, "hidden_changes", "Git is not tracking some edits in this worktree. Review them before removing."},
		{"incomplete", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeCheckIncomplete}, 422, "check_incomplete", "reet couldn't check all of this worktree. Remove it yourself."},
		{"not registered", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeNotRegistered}, 422, "not_registered", "Git doesn't list that folder as this run's worktree, so reet won't touch it."},
		{"outside home", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeOutsideHome}, 422, "outside_home", "The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again."},
		{"git failed", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "fatal: worktree is locked"}, 422, "git_failed", "Git couldn't remove that worktree."},
		{"not found", terminalproto.WorktreeResult{Code: terminalproto.NotFound}, 404, "not_found", "That worktree isn't on that machine. It may have been removed already."},
		{"unexpected reason", terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeRepoBusy}, 422, "worktree_refused", "That machine refused to remove the worktree."},
		{"denied", terminalproto.WorktreeResult{Code: terminalproto.Denied}, 403, "", "terminal access denied"},
		{"busy", terminalproto.WorktreeResult{Code: terminalproto.Conflict}, 409, "worktree_busy", "That machine is already working on a worktree. Try again in a minute."},
		{"unavailable", terminalproto.WorktreeResult{Code: terminalproto.Unavailable}, 503, "worktree_unavailable", "That machine couldn't do that just now. Try again in a moment."},
		{"failed", terminalproto.WorktreeResult{Code: terminalproto.Failed}, 503, "worktree_unavailable", "That machine couldn't do that just now. Try again in a moment."},
	} {
		f.answer(func(body terminalproto.Body) terminalproto.Body {
			if _, ok := body.(terminalproto.WorktreeStatus); ok {
				t.Error("remove asked for status first")
				return terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
			}
			return tc.reply
		})
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}))
		if code != tc.status || out["error"] != tc.sentence || tc.code != "" && out["code"] != tc.code {
			t.Fatal(tc.name, code, raw)
		}
		if tc.reply.Detail != "" && out["detail"] != tc.reply.Detail {
			t.Fatal(tc.name, "detail", raw)
		}
	}
	// Status refusals use the same words, with git's failure worded for a check.
	for _, tc := range []struct {
		reply    terminalproto.WorktreeResult
		status   int
		code     string
		sentence string
	}{
		{terminalproto.WorktreeResult{Code: terminalproto.NotFound}, 404, "not_found", "That worktree isn't on that machine. It may have been removed already."},
		{terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeGitFailed, Detail: "git is not installed"}, 422, "git_failed", "Git couldn't check that worktree."},
		{terminalproto.WorktreeResult{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeNotRegistered}, 422, "not_registered", "Git doesn't list that folder as this run's worktree, so reet won't touch it."},
		{terminalproto.WorktreeResult{Code: terminalproto.Conflict}, 409, "worktree_busy", "That machine is already working on a worktree. Try again in a minute."},
		{terminalproto.WorktreeResult{Code: terminalproto.Capacity}, 503, "worktree_unavailable", "That machine couldn't do that just now. Try again in a moment."},
	} {
		f.answer(func(terminalproto.Body) terminalproto.Body { return tc.reply })
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", worktreeRequest(f.agent.ID, nil))
		if code != tc.status || out["code"] != tc.code || out["error"] != tc.sentence {
			t.Fatal("status", tc.code, code, raw)
		}
	}
	if len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) != 0 {
		t.Fatal("a refused remove was audited")
	}
}

func TestTerminalWorktreeRemoveAuditsOnlyASuccessfulRemoval(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.WorktreeStatus); ok {
			t.Error("remove asked for status first")
			return terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
		}
		return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: worktreeTestPath, Branch: worktreeTestBranch}
	})
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 2}))
	if code != 200 || out["ok"] != true || out["path"] != worktreeTestPath || out["branch"] != worktreeTestBranch {
		t.Fatal(code, raw)
	}
	sent := f.worktreeSent()
	if len(sent) != 1 || sent[0] != (terminalproto.WorktreeRemove{Run: worktreeTestRun, Project: worktreeTestProject, Repo: "site", Changed: 0, Unpushed: 2}) {
		t.Fatal("remove request", sent)
	}
	events := worktreeEvents(t, f.st, f.uid, "worktree-remove")
	if len(events) != 1 || events[0].Source != "ui" {
		t.Fatal("remove audited", events)
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["machine"] != f.agent.ID || detail["host"] != f.agent.Host || detail["path"] != worktreeTestPath || detail["branch"] != worktreeTestBranch || len(detail) != 4 {
		t.Fatal("remove audit detail", events[0].Detail)
	}
}

func TestTerminalWorktreeCapabilityReachesTheWeb(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	code, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if code != 200 || !strings.Contains(raw, `"worktree":true`) {
		t.Fatal("terminal machines", code, raw)
	}
	r := httptest.NewRequest("GET", "/api/v2/machines/health", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"worktree":true`) {
		t.Fatal("machine health", w.Code, w.Body.String())
	}
	old := launchAPIFixture(t, launchCaps("claude"))
	_, _, raw = old.do(t, old.cookie, "GET", "/api/v2/terminal/machines", nil)
	if strings.Contains(raw, `"worktree"`) {
		t.Fatal("old machine reported worktree", raw)
	}
}

func TestPrivateControlAdvertisesWorktreeBesideLaunch(t *testing.T) {
	srv, _, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	c, response, err := websocket.Dial(t.Context(), private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	features := response.Header.Get(terminalproto.FeatureHeader)
	if features != "command-history-v1,launch-v1,resume-v1,worktree-v1,browse-v1,branches-v1,clipboard-v1,upload-v1,models-v1,acp-v1,acp-history-v1,acp-approval-v1,acp-tool-details-v1,acp-image-v1,shell-dir-v1,attention-v1,software-v1,"+terminalproto.ProgressFeature+","+terminalproto.AgentsFeature || !terminalproto.SupportsResume(features) || !terminalproto.SupportsWorktree(features) || !terminalproto.SupportsClipboard(features) || !terminalproto.SupportsUpload(features) || !terminalproto.SupportsModels(features) || !terminalproto.SupportsACPApproval(features) || !terminalproto.SupportsACPImages(features) || !terminalproto.SupportsAttention(features) || !terminalproto.SupportsSoftware(features) {
		t.Fatal("features", features)
	}
}

// When a remove's reply is lost, the controller asks for status once: not
// found means the remove happened, and it is audited as confirmed by status.
func TestTerminalWorktreeRemoveWithALostReplyIsConfirmedByStatus(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	for _, tc := range []struct {
		name   string
		status terminalproto.Body
		code   int
		audits int
	}{
		{"gone", terminalproto.WorktreeResult{Code: terminalproto.NotFound}, 200, 1},
		{"still there", freshState(), 503, 1},
		{"busy", terminalproto.WorktreeResult{Code: terminalproto.Conflict}, 503, 1},
	} {
		f.answer(func(body terminalproto.Body) terminalproto.Body {
			if _, ok := body.(terminalproto.WorktreeRemove); ok {
				return nil // the reply never comes
			}
			return tc.status
		})
		ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
		code, out, raw := f.doCtx(t, ctx, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}))
		cancel()
		if code != tc.code {
			t.Fatal(tc.name, code, raw)
		}
		if code == 200 && (out["ok"] != true || out["branch"] != worktreeTestBranch || out["confirmedBy"] != "status" || out["path"] != nil) {
			t.Fatal(tc.name, raw)
		}
		if events := worktreeEvents(t, f.st, f.uid, "worktree-remove"); len(events) != tc.audits {
			t.Fatal(tc.name, "audit rows", len(events))
		}
	}
	events := worktreeEvents(t, f.st, f.uid, "worktree-remove")
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["confirmedBy"] != "status" || detail["branch"] != worktreeTestBranch || detail["machine"] != f.agent.ID || detail["path"] != nil {
		t.Fatal("confirmed remove audit", events[0].Detail)
	}
}

// Another account naming this account's machine is refused on all three
// worktree routes, and nothing reaches the machine.
func TestTerminalWorktreeRoutesAreScopedToTheOwner(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(terminalproto.Body) terminalproto.Body { return freshState() })
	other, err := f.st.CreateUser("other-owner@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	cookie := terminalLogin(t, f.st, other.ID)
	for path, body := range map[string]map[string]any{
		"/api/v2/terminal/sessions":         worktreeLaunchBody(f.agent.ID, terminalbroker.ID()),
		"/api/v2/terminal/worktrees/status": worktreeRequest(f.agent.ID, nil),
		"/api/v2/terminal/worktrees/remove": worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}),
	} {
		if code, _, raw := f.do(t, cookie, "POST", path, body); code == 200 || code < 400 {
			t.Fatal("another account", path, code, raw)
		}
	}
	if len(f.sent()) != 0 || len(f.worktreeSent()) != 0 {
		t.Fatal("another account's request reached the machine", f.sent(), f.worktreeSent())
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 || len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) != 0 || len(worktreeEvents(t, f.st, other.ID, "worktree-remove")) != 0 {
		t.Fatal("another account's request was audited")
	}
	// The owner still reaches it.
	if code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/status", worktreeRequest(f.agent.ID, nil)); code != 200 {
		t.Fatal("owner", code, raw)
	}
}

// A machine that answers about a different folder or branch is not
// believed: nothing is recorded or audited and the person is asked to check.
func TestTerminalWorktreeAnswersThatDoNotMatchAreRefused(t *testing.T) {
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(terminalproto.Body) terminalproto.Body {
		return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: "/home/tester/elsewhere", Branch: "main"}
	})
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}))
	if code != 503 || out["code"] != "worktree_mismatch" || out["error"] != "That machine's answer did not match the request. Check again." {
		t.Fatal("remove", code, raw)
	}
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, WorktreePath: "/home/tester/elsewhere", WorktreeRepo: "site"}
	}
	code, out, raw = f.create(t, worktreeLaunchBody(f.agent.ID, terminalbroker.ID()))
	if code != 503 || out["code"] != "worktree_mismatch" {
		t.Fatal("create", code, raw)
	}
	if len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) != 0 || len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("a mismatched answer was audited")
	}
}

// A remove's ok answer that arrives after the controller gave up is still
// audited, marked late.
func TestTerminalWorktreeLateRemoveIsAudited(t *testing.T) {
	ordinary := terminalbroker.WorktreeCallTimeout
	terminalbroker.WorktreeCallTimeout = 200 * time.Millisecond
	t.Cleanup(func() { terminalbroker.WorktreeCallTimeout = ordinary })
	f := launchAPIFixture(t, worktreeCaps("claude"))
	f.answer(func(body terminalproto.Body) terminalproto.Body {
		if _, ok := body.(terminalproto.WorktreeRemove); ok {
			return nil
		}
		return terminalproto.WorktreeResult{Code: terminalproto.Conflict} // the slot is still held
	})
	f.mu.Lock()
	f.late = func(m terminalproto.Message) {
		go func() {
			time.Sleep(600 * time.Millisecond)
			m.Body = &terminalproto.WorktreeResult{Code: terminalproto.OK, Path: worktreeTestPath, Branch: worktreeTestBranch}
			_ = f.peer.Receive(m)
		}()
	}
	f.mu.Unlock()
	code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/worktrees/remove", worktreeRequest(f.agent.ID, map[string]any{"changed": 0, "unpushed": 0}))
	if code != 503 {
		t.Fatal("the remove's answer was lost and status was busy", code, raw)
	}
	deadline := time.Now().Add(3 * time.Second)
	for len(worktreeEvents(t, f.st, f.uid, "worktree-remove")) == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	events := worktreeEvents(t, f.st, f.uid, "worktree-remove")
	if len(events) != 1 {
		t.Fatal("late removal audited", len(events))
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["late"] != "true" || detail["path"] != worktreeTestPath || detail["branch"] != worktreeTestBranch || detail["machine"] != f.agent.ID || detail["host"] != f.agent.Host {
		t.Fatal("late removal audit", events[0].Detail)
	}
}

// A YOLO session that starts but reports the wrong worktree is still audited:
// the refusal reaches the caller, but the launch happened.
func TestYoloWorktreeMismatchStillAuditsTheLaunch(t *testing.T) {
	caps := worktreeCaps("cline")
	caps.AgentsV2 = true
	caps.Yolo = true
	f := launchAPIFixture(t, caps)
	if err := f.st.SetSetting(yoloSettingKey, "1"); err != nil {
		t.Fatal(err)
	}
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, WorktreePath: "/home/tester/elsewhere", WorktreeRepo: "site"}
	}
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "cline", "dir": "/home/tester/site", "prompt": launchTestPrompt, "worktree": worktreeTestRun, "project": worktreeTestProject, "yolo": true,
	}))
	if code != 503 || out["code"] != "worktree_mismatch" {
		t.Fatal(code, raw)
	}
	events := worktreeEvents(t, f.st, f.uid, "yolo-launch")
	if len(events) != 1 {
		t.Fatalf("yolo-launch rows: %v", events)
	}
	if len(launchEvents(t, f.st, f.uid)) != 0 {
		t.Fatal("a mismatched answer wrote a terminal-launch row")
	}
}
