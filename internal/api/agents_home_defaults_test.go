package api

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

func TestProfileDefaultsRoundTrip(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/profile", nil)
	profile, _ := out["profile"].(map[string]any)
	if code != 200 || profile["worktreesByDefault"] != true || profile["defaultBaseBranch"] != "" {
		t.Fatal("new person", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultBaseBranch": "release/2.0", "worktreesByDefault": false})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["worktreesByDefault"] != false || profile["defaultBaseBranch"] != "release/2.0" {
		t.Fatal("save", code, raw)
	}
	// Leaving a field out keeps it; the other settings are untouched.
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"theme": "dark"})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["worktreesByDefault"] != false || profile["defaultBaseBranch"] != "release/2.0" || profile["theme"] != "dark" {
		t.Fatal("partial save", code, raw)
	}
	for _, bad := range []string{"..", "-main", "a b", "main.lock", "refs//x", "x~1", strings.Repeat("b", 256)} {
		code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultBaseBranch": bad, "worktreesByDefault": true})
		if code != 400 || !strings.Contains(out["error"].(string), "defaultBaseBranch") {
			t.Fatal("invalid ref accepted", bad, code, raw)
		}
	}
	// A refused save changes nothing, not even the other field it carried.
	if p, err := f.st.ProfileFor(f.uid); err != nil || p.DefaultBaseBranch != "release/2.0" || p.WorktreesByDefault {
		t.Fatal("refused save applied", err, p)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultBaseBranch": "", "worktreesByDefault": true})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["worktreesByDefault"] != true || profile["defaultBaseBranch"] != "" {
		t.Fatal("clear", code, raw)
	}
}

func TestSessionListingCarriesLaunchMetadataNotPrompt(t *testing.T) {
	var logs bytes.Buffer
	original := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(original) })
	f := launchAPIFixture(t, launchCaps("claude"))
	f.srv.s.debug = true
	project, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/site"})
	if err != nil {
		t.Fatal(err)
	}
	body := launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "model": "sonnet", "dir": "/home/tester/site", "prompt": launchTestPrompt, "projectId": project.ID})
	code, created, raw := f.create(t, body)
	if code != 200 || created["projectId"] != project.ID || strings.Contains(raw, "flaky login") {
		t.Fatal("create", code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].ProjectID != "" || sent[0].Prompt != launchTestPrompt {
		t.Fatal("the project id is controller-only; the prompt still reaches the machine", sent)
	}
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
	sessions, _ := out["sessions"].([]any)
	if code != 200 || len(sessions) != 1 {
		t.Fatal(code, raw)
	}
	s := sessions[0].(map[string]any)
	if s["tool"] != "claude" || s["model"] != "sonnet" || s["effort"] != "" || s["yolo"] != false || s["projectId"] != project.ID {
		t.Fatal("listing", raw)
	}
	if _, has := s["worktreeBranch"]; has {
		t.Fatal("listing", raw)
	}
	if strings.Contains(raw, "flaky login") || strings.Contains(raw, `"prompt"`) {
		t.Fatal("the prompt reached the listing", raw)
	}
	events, _ := f.st.ListEventsFor(f.uid, 100, "")
	everything, _ := json.Marshal(events)
	if strings.Contains(string(everything), "flaky login") || strings.Contains(logs.String(), "flaky login") {
		t.Fatal("the prompt reached audit or logs")
	}
	if launch := launchEvents(t, f.st, f.uid); len(launch) != 1 || !strings.Contains(launch[0].Detail, project.ID) {
		t.Fatal("launch audit lacks the project id", launch)
	}
}

func TestCreateWithUnknownProjectIDRefused(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	other, err := f.st.CreateUser("other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := f.st.CreateFolderProjectFor(other.ID, store.FolderProject{Host: f.agent.ID, Dir: "/home/other/site"})
	if err != nil {
		t.Fatal(err)
	}
	var replies []string
	for _, id := range []string{"0123456789abcdef0123456789abcdef", theirs.ID} {
		code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "projectId": id}))
		if code != 404 || out["code"] != "project_not_found" {
			t.Fatal(id, code, raw)
		}
		replies = append(replies, raw)
	}
	// The project is checked before the machine: an offline machine gives the
	// same refusal, not "reconnecting".
	offline, _, err := f.st.EnrollSyncAgent(f.uid, "desk", "test")
	if err != nil {
		t.Fatal(err)
	}
	code, _, raw := f.create(t, launchBody(offline.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "projectId": theirs.ID}))
	if code != 404 {
		t.Fatal("offline machine", code, raw)
	}
	replies = append(replies, raw)
	if replies[0] != replies[1] || replies[1] != replies[2] {
		t.Fatal("another account's project is told apart from an unknown one", replies)
	}
	if len(f.sent()) != 0 {
		t.Fatal("a refused create reached the machine")
	}
	if all, _ := f.st.TerminalSessionsFor(f.uid); len(all) != 0 {
		t.Fatal("a refused create reserved a session")
	}
}

func TestCreateWithProjectIDOnAnotherMachineRefused(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	elsewhere, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: "server", Dir: "/srv/site"})
	if err != nil {
		t.Fatal(err)
	}
	code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "projectId": elsewhere.ID}))
	if code != 422 || out["code"] != "project_machine_mismatch" {
		t.Fatal(code, raw)
	}
	if len(f.sent()) != 0 {
		t.Fatal("a refused create reached the machine")
	}
	// A project names its machine by enrollment id; the machine's name is not
	// its key.
	named, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.Host, Dir: "/srv/named"})
	if err != nil {
		t.Fatal(err)
	}
	if code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"projectId": named.ID})); code != 422 || out["code"] != "project_machine_mismatch" {
		t.Fatal("project by machine name", code, raw)
	}
	here, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/srv/site"})
	if err != nil {
		t.Fatal(err)
	}
	// A session is in a project only in its folder (see TestCreateWithProjectIDInAnotherFolderRefused).
	inside := map[string]any{"tool": "claude", "dir": "/srv/site", "projectId": here.ID}
	if code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), inside)); code != 200 || out["projectId"] != here.ID {
		t.Fatal("an agent in a project", code, raw)
	}
	// Renaming the machine keeps its id, and its projects still launch.
	if err := f.st.MergeMachinesFor(f.uid, f.agent.Host, "work-laptop"); err != nil {
		t.Fatal(err)
	}
	if code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), inside)); code != 200 || out["projectId"] != here.ID {
		t.Fatal("after a rename", code, raw)
	}
}

// A create whose reply was lost is resent with the same request id. If the
// project was removed meanwhile, the resend still resolves to the session that
// started, rather than being refused by the project check.
func TestCreateRetryAfterTheProjectIsRemoved(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	project, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/site"})
	if err != nil {
		t.Fatal(err)
	}
	body := launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "dir": "/home/tester/site", "projectId": project.ID})
	code, first, raw := f.create(t, body)
	if code != 200 {
		t.Fatal("create", code, raw)
	}
	if err := f.st.DeleteFolderProjectFor(f.uid, project.ID, 0); err != nil {
		t.Fatal(err)
	}
	code, again, raw := f.create(t, body)
	if code != 200 || again["id"] != first["id"] || again["projectId"] != project.ID {
		t.Fatal("retry", code, raw)
	}
	if all, _ := f.st.TerminalSessionsFor(f.uid); len(all) != 1 {
		t.Fatal("a retry made a second session", all)
	}
	// A new request for the removed project is still refused.
	if code, out, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "projectId": project.ID})); code != 404 || out["code"] != "project_not_found" {
		t.Fatal("new request", code, raw)
	}
}
