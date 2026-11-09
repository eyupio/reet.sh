package api

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestProfileDefaultsRoundTrip(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/profile", nil)
	profile, _ := out["profile"].(map[string]any)
	if code != 200 || profile["gafferAnimations"] != true || profile["worktreesByDefault"] != true || profile["defaultBaseBranch"] != "" {
		t.Fatal("new person", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultBaseBranch": "release/2.0", "worktreesByDefault": false, "gafferAnimations": false})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferAnimations"] != false || profile["worktreesByDefault"] != false || profile["defaultBaseBranch"] != "release/2.0" {
		t.Fatal("save", code, raw)
	}
	// Leaving a field out keeps it; the other settings are untouched.
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"theme": "dark"})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferAnimations"] != false || profile["worktreesByDefault"] != false || profile["defaultBaseBranch"] != "release/2.0" || profile["theme"] != "dark" {
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
	// The Gaffer card starts as one line; a saved full-card choice survives other saves and can be turned back.
	if profile["gafferCompact"] != true {
		t.Fatal("compact default", profile)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"gafferCompact": false})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferCompact"] != false {
		t.Fatal("save full Gaffer card", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"theme": "dark"})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferCompact"] != false {
		t.Fatal("full Gaffer card lost by an unrelated save", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"gafferCompact": true, "gafferAnimations": true})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferCompact"] != true || profile["gafferAnimations"] != true {
		t.Fatal("resume one-line Gaffer card and animation", code, raw)
	}
	code, out, raw = f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"gafferAnimations": true})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferAnimations"] != true {
		t.Fatal("resume Gaffer animation", code, raw)
	}
	viewer, err := f.st.CreateUser("gaffer-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	code, out, raw = f.do(t, terminalLogin(t, f.st, viewer.ID), "PATCH", "/api/v2/profile", map[string]any{"gafferAnimations": false})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["gafferAnimations"] != false {
		t.Fatal("viewer preference", code, raw)
	}
	if owner, err := f.st.ProfileFor(f.uid); err != nil || !owner.GafferAnimations {
		t.Fatal("viewer preference changed owner", err, owner)
	}
}

