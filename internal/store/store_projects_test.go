package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func projectUser(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	u, err := s.CreateUser(name+"@example.com", name, "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func baseProject(name string) Project {
	return Project{Name: name, DefaultTool: "claude", DefaultModel: "claude-opus-4.1", SharedPrompt: "update the docs\nthen test", UseWorktrees: true,
		Members: []ProjectMember{{Host: "laptop", Dir: "/home/a/api"}, {Host: "laptop", Dir: "/home/a/web", Tool: "codex"}, {Host: "server", Dir: "/srv/api", Model: "m1"}}}
}

func mustProject(t *testing.T, s *Store, uid int64, p Project) Project {
	t.Helper()
	got, err := s.CreateProjectFor(uid, p)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestProjectCreateReadRoundTrip(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("  Docs sweep "))
	if p.ID == "" || p.Name != "Docs sweep" || p.CreatedAt == 0 || p.UpdatedAt != p.CreatedAt {
		t.Fatalf("%+v", p)
	}
	got, err := s.ProjectFor(uid, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Docs sweep" || got.DefaultTool != "claude" || got.DefaultModel != "claude-opus-4.1" || got.SharedPrompt != "update the docs\nthen test" || !got.UseWorktrees || len(got.Members) != 3 {
		t.Fatalf("%+v", got)
	}
	for i, m := range got.Members {
		if m.Position != i {
			t.Fatalf("positions %+v", got.Members)
		}
	}
	list, err := s.ListProjectsFor(uid)
	if err != nil || len(list) != 1 || len(list[0].Members) != 3 {
		t.Fatalf("%v %+v", err, list)
	}
	if other, _ := s.ListProjectsFor(projectUser(t, s, "b")); other == nil || len(other) != 0 {
		t.Fatalf("empty list should be non-nil empty: %#v", other)
	}
}

func TestProjectNameRulesAndUniqueness(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	mustProject(t, s, a, baseProject("Docs"))
	for _, name := range []string{"docs", "DOCS", " Docs "} {
		if _, err := s.CreateProjectFor(a, baseProject(name)); !errors.Is(err, ErrProjectNameTaken) || !errors.Is(err, ErrProjectName) {
			t.Fatalf("%q: %v", name, err)
		}
	}
	if _, err := s.CreateProjectFor(b, baseProject("docs")); err != nil {
		t.Fatalf("another account may reuse the name: %v", err)
	}
	for _, name := range []string{"", "   ", strings.Repeat("a", 81), "bad\nname", "tab\tname", "nul\x00", "\xff\xfe", "line\u2028sep"} {
		if _, err := s.CreateProjectFor(a, baseProject(name)); !errors.Is(err, ErrProjectName) || errors.Is(err, ErrProjectNameTaken) {
			t.Fatalf("%q: %v", name, err)
		}
	}
	if _, err := s.CreateProjectFor(a, baseProject(strings.Repeat("é", 80))); err != nil {
		t.Fatalf("80 characters (160 bytes) must fit: %v", err)
	}
	if _, err := s.CreateProjectFor(a, baseProject(strings.Repeat("é", 81))); !errors.Is(err, ErrProjectName) {
		t.Fatal(err)
	}
	// Renaming to a taken name, case-insensitively, is refused; to its own name in another case is fine.
	p := mustProject(t, s, a, baseProject("Other"))
	p.Name = "dOcS"
	if _, err := s.UpdateProjectFor(a, p); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatal(err)
	}
	p.Name = "OTHER"
	if _, err := s.UpdateProjectFor(a, p); err != nil {
		t.Fatal(err)
	}
}

