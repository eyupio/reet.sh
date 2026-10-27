package projectrun

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// selectionSentinel is the opening prompt of a start in several projects: its
// first line is every member's title.
const selectionSentinel = "Fix the failing tests\nquokka-sentinel: and say what done looks like"

// folder saves a folder project on a machine.
func (f *fixture) folder(t *testing.T, p store.FolderProject) store.FolderProject {
	t.Helper()
	saved, err := f.st.CreateFolderProjectFor(f.uid, p)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func (f *fixture) selection(t *testing.T, l store.RunLaunch, projects ...store.FolderProject) store.Run {
	t.Helper()
	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	r, created, err := f.st.StartSelectionRunFor(f.uid, terminalbroker.ID(), ids, l)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	return r
}

func clineLaunch() store.RunLaunch {
	return store.RunLaunch{Tool: "cline", Model: "sonnet", Effort: "high", Prompt: selectionSentinel, Worktree: true}
}

// Each member's create carries its own project: its folder, its project id,
// the project's slug and its base branch (the project's own, else the
// account's default), and the run's agent, model, effort and prompt. A folder
// that is not a repository gets no worktree.
func TestSelectionRunMemberUsesProjectBase(t *testing.T) {
	f := newFixture(t)
	p, err := f.st.ProfileFor(f.uid)
	if err != nil {
		t.Fatal(err)
	}
	p.DefaultBaseBranch = "main"
	if err := f.st.SaveProfileFor(f.uid, p); err != nil {
		t.Fatal(err)
	}
	reet := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/home/t/reet", Name: "Reet App", IsRepo: true, BaseBranch: "develop"})
	site := f.folder(t, store.FolderProject{Host: f.server, Dir: "/srv/site", Name: "site", IsRepo: true})
	notes := f.folder(t, store.FolderProject{Host: f.desk, Dir: "/home/t/notes", Name: "notes"})
	o := f.orchestrator(t)
	r := f.selection(t, clineLaunch(), reet, site, notes)
	o.Start(f.uid, r.ID)
	got := settle(t, o, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got.Members)
	}
	want := map[string]struct {
		agent, project, slug, base string
		worktree                   bool
	}{
		"/home/t/reet":  {f.laptop, reet.ID, "reet-app", "develop", true},
		"/srv/site":     {f.server, site.ID, "site", "main", true},
		"/home/t/notes": {f.desk, notes.ID, "", "", false},
	}
	calls := f.broker.sent()
	if len(calls) != 3 {
		t.Fatalf("%d creates", len(calls))
	}
	for _, c := range calls {
		w, ok := want[c.in.Dir]
		if !ok {
			t.Fatalf("%+v", c)
		}
		var m store.RunMember
		for _, x := range got.Members {
			if x.Dir == c.in.Dir {
				m = x
			}
		}
		in := c.in
		if c.agent != w.agent || in.ProjectID != w.project || in.Project != w.slug || in.Base != w.base || (in.Worktree != "") != w.worktree ||
			in.Tool != "cline" || in.Model != "sonnet" || in.Effort != "high" || in.Prompt != selectionSentinel || in.Yolo || in.Title != "Fix the failing tests" ||
			c.request != RequestID(r.ID, m.Position, 1) || in.Validate() != nil {
			t.Fatalf("create for %s: %+v %+v", c.in.Dir, c, in)
		}
		if w.worktree && (in.Worktree != WorktreeToken(r.ID, m.Position) || m.WorktreeToken != in.Worktree || m.WorktreeBranch != "reet/"+w.slug+"-"+in.Worktree) {
			t.Fatalf("worktree for %s: %+v", c.in.Dir, m)
		}
		if !w.worktree && (m.WorktreeToken != "" || m.WorktreePath != "") {
			t.Fatalf("a folder got a worktree: %+v", m)
		}
	}
	if len(f.launched()) != 3 {
		t.Fatal(f.launched())
	}
	// Without a prompt a member's title is its project's name.
	other := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/home/t/other", Name: "other", IsRepo: true})
	quiet := clineLaunch()
	quiet.Prompt, quiet.Worktree = "", false
	r2 := f.selection(t, quiet, other, notes)
	o.Start(f.uid, r2.ID)
	settle(t, o, f.st, f.uid, r2.ID)
	for _, c := range f.broker.sent()[3:] {
		if c.in.Title != map[string]string{"/home/t/other": "other", "/home/t/notes": "notes"}[c.in.Dir] || c.in.Worktree != "" || c.in.Base != "" || c.in.Prompt != "" {
			t.Fatalf("%+v", c.in)
		}
	}
	f.logMu.Lock()
	logs := f.logs.String()
	f.logMu.Unlock()
	if strings.Contains(logs, "quokka") || strings.Contains(fmt.Sprintf("%+v", got.Members), "quokka") {
		t.Fatal("the prompt leaked", logs)
	}
}

