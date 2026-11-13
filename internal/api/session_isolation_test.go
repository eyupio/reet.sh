package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestWorktreeRunTokenIsValidAndStable(t *testing.T) {
	a := worktreeRunToken("request-1")
	if !terminalproto.ValidRunToken(a) {
		t.Fatalf("invalid token %q", a)
	}
	if a != worktreeRunToken("request-1") || a == worktreeRunToken("request-2") {
		t.Fatal("token must follow the request id")
	}
}

func TestIsolateWorktreeOnlyTouchesRepoLaunches(t *testing.T) {
	s, uid := isolationFixture(t)
	// No saved repository: nothing to isolate, and nothing is invented.
	c := terminalproto.Create{Tool: "claude", Dir: "/home/a/reet"}
	if s.isolateWorktree(uid, "laptop", "req", &c) || c.Worktree != "" {
		t.Fatalf("unsaved folder was changed: %+v", c)
	}
	if _, err := s.st.CreateFolderProjectFor(uid, store.FolderProject{Host: "laptop", Dir: "/home/a/reet", Name: "Reet", IsRepo: true}); err != nil {
		t.Fatal(err)
	}
	// A saved repository gets a worktree, the same one on a retry.
	c = terminalproto.Create{Tool: "claude", Dir: "/home/a/reet", Project: ""}
	if !s.isolateWorktree(uid, "laptop", "req", &c) || c.Worktree != worktreeRunToken("req") || c.Project != "reet" || c.ProjectID == "" {
		t.Fatalf("repo launch not isolated: %+v", c)
	}
	for name, in := range map[string]terminalproto.Create{
		"shell":    {Dir: "/home/a/reet"},
		"named":    {Tool: "claude", Dir: "/home/a/reet", Worktree: worktreeRunToken("x"), Project: "p"},
		"resume":   {Tool: "claude", Dir: "/home/a/reet", Resume: true},
		"inside":   {Tool: "claude", Dir: "/home/a/.reet/worktrees/p/reet/" + worktreeRunToken("y")},
		"no dir":   {Tool: "claude"},
		"reopened": {Tool: "claude", Dir: "/home/a/reet", ResumeACP: "abc"},
	} {
		in := in
		if s.isolateWorktree(uid, "laptop", "req", &in) {
			t.Errorf("%s: should be left alone", name)
		}
	}
}

// isolationFixture is a controller with a store and one account, for the
// checks that only exercise isolateWorktree's own decisions.
func isolationFixture(t *testing.T) (*server, int64) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Skip("cannot open store:", err)
	}
	t.Cleanup(func() { st.Close() })
	u, err := st.CreateUser("a@example.com", "a", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	return &server{st: st}, u.ID
}

func TestIsolatedLaunchWithoutABaseUsesTheProjectsBase(t *testing.T) {
	s, uid := isolationFixture(t)
	if _, err := s.st.CreateFolderProjectFor(uid, store.FolderProject{Host: "laptop", Dir: "/home/a/reet", Name: "Reet", IsRepo: true, BaseBranch: "develop"}); err != nil {
		t.Fatal(err)
	}
	c := terminalproto.Create{Tool: "claude", Dir: "/home/a/reet"}
	if !s.isolateWorktree(uid, "laptop", "req", &c) || c.Base != "develop" {
		t.Fatalf("the project's base was not named: %+v", c)
	}
	// A project that saved no base leaves the choice to the machine, which
	// starts from the remote's default branch.
	if _, err := s.st.CreateFolderProjectFor(uid, store.FolderProject{Host: "laptop", Dir: "/home/a/plain", Name: "Plain", IsRepo: true}); err != nil {
		t.Fatal(err)
	}
	c = terminalproto.Create{Tool: "claude", Dir: "/home/a/plain"}
	if !s.isolateWorktree(uid, "laptop", "req-2", &c) || c.Base != "" {
		t.Fatalf("a base was invented for a project without one: %+v", c)
	}
	// A launch that is not isolated is left alone, base included.
	c = terminalproto.Create{Tool: "claude", Dir: "/home/a/elsewhere"}
	if s.isolateWorktree(uid, "laptop", "req-3", &c) || c.Base != "" {
		t.Fatalf("an unsaved folder gained a base: %+v", c)
	}
}

func TestIsolatedLaunchKeepsANamedBase(t *testing.T) {
	s, uid := isolationFixture(t)
	if _, err := s.st.CreateFolderProjectFor(uid, store.FolderProject{Host: "laptop", Dir: "/home/a/reet", Name: "Reet", IsRepo: true, BaseBranch: "develop"}); err != nil {
		t.Fatal(err)
	}
	c := terminalproto.Create{Tool: "claude", Dir: "/home/a/reet", Base: "release"}
	if !s.isolateWorktree(uid, "laptop", "req", &c) || c.Base != "release" {
		t.Fatalf("a named base was overwritten: %+v", c)
	}
}

// isolatedProjectLaunch is a browser launch in the saved repository the
// controller isolates: no worktree, no project and no base are asked for.
func isolatedProjectLaunch(agent, request string) map[string]any {
	return launchBody(agent, request, map[string]any{"tool": "claude", "model": "sonnet", "dir": "/home/tester/site", "prompt": launchTestPrompt})
}

