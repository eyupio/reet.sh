package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const selectionPrompt = "quokka-selection sentinel: fix the failing tests"

func selectionLaunch() RunLaunch {
	return RunLaunch{Tool: "cline", Model: "sonnet", Effort: "high", Prompt: selectionPrompt, Worktree: true}
}

// A start in several projects snapshots one member per folder project, in
// the order given: its machine, folder, name and base branch as they were.
func TestSelectionRunSnapshotsFolderProjects(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	reet := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/home/a/reet", Name: "reet", IsRepo: true, BaseBranch: "develop"})
	site := mustFP(t, s, uid, FolderProject{Host: "server", Dir: "/srv/site", Name: "site", IsRepo: true})
	notes := mustFP(t, s, uid, FolderProject{Host: "desk", Dir: "/home/a/notes", Name: "notes"})
	request := syncID()
	run, created, err := s.StartSelectionRunFor(uid, request, []string{site.ID, reet.ID, notes.ID}, selectionLaunch())
	if err != nil || !created {
		t.Fatal(created, err)
	}
	if run.Origin != RunOriginSelection || run.ProjectID != "" || run.ProjectName != "site, reet, notes" || !run.UseWorktrees || run.Effort != "high" || run.Yolo || run.State != RunRunning {
		t.Fatalf("%+v", run)
	}
	want := []RunMember{
		{Position: 0, Host: "server", Dir: "/srv/site", ProjectID: site.ID, Name: "site", Tool: "cline", Model: "sonnet"},
		{Position: 1, Host: "studio", Dir: "/home/a/reet", ProjectID: reet.ID, Name: "reet", Base: "develop", Tool: "cline", Model: "sonnet"},
		// Not a repository: it works in its folder, never in a worktree.
		{Position: 2, Host: "desk", Dir: "/home/a/notes", ProjectID: notes.ID, Name: "notes", NoWorktree: true, Tool: "cline", Model: "sonnet"},
	}
	check := func(r Run) {
		t.Helper()
		if len(r.Members) != len(want) {
			t.Fatalf("%+v", r.Members)
		}
		for i, m := range r.Members {
			w := want[i]
			if m.Position != w.Position || m.Host != w.Host || m.Dir != w.Dir || m.ProjectID != w.ProjectID || m.Name != w.Name || m.Base != w.Base ||
				m.NoWorktree != w.NoWorktree || m.Tool != w.Tool || m.Model != w.Model || m.State != RunMemberPending || m.Attempt != 1 {
				t.Fatalf("member %d: %+v", i, m)
			}
		}
	}
	check(run)
	// A later change to a project does not change the run.
	reet.BaseBranch, reet.Dir, reet.Name = "main", "/home/a/reet2", "reet two"
	if _, err := s.UpdateFolderProjectFor(uid, reet, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.RunFor(uid, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(got)
	if got.Origin != RunOriginSelection || got.Prompt != selectionPrompt || got.Effort != "high" {
		t.Fatalf("%+v", got)
	}
	// The same request again answers with the same run and makes nothing new.
	again, created, err := s.StartSelectionRunFor(uid, request, []string{site.ID, reet.ID, notes.ID}, selectionLaunch())
	if err != nil || created || again.ID != run.ID {
		t.Fatal(created, err, again.ID)
	}
	// The same request id with anything else changed is refused.
	for name, change := range map[string]func(*[]string, *RunLaunch){
		"projects": func(ids *[]string, _ *RunLaunch) { *ids = (*ids)[:2] },
		"order":    func(ids *[]string, _ *RunLaunch) { (*ids)[0], (*ids)[1] = (*ids)[1], (*ids)[0] },
		"tool":     func(_ *[]string, l *RunLaunch) { l.Tool = "codex"; l.Effort = "" },
		"model":    func(_ *[]string, l *RunLaunch) { l.Model = "" },
		"effort":   func(_ *[]string, l *RunLaunch) { l.Effort = "low" },
		"prompt":   func(_ *[]string, l *RunLaunch) { l.Prompt = "something else entirely" },
		"worktree": func(_ *[]string, l *RunLaunch) { l.Worktree = false },
	} {
		ids, l := []string{site.ID, reet.ID, notes.ID}, selectionLaunch()
		change(&ids, &l)
		if _, _, err := s.StartSelectionRunFor(uid, request, ids, l); !errors.Is(err, ErrRunRequestChanged) {
			t.Fatal(name, err)
		}
	}
	// A new start that names a project with a member still pending waits.
	other := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/home/a/other", IsRepo: true})
	if _, _, err := s.StartSelectionRunFor(uid, syncID(), []string{other.ID, site.ID}, selectionLaunch()); !errors.Is(err, ErrRunInProgress) {
		t.Fatal("overlap", err)
	}
	// Once those are decided, it may start.
	for _, m := range run.Members {
		if err := s.SetRunMemberState(uid, run.ID, m.Position, RunMemberFailed, "session_limit", "", "", "", "", 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, created, err := s.StartSelectionRunFor(uid, syncID(), []string{other.ID, site.ID}, selectionLaunch()); err != nil || !created {
		t.Fatal(created, err)
	}
	// Without a worktree nobody gets a base, and nobody is marked as working apart.
	plain := selectionLaunch()
	plain.Worktree = false
	off, _, err := s.StartSelectionRunFor(uid, syncID(), []string{reet.ID, notes.ID}, plain)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range off.Members {
		if m.Base != "" || m.NoWorktree || off.UseWorktrees {
			t.Fatalf("%+v", m)
		}
	}
	// A selection run is nobody's saved Project: an old Project's run list never shows it.
	legacy := mustProject(t, s, uid, baseProject("Old set"))
	if runs, err := s.RunsFor(uid, legacy.ID, 0); err != nil || len(runs) != 0 {
		t.Fatal(runs, err)
	}
	setRun, err := s.CreateRunFor(uid, legacy.ID)
	if err != nil || setRun.Origin != RunOriginSet {
		t.Fatal(setRun.Origin, err)
	}
	// The prompt is never in the JSON of a run.
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "quokka") {
		t.Fatal("the prompt reached JSON", string(raw))
	}
	// Another account sees none of it.
	b := projectUser(t, s, "b")
	if _, err := s.RunFor(b, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
}

// A run's base for each member: the project's own base branch, else the
// account's default, else none; a folder that isn't a repository has none.
func TestSelectionRunMemberUsesProjectBase(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	own := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/w/reet", IsRepo: true, BaseBranch: "release/1.2"})
	inherit := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/w/site", IsRepo: true})
	folder := mustFP(t, s, uid, FolderProject{Host: "server", Dir: "/w/notes", BaseBranch: "ignored"})
	ids := []string{own.ID, inherit.ID, folder.ID}
	run, _, err := s.StartSelectionRunFor(uid, syncID(), ids, selectionLaunch())
	if err != nil {
		t.Fatal(err)
	}
	if run.Members[0].Base != "release/1.2" || run.Members[1].Base != "" || run.Members[2].Base != "" || !run.Members[2].NoWorktree {
		t.Fatalf("no profile default: %+v", run.Members)
	}
	for _, m := range run.Members {
		if err := s.SetRunMemberState(uid, run.ID, m.Position, RunMemberFailed, "session_limit", "", "", "", "", 1); err != nil {
			t.Fatal(err)
		}
	}
	p, err := s.ProfileFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	p.DefaultBaseBranch = "main"
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	run, _, err = s.StartSelectionRunFor(uid, syncID(), ids, selectionLaunch())
	if err != nil {
		t.Fatal(err)
	}
	if run.Members[0].Base != "release/1.2" || run.Members[1].Base != "main" || run.Members[2].Base != "" {
		t.Fatalf("profile default: %+v", run.Members)
	}
}

// Two to twelve distinct projects of the account, and a launch the
// connector would take; anything else is refused and nothing is made.
func TestSelectionRunLimits(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	var ids []string
	for i := 0; i < MaxSelectionProjects+1; i++ {
		p := mustFP(t, s, uid, FolderProject{Host: fmt.Sprintf("m%d", i%3), Dir: fmt.Sprintf("/w/p%d", i), IsRepo: true})
		ids = append(ids, p.ID)
	}
	other := projectUser(t, s, "b")
	theirs := mustFP(t, s, other, FolderProject{Host: "m0", Dir: "/w/theirs"})
	for name, tc := range map[string]struct {
		ids  []string
		l    RunLaunch
		want error
	}{
		"one":           {ids[:1], selectionLaunch(), ErrSelectionSize},
		"none":          {nil, selectionLaunch(), ErrSelectionSize},
		"thirteen":      {ids, selectionLaunch(), ErrSelectionSize},
		"twice":         {[]string{ids[0], ids[1], ids[0]}, selectionLaunch(), ErrSelectionSize},
		"unknown":       {[]string{ids[0], "0123456789abcdef0123456789abcdef"}, selectionLaunch(), ErrFolderProjectNotFound},
		"other account": {[]string{ids[0], theirs.ID}, selectionLaunch(), ErrFolderProjectNotFound},
		"tool":          {ids[:2], RunLaunch{Tool: "nope"}, ErrProjectInvalid},
		"no tool":       {ids[:2], RunLaunch{}, ErrProjectInvalid},
		"model":         {ids[:2], RunLaunch{Tool: "claude", Model: "bad model"}, ErrProjectInvalid},
		"effort":        {ids[:2], RunLaunch{Tool: "cline", Effort: "max"}, ErrProjectInvalid},
		"no effort":     {ids[:2], RunLaunch{Tool: "claude", Effort: "low"}, ErrProjectInvalid},
		"dash":          {ids[:2], RunLaunch{Tool: "claude", Prompt: "-rf everything"}, ErrProjectInvalid},
		"one word":      {ids[:2], RunLaunch{Tool: "claude", Prompt: "update"}, ErrProjectInvalid},
		"long":          {ids[:2], RunLaunch{Tool: "claude", Prompt: strings.Repeat("ab ", 700)}, ErrProjectInvalid},
		"control":       {ids[:2], RunLaunch{Tool: "claude", Prompt: "fix it\u0007 now"}, ErrProjectInvalid},
	} {
		_, _, err := s.StartSelectionRunFor(uid, syncID(), tc.ids, tc.l)
		if !errors.Is(err, tc.want) {
			t.Fatal(name, err)
		}
		if err != nil && strings.Contains(err.Error(), "quokka") {
			t.Fatal(name, "the prompt is in the error", err)
		}
	}
	if _, _, err := s.StartSelectionRunFor(uid, "short", ids[:2], selectionLaunch()); !errors.Is(err, ErrProjectInvalid) {
		t.Fatal("request id", err)
	}
	if pending, err := s.PendingRuns(); err != nil || len(pending) != 0 {
		t.Fatal("a refused start made a run", pending, err)
	}
	run, created, err := s.StartSelectionRunFor(uid, syncID(), ids[:MaxSelectionProjects], selectionLaunch())
	if err != nil || !created || len(run.Members) != MaxSelectionProjects {
		t.Fatal(created, err, len(run.Members))
	}
	if run, _, err := s.StartSelectionRunFor(uid, syncID(), []string{ids[MaxSelectionProjects], ids[0]}, selectionLaunch()); !errors.Is(err, ErrRunInProgress) {
		t.Fatal(run, err)
	}
}

// A session that names a folder project must be on its machine and in its
// folder: a session is never put in a project whose folder it isn't in.
func TestTerminalProjectNeedsItsFolder(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	here := mustFP(t, s, uid, fp(a.ID, "/home/tester/site"))
	for name, dir := range map[string]string{"no folder": "", "another folder": "/home/tester/api", "a folder inside": "/home/tester/site/web", "a prefix": "/home/tester/sit"} {
		in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: dir, ProjectID: here.ID}
		if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in); !errors.Is(err, ErrProjectFolderMismatch) {
			t.Fatal(name, err)
		}
		if err := s.TerminalProjectCheckFor(uid, a.ID, here.ID, dir); !errors.Is(err, ErrProjectFolderMismatch) {
			t.Fatal(name, "check", err)
		}
	}
	if all, _ := s.TerminalSessionsFor(uid); len(all) != 0 {
		t.Fatal("a refused create reserved a session", all)
	}
	// The same folder, spelt with a trailing slash, is the project's folder.
	for _, dir := range []string{"/home/tester/site", "/home/tester/site/"} {
		in := terminalproto.Create{Title: "Agent", Cols: 80, Rows: 24, Tool: "claude", Dir: dir, ProjectID: here.ID}
		if _, created, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in); err != nil || !created {
			t.Fatal(dir, created, err)
		}
	}
}

// A member's base taken from the account's default is marked inherited until it
// is settled, once: kept, or cleared for a machine that can't take it. A
// project's own base is never inherited, and settling again changes nothing.
func TestSelectionRunInheritedBaseIsSettledOnce(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p, err := s.ProfileFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	p.DefaultBaseBranch = "main"
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	own := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/home/a/own", Name: "own", IsRepo: true, BaseBranch: "develop"})
	inherits := mustFP(t, s, uid, FolderProject{Host: "studio", Dir: "/home/a/inherits", Name: "inherits", IsRepo: true})
	run, _, err := s.StartSelectionRunFor(uid, syncID(), []string{own.ID, inherits.ID}, selectionLaunch())
	if err != nil {
		t.Fatal(err)
	}
	if m := run.Members[0]; m.Base != "develop" || m.BaseInherited {
		t.Fatalf("own base: %+v", m)
	}
	if m := run.Members[1]; m.Base != "main" || !m.BaseInherited {
		t.Fatalf("inherited base: %+v", m)
	}
	for _, keep := range []bool{false, true} { // the second call is too late to change it
		if err := s.SettleRunMemberBase(uid, run.ID, 1, keep); err != nil {
			t.Fatal(err)
		}
		if err := s.SettleRunMemberBase(uid, run.ID, 0, false); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.RunFor(uid, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if m := got.Members[1]; m.Base != "" || m.BaseInherited {
		t.Fatalf("settled: %+v", m)
	}
	if m := got.Members[0]; m.Base != "develop" {
		t.Fatalf("a project's own base was cleared: %+v", m)
	}
	if err := s.SettleRunMemberBase(uid, run.ID, 9, true); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	if err := s.SettleRunMemberBase(projectUser(t, s, "b"), run.ID, 1, true); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
}
