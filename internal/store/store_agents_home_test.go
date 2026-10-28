package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const agentsHomeTestPrompt = "rewrite the billing export and keep it quiet"

func TestProfileDefaultsRoundTrip(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p, err := s.ProfileFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if !p.WorktreesByDefault || p.DefaultBaseBranch != "" {
		t.Fatalf("new person: %+v", p)
	}
	// The first read stored the row; a second read is the stored row.
	if p, err = s.ProfileFor(uid); err != nil || !p.WorktreesByDefault || p.DefaultBaseBranch != "" {
		t.Fatalf("stored defaults: %v %+v", err, p)
	}
	p.DefaultBaseBranch, p.WorktreesByDefault = "release/2.0", false
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	if got, err := s.ProfileFor(uid); err != nil || got.DefaultBaseBranch != "release/2.0" || got.WorktreesByDefault {
		t.Fatalf("saved: %v %+v", err, got)
	}
	p.DefaultBaseBranch, p.WorktreesByDefault = "", true
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.ProfileFor(uid); got.DefaultBaseBranch != "" || !got.WorktreesByDefault {
		t.Fatalf("cleared: %+v", got)
	}
	raw, _ := json.Marshal(p)
	if !strings.Contains(string(raw), `"defaultBaseBranch":""`) || !strings.Contains(string(raw), `"worktreesByDefault":true`) {
		t.Fatal(string(raw))
	}
}

// A profile stored before the two settings existed reads "worktrees by
// default" as yes, and the migration runs again harmlessly.
func TestProfileDefaultsOnAnOldProfileReadWorktreesOn(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid := projectUser(t, s, "a")
	if err := s.SaveProfileFor(uid, Profile{Theme: "dark", DateFormat: "ymd", ShelfView: "board", HostedPrefix: DefaultHostedPrefix}); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{`ALTER TABLE user_profiles DROP COLUMN default_base_branch`, `ALTER TABLE user_profiles DROP COLUMN worktrees_by_default`} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		p, err := s.ProfileFor(uid)
		if err != nil || !p.WorktreesByDefault || p.DefaultBaseBranch != "" || p.Theme != "dark" || p.ShelfView != "board" {
			t.Fatalf("open %d: %v %+v", i, err, p)
		}
		s.Close()
	}
}

func listed(t *testing.T, s *Store, uid int64, id string) (TerminalSession, string) {
	t.Helper()
	all, err := s.TerminalSessionsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(all)
	for _, x := range all {
		if x.ID == id {
			return x, string(raw)
		}
	}
	t.Fatalf("session %s not listed: %s", id, raw)
	return TerminalSession{}, ""
}

func TestSessionListingCarriesLaunchMetadataNotPrompt(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	project := mustFP(t, s, uid, fp(a.ID, "/home/tester/site"))
	in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Model: "sonnet", Dir: "/home/tester/site", Prompt: agentsHomeTestPrompt, ProjectID: project.ID}
	got, created, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	if got.ProjectID == nil || *got.ProjectID != project.ID || got.Tool != "claude" || got.Model != "sonnet" {
		t.Fatalf("reserved: %+v", got)
	}
	x, raw := listed(t, s, uid, got.ID)
	if x.Tool != "claude" || x.Model != "sonnet" || x.Effort != "" || x.Yolo || x.ProjectID == nil || *x.ProjectID != project.ID || x.WorktreeBranch != "" {
		t.Fatalf("listed: %+v", x)
	}
	for _, key := range []string{`"tool":"claude"`, `"model":"sonnet"`, `"effort":""`, `"yolo":false`, `"projectId":"` + project.ID + `"`} {
		if !strings.Contains(raw, key) {
			t.Fatalf("listing lacks %s: %s", key, raw)
		}
	}
	if strings.Contains(raw, "worktree") {
		t.Fatal("a session without a worktree lists worktree fields", raw)
	}
	if strings.Contains(raw, "billing export") || strings.Contains(raw, "prompt") {
		t.Fatal("the prompt reached the listing", raw)
	}
	// A plain shell has no project: projectId is null, not "".
	plain := createTerminal(t, s, uid, a, c)
	if x, raw := listed(t, s, uid, plain.ID); x.ProjectID != nil || x.Tool != "" || !strings.Contains(raw, `"projectId":null`) {
		t.Fatalf("plain: %+v %s", x, raw)
	}
	// A retry with the same request but another project is a conflict.
	request := syncID()
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in); err != nil {
		t.Fatal(err)
	}
	moved := in
	moved.ProjectID = ""
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, moved); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("retry without the project", err)
	}
	if _, created, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in); err != nil || created {
		t.Fatal("same retry", created, err)
	}
}

