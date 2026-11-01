package api

import (
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

func TestSessionBacklogAPIAuthorizationAndValidation(t *testing.T) {
	f := projectFixture(t)
	path := "/api/v2/session-backlog"
	input := map[string]any{"title": "Polish the board", "prompt": "Make the filters clearer", "agentId": f.agent.ID, "projectId": "", "dir": "/home/tester/site", "tool": "claude", "model": "sonnet", "effort": "", "images": []any{}, "version": 0}
	code, out, raw := f.do(t, f.cookie, "POST", path, input)
	if code != 201 {
		t.Fatalf("create: %d %s", code, raw)
	}
	id := out["id"].(string)
	if code, _, raw = f.do(t, f.cookie, "GET", path, nil); code != 200 || !strings.Contains(raw, id) {
		t.Fatalf("list: %d %s", code, raw)
	}
	if code, _, raw = f.do(t, "", "GET", path, nil); code != 401 {
		t.Fatalf("signed out: %d %s", code, raw)
	}
	viewer, err := f.st.CreateUser("backlog-viewer@example.test", "Backlog viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw = f.do(t, terminalLogin(t, f.st, viewer.ID), "GET", path, nil); code != 403 {
		t.Fatalf("viewer: %d %s", code, raw)
	}
	other, err := f.st.CreateUser("other-backlog@example.test", "Other backlog", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw = f.do(t, terminalLogin(t, f.st, other.ID), "GET", path+"/"+id, nil); code != 404 {
		t.Fatalf("cross account: %d %s", code, raw)
	}
	if code, raw = f.raw(t, "POST", path, `{}`, f.cookie, "https://other.example", ""); code != 403 {
		t.Fatalf("origin: %d %s", code, raw)
	}
	for _, change := range []map[string]any{
		{"agentId": "not-my-machine"}, {"model": "-bad"}, {"effort": "extreme"}, {"dir": "relative"}, {"tool": "unknown"}, {"prompt": strings.Repeat("a", 2001)},
		{"images": []store.DraftImage{{Name: "fake.png", Data: "data:image/png;base64,aGVsbG8="}}}, {"projectId": "not-my-project"},
		{"base": "main"}, {"worktree": true, "base": "bad branch"}, {"chat": true}, {"trust": true, "chat": true},
	} {
		bad := map[string]any{}
		for k, v := range input {
			bad[k] = v
		}
		for k, v := range change {
			bad[k] = v
		}
		if code, _, raw = f.do(t, f.cookie, "POST", path, bad); code != 422 {
			t.Fatalf("%v: %d %s", change, code, raw)
		}
	}
	if code, _, raw = f.do(t, f.cookie, "POST", path, `{"unexpected":true}`); code != 400 {
		t.Fatalf("unknown fields: %d %s", code, raw)
	}
	input["version"] = 1
	if code, _, raw = f.do(t, f.cookie, "PUT", path+"/"+id, input); code != 200 {
		t.Fatalf("update: %d %s", code, raw)
	}
	if code, _, raw = f.do(t, f.cookie, "PUT", path+"/"+id, input); code != 409 {
		t.Fatalf("stale update: %d %s", code, raw)
	}
	if code, _, raw = f.do(t, f.cookie, "DELETE", path+"/"+id, map[string]int{"version": 2}); code != 200 {
		t.Fatalf("delete: %d %s", code, raw)
	}
}

func TestSessionBacklogStartUsesOneFrozenTerminalRequest(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	d, err := f.st.SaveSessionDraftFor(f.uid, store.SessionDraft{Title: "Polish the board", Prompt: "Make the filters clearer", AgentID: f.agent.ID, Dir: "/home/tester/site", Tool: "claude", Model: "sonnet"}, true)
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/session-backlog/" + d.ID + "/start"
	input := map[string]any{"version": d.Version, "prompt": d.Prompt}
	code, first, raw := f.do(t, f.cookie, "POST", path, input)
	if code != 200 {
		t.Fatalf("start: %d %s", code, raw)
	}
	input["prompt"] = "A different retry prompt"
	code, second, raw := f.do(t, f.cookie, "POST", path, input)
	if code != 200 || first["id"] != second["id"] {
		t.Fatalf("retry: %d %s", code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Prompt != d.Prompt || sent[0].Model != "sonnet" || sent[0].Dir != d.Dir {
		t.Fatalf("dispatches: %+v", sent)
	}
	list, err := f.st.SessionDraftsFor(f.uid)
	if err != nil || len(list) != 1 || list[0].SessionID != first["id"] {
		t.Fatalf("link to actual session: %+v %v", list, err)
	}
	if len(launchEvents(t, f.st, f.uid)) != 1 {
		t.Fatal("retry audited twice")
	}
}

func TestSessionBacklogRefusedStartCanBeEditedAgain(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("codex"))
	d, err := f.st.SaveSessionDraftFor(f.uid, store.SessionDraft{Title: "Polish the board", Prompt: "Make the filters clearer", AgentID: f.agent.ID, Tool: "claude"}, true)
	if err != nil {
		t.Fatal(err)
	}
	code, _, raw := f.do(t, f.cookie, "POST", "/api/v2/session-backlog/"+d.ID+"/start", map[string]any{"version": d.Version, "prompt": d.Prompt})
	if code != 422 {
		t.Fatalf("refusal: %d %s", code, raw)
	}
	reloaded, err := f.st.SessionDraftFor(f.uid, d.ID)
	if err != nil || reloaded.PreparedPrompt != "" {
		t.Fatalf("remained frozen after refusal: %+v %v", reloaded, err)
	}
	reloaded.Tool = "codex"
	if _, err := f.st.SaveSessionDraftFor(f.uid, reloaded, false); err != nil {
		t.Fatal("cannot correct refused draft", err)
	}
}

func TestSessionBacklogYoloIsConfirmedAndGatedAtStart(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	input := map[string]any{"title": "Risky", "prompt": "Make the filters clearer", "agentId": f.agent.ID, "dir": "/home/tester/site", "tool": "claude", "yolo": true, "version": 0}
	code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/session-backlog", input)
	if code != 201 || out["yolo"] != true {
		t.Fatalf("save keeps the intent without a gate: %d %s", code, raw)
	}
	id := out["id"].(string)
	code, _, raw = f.do(t, f.cookie, "POST", "/api/v2/session-backlog/"+id+"/start", map[string]any{"version": 1, "prompt": "Make the filters clearer"})
	if code != 403 || !strings.Contains(raw, "yolo_not_allowed") {
		t.Fatalf("start must hit the account YOLO gate: %d %s", code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatal("nothing may be dispatched when YOLO is refused")
	}
	d, err := f.st.SessionDraftFor(f.uid, id)
	if err != nil || d.PreparedPrompt != "" || !d.Yolo {
		t.Fatalf("refused start stays editable with its choice: %+v %v", d, err)
	}
}

func TestSessionBacklogIdeaIsAPlaceholderThatCannotStart(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	path := "/api/v2/session-backlog"
	idea := func(over map[string]any) map[string]any {
		in := map[string]any{"title": "Faster filters", "prompt": "", "agentId": f.agent.ID, "dir": "/home/tester/site", "tool": "claude", "idea": true, "version": 0}
		for k, v := range over {
			in[k] = v
		}
		return in
	}
	// A title alone is enough, and so is a note alone (the title is the client's to derive).
	code, out, raw := f.do(t, f.cookie, "POST", path, idea(nil))
	if code != 201 || out["idea"] != true || out["prompt"] != "" {
		t.Fatalf("idea with a title only: %d %s", code, raw)
	}
	id := out["id"].(string)
	for _, bad := range []map[string]any{
		{"title": ""},                 // no title and no note
		{"agentId": "not-my-machine"}, // still against one of this account's machines
		{"idea": false},               // a normal item still needs a prompt
		{"prompt": "-leading dash"},   // a note that is given is still validated
	} {
		if code, _, raw = f.do(t, f.cookie, "POST", path, idea(bad)); code != 422 {
			t.Fatalf("%v: %d %s", bad, code, raw)
		}
	}
	code, _, raw = f.do(t, f.cookie, "POST", path+"/"+id+"/start", map[string]any{"version": 1, "prompt": "Make the filters clearer"})
	if code != 422 || !strings.Contains(raw, "idea_not_startable") {
		t.Fatalf("an idea must not start: %d %s", code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatal("nothing may be dispatched for an idea")
	}
	d, err := f.st.SessionDraftFor(f.uid, id)
	if err != nil || d.PreparedPrompt != "" || !d.Idea {
		t.Fatalf("a refused idea stays an editable idea: %+v %v", d, err)
	}
	// Promoting it: add a prompt, switch the flag off, and it starts.
	code, _, raw = f.do(t, f.cookie, "PUT", path+"/"+id, idea(map[string]any{"prompt": "Make the filters clearer", "idea": false, "version": 1}))
	if code != 200 {
		t.Fatalf("promote: %d %s", code, raw)
	}
	if code, _, raw = f.do(t, f.cookie, "POST", path+"/"+id+"/start", map[string]any{"version": 2, "prompt": "Make the filters clearer"}); code != 200 {
		t.Fatalf("promoted start: %d %s", code, raw)
	}
}