func TestSessionListingCarriesLaunchMetadataAndOnlyAnExcerptOfThePrompt(t *testing.T) {
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
	// The reply is the session as the listing will show it: the prompt only as
	// its excerpt, which for a prompt this short is all of it.
	if code != 200 || created["projectId"] != project.ID || created["promptExcerpt"] != launchTestPrompt {
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
	if s["promptExcerpt"] != launchTestPrompt || strings.Contains(raw, "promptMore") || strings.Contains(raw, `"prompt"`) {
		t.Fatal("the listing carries the prompt only as its excerpt", raw)
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

// The default Cline source is a provider id Cline reports: blank clears it, an
// id that fails the model-name rule is refused without changing anything, and a
// read-only account cannot set it. The audit row names the value.
func TestProfileDefaultClineSource(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	patch := func(cookie string, body map[string]any) (int, map[string]any, string) {
		return f.do(t, cookie, "PATCH", "/api/v2/profile", body)
	}
	code, out, raw := patch(f.cookie, map[string]any{"defaultClineSource": "cline-pass"})
	profile, _ := out["profile"].(map[string]any)
	if code != 200 || profile["defaultClineSource"] != "cline-pass" {
		t.Fatal("save", code, raw)
	}
	// Leaving the field out keeps it.
	code, out, raw = patch(f.cookie, map[string]any{"theme": "dark"})
	profile, _ = out["profile"].(map[string]any)
	if code != 200 || profile["defaultClineSource"] != "cline-pass" {
		t.Fatal("partial save", code, raw)
	}
	for _, bad := range []string{"bad value", "-lead", strings.Repeat("s", 101)} {
		code, out, raw = patch(f.cookie, map[string]any{"defaultClineSource": bad, "theme": "light"})
		if code != 400 || !strings.Contains(out["error"].(string), "defaultClineSource") {
			t.Fatal("invalid source accepted", bad, code, raw)
		}
	}
	// A refused save changes nothing, not even the field it carried.
	if p, err := f.st.ProfileFor(f.uid); err != nil || p.DefaultClineSource != "cline-pass" || p.Theme != "dark" {
		t.Fatal("refused save applied", err, p)
	}
	viewer, err := f.st.CreateUser("source-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw := patch(terminalLogin(t, f.st, viewer.ID), map[string]any{"defaultClineSource": "cline"}); code != 403 {
		t.Fatal("a viewer set a launch default", code, raw)
	}
	if code, out, raw = patch(f.cookie, map[string]any{"defaultClineSource": ""}); code != 200 || out["profile"].(map[string]any)["defaultClineSource"] != "" {
		t.Fatal("clear", code, raw)
	}
	found := false
	for _, e := range projectEvents(t, f.st, f.uid, "profile") {
		found = found || e["defaultClineSource"] == "cline-pass"
	}
	if !found {
		t.Fatal("the audit row did not name the saved source")
	}
}

// An older browser can still send a retired agent as a default: it is refused as
// an unknown agent and nothing is stored.
func TestRetiredToolIsNotAcceptedAsADefault(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	code, out, raw := f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"defaultTool": "gemini"})
	if code != 400 || !strings.Contains(out["error"].(string), "defaultTool must be blank or one of the agents reet can start") {
		t.Fatal("a retired default agent was accepted", code, raw)
	}
	if p, err := f.st.ProfileFor(f.uid); err != nil || p.DefaultTool != "" {
		t.Fatal("a refused default was stored", err, p)
	}
}

// The per-account switches for gated chats: a writer turns Claude Code and Codex chats on or off, the effective values
// come back beside the profile, an unknown agent is refused without changing anything, a viewer cannot change them and
// the audit row names agent and value only.
func TestProfileChatAgentsValidationPermissionAndAudit(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	patch := func(cookie string, body map[string]any) (int, map[string]any, string) {
		return f.do(t, cookie, "PATCH", "/api/v2/profile", body)
	}
	enabled := func(out map[string]any) map[string]any {
		got, _ := out["chatEnabled"].(map[string]any)
		return got
	}
	code, out, raw := f.do(t, f.cookie, "GET", "/api/v2/profile", nil)
	if got := enabled(out); code != 200 || got["claude"] != false || got["codex"] != false || len(got) != 2 {
		t.Fatal("default", code, raw)
	}
	code, out, raw = patch(f.cookie, map[string]any{"chatAgents": map[string]any{"claude": true}})
	if got := enabled(out); code != 200 || got["claude"] != true || got["codex"] != false {
		t.Fatal("turn claude on", code, raw)
	}
	// Leaving the field out keeps the choice; a partial map changes only what it names.
	code, out, raw = patch(f.cookie, map[string]any{"theme": "dark"})
	if got := enabled(out); code != 200 || got["claude"] != true {
		t.Fatal("partial save", code, raw)
	}
	for _, bad := range []map[string]any{{"gemini": true}, {"opencode": false}, {"": true}} {
		code, out, raw = patch(f.cookie, map[string]any{"chatAgents": bad, "theme": "light"})
		if code != 400 || !strings.Contains(out["error"].(string), "chatAgents") {
			t.Fatal("unknown agent accepted", bad, code, raw)
		}
	}
	if code, _, raw = patch(f.cookie, map[string]any{"chatAgents": map[string]any{"claude": "yes"}}); code != 400 {
		t.Fatal("a non-boolean was accepted", code, raw)
	}
	// A refused save changes nothing, not even the other fields it carried.
	if p, err := f.st.ProfileFor(f.uid); err != nil || !p.ChatEnabled("claude") || p.Theme != "dark" {
		t.Fatal("refused save applied", err, p)
	}
	viewer, err := f.st.CreateUser("chat-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, raw := patch(terminalLogin(t, f.st, viewer.ID), map[string]any{"chatAgents": map[string]any{"codex": true}}); code != 403 {
		t.Fatal("a viewer changed a chat switch", code, raw)
	}
	if code, out, raw = patch(f.cookie, map[string]any{"chatAgents": map[string]any{"claude": false}}); code != 200 || enabled(out)["claude"] != false {
		t.Fatal("turn claude off", code, raw)
	}
	found := false
	for _, e := range projectEvents(t, f.st, f.uid, "profile") {
		if got, ok := e["chatAgents"].(map[string]any); ok && got["claude"] == true && len(got) == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("the audit row did not name the agent and value")
	}
}

func gatedChatCaps() terminalproto.Capabilities {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions, caps.ACPModel = true, true, true, true
	caps.Tools = []string{"claude", "opencode"}
	entries := []terminalproto.ACPAgent{
		{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1", Model: true, Approval: true},
		{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true, Images: true, Model: true, Approval: true},
	}
	caps.ACPAgents = &entries
	return caps
}

// A machine reports Claude Code's chat as available, but an account that has not turned it on cannot start one; once it
// does, the same machine accepts it.
func TestStructuredClaudeCreateFollowsTheAccountSetting(t *testing.T) {
	f := launchAPIFixture(t, gatedChatCaps())
	create := func() (int, string) {
		status, _, raw := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "structured": true, "dir": "/tmp", "prompt": "hello there"}))
		return status, raw
	}
	// The refusal names the account's switch, not the machine: updating reet there would change nothing.
	if status, raw := create(); status != 409 || !strings.Contains(raw, `chat_off`) || strings.Contains(raw, "Update reet") {
		t.Fatalf("a Claude chat for an account that has them off: %d %s", status, raw)
	}
	if code, _, raw := f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"chatAgents": map[string]any{"claude": true}}); code != 200 {
		t.Fatal("turn on", code, raw)
	}
	if status, raw := create(); status != 200 {
		t.Fatalf("a Claude chat was refused for an account that turned them on: %d %s", status, raw)
	}
}

// Recording a software update reads a machine's record and stores the whole of it back. The account's setting must not
// leak into what is stored: turning a chat on afterwards needs no new report from the machine.
func TestSoftwareRefreshWhileChatsAreOffDoesNotPersistTheMask(t *testing.T) {
	f := launchAPIFixture(t, gatedChatCaps())
	entry := terminalproto.SoftwareEntry{Tool: "opencode", State: terminalproto.SoftwareInstalled, Version: "1.19.5", CheckedAt: 1}
	f.srv.s.recordSoftwareEntry(f.uid, f.agent.ID, 0, entry)
	raw, err := f.st.RemoteCapabilitiesRawFor(f.uid, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, a := range *raw.Capabilities.ACPAgents {
		states[a.Tool] = a.State
	}
	if states["claude"] != "available" {
		t.Fatalf("the software refresh stored the mask: %v", states)
	}
	if code, _, body := f.do(t, f.cookie, "PATCH", "/api/v2/profile", map[string]any{"chatAgents": map[string]any{"claude": true}}); code != 200 {
		t.Fatal("turn on", code, body)
	}
	caps, err := f.st.RemoteCapabilitiesFor(f.uid, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := caps.Capabilities.ChatAgent("claude"); !ok || !a.Approval {
		t.Fatalf("Claude is not available after turning it on: %+v %v", a, ok)
	}
}