func TestReserveWithProjectIDChecksAccountAndMachine(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	other := projectUser(t, s, "b")
	theirs := mustFP(t, s, other, fp("laptop", "/home/b/site"))
	server, _ := terminalAgent(t, s, uid, "server")
	elsewhere := mustFP(t, s, uid, fp(server.ID, "/srv/site"))
	// The machine's name is not its key: only the enrollment id matches.
	byName := mustFP(t, s, uid, fp(a.Host, "/srv/named"))
	upper := mustFP(t, s, uid, fp(strings.ToUpper(a.ID), "/srv/upper"))
	for name, tc := range map[string]struct {
		id   string
		want error
	}{
		"unknown":         {"0123456789abcdef0123456789abcdef", ErrFolderProjectNotFound},
		"other account":   {theirs.ID, ErrFolderProjectNotFound},
		"another machine": {elsewhere.ID, ErrProjectMachineMismatch},
		"by name":         {byName.ID, ErrProjectMachineMismatch},
		"id in upper":     {upper.ID, ErrProjectMachineMismatch},
	} {
		in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, ProjectID: tc.id}
		if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in); !errors.Is(err, tc.want) {
			t.Fatal(name, err)
		}
	}
	all, _ := s.TerminalSessionsFor(uid)
	if len(all) != 0 {
		t.Fatal("a refused create reserved a session", all)
	}
	// Renaming the machine keeps its id, so its projects still launch there.
	here := mustFP(t, s, uid, fp(a.ID, "/home/tester/site"))
	if err := s.MergeMachinesFor(uid, a.Host, "work-laptop"); err != nil {
		t.Fatal(err)
	}
	if err := s.TerminalProjectCheckFor(uid, a.ID, here.ID, "/home/tester/site"); err != nil {
		t.Fatal("after a rename", err)
	}
	in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: "/home/tester/site", ProjectID: here.ID}
	if _, created, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in); err != nil || !created {
		t.Fatal("launch after a rename", created, err)
	}
}

// A database from before tool, model, effort, yolo and project_id were
// columns: opening it adds them, fills tool and model from the stored create
// (never the prompt), and opening it again changes nothing.
func TestSessionListingOnAnOldDatabase(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	a, c := terminalAgent(t, s, uid, "laptop")
	launched, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "codex", Model: "gpt-5", Prompt: agentsHomeTestPrompt})
	if err != nil {
		t.Fatal(err)
	}
	plain := createTerminal(t, s, uid, a, c)
	for _, col := range []string{"tool", "model", "effort", "yolo", "project_id"} {
		if _, err := s.db.Exec(`ALTER TABLE terminal_sessions DROP COLUMN ` + col); err != nil {
			t.Fatal(col, err)
		}
	}
	s.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		x, raw := listed(t, s, uid, launched.ID)
		if x.Tool != "codex" || x.Model != "gpt-5" || x.Effort != "" || x.Yolo || x.ProjectID != nil {
			t.Fatalf("open %d: %+v", i, x)
		}
		if strings.Contains(raw, "billing export") {
			t.Fatal("prompt in listing", raw)
		}
		if p, _ := listed(t, s, uid, plain.ID); p.Tool != "" || p.Model != "" || p.ProjectID != nil {
			t.Fatalf("plain %d: %+v", i, p)
		}
		s.Close()
	}
}