func TestProjectLimit(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	for i := 0; i < MaxProjectsPerAccount; i++ {
		p := baseProject(fmt.Sprintf("p%02d", i))
		p.Members = nil
		mustProject(t, s, a, p)
	}
	if _, err := s.CreateProjectFor(a, baseProject("one more")); !errors.Is(err, ErrProjectLimit) {
		t.Fatal(err)
	}
	if _, err := s.CreateProjectFor(b, baseProject("fine")); err != nil {
		t.Fatalf("limit is per account: %v", err)
	}
	list, _ := s.ListProjectsFor(a)
	if len(list) != 50 {
		t.Fatal(len(list))
	}
	if err := s.DeleteProjectFor(a, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProjectFor(a, baseProject("fits again")); err != nil {
		t.Fatal(err)
	}
}

func TestProjectMemberLimitAndDuplicates(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := baseProject("many")
	p.Members = nil
	for i := 0; i < MaxProjectMembers; i++ {
		p.Members = append(p.Members, ProjectMember{Host: "m", Dir: fmt.Sprintf("/r/%d", i)})
	}
	made := mustProject(t, s, a, p)
	p.Members = append(p.Members, ProjectMember{Host: "m", Dir: "/r/extra"})
	if _, err := s.CreateProjectFor(a, p); !errors.Is(err, ErrMemberLimit) {
		t.Fatal(err)
	}
	made.Members = p.Members
	if _, err := s.UpdateProjectFor(a, made); !errors.Is(err, ErrMemberLimit) {
		t.Fatal(err)
	}

	for _, dups := range [][]ProjectMember{
		{{Host: "m", Dir: "/r/a"}, {Host: "m", Dir: "/r/a"}},
		{{Host: "m", Dir: "/r/a"}, {Host: "m", Dir: "/r/a/"}},
		{{Host: "m", Dir: "/r/a"}, {Host: "m", Dir: "/r/b/../a"}},
		{{Host: "m", Dir: "/r//a"}, {Host: "m", Dir: "/r/./a"}},
		{{Host: "m", Dir: "/r/a", Tool: "codex"}, {Host: "m", Dir: "/r/a", Model: "x"}},
	} {
		q := baseProject("dup")
		q.Members = dups
		_, err := s.CreateProjectFor(a, q)
		if !errors.Is(err, ErrDuplicateMember) || !strings.Contains(err.Error(), `"/r/a"`) || !strings.Contains(err.Error(), `"m"`) {
			t.Fatalf("%+v: %v", dups, err)
		}
	}
	// Same folder on another machine, and same machine different folder, are fine.
	q := baseProject("fine")
	q.Members = []ProjectMember{{Host: "m", Dir: "/r/a"}, {Host: "n", Dir: "/r/a"}, {Host: "m", Dir: "/r/b"}}
	got := mustProject(t, s, a, q)
	if got.Members[0].Dir != "/r/a" {
		t.Fatal(got.Members)
	}
	// Folders are stored cleaned.
	q = baseProject("cleaned")
	q.Members = []ProjectMember{{Host: "m", Dir: "/r//x/./y/../z/"}}
	got = mustProject(t, s, a, q)
	if got.Members[0].Dir != "/r/x/z" {
		t.Fatal(got.Members[0].Dir)
	}
	if again, _ := s.ProjectFor(a, got.ID); again.Members[0].Dir != "/r/x/z" {
		t.Fatal(again.Members[0].Dir)
	}
}

func TestProjectFieldValidation(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	secret := "SECRET PROMPT TEXT"
	cases := map[string]func(p *Project){
		"empty default tool":    func(p *Project) { p.DefaultTool = "" },
		"unknown default tool":  func(p *Project) { p.DefaultTool = "vim" },
		"member tool":           func(p *Project) { p.Members[0].Tool = "vim" },
		"default model chars":   func(p *Project) { p.DefaultModel = "-bad" },
		"default model nul":     func(p *Project) { p.DefaultModel = "a\x00" },
		"default model long":    func(p *Project) { p.DefaultModel = strings.Repeat("a", 101) },
		"member model chars":    func(p *Project) { p.Members[0].Model = "a b" },
		"member model long":     func(p *Project) { p.Members[0].Model = strings.Repeat("a", 101) },
		"relative dir":          func(p *Project) { p.Members[0].Dir = "home/a" },
		"empty dir":             func(p *Project) { p.Members[0].Dir = "" },
		"dot dir":               func(p *Project) { p.Members[0].Dir = "./a" },
		"dir newline":           func(p *Project) { p.Members[0].Dir = "/a\n/b" },
		"dir invalid utf8":      func(p *Project) { p.Members[0].Dir = "/a\xff" },
		"dir too long":          func(p *Project) { p.Members[0].Dir = "/" + strings.Repeat("a", 4096) },
		"empty host":            func(p *Project) { p.Members[0].Host = "" },
		"host control":          func(p *Project) { p.Members[0].Host = "a\nb" },
		"host long":             func(p *Project) { p.Members[0].Host = strings.Repeat("h", 256) },
		"prompt dash":           func(p *Project) { p.SharedPrompt = "-" + secret },
		"prompt too long":       func(p *Project) { p.SharedPrompt = secret + strings.Repeat("a", 2000) },
		"prompt control":        func(p *Project) { p.SharedPrompt = secret + "\x07" },
		"prompt tab":            func(p *Project) { p.SharedPrompt = secret + "\t" },
		"prompt invalid utf8":   func(p *Project) { p.SharedPrompt = secret + "\xff" },
		"prompt one word":       func(p *Project) { p.SharedPrompt = "refactor" },
		"prompt one word codex": func(p *Project) { p.SharedPrompt = "refactor"; p.DefaultTool = "gemini"; p.Members[1].Tool = "codex" },
	}
	for name, mutate := range cases {
		p := baseProject("x " + name)
		mutate(&p)
		_, err := s.CreateProjectFor(a, p)
		if !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("%s: error repeats the prompt: %v", name, err)
		}
	}
	if list, _ := s.ListProjectsFor(a); len(list) != 0 {
		t.Fatalf("a refused Project was stored: %+v", list)
	}
	// Exact limits are accepted.
	p := baseProject("limits")
	p.SharedPrompt = strings.Repeat("a ", 1000)
	p.DefaultModel = strings.Repeat("a", 100)
	p.Members = []ProjectMember{{Host: strings.Repeat("h", 255), Dir: "/" + strings.Repeat("a", 4095), Model: strings.Repeat("b", 100)}}
	mustProject(t, s, a, p)
	// Optional fields may be empty.
	min := Project{Name: "min", DefaultTool: "gemini"}
	got := mustProject(t, s, a, min)
	if got.Members == nil && false {
		t.Fatal()
	}
	// A single-word prompt is fine for a tool that does not read it as a command.
	p = baseProject("oneword")
	p.DefaultTool = "gemini"
	p.SharedPrompt = "refactor"
	p.Members = []ProjectMember{{Host: "m", Dir: "/a"}}
	mustProject(t, s, a, p)
}