// A controller that stops with members in flight resumes only the members
// still pending, with their unchanged request ids and worktree tokens; a
// started member is never started again.
func TestSelectionRunResumesOnlyPendingAfterRestart(t *testing.T) {
	f := newFixture(t)
	gate := make(chan struct{})
	f.broker.gate = gate
	o := f.orchestrator(t)
	var projects []store.FolderProject
	for i, host := range []string{f.laptop, f.server, f.laptop, f.server} {
		projects = append(projects, f.folder(t, store.FolderProject{Host: host, Dir: fmt.Sprintf("/a/%d", i), IsRepo: true}))
	}
	r := f.selection(t, clineLaunch(), projects...)
	o.Start(f.uid, r.ID)
	waitFor(t, "two creates", func() bool { return f.broker.inflight() == 2 })
	gate <- struct{}{} // one of the first two starts
	waitFor(t, "the third create", func() bool { return len(f.broker.sent()) == 3 && f.broker.inflight() == 2 })
	o.Close() // the controller stops with two calls in flight and one never sent
	before, err := f.st.RunFor(f.uid, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	startedBefore, pending := "", 0
	for _, m := range before.Members {
		switch m.State {
		case store.RunMemberStarted:
			startedBefore = m.SessionID
		case store.RunMemberPending:
			pending++
		default:
			t.Fatalf("shutdown failed a member: %+v", m)
		}
	}
	if startedBefore == "" || pending != 3 {
		t.Fatalf("%+v", before.Members)
	}
	first := len(f.broker.sent())
	f.broker.mu.Lock()
	f.broker.gate = nil
	f.broker.mu.Unlock()
	o2 := f.orchestrator(t)
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	got := settle(t, o2, f.st, f.uid, r.ID)
	if got.State != store.RunComplete {
		t.Fatalf("%+v", got.Members)
	}
	again := f.broker.sent()[first:]
	if len(again) != 3 {
		t.Fatalf("resume sent %d creates", len(again))
	}
	for _, c := range again {
		var pos int
		fmt.Sscanf(c.in.Dir, "/a/%d", &pos)
		if c.request != RequestID(r.ID, pos, 1) || c.in.Worktree != WorktreeToken(r.ID, pos) || c.in.ProjectID != projects[pos].ID || c.in.Prompt != selectionSentinel {
			t.Fatalf("resumed with a new identity: %+v", c)
		}
		if member(got, pos).SessionID == startedBefore {
			t.Fatal("a started member was started again")
		}
	}
	oneAgentEach(t, f.broker)
	if len(f.launched()) != 4 {
		t.Fatal("each started member is audited once", f.launched())
	}
	if err := o2.Resume(); err != nil {
		t.Fatal(err)
	}
	settle(t, o2, f.st, f.uid, r.ID)
	if len(f.broker.sent()) != first+3 {
		t.Fatal("a second resume sent again")
	}
}

// Refusals the broker or the store give before anything is reserved are
// definite: each gets its own code, and a retry takes the next attempt.
func TestSelectionRunRefusalsBeforeSendingAreDefinite(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{terminalbroker.ErrYoloRefused, CodeYoloRefused},
		// The machine's capabilities are not known here: the option is put down to its version.
		{terminalbroker.ErrLaunchOptionUnsupported, CodeOptionNeedsUpdate},
		{terminalbroker.ErrToolUnavailable, CodeToolUnavailable},
		{terminalbroker.ErrToolNotInstalled, CodeToolNotInstalled},
		{store.ErrProjectFolderMismatch, CodeProjectMoved},
		{store.ErrProjectMachineMismatch, CodeProjectMoved},
		{store.ErrFolderProjectNotFound, CodeProjectMoved},
		{&terminalbroker.WorktreeError{Reason: terminalproto.WorktreeBaseMissing}, "base_missing"},
	} {
		f := newFixture(t)
		refused := true
		f.broker.answer = func(s sent) answer {
			if refused {
				refused = false
				return answer{kind: before, err: tc.err}
			}
			return answer{}
		}
		o := f.orchestrator(t)
		a := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/a/0", IsRepo: true})
		b := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/a/1", IsRepo: true})
		r := f.selection(t, clineLaunch(), a, b)
		o.Start(f.uid, r.ID)
		got := settle(t, o, f.st, f.uid, r.ID)
		m := member(got, 0)
		if m.State != store.RunMemberFailed || m.Code != tc.code || m.Sent {
			t.Fatalf("%v: %+v", tc.err, m)
		}
		if member(got, 1).State != store.RunMemberStarted {
			t.Fatalf("%v: the other member %+v", tc.err, member(got, 1))
		}
		if m, err := o.Retry(f.uid, r.ID, 0); err != nil || m.Attempt != 2 {
			t.Fatal(tc.err, err, m)
		}
		if got := settle(t, o, f.st, f.uid, r.ID); got.State != store.RunComplete {
			t.Fatalf("%v: %+v", tc.err, got.Members)
		}
	}
}

// An option refused by a machine that negotiated agents-v2 is the agent's
// own limit; on an older machine it is the version, as a single launch says.
func TestSelectionRunOptionRefusalNamesTheMachineOrTheAgent(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		f := newFixture(t)
		caps := terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "t", TerminalControl: true, TerminalEnabled: true, Readiness: terminalproto.Ready, Launch: true, Worktree: true, AgentsV2: v2, Tools: []string{"claude"}}
		if _, err := f.st.BeginRemoteConnectionFor(f.uid, f.laptop, terminalbroker.ID(), caps); err != nil {
			t.Fatal(err)
		}
		f.broker.answer = func(s sent) answer { return answer{kind: before, err: terminalbroker.ErrLaunchOptionUnsupported} }
		o := f.orchestrator(t)
		a := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/a/0", IsRepo: true, BaseBranch: "main"})
		b := f.folder(t, store.FolderProject{Host: f.laptop, Dir: "/a/1", IsRepo: true})
		r := f.selection(t, clineLaunch(), a, b)
		o.Start(f.uid, r.ID)
		got := settle(t, o, f.st, f.uid, r.ID)
		want := CodeOptionNeedsUpdate
		if v2 {
			want = CodeLaunchOptionUnsupported
		}
		for _, m := range got.Members {
			if m.State != store.RunMemberFailed || m.Code != want || m.Sent {
				t.Fatalf("agents-v2 %v: %+v", v2, m)
			}
		}
	}
}