// baseFixture connects a peer that makes worktrees and reports what they
// started from, and saves a repository project with base.
func baseFixture(t *testing.T, base string) *launchAPI {
	t.Helper()
	caps := worktreeCaps("claude")
	caps.AgentsV2, caps.BaseFresh = true, true
	f := launchAPIFixture(t, caps)
	if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/site", Name: "Site", IsRepo: true, BaseBranch: base}); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestIsolatedRetryKeepsTheOriginalBase(t *testing.T) {
	f := baseFixture(t, "develop")
	request := terminalbroker.ID()
	code, first, raw := f.create(t, isolatedProjectLaunch(f.agent.ID, request))
	if code != 200 {
		t.Fatal(code, raw)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Base != "develop" {
		t.Fatalf("the project's base did not reach the machine: %+v", sent)
	}
	// The project's base changes while the reply is uncertain; the retry is
	// still the same create, so it must carry the base it was reserved with.
	project, ok, err := f.st.RepoProjectForDir(f.uid, f.agent.ID, "/home/tester/site")
	if err != nil || !ok {
		t.Fatal(err)
	}
	project.BaseBranch = "main"
	if _, err := f.st.UpdateFolderProjectFor(f.uid, project, project.Version); err != nil {
		t.Fatal(err)
	}
	code, again, raw := f.create(t, isolatedProjectLaunch(f.agent.ID, request))
	if code != 200 || again["id"] != first["id"] || len(f.sent()) != 1 {
		t.Fatal("retry after the project's base changed", code, raw, len(f.sent()))
	}
}

func TestGafferOutcomeLaunchCarriesTheProjectsBase(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.Worktree, caps.ACPWorktree, caps.BaseFresh = true, true, true, true
	f := launchAPIFixture(t, caps)
	if _, err := f.st.CreateFolderProjectFor(f.uid, store.FolderProject{Host: f.agent.ID, Dir: "/home/tester/site", Name: "Site", IsRepo: true, BaseBranch: "develop"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (outcomeStarter{f.srv.s}).StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Dir: "/home/tester/site", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
	}); err != nil {
		t.Fatal(err)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Base != "develop" || sent[0].Worktree == "" {
		t.Fatalf("outcome launch did not carry the project's base: %+v", sent)
	}
}

func TestSessionRecordsItsBase(t *testing.T) {
	f := baseFixture(t, "develop")
	start := strings.Repeat("ab", 20)
	var reported terminalproto.BaseState
	f.reply = func(in terminalproto.Create) terminalproto.Body {
		state := reported
		return terminalproto.SessionChanged{State: terminalproto.Running, Base: &state,
			WorktreePath: "/home/tester/.reet/worktrees/" + in.Project + "/site/" + in.Worktree, WorktreeRepo: "site"}
	}
	reported = terminalproto.BaseState{Base: "develop", Relation: terminalproto.BaseAhead, Local: start, Start: start}
	code, out, raw := f.create(t, isolatedProjectLaunch(f.agent.ID, terminalbroker.ID()))
	if code != 200 || out["baseRelation"] != "ahead" || out["baseStart"] != start {
		t.Fatal("session JSON without its base", code, raw)
	}
	events := worktreeEvents(t, f.st, f.uid, "worktree-base")
	if len(events) != 1 {
		t.Fatal("worktree-base recorded", len(events), "times")
	}
	var detail map[string]any
	if err := json.Unmarshal([]byte(events[0].Detail), &detail); err != nil {
		t.Fatal(err)
	}
	if detail["machine"] != f.agent.ID || detail["session"] != out["id"] || detail["base"] != "develop" ||
		detail["relation"] != "ahead" || detail["start"] != start {
		t.Fatal("worktree-base detail", events[0].Detail)
	}
	// A base that is the same commit as GitHub's is not worth a notice.
	reported = terminalproto.BaseState{Base: "develop", Relation: terminalproto.BaseSame, Local: start, Remote: start, Start: start}
	code, same, raw := f.create(t, isolatedProjectLaunch(f.agent.ID, terminalbroker.ID()))
	if code != 200 || same["baseRelation"] != "same" || same["baseStart"] != start {
		t.Fatal("second session JSON", code, raw)
	}
	if events := worktreeEvents(t, f.st, f.uid, "worktree-base"); len(events) != 1 {
		t.Fatal("a base that matched GitHub was recorded", len(events))
	}
}

func TestReleaseSessionWorktreeHolds(t *testing.T) {
	s := &server{}
	// No worktree: nothing to hold the removal.
	if h := s.releaseSessionWorktree(context.Background(), 1, store.TerminalSession{State: terminalproto.Ended}, worktreeConfirm{}); h != nil {
		t.Fatalf("held a session with no worktree: %+v", h)
	}
	// A session that is not closed keeps its worktree in use, so the removal is held before the machine is asked.
	run := worktreeRunToken("r")
	sess := store.TerminalSession{ID: "s", AgentID: "m", State: terminalproto.EndRequested, WorktreeRepo: "reet", WorktreePath: "/home/a/.reet/worktrees/reet/reet/" + run}
	h := s.releaseSessionWorktree(context.Background(), 1, sess, worktreeConfirm{})
	if h == nil || h.Code != "session_running" || h.Status != http.StatusConflict {
		t.Fatalf("running session not held: %+v", h)
	}
}