func TestProjectScopingEveryMethod(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	p := mustProject(t, s, a, baseProject("mine"))
	run, err := s.CreateRunFor(a, p.ID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.ProjectFor(b, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if list, _ := s.ListProjectsFor(b); len(list) != 0 {
		t.Fatal(list)
	}
	steal := p
	steal.Name = "stolen"
	steal.Members = nil
	if _, err := s.UpdateProjectFor(b, steal); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if err := s.DeleteProjectFor(b, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.CreateRunFor(b, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.RunFor(b, run.ID); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	if _, err := s.RunsFor(b, p.ID, 20); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if err := s.SetRunMemberState(b, run.ID, 0, RunMemberFailed, "x", "", "", "", "", 1); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	if _, err := s.RunMemberStart(b, run.ID, 0); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	if err := s.SetRunMemberWorktreeToken(b, run.ID, 0, strings.Repeat("a", 24)); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	// A foreign claim does not block the owner either.
	if _, err := s.RunMemberStart(a, run.ID, 0); err != nil {
		t.Fatal(err)
	}
	// Nothing of a's changed.
	again, _ := s.ProjectFor(a, p.ID)
	if again.Name != "mine" || len(again.Members) != 3 {
		t.Fatalf("%+v", again)
	}
	r, _ := s.RunFor(a, run.ID)
	if r.Members[0].State != RunMemberPending || r.Members[0].Code != "" {
		t.Fatalf("%+v", r.Members[0])
	}
	// A nonexistent account cannot create.
	if _, err := s.CreateProjectFor(99999, baseProject("ghost")); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
}

func TestProjectUpdateReplacesMembersAtomically(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	other := mustProject(t, s, a, Project{Name: "Taken", DefaultTool: "claude"})
	_ = other
	p := mustProject(t, s, a, baseProject("Original"))

	changed := p
	changed.Name = "TAKEN" // fails last, after the members were replaced inside the transaction
	changed.Members = []ProjectMember{{Host: "x", Dir: "/new"}}
	if _, err := s.UpdateProjectFor(a, changed); !errors.Is(err, ErrProjectNameTaken) {
		t.Fatal(err)
	}
	got, _ := s.ProjectFor(a, p.ID)
	if got.Name != "Original" || len(got.Members) != 3 || got.Members[0].Dir != "/home/a/api" || got.UpdatedAt != p.UpdatedAt {
		t.Fatalf("old Project must survive a refused update: %+v", got)
	}
	// Invalid input is refused before touching anything.
	bad := p
	bad.Members = []ProjectMember{{Host: "x", Dir: "relative"}}
	if _, err := s.UpdateProjectFor(a, bad); !errors.Is(err, ErrProjectInvalid) {
		t.Fatal(err)
	}
	if got, _ := s.ProjectFor(a, p.ID); len(got.Members) != 3 {
		t.Fatal("members changed")
	}

	// A good update replaces everything, renumbers, keeps created_at and ID.
	good := p
	good.Name = "Renamed"
	good.DefaultTool = "codex"
	good.UseWorktrees = false
	good.SharedPrompt = ""
	good.Members = []ProjectMember{{Host: "y", Dir: "/b", Position: 7}, {Host: "y", Dir: "/a"}}
	upd, err := s.UpdateProjectFor(a, good)
	if err != nil {
		t.Fatal(err)
	}
	got, _ = s.ProjectFor(a, p.ID)
	if got.ID != p.ID || got.CreatedAt != p.CreatedAt || got.UpdatedAt < p.UpdatedAt || got.UpdatedAt != upd.UpdatedAt ||
		got.Name != "Renamed" || got.DefaultTool != "codex" || got.UseWorktrees || got.SharedPrompt != "" ||
		len(got.Members) != 2 || got.Members[0].Dir != "/b" || got.Members[0].Position != 0 || got.Members[1].Position != 1 {
		t.Fatalf("%+v", got)
	}
	if _, err := s.UpdateProjectFor(a, Project{ID: "nope", Name: "n", DefaultTool: "claude"}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	var rows int
	s.db.QueryRow(`SELECT COUNT(*) FROM project_members`).Scan(&rows)
	if rows != 2 {
		t.Fatalf("member rows %d", rows)
	}
}

func TestRunSnapshotSurvivesRenameAndDelete(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("Before"))
	run, err := s.CreateRunFor(a, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" || run.ProjectName != "Before" || run.ProjectID != p.ID || run.State != RunRunning || run.Prompt != "update the docs\nthen test" || !run.UseWorktrees || !run.HasPrompt() || len(run.Members) != 3 {
		t.Fatalf("%+v", run)
	}
	// Resolved tool and model: defaults, tool override drops the default model, model override.
	want := []struct{ tool, model string }{{"claude", "claude-opus-4.1"}, {"codex", ""}, {"claude", "m1"}}
	for i, m := range run.Members {
		if m.Tool != want[i].tool || m.Model != want[i].model || m.State != RunMemberPending || m.Attempt != 1 || m.Position != i {
			t.Fatalf("%d: %+v", i, m)
		}
	}
	snapshot := func() Run {
		r, err := s.RunFor(a, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	before := snapshot()

	p.Name = "After"
	p.DefaultTool = "gemini"
	p.SharedPrompt = "something else entirely"
	p.UseWorktrees = false
	p.Members = []ProjectMember{{Host: "z", Dir: "/z"}}
	if _, err := s.UpdateProjectFor(a, p); err != nil {
		t.Fatal(err)
	}
	if got := snapshot(); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Fatalf("rename rewrote the run:\n%+v\n%+v", before, got)
	}
	if err := s.DeleteProjectFor(a, p.ID); err != nil {
		t.Fatal(err)
	}
	got := snapshot()
	if got.ProjectName != "After" && got.ProjectName != "Before" {
		t.Fatal(got.ProjectName)
	}
	want2 := before
	want2.ProjectID = ""
	if fmt.Sprint(got) != fmt.Sprint(want2) {
		t.Fatalf("delete rewrote the run:\n%+v\n%+v", want2, got)
	}
	if got.ProjectName != "Before" || len(got.Members) != 3 || got.Prompt != "update the docs\nthen test" {
		t.Fatalf("%+v", got)
	}
	// The run can still be moved on after its Project is gone.
	if err := s.SetRunMemberState(a, run.ID, 0, RunMemberStarted, "", "sess", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
}

func TestCreateRunSubsetAndRefusals(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("sub"))
	run, err := s.CreateRunFor(a, p.ID, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Members) != 2 || run.Members[0].Position != 0 || run.Members[1].Position != 2 || run.Members[1].Host != "server" {
		t.Fatalf("%+v", run.Members)
	}
	for _, bad := range [][]int{{5}, {-1}, {1, 1}} {
		if _, err := s.CreateRunFor(a, p.ID, bad...); !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("%v: %v", bad, err)
		}
	}
	empty := mustProject(t, s, a, Project{Name: "empty", DefaultTool: "claude"})
	if _, err := s.CreateRunFor(a, empty.ID); !errors.Is(err, ErrRunNoMembers) {
		t.Fatal(err)
	}
	if _, err := s.CreateRunFor(a, "missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM project_runs`).Scan(&n)
	if n != 1 {
		t.Fatalf("refused runs left rows: %d", n)
	}
}

func TestRunMemberTransitionTable(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("t"))
	fresh := func() Run {
		r, err := s.CreateRunFor(a, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	set := func(r Run, pos int, state, code, sess string, attempt int) error {
		return s.SetRunMemberState(a, r.ID, pos, state, code, sess, "", "", "", attempt)
	}
	member := func(r Run, pos int) RunMember {
		got, err := s.RunFor(a, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Members[pos]
	}

	r := fresh()
	// pending -> pending, unknown states, bad payloads, stale attempt.
	for _, c := range []struct {
		name  string
		err   error
		state string
		code  string
		sess  string
		att   int
	}{
		{"pending to pending", ErrRunMemberTransition, "pending", "", "", 1},
		{"unknown state", ErrRunMemberTransition, "removed", "", "", 1},
		{"empty state", ErrRunMemberTransition, "", "", "", 1},
		{"started without session", ErrProjectInvalid, "started", "", "", 1},
		{"started with code", ErrProjectInvalid, "started", "x", "sess", 1},
		{"started stale attempt", ErrRunMemberStale, "started", "", "sess", 2},
		{"failed without code", ErrProjectInvalid, "failed", "", "", 1},
		{"failed bad code", ErrProjectInvalid, "failed", "Bad Code", "", 1},
		{"failed with session", ErrProjectInvalid, "failed", "x", "sess", 1},
		{"failed stale attempt", ErrRunMemberStale, "failed", "x", "", 0},
	} {
		if err := set(r, 0, c.state, c.code, c.sess, c.att); !errors.Is(err, c.err) {
			t.Fatalf("%s: %v", c.name, err)
		}
		if m := member(r, 0); m.State != RunMemberPending || m.SessionID != "" || m.Code != "" {
			t.Fatalf("%s changed the member: %+v", c.name, m)
		}
	}
	if err := s.SetRunMemberState(a, r.ID, 9, "failed", "x", "", "", "", "", 1); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}

	// pending -> started keeps what the machine reported; nothing moves a started member.
	if err := s.SetRunMemberState(a, r.ID, 0, "started", "", "sess-1", "/w/path", "api", "reet/x", 1); err != nil {
		t.Fatal(err)
	}
	m := member(r, 0)
	if m.State != "started" || m.SessionID != "sess-1" || m.WorktreePath != "/w/path" || m.WorktreeRepo != "api" || m.WorktreeBranch != "reet/x" {
		t.Fatalf("%+v", m)
	}
	for _, to := range []string{"pending", "started", "failed", "removed"} {
		for _, att := range []int{1, 2} {
			code, sess := "", ""
			if to == "failed" {
				code = "x"
			}
			if to == "started" {
				sess = "other"
			}
			if err := set(r, 0, to, code, sess, att); !errors.Is(err, ErrRunMemberTransition) {
				t.Fatalf("started -> %s attempt %d: %v", to, att, err)
			}
		}
	}
	if m := member(r, 0); m.SessionID != "sess-1" || m.State != "started" || m.Attempt != 1 {
		t.Fatalf("a started member moved: %+v", m)
	}

	// pending -> failed, then failed -> failed / started refused, retry rules.
	if err := set(r, 1, "failed", "session_limit", "", 1); err != nil {
		t.Fatal(err)
	}
	if m := member(r, 1); m.State != "failed" || m.Code != "session_limit" {
		t.Fatalf("%+v", m)
	}
	if err := set(r, 1, "failed", "other", "", 1); !errors.Is(err, ErrRunMemberTransition) {
		t.Fatal(err)
	}
	if err := set(r, 1, "started", "", "sess", 1); !errors.Is(err, ErrRunMemberTransition) {
		t.Fatal(err)
	}
	if err := s.SetRunMemberWorktreeToken(a, r.ID, 1, strings.Repeat("ab", 12)); err != nil {
		t.Fatal(err)
	}
	for _, att := range []int{0, 3} {
		if err := set(r, 1, "pending", "", "", att); !errors.Is(err, ErrRunMemberStale) {
			t.Fatalf("attempt %d: %v", att, err)
		}
	}
	if err := set(r, 1, "pending", "x", "", 2); !errors.Is(err, ErrProjectInvalid) {
		t.Fatal(err)
	}
	if err := set(r, 1, "pending", "", "", 2); err != nil { // definite refusal: new attempt
		t.Fatal(err)
	}
	m = member(r, 1)
	if m.State != "pending" || m.Attempt != 2 || m.Code != "" || m.WorktreeToken != strings.Repeat("ab", 12) {
		t.Fatalf("retry must clear the failure, bump the attempt and keep the token: %+v", m)
	}
	if err := set(r, 1, "failed", "x", "", 1); !errors.Is(err, ErrRunMemberStale) {
		t.Fatal("attempt 1 is gone", err)
	}
	if err := set(r, 1, "failed", "unknown", "", 2); err != nil {
		t.Fatal(err)
	}
	if err := set(r, 1, "pending", "", "", 2); err != nil { // outcome unknown: same attempt, same request id
		t.Fatal(err)
	}
	if m := member(r, 1); m.Attempt != 2 || m.State != "pending" {
		t.Fatalf("%+v", m)
	}
	if err := set(r, 1, "started", "", "sess-2", 2); err != nil {
		t.Fatal(err)
	}
}

func TestRunMemberWorktreeFactsAreBounded(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("w"))
	r, _ := s.CreateRunFor(a, p.ID)
	long := strings.Repeat("x", 4097)
	for _, args := range [][4]string{{"s", long, "", ""}, {"s", "", long[:256], ""}, {"s", "", "", long[:256]}, {"s", "/a\nb", "", ""}, {strings.Repeat("s", 129), "", "", ""}} {
		if err := s.SetRunMemberState(a, r.ID, 0, "started", "", args[0], args[1], args[2], args[3], 1); !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("%q: %v", args, err)
		}
	}
	if err := s.SetRunMemberState(a, r.ID, 0, "started", "", "s", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
}

func TestWorktreeTokenIsWriteOnceAndValidated(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("tok"))
	r, _ := s.CreateRunFor(a, p.ID)
	tok := strings.Repeat("0f", 12)
	for _, bad := range []string{"", "short", strings.ToUpper(tok), strings.Repeat("a", 49), "zz" + tok[2:]} {
		if err := s.SetRunMemberWorktreeToken(a, r.ID, 0, bad); !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	if err := s.SetRunMemberWorktreeToken(a, r.ID, 0, tok); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunMemberWorktreeToken(a, r.ID, 0, tok); err != nil {
		t.Fatalf("same token again: %v", err)
	}
	if err := s.SetRunMemberWorktreeToken(a, r.ID, 0, strings.Repeat("1e", 12)); !errors.Is(err, ErrRunMemberTransition) {
		t.Fatal(err)
	}
	if err := s.SetRunMemberWorktreeToken(a, r.ID, 7, tok); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	got, _ := s.RunFor(a, r.ID)
	if got.Members[0].WorktreeToken != tok || got.Members[1].WorktreeToken != "" {
		t.Fatalf("%+v", got.Members)
	}
}

func TestDerivedRunState(t *testing.T) {
	m := func(states ...string) []RunMember {
		var out []RunMember
		for _, s := range states {
			out = append(out, RunMember{State: s})
		}
		return out
	}
	for want, in := range map[string][]RunMember{
		RunRunning:  m("pending"),
		RunComplete: m("started", "started"),
		RunFailed:   m("failed", "failed"),
		RunPartial:  m("started", "failed"),
	} {
		if got := DeriveRunState(in); got != want {
			t.Fatalf("%v: %s want %s", in, got, want)
		}
	}
	if DeriveRunState(m("started", "pending", "failed")) != RunRunning || DeriveRunState(m("failed", "pending")) != RunRunning {
		t.Fatal("any pending member means running")
	}

	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("d"))
	r, _ := s.CreateRunFor(a, p.ID)
	state := func() string { g, _ := s.RunFor(a, r.ID); return g.State }
	if state() != RunRunning {
		t.Fatal(state())
	}
	s.SetRunMemberState(a, r.ID, 0, "started", "", "s0", "", "", "", 1)
	s.SetRunMemberState(a, r.ID, 1, "failed", "c", "", "", "", "", 1)
	if state() != RunRunning {
		t.Fatal("one member is still pending")
	}
	s.SetRunMemberState(a, r.ID, 2, "started", "", "s2", "", "", "", 1)
	if state() != RunPartial {
		t.Fatal(state())
	}
	list, _ := s.RunsFor(a, p.ID, 20)
	if list[0].State != RunPartial {
		t.Fatal(list[0].State)
	}
}

func TestRunsForOrderingAndLimit(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("l"))
	q := mustProject(t, s, a, baseProject("l2"))
	var ids []string
	for i := 0; i < 25; i++ {
		r, err := s.CreateRunFor(a, p.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	if _, err := s.CreateRunFor(a, q.ID); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ limit, want int }{{0, 20}, {-3, 20}, {5, 5}, {20, 20}, {100, 20}} {
		got, err := s.RunsFor(a, p.ID, c.limit)
		if err != nil || len(got) != c.want {
			t.Fatalf("limit %d: %v %d", c.limit, err, len(got))
		}
		for i, r := range got {
			if r.ID != ids[len(ids)-1-i] || r.ProjectID != p.ID || len(r.Members) != 1 {
				t.Fatalf("limit %d: order or scope wrong at %d: %+v", c.limit, i, r)
			}
		}
	}
	if none, err := s.RunsFor(a, q.ID, 20); err != nil || len(none) != 1 {
		t.Fatal(err, none)
	}
	// Equal timestamps still list newest-inserted first.
	if _, err := s.db.Exec(`UPDATE project_runs SET created_at=1`); err != nil {
		t.Fatal(err)
	}
	got, _ := s.RunsFor(a, p.ID, 3)
	if got[0].ID != ids[24] || got[1].ID != ids[23] {
		t.Fatal("ties must fall back to insertion order, newest first")
	}
}

func TestPendingRunsAcrossAccounts(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	pa := mustProject(t, s, a, baseProject("pa"))
	pb := mustProject(t, s, b, baseProject("pb"))
	if got, err := s.PendingRuns(); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("%v %#v", err, got)
	}
	ra, _ := s.CreateRunFor(a, pa.ID, 0)
	rb, _ := s.CreateRunFor(b, pb.ID, 0, 1)
	done, _ := s.CreateRunFor(a, pa.ID, 0)
	s.SetRunMemberState(a, done.ID, 0, "started", "", "s", "", "", "", 1)
	got, err := s.PendingRuns()
	if err != nil || len(got) != 2 || got[0] != (PendingRun{a, ra.ID}) || got[1] != (PendingRun{b, rb.ID}) {
		t.Fatalf("%v %+v", err, got)
	}
	// A run with one member still pending stays listed; once all are decided it leaves.
	s.SetRunMemberState(b, rb.ID, 0, "failed", "x", "", "", "", "", 1)
	if got, _ := s.PendingRuns(); len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	s.SetRunMemberState(b, rb.ID, 1, "failed", "x", "", "", "", "", 1)
	if got, _ := s.PendingRuns(); len(got) != 1 || got[0].RunID != ra.ID {
		t.Fatalf("%+v", got)
	}
	// An explicit retry puts the run back.
	s.SetRunMemberState(b, rb.ID, 1, "pending", "", "", "", "", "", 2)
	if got, _ := s.PendingRuns(); len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestRunMemberStartIsExclusive(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("race"))
	r, _ := s.CreateRunFor(a, p.ID)

	var won, lost int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m, err := s.RunMemberStart(a, r.ID, 0)
			switch {
			case err == nil && m.Position == 0 && m.State == RunMemberPending && m.Attempt == 1:
				atomic.AddInt32(&won, 1)
			case errors.Is(err, ErrRunMemberNotPending):
				atomic.AddInt32(&lost, 1)
			default:
				t.Errorf("%+v %v", m, err)
			}
		}()
	}
	wg.Wait()
	if won != 1 || lost != 15 {
		t.Fatalf("won %d lost %d", won, lost)
	}
	// A different member is independent.
	if _, err := s.RunMemberStart(a, r.ID, 1); err != nil {
		t.Fatal(err)
	}
	// The claim ends when the member is decided; a started member can never be claimed.
	if err := s.SetRunMemberState(a, r.ID, 0, "started", "", "s", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunMemberStart(a, r.ID, 0); !errors.Is(err, ErrRunMemberNotPending) {
		t.Fatal(err)
	}
	// Failed then retried: claimable again with the new attempt.
	s.SetRunMemberState(a, r.ID, 1, "failed", "x", "", "", "", "", 1)
	if _, err := s.RunMemberStart(a, r.ID, 1); !errors.Is(err, ErrRunMemberNotPending) {
		t.Fatal("failed member is not pending", err)
	}
	s.SetRunMemberState(a, r.ID, 1, "pending", "", "", "", "", "", 2)
	m, err := s.RunMemberStart(a, r.ID, 1)
	if err != nil || m.Attempt != 2 {
		t.Fatalf("%+v %v", m, err)
	}
	// Releasing a claim without an outcome lets the member be claimed again.
	if _, err := s.RunMemberStart(a, r.ID, 2); err != nil {
		t.Fatal(err)
	}
	s.ReleaseRunMember(a, r.ID, 2)
	if _, err := s.RunMemberStart(a, r.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RunMemberStart(a, r.ID, 42); !errors.Is(err, ErrRunNotFound) {
		t.Fatal(err)
	}
	// A refused lookup leaves no claim behind.
	if _, err := s.RunMemberStart(a, r.ID, 42); !errors.Is(err, ErrRunNotFound) {
		t.Fatal("claim leaked", err)
	}
}

func TestRunMemberClaimsDoNotSurviveRestart(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("restart"))
	r, _ := s.CreateRunFor(a, p.ID)
	if _, err := s.RunMemberStart(a, r.ID, 0); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	pending, _ := s2.PendingRuns()
	if len(pending) != 1 || pending[0].RunID != r.ID {
		t.Fatal(pending)
	}
	if _, err := s2.RunMemberStart(a, r.ID, 0); err != nil {
		t.Fatalf("a restarted controller must be able to resume a pending member: %v", err)
	}
	// Data survived.
	if got, err := s2.RunFor(a, r.ID); err != nil || got.ProjectName != "restart" || got.Prompt == "" {
		t.Fatal(err, got)
	}
}

func TestPromptNeverInErrorsOrJSON(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := baseProject("secret")
	p.SharedPrompt = "PRIVATE words here"
	made := mustProject(t, s, a, p)
	r, _ := s.CreateRunFor(a, made.ID)
	raw, err := jsonOf(r)
	if err != nil || strings.Contains(raw, "PRIVATE") {
		t.Fatalf("run JSON leaks the prompt: %v %s", err, raw)
	}
	for _, e := range []error{ErrProjectLimit, ErrProjectName, ErrProjectNameTaken, ErrMemberLimit, ErrDuplicateMember, ErrProjectInvalid, ErrRunMemberNotPending, ErrRunMemberTransition} {
		if strings.Contains(e.Error(), "PRIVATE") {
			t.Fatal(e)
		}
	}
}

func TestProjectMigrationIsIdempotentAndOpensExistingDB(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("keep"))
	r, _ := s.CreateRunFor(a, p.ID)
	for i := 0; i < 3; i++ {
		if err := s.migrateProjects(); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if got, err := s2.ProjectFor(a, p.ID); err != nil || len(got.Members) != 3 {
		t.Fatal(err, got)
	}
	if got, err := s2.RunFor(a, r.ID); err != nil || len(got.Members) != 3 {
		t.Fatal(err, got)
	}
	// A database from before Projects (no such tables) opens and gains them.
	s2.db.Exec(`DROP TABLE project_run_members`)
	s2.db.Exec(`DROP TABLE project_runs`)
	s2.db.Exec(`DROP TABLE project_members`)
	s2.db.Exec(`DROP TABLE projects`)
	if err := s2.migrateProjects(); err != nil {
		t.Fatal(err)
	}
	if list, err := s2.ListProjectsFor(a); err != nil || len(list) != 0 {
		t.Fatal(err, list)
	}
}

func TestProjectsDieWithTheirAccount(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "gone"), projectUser(t, s, "stays")
	pa := mustProject(t, s, a, baseProject("a1"))
	pb := mustProject(t, s, b, baseProject("b1"))
	s.CreateRunFor(a, pa.ID)
	s.CreateRunFor(b, pb.ID)
	if err := s.DeleteUser(a); err != nil {
		t.Fatal(err)
	}
	count := func(table string, uid int64) int {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE user_id=?`, uid).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	for _, table := range []string{"projects", "project_members", "project_runs", "project_run_members"} {
		if count(table, a) != 0 {
			t.Fatalf("%s rows survive the account", table)
		}
		if count(table, b) == 0 {
			t.Fatalf("%s rows of another account were removed", table)
		}
	}
	if pending, _ := s.PendingRuns(); len(pending) != 1 || pending[0].UID != b {
		t.Fatalf("%+v", pending)
	}
}

func TestDeleteProjectRemovesMembersOnly(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("del"))
	r, _ := s.CreateRunFor(a, p.ID)
	if err := s.DeleteProjectFor(a, p.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM project_members`).Scan(&n)
	if n != 0 {
		t.Fatal("members survive their Project")
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM project_run_members WHERE run_id=?`, r.ID).Scan(&n)
	if n != 3 {
		t.Fatal("run members were removed")
	}
	if err := s.DeleteProjectFor(a, p.ID); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.RunsFor(a, p.ID, 20); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	// The name is free again.
	if _, err := s.CreateProjectFor(a, baseProject("DEL")); err != nil {
		t.Fatal(err)
	}
}

func jsonOf(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

func TestInvisibleCharactersAreRefused(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	for _, r := range []string{"\u202e", "\u200f", "\u2066", "\u200b", "\ufeff", "\u2028", "\u2029", "\u00ad"} {
		p := baseProject("name" + r + "x")
		if _, err := s.CreateProjectFor(a, p); !errors.Is(err, ErrProjectName) {
			t.Fatalf("name %q: %v", r, err)
		}
		p = baseProject("host name")
		p.Members[0].Host = "lap" + r + "top"
		if _, err := s.CreateProjectFor(a, p); !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("host %q: %v", r, err)
		}
		p = baseProject("dir name")
		p.Members[0].Dir = "/home/a" + r + "/api"
		if _, err := s.CreateProjectFor(a, p); !errors.Is(err, ErrProjectInvalid) {
			t.Fatalf("dir %q: %v", r, err)
		}
	}
	// Ordinary non-ASCII text is still fine.
	p := baseProject("Café ü 日本")
	p.Members[0].Dir = "/home/ü/日本"
	mustProject(t, s, a, p)
}

func TestDuplicateMachinesUseTheServersHostRule(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	for _, hosts := range [][2]string{{"Mac", "mac "}, {"Mac", "MAC"}, {" mac", "mAc"}} {
		p := baseProject("dup")
		p.Members = []ProjectMember{{Host: hosts[0], Dir: "/r/a"}, {Host: hosts[1], Dir: "/r//a/"}}
		if _, err := s.CreateProjectFor(a, p); !errors.Is(err, ErrDuplicateMember) {
			t.Fatalf("%q: %v", hosts, err)
		}
	}
	p := baseProject("ok")
	p.Members = []ProjectMember{{Host: "Mac", Dir: "/r/a"}, {Host: "mac ", Dir: "/r/b"}}
	got := mustProject(t, s, a, p)
	if got.Members[1].Host != "mac" {
		t.Fatalf("host is stored trimmed: %q", got.Members[1].Host)
	}
	if got.Members[0].Host != "Mac" {
		t.Fatal("the user's spelling is kept")
	}
	// The database refuses it too, whatever the Go check says.
	if _, err := s.db.Exec(`INSERT INTO project_members(project_id,user_id,position,host,dir) VALUES(?,?,5,'MAC','/r/a')`, got.ID, a); err == nil {
		t.Fatal("database allowed a case-variant duplicate")
	}
	// Case of the folder is significant (documented).
	p = baseProject("case")
	p.Members = []ProjectMember{{Host: "m", Dir: "/r/A"}, {Host: "m", Dir: "/r/a"}}
	mustProject(t, s, a, p)
}

func TestRunPromptStaysOutOfListsAndJSON(t *testing.T) {
	raw, err := jsonOf(Run{Prompt: "sentinel", ProjectName: "x"})
	if err != nil || strings.Contains(raw, "sentinel") {
		t.Fatalf("%v %s", err, raw)
	}
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("pr"))
	r, _ := s.CreateRunFor(a, p.ID)
	list, _ := s.RunsFor(a, p.ID, 20)
	if len(list) != 1 || list[0].Prompt != "" || list[0].HasPrompt() {
		t.Fatalf("%+v", list)
	}
	if got, _ := s.RunFor(a, r.ID); got.Prompt == "" {
		t.Fatal("RunFor keeps the prompt for the orchestrator")
	}
}

func TestMigrationAddsLookupIndexes(t *testing.T) {
	s := newStore(t)
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name IN ('project_members_user','project_members_machine')`).Scan(&n)
	if n != 2 {
		t.Fatal(n)
	}
}

func TestStaleRunMemberClaimsExpire(t *testing.T) {
	s := newStore(t)
	a := projectUser(t, s, "a")
	p := mustProject(t, s, a, baseProject("claims"))
	r, _ := s.CreateRunFor(a, p.ID)
	clock := int64(1_000_000)
	s.runClaims.now = func() int64 { return clock }
	if _, err := s.RunMemberStart(a, r.ID, 0); err != nil {
		t.Fatal(err)
	}
	clock += RunClaimTTL.Milliseconds() - 1
	if _, err := s.RunMemberStart(a, r.ID, 0); !errors.Is(err, ErrRunMemberNotPending) {
		t.Fatal("claim must hold until the TTL", err)
	}
	clock += 2
	if _, err := s.RunMemberStart(a, r.ID, 0); err != nil {
		t.Fatalf("an old claim on a still-pending member is claimable again: %v", err)
	}
	// A decided member is never claimable, however old the claim.
	s.SetRunMemberState(a, r.ID, 0, "started", "", "s", "", "", "", 1)
	clock += 10 * RunClaimTTL.Milliseconds()
	if _, err := s.RunMemberStart(a, r.ID, 0); !errors.Is(err, ErrRunMemberNotPending) {
		t.Fatal(err)
	}
}

func TestProjectVersionGuardsUpdates(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("guarded"))
	if p.Version != 1 {
		t.Fatal("new version", p.Version)
	}
	if got, _ := s.ProjectFor(uid, p.ID); got.Version != 1 {
		t.Fatal("read version", got.Version)
	}
	first := p
	first.Name = "first"
	saved, err := s.UpdateProjectFor(uid, first) // Version 1 is the precondition
	if err != nil || saved.Version != 2 {
		t.Fatal(err, saved.Version)
	}
	stale := p
	stale.Name = "stale"
	stale.Members = stale.Members[:1]
	if _, err := s.UpdateProjectFor(uid, stale); !errors.Is(err, ErrProjectStale) {
		t.Fatal("stale update", err)
	}
	if got, _ := s.ProjectFor(uid, p.ID); got.Name != "first" || len(got.Members) != 3 || got.Version != 2 {
		t.Fatalf("stale update changed the Project %+v", got)
	}
	// No precondition: last write wins, the version still moves.
	free := p
	free.Version = 0
	free.Name = "free"
	if saved, err := s.UpdateProjectFor(uid, free); err != nil || saved.Version != 3 {
		t.Fatal(err, saved.Version)
	}
	// Another account's id with the right version is still not found.
	other := projectUser(t, s, "b")
	if _, err := s.UpdateProjectFor(other, Project{ID: p.ID, Name: "x", DefaultTool: "claude", Version: 3}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
	if list, _ := s.ListProjectsFor(uid); list[0].Version != 3 {
		t.Fatal("list version", list[0].Version)
	}
}

func TestProjectVersionColumnIsAddedToAnOldTable(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("old"))
	if _, err := s.db.Exec(`ALTER TABLE projects DROP COLUMN version`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.migrateProjects(); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := s.ProjectFor(uid, p.ID); err != nil || got.Version != 1 {
		t.Fatal(err, got.Version)
	}
}

func TestProjectMayHaveWorktreesLooksAtEveryRun(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("wt"))
	if has, err := s.ProjectMayHaveWorktrees(uid, p.ID); err != nil || has {
		t.Fatal(has, err)
	}
	first, err := s.CreateRunFor(uid, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunMemberState(uid, first.ID, 0, RunMemberStarted, "", "s1", "/home/a/.reet/worktrees/wt/api/0123456789abcdef", "api", "reet/wt-0123456789abcdef", 1); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxRunsListed+1; i++ { // the worktree run is beyond the newest 20
		if _, err := s.CreateRunFor(uid, p.ID, 1); err != nil {
			t.Fatal(err)
		}
	}
	if has, err := s.ProjectMayHaveWorktrees(uid, p.ID); err != nil || !has {
		t.Fatal(has, err)
	}
	if has, _ := s.ProjectMayHaveWorktrees(projectUser(t, s, "b"), p.ID); has {
		t.Fatal("another account saw worktrees")
	}
	if _, err := s.ProjectMayHaveWorktrees(uid, "missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal(err)
	}
}

func TestProjectFolderErrorsNameTheRepository(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := baseProject("dirs")
	p.Members[1].Dir = "/home/a/ web"
	_, err := s.CreateProjectFor(uid, p)
	var me *ProjectMemberError
	if !errors.As(err, &me) || me.Index != 1 || me.Field != "dir" || !errors.Is(err, ErrProjectInvalid) {
		t.Fatalf("%#v", err)
	}
}

func TestRunMemberSentIsStickyUntilKnownToHaveStartedNothing(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("Sent"))
	run, err := s.CreateRunFor(uid, p.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	fail := func(code string, attempt int) {
		t.Helper()
		if err := s.SetRunMemberState(uid, run.ID, 0, RunMemberFailed, code, "", "", "", "", attempt); err != nil {
			t.Fatal(err)
		}
	}
	member := func() RunMember {
		t.Helper()
		r, err := s.RunFor(uid, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r.Members[0]
	}
	if m := member(); m.Sent || m.Attempt != 1 {
		t.Fatalf("%+v", m)
	}
	if err := s.MarkRunMemberSent(uid, run.ID, 0, 2); !errors.Is(err, ErrRunMemberStale) {
		t.Fatal("marked another attempt", err)
	}
	if err := s.MarkRunMemberSent(uid, run.ID, 0, 1); err != nil {
		t.Fatal(err)
	}
	// Possibly sent, then a precheck refusal on the next try: the code says
	// "definite" but the flag keeps the request id.
	fail("unknown_outcome", 1)
	if m, err := s.RetryRunMember(uid, run.ID, 0); err != nil || m.Attempt != 1 || !m.Sent {
		t.Fatal(m, err)
	}
	fail("machine_unavailable", 1)
	if err := s.SetRunMemberState(uid, run.ID, 0, RunMemberPending, "", "", "", "", "", 2); !errors.Is(err, ErrRunMemberStale) {
		t.Fatal("a new attempt while the create may have been sent", err)
	}
	if m, err := s.RetryRunMember(uid, run.ID, 0); err != nil || m.Attempt != 1 || !member().Sent {
		t.Fatal(m, err)
	}
	// The broker then reports a definite refusal for that very request id.
	if err := s.ClearRunMemberSent(uid, run.ID, 0, 1); err != nil {
		t.Fatal(err)
	}
	fail("tool_not_installed", 1)
	if m, err := s.RetryRunMember(uid, run.ID, 0); err != nil || m.Attempt != 2 || member().Sent {
		t.Fatal(m, err)
	}
	// Never sent at attempt 2 (a precheck refusal): the next attempt is allowed.
	fail("machine_unavailable", 2)
	if m, err := s.RetryRunMember(uid, run.ID, 0); err != nil || m.Attempt != 3 {
		t.Fatal(m, err)
	}
	if _, err := s.RetryRunMember(uid, run.ID, 0); !errors.Is(err, ErrRunMemberNotFailed) {
		t.Fatal("retried a pending member", err)
	}
	if _, err := s.RetryRunMember(projectUser(t, s, "b"), run.ID, 0); !errors.Is(err, ErrRunNotFound) {
		t.Fatal("another account retried", err)
	}
	if err := s.ClearRunMemberSent(uid, run.ID, 0, 1); !errors.Is(err, ErrRunMemberStale) {
		t.Fatal("cleared an old attempt", err)
	}
}

func TestRacingRetriesAgreeOnOneAttempt(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustProject(t, s, uid, baseProject("Race"))
	for _, sent := range []bool{true, false} {
		run, err := s.CreateRunFor(uid, p.ID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if sent {
			if err := s.MarkRunMemberSent(uid, run.ID, 0, 1); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.SetRunMemberState(uid, run.ID, 0, RunMemberFailed, "unknown_outcome", "", "", "", "", 1); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		var won atomic.Int32
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := s.RetryRunMember(uid, run.ID, 0); err == nil {
					won.Add(1)
				} else if !errors.Is(err, ErrRunMemberNotFailed) {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		r, _ := s.RunFor(uid, run.ID)
		want := 2
		if sent {
			want = 1
		}
		if won.Load() != 1 || r.Members[0].Attempt != want || r.Members[0].State != RunMemberPending {
			t.Fatal(sent, won.Load(), r.Members[0])
		}
	}
}

func TestStartRunIsIdempotentAndRefusesOverlappingRuns(t *testing.T) {
	s := newStore(t)
	uid, other := projectUser(t, s, "a"), projectUser(t, s, "b")
	p := mustProject(t, s, uid, baseProject("Start"))
	q := mustProject(t, s, other, baseProject("Start"))
	const req = "start-request-0000000001"
	if _, _, err := s.StartRunFor(uid, p.ID, "short", []int{0}); !errors.Is(err, ErrProjectInvalid) {
		t.Fatal("a bad request id", err)
	}
	first, created, err := s.StartRunFor(uid, p.ID, req, []int{0, 2})
	if err != nil || !created || len(first.Members) != 2 {
		t.Fatal(first, created, err)
	}
	again, created, err := s.StartRunFor(uid, p.ID, req, []int{2, 0})
	if err != nil || created || again.ID != first.ID {
		t.Fatal("replay", again.ID, created, err)
	}
	if _, _, err := s.StartRunFor(uid, p.ID, req, []int{0}); !errors.Is(err, ErrRunRequestChanged) {
		t.Fatal("changed members", err)
	}
	if _, _, err := s.StartRunFor(uid, p.ID, "start-request-0000000002", []int{1}); !errors.Is(err, ErrRunInProgress) {
		t.Fatal("overlap", err)
	}
	// Another account's run with the same request id is its own.
	theirs, created, err := s.StartRunFor(other, q.ID, req, []int{0, 2})
	if err != nil || !created || theirs.ID == first.ID {
		t.Fatal(theirs, created, err)
	}
	if _, _, err := s.StartRunFor(other, p.ID, "start-request-0000000003", []int{0}); !errors.Is(err, ErrProjectNotFound) {
		t.Fatal("another account's Project", err)
	}
	for _, pos := range []int{0, 2} {
		if err := s.SetRunMemberState(uid, first.ID, pos, RunMemberFailed, "session_limit", "", "", "", "", 1); err != nil {
			t.Fatal(err)
		}
	}
	if next, created, err := s.StartRunFor(uid, p.ID, "start-request-0000000002", []int{1}); err != nil || !created || next.ID == first.ID {
		t.Fatal("after the first run finished", err)
	}
}
