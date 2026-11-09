package store

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func execRetired(t *testing.T, s *Store, query string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}

func countRetired(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// An upgrade clears every default that names a retired agent, and the rows that
// are derived from it, and leaves other agents' rows and all history alone.
func TestRetiredAgentDefaultsAreClearedAndHistoryIsKept(t *testing.T) {
	s := newStore(t)
	uid, other := projectUser(t, s, "a"), projectUser(t, s, "b")
	m := enrolledID(t, s, uid, "studio")
	agent, caps := terminalAgent(t, s, uid, "laptop")

	setProfile := func(user int64, tool, model, effort string) {
		p, err := s.ProfileFor(user)
		if err != nil {
			t.Fatal(err)
		}
		p.DefaultTool, p.DefaultModel, p.DefaultEffort = tool, model, effort
		if err := s.SaveProfileFor(user, p); err != nil {
			t.Fatal(err)
		}
	}
	setProfile(uid, "gemini", "pro", "low")
	setProfile(other, "claude", "sonnet", "")

	// The agent was offered when these were saved, so set it by SQL as an older release would have.
	retired := mustFP(t, s, uid, FolderProject{Host: m, Dir: "/w/retired", Name: "Retired", DefaultTool: "claude", DefaultModel: "sonnet"})
	kept := mustFP(t, s, uid, FolderProject{Host: m, Dir: "/w/kept", Name: "Kept", DefaultTool: "opencode", DefaultModel: "m1"})
	execRetired(t, s, `UPDATE folder_projects SET default_tool='gemini', default_model='pro', default_effort='low' WHERE id=?`, retired.ID)
	retiredBefore, err := s.FolderProjectFor(uid, retired.ID)
	if err != nil {
		t.Fatal(err)
	}
	keptBefore, err := s.FolderProjectFor(uid, kept.ID)
	if err != nil {
		t.Fatal(err)
	}

	draft, err := s.SaveSessionDraftFor(uid, SessionDraft{Title: "Idea", Prompt: "remember this", Tool: "claude", Model: "sonnet"}, true)
	if err != nil {
		t.Fatal(err)
	}
	keptDraft, err := s.SaveSessionDraftFor(uid, SessionDraft{Title: "Other", Prompt: "keep me", Tool: "opencode", Model: "m1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	execRetired(t, s, `UPDATE session_backlog SET body=json_set(body,'$.tool','gemini','$.model','pro','$.effort','low') WHERE user_id=? AND id=?`, uid, draft.ID)
	draftBefore, err := s.SessionDraftFor(uid, draft.ID)
	if err != nil {
		t.Fatal(err)
	}

	for _, tool := range []string{"gemini", "opencode"} {
		execRetired(t, s, `INSERT INTO model_favourites(user_id,tool,model,position,agent_default,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, tool, "m1", 0, 1, 1, 1)
		execRetired(t, s, `INSERT INTO machine_agent_models(user_id,agent_id,tool,state) VALUES(?,?,?,'listed')`, uid, m, tool)
	}

	session, _, err := s.ReserveTerminalCreateFor(uid, agent.ID, syncID(), caps.Generation, terminalproto.Create{Title: "Past", Cols: 80, Rows: 24, Tool: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	execRetired(t, s, `UPDATE terminal_sessions SET tool='gemini' WHERE id=?`, session.ID)

	for pass := 1; pass <= 2; pass++ {
		if err := s.retireAgents(); err != nil {
			t.Fatal(err)
		}
		if p, err := s.ProfileFor(uid); err != nil || p.DefaultTool != "" || p.DefaultModel != "" || p.DefaultEffort != "" {
			t.Fatalf("pass %d: profile kept a retired default: %+v %v", pass, p, err)
		}
		if p, err := s.ProfileFor(other); err != nil || p.DefaultTool != "claude" || p.DefaultModel != "sonnet" {
			t.Fatalf("pass %d: another agent's profile default changed: %+v %v", pass, p, err)
		}
		got, err := s.FolderProjectFor(uid, retired.ID)
		if err != nil || got.DefaultTool != "" || got.DefaultModel != "" || got.DefaultEffort != "" || got.Version != retiredBefore.Version+1 {
			t.Fatalf("pass %d: retired project: %+v %v (version was %d)", pass, got, err, retiredBefore.Version)
		}
		if got, err := s.FolderProjectFor(uid, kept.ID); err != nil || got.DefaultTool != "opencode" || got.DefaultModel != "m1" || got.Version != keptBefore.Version {
			t.Fatalf("pass %d: another agent's project changed: %+v %v", pass, got, err)
		}
		if got, err := s.SessionDraftFor(uid, draft.ID); err != nil || got.Tool != "" || got.Model != "" || got.Effort != "" || got.Prompt != "remember this" || got.Version != draftBefore.Version+1 {
			t.Fatalf("pass %d: retired draft: %+v %v (version was %d)", pass, got, err, draftBefore.Version)
		}
		if got, err := s.SessionDraftFor(uid, keptDraft.ID); err != nil || got.Tool != "opencode" || got.Version != keptDraft.Version {
			t.Fatalf("pass %d: another agent's draft changed: %+v %v", pass, got, err)
		}
		if n := countRetired(t, s, `SELECT COUNT(*) FROM model_favourites WHERE tool='gemini'`); n != 0 {
			t.Fatalf("pass %d: %d retired favourites remain", pass, n)
		}
		if n := countRetired(t, s, `SELECT COUNT(*) FROM machine_agent_models WHERE tool='gemini'`); n != 0 {
			t.Fatalf("pass %d: %d retired model lists remain", pass, n)
		}
		if n := countRetired(t, s, `SELECT COUNT(*) FROM model_favourites WHERE tool='opencode'`) + countRetired(t, s, `SELECT COUNT(*) FROM machine_agent_models WHERE tool='opencode'`); n != 2 {
			t.Fatalf("pass %d: another agent's derived rows changed: %d", pass, n)
		}
		if got, err := s.TerminalSessionFor(uid, session.ID); err != nil || got.Tool != "gemini" {
			t.Fatalf("pass %d: past session history changed: %+v %v", pass, got, err)
		}
	}
}

// The cleanup runs when the store opens, so an upgrade needs no separate step.
func TestRetiredAgentDefaultsAreClearedWhenTheStoreOpens(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	user, err := s.EnsureLocalOwner()
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.ProfileFor(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	p.DefaultTool, p.DefaultModel, p.DefaultEffort = "gemini", "pro", "low"
	if err := s.SaveProfileFor(user.ID, p); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, err := s.ProfileFor(user.ID); err != nil || got.DefaultTool != "" || got.DefaultModel != "" || got.DefaultEffort != "" {
		t.Fatalf("a retired default survived the upgrade: %+v %v", got, err)
	}
}
