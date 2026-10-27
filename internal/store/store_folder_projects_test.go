package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fp(host, dir string) FolderProject { return FolderProject{Host: host, Dir: dir} }

func mustFP(t *testing.T, s *Store, uid int64, p FolderProject) FolderProject {
	t.Helper()
	got, err := s.CreateFolderProjectFor(uid, p)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Fatalf("no id: %+v", got)
	}
	return got
}

func TestFolderProjectRoundTrip(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	in := FolderProject{Host: "laptop", Dir: "/home/a/reet", Name: "  Reet ", IsRepo: true, BaseBranch: "main", WorktreeDefault: "yes",
		DefaultTool: "claude", DefaultModel: "claude-opus-4.1", StarterPrompt: "fix the build", LegacyProjectID: "old:1", ID: "ignored", Version: 9, Position: 7}
	got := mustFP(t, s, uid, in)
	if got.ID == "ignored" || got.Name != "Reet" || got.Version != 1 || got.Position != 0 || got.CreatedAt == 0 || got.UpdatedAt != got.CreatedAt {
		t.Fatalf("%+v", got)
	}
	read, err := s.FolderProjectFor(uid, got.ID)
	if err != nil || read != got {
		t.Fatalf("%v\n%+v\n%+v", err, read, got)
	}
	if read.BaseBranch != "main" || !read.IsRepo || read.WorktreeDefault != "yes" || read.StarterPrompt != "fix the build" || read.LegacyProjectID != "old:1" || read.DefaultTool != "claude" || read.DefaultModel != "claude-opus-4.1" || read.Colour != "" {
		t.Fatalf("%+v", read)
	}
	// Blank worktreeDefault means inherit; positions follow creation order.
	b := mustFP(t, s, uid, fp("laptop", "/home/a/b"))
	if b.WorktreeDefault != "inherit" || b.Position != 1 {
		t.Fatalf("%+v", b)
	}
	list, err := s.ListFolderProjectsFor(uid)
	if err != nil || len(list) != 2 || list[0].ID != got.ID || list[1].ID != b.ID {
		t.Fatalf("%v %+v", err, list)
	}
	if _, err := s.FolderProjectFor(uid, "nope"); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal(err)
	}
	if empty, err := s.ListFolderProjectsFor(projectUser(t, s, "z")); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("%v %#v", err, empty)
	}
}

func TestFolderProjectLimit(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	other := projectUser(t, s, "b")
	for i := 0; i < MaxFolderProjects; i++ {
		mustFP(t, s, uid, fp("m", fmt.Sprintf("/d/%d", i)))
	}
	if _, err := s.CreateFolderProjectFor(uid, fp("m", "/d/extra")); !errors.Is(err, ErrFolderProjectLimit) {
		t.Fatalf("101st: %v", err)
	}
	// The limit is per account.
	mustFP(t, s, other, fp("m", "/d/extra"))
	// Deleting one makes room again.
	list, _ := s.ListFolderProjectsFor(uid)
	if len(list) != 100 {
		t.Fatal(len(list))
	}
	if err := s.DeleteFolderProjectFor(uid, list[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	mustFP(t, s, uid, fp("m", "/d/extra"))
}

// enrolledID enrolls a machine named host for uid and returns its id, the
// value a folder project's Host holds.
func enrolledID(t *testing.T, s *Store, uid int64, host string) string {
	t.Helper()
	a, _, err := s.EnrollSyncAgent(uid, host, "test")
	if err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func TestFolderProjectUniqueMachineFolder(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	laptop, server := enrolledID(t, s, uid, "laptop"), enrolledID(t, s, uid, "server")
	mustFP(t, s, uid, fp(laptop, "/a/b/"))
	if _, err := s.CreateFolderProjectFor(uid, fp(laptop, "/a/b")); !errors.Is(err, ErrFolderProjectExists) {
		t.Fatalf("cleaned dir: %v", err)
	}
	if _, err := s.CreateFolderProjectFor(uid, fp(laptop, "/a//b/./")); !errors.Is(err, ErrFolderProjectExists) {
		t.Fatalf("dotted dir: %v", err)
	}
	// The machine is its enrollment id, compared exactly: another spelling is
	// another key (the API admits only real ids).
	if _, err := s.CreateFolderProjectFor(uid, fp(strings.ToUpper(laptop), "/a/b")); err != nil {
		t.Fatalf("hosts compared ignoring case: %v", err)
	}
	// Another machine, or another account, may use the same folder.
	got := mustFP(t, s, uid, fp(server, "/a/b"))
	if got.Dir != "/a/b" || got.Host != server {
		t.Fatalf("stored: %+v", got)
	}
	mustFP(t, s, projectUser(t, s, "b"), fp(laptop, "/a/b"))
	// Moving a project onto a taken folder is refused too.
	c := mustFP(t, s, uid, fp(laptop, "/c"))
	c.Dir = "/a/b/"
	if _, err := s.UpdateFolderProjectFor(uid, c, 0); !errors.Is(err, ErrFolderProjectExists) {
		t.Fatalf("update: %v", err)
	}
	// A legacy source can only be imported once.
	l := fp(laptop, "/l1")
	l.LegacyProjectID = "p:0"
	mustFP(t, s, uid, l)
	l.Dir = "/l2"
	if _, err := s.CreateFolderProjectFor(uid, l); !errors.Is(err, ErrFolderProjectExists) {
		t.Fatalf("legacy: %v", err)
	}
}

// A database made before hosts were enrollment ids: opening it turns each
// host name into the account's machine id where exactly one machine has that
// name, leaves anything else as it was, and swaps the unique index.
func TestFolderProjectHostNamesBecomeAgentIDs(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid := projectUser(t, s, "a")
	other := projectUser(t, s, "b")
	laptop, server := enrolledID(t, s, uid, "laptop"), enrolledID(t, s, uid, "server")
	theirLaptop := enrolledID(t, s, other, "laptop")
	for _, stmt := range []string{`DROP INDEX IF EXISTS folder_projects_agent_dir`,
		`CREATE UNIQUE INDEX IF NOT EXISTS folder_projects_machine ON folder_projects(user_id,lower(host),dir)`} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	byName := mustFP(t, s, uid, fp("LAPTOP", "/a"))
	byName2 := mustFP(t, s, uid, fp("server", "/b"))
	gone := mustFP(t, s, uid, fp("gone", "/c"))
	byID := mustFP(t, s, uid, fp(laptop, "/d"))
	// The same folder saved under both the id and the name: converting the
	// name would collide, so that row is left alone.
	clashID := mustFP(t, s, uid, fp(laptop, "/e"))
	clashName := mustFP(t, s, uid, fp("laptop", "/e"))
	theirs := mustFP(t, s, other, fp("laptop", "/z"))
	s.Close()
	want := map[string]string{byName.ID: laptop, byName2.ID: server, gone.ID: "gone", byID.ID: laptop, clashID.ID: laptop, clashName.ID: "laptop"}
	for i := 0; i < 2; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal(i, err)
		}
		for id, host := range want {
			p, err := s.FolderProjectFor(uid, id)
			if err != nil || p.Host != host {
				t.Fatalf("open %d: %s host %q, want %q (%v)", i, id, p.Host, host, err)
			}
		}
		if p, _ := s.FolderProjectFor(other, theirs.ID); p.Host != theirLaptop {
			t.Fatalf("open %d: another account's row: %+v", i, p)
		}
		var oldIndex, newIndex int
		s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='folder_projects_machine'`).Scan(&oldIndex)
		s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='folder_projects_agent_dir'`).Scan(&newIndex)
		if oldIndex != 0 || newIndex != 1 {
			t.Fatalf("open %d: indexes old=%d new=%d", i, oldIndex, newIndex)
		}
		if _, err := s.CreateFolderProjectFor(uid, fp(laptop, "/a")); !errors.Is(err, ErrFolderProjectExists) {
			t.Fatalf("open %d: converted row not unique: %v", i, err)
		}
		s.Close()
	}
}

func TestFolderProjectNameDefaultsAndClash(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	a := mustFP(t, s, uid, fp("laptop", "/home/a/reet"))
	if a.Name != "reet" {
		t.Fatalf("%q", a.Name)
	}
	b := mustFP(t, s, uid, fp("laptop", "/work/reet"))
	if b.Name != "reet (2)" {
		t.Fatalf("%q", b.Name)
	}
	c := mustFP(t, s, uid, fp("laptop", "/other/REET"))
	if c.Name != "REET (3)" {
		t.Fatalf("%q", c.Name)
	}
	// Explicit clash, ignoring case, is refused.
	x := fp("laptop", "/x")
	x.Name = "Reet"
	if _, err := s.CreateFolderProjectFor(uid, x); !errors.Is(err, ErrFolderProjectName) {
		t.Fatalf("%v", err)
	}
	// Another account has its own names.
	if o := mustFP(t, s, projectUser(t, s, "b"), fp("laptop", "/home/a/reet")); o.Name != "reet" {
		t.Fatalf("%q", o.Name)
	}
	// Renaming to another's name is refused; keeping one's own name is fine.
	b.Name = "REET"
	if _, err := s.UpdateFolderProjectFor(uid, b, 0); !errors.Is(err, ErrFolderProjectName) {
		t.Fatalf("%v", err)
	}
	a2 := a
	a2.Name = "REET"
	if got, err := s.UpdateFolderProjectFor(uid, a2, 0); err != nil || got.Name != "REET" {
		t.Fatalf("%v %+v", err, got)
	}
	// A long folder name is cut to 80 characters, suffix included.
	long := strings.Repeat("é", 90)
	l1 := mustFP(t, s, uid, fp("laptop", "/p/"+long))
	l2 := mustFP(t, s, uid, fp("laptop", "/q/"+long))
	if n := len([]rune(l1.Name)); n != 80 {
		t.Fatal(n)
	}
	if n := len([]rune(l2.Name)); n > 80 || !strings.HasSuffix(l2.Name, " (2)") {
		t.Fatalf("%d %q", n, l2.Name)
	}
}

func TestFolderProjectValidation(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	cases := map[string]func(p *FolderProject){
		"relative dir":       func(p *FolderProject) { p.Dir = "work/api" },
		"empty dir":          func(p *FolderProject) { p.Dir = "" },
		"control in dir":     func(p *FolderProject) { p.Dir = "/a\nb" },
		"empty host":         func(p *FolderProject) { p.Host = "  " },
		"blank host":         func(p *FolderProject) { p.Host = "" },
		"129-byte host":      func(p *FolderProject) { p.Host = strings.Repeat("h", 129) },
		"host with a space":  func(p *FolderProject) { p.Host = " laptop" },
		"control in host":    func(p *FolderProject) { p.Host = "lap\ttop" },
		"81 char name":       func(p *FolderProject) { p.Name = strings.Repeat("n", 81) },
		"control name":       func(p *FolderProject) { p.Name = "a\x07b" },
		"format name":        func(p *FolderProject) { p.Name = "a‮b" },
		"prompt dash":        func(p *FolderProject) { p.StarterPrompt = "-rf now please" },
		"prompt 2001 bytes":  func(p *FolderProject) { p.StarterPrompt = strings.Repeat("a", 2001) },
		"prompt control":     func(p *FolderProject) { p.StarterPrompt = "a\x00b c" },
		"bad ref name":       func(p *FolderProject) { p.BaseBranch = "a..b" },
		"ref name dash":      func(p *FolderProject) { p.BaseBranch = "-x" },
		"ref name space":     func(p *FolderProject) { p.BaseBranch = "a b" },
		"worktree maybe":     func(p *FolderProject) { p.WorktreeDefault = "maybe" },
		"worktree uppercase": func(p *FolderProject) { p.WorktreeDefault = "YES" },
		"bad tool":           func(p *FolderProject) { p.DefaultTool = "vim" },
		"bad model":          func(p *FolderProject) { p.DefaultModel = "m o d" },
		"bad colour":         func(p *FolderProject) { p.Colour = "chartreuse" },
		"colour uppercase":   func(p *FolderProject) { p.Colour = "BLUE" },
	}
	for name, mod := range cases {
		p := fp("laptop", "/ok")
		mod(&p)
		_, err := s.CreateFolderProjectFor(uid, p)
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if strings.HasPrefix(name, "81") || strings.HasPrefix(name, "control name") || strings.HasPrefix(name, "format") {
			if !errors.Is(err, ErrFolderProjectName) {
				t.Errorf("%s: %v", name, err)
			}
		} else if !errors.Is(err, ErrFolderProjectInvalid) {
			t.Errorf("%s: %v", name, err)
		}
		// Updates refuse the same values and the project is untouched.
	}
	if l, _ := s.ListFolderProjectsFor(uid); len(l) != 0 {
		t.Fatalf("refused projects were stored: %+v", l)
	}
	// Values at the limits are accepted; a prompt is never echoed in errors.
	ok := fp("laptop", "/ok")
	ok.Name = strings.Repeat("é", 80)
	ok.StarterPrompt = strings.Repeat("a", 2000)
	ok.WorktreeDefault = "no"
	ok.Colour = "blue"
	ok.BaseBranch = "feature/x-1"
	ok.DefaultTool = "codex"
	made := mustFP(t, s, uid, ok)
	if made.Colour != "blue" {
		t.Fatalf("colour round trip: %+v", made)
	}
	bad := made
	bad.StarterPrompt = "-secret prompt text"
	_, err := s.UpdateFolderProjectFor(uid, bad, 0)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("update: %v", err)
	}
	if got, _ := s.FolderProjectFor(uid, made.ID); got != made {
		t.Fatalf("refused update changed the project: %+v", got)
	}
}

func TestFolderProjectVersioning(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p := mustFP(t, s, uid, fp("laptop", "/a"))
	created := p.CreatedAt
	p.Name = "one"
	p.BaseBranch = "dev"
	p.Position, p.LegacyProjectID, p.CreatedAt = 5, "evil", 1
	u1, err := s.UpdateFolderProjectFor(uid, p, 1)
	if err != nil || u1.Version != 2 || u1.Name != "one" || u1.Position != 0 || u1.LegacyProjectID != "" || u1.CreatedAt != created {
		t.Fatalf("%v %+v", err, u1)
	}
	if u1.UpdatedAt < p.UpdatedAt {
		t.Fatal("updatedAt went back")
	}
	// A stale expectation is refused and nothing changes.
	p.Name = "two"
	if _, err := s.UpdateFolderProjectFor(uid, p, 1); !errors.Is(err, ErrFolderProjectChanged) {
		t.Fatalf("%v", err)
	}
	if got, _ := s.FolderProjectFor(uid, p.ID); got != u1 {
		t.Fatalf("%+v", got)
	}
	// expect 0: last write wins; the version still goes up.
	u2, err := s.UpdateFolderProjectFor(uid, p, 0)
	if err != nil || u2.Version != 3 || u2.Name != "two" {
		t.Fatalf("%v %+v", err, u2)
	}
	// A stale delete is refused; expect 0 deletes.
	if err := s.DeleteFolderProjectFor(uid, p.ID, 2); !errors.Is(err, ErrFolderProjectChanged) {
		t.Fatalf("%v", err)
	}
	if _, err := s.FolderProjectFor(uid, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFolderProjectFor(uid, p.ID, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FolderProjectFor(uid, p.ID); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal(err)
	}
	if err := s.DeleteFolderProjectFor(uid, p.ID, 0); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.UpdateFolderProjectFor(uid, p, 0); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal(err)
	}
	q := mustFP(t, s, uid, fp("laptop", "/q"))
	if err := s.DeleteFolderProjectFor(uid, q.ID, 0); err != nil {
		t.Fatalf("expect 0: %v", err)
	}
}

func TestFolderProjectScopedByAccount(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	pa := mustFP(t, s, a, fp("laptop", "/secret"))
	pa2 := mustFP(t, s, a, fp("laptop", "/secret2"))
	if _, err := s.FolderProjectFor(b, pa.ID); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("read: %v", err)
	}
	if l, _ := s.ListFolderProjectsFor(b); len(l) != 0 {
		t.Fatalf("list: %+v", l)
	}
	steal := pa
	steal.Name = "stolen"
	if _, err := s.UpdateFolderProjectFor(b, steal, 0); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("update: %v", err)
	}
	if _, err := s.UpdateFolderProjectFor(b, steal, pa.Version); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("update cas: %v", err)
	}
	if err := s.DeleteFolderProjectFor(b, pa.ID, 0); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteFolderProjectFor(b, pa.ID, pa.Version); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("delete cas: %v", err)
	}
	pb := mustFP(t, s, b, fp("laptop", "/b"))
	// b cannot reorder a's projects, nor mix them in with its own.
	if err := s.ReorderFolderProjectsFor(b, []string{pa2.ID, pa.ID}, []int64{pa2.Version, pa.Version}); err == nil {
		t.Fatal("reorder of another account's projects accepted")
	}
	if err := s.ReorderFolderProjectsFor(b, []string{pa.ID, pb.ID}, []int64{pa.Version, pb.Version}); err == nil {
		t.Fatal("mixed reorder accepted")
	}
	got, err := s.FolderProjectFor(a, pa.ID)
	if err != nil || got != pa {
		t.Fatalf("a's project changed: %v %+v", err, got)
	}
	if g2, _ := s.FolderProjectFor(a, pa2.ID); g2 != pa2 {
		t.Fatalf("a's second project changed: %+v", g2)
	}
	if gb, _ := s.FolderProjectFor(b, pb.ID); gb != pb {
		t.Fatalf("b's project changed: %+v", gb)
	}
	// Same id under two accounts never crosses over: ids are random, so an
	// id guessed from another account is simply unknown.
	if pa.ID == pb.ID {
		t.Fatal("ids collide")
	}
}

func TestFolderProjectReorderChecksVersions(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	a := mustFP(t, s, uid, fp("m", "/a"))
	b := mustFP(t, s, uid, fp("m", "/b"))
	c := mustFP(t, s, uid, fp("m", "/c"))
	order := func() string {
		l, err := s.ListFolderProjectsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for i, p := range l {
			if p.Position != i {
				t.Fatalf("positions not dense: %+v", l)
			}
			ids = append(ids, p.Name)
		}
		return strings.Join(ids, ",")
	}
	// One stale version, in the last slot: nothing at all moves.
	before, _ := s.ListFolderProjectsFor(uid)
	err := s.ReorderFolderProjectsFor(uid, []string{c.ID, b.ID, a.ID}, []int64{c.Version, b.Version, a.Version + 5})
	if !errors.Is(err, ErrFolderProjectChanged) {
		t.Fatalf("%v", err)
	}
	after, _ := s.ListFolderProjectsFor(uid)
	if fmt.Sprint(before) != fmt.Sprint(after) || order() != "a,b,c" {
		t.Fatalf("partial reorder: %s", order())
	}
	// Bad shapes are refused without effect.
	for name, call := range map[string]func() error{
		"length mismatch": func() error { return s.ReorderFolderProjectsFor(uid, []string{c.ID, b.ID, a.ID}, []int64{1, 1}) },
		"duplicate":       func() error { return s.ReorderFolderProjectsFor(uid, []string{c.ID, c.ID, a.ID}, []int64{1, 1, 1}) },
		"missing one":     func() error { return s.ReorderFolderProjectsFor(uid, []string{c.ID, b.ID}, []int64{1, 1}) },
		"unknown id":      func() error { return s.ReorderFolderProjectsFor(uid, []string{c.ID, b.ID, "zz"}, []int64{1, 1, 1}) },
		"zero version":    func() error { return s.ReorderFolderProjectsFor(uid, []string{c.ID, b.ID, a.ID}, []int64{0, 0, 0}) },
	} {
		if call() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if order() != "a,b,c" {
		t.Fatalf("bad shape moved things: %s", order())
	}
	if err := s.ReorderFolderProjectsFor(uid, []string{c.ID, a.ID, b.ID}, []int64{c.Version, a.Version, b.Version}); err != nil {
		t.Fatal(err)
	}
	if order() != "c,a,b" {
		t.Fatalf("%s", order())
	}
	l, _ := s.ListFolderProjectsFor(uid)
	for _, p := range l {
		if p.Version != 2 {
			t.Fatalf("moved project keeps its version: %+v", p)
		}
	}
	// The old versions are now stale.
	if err := s.ReorderFolderProjectsFor(uid, []string{a.ID, b.ID, c.ID}, []int64{a.Version, b.Version, c.Version}); !errors.Is(err, ErrFolderProjectChanged) {
		t.Fatalf("%v", err)
	}
	// Another tab deleted a project: the stale view is "changed", not "not
	// found", as long as the account still has projects. An account with none
	// gets not found, and another account's id reads exactly like an unknown one.
	x := mustFP(t, s, uid, fp("m", "/x"))
	if err := s.DeleteFolderProjectFor(uid, x.ID, 0); err != nil {
		t.Fatal(err)
	}
	l, _ = s.ListFolderProjectsFor(uid)
	ids, versions := []string{x.ID}, []int64{x.Version}
	for _, p := range l {
		ids, versions = append(ids, p.ID), append(versions, p.Version)
	}
	if err := s.ReorderFolderProjectsFor(uid, ids, versions); !errors.Is(err, ErrFolderProjectChanged) {
		t.Fatalf("after a delete: %v", err)
	}
	other := projectUser(t, s, "b")
	if err := s.ReorderFolderProjectsFor(other, []string{a.ID}, []int64{a.Version}); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatalf("account with none: %v", err)
	}
	mine := mustFP(t, s, other, fp("m", "/mine"))
	for _, id := range []string{a.ID, "0123456789abcdef0123456789abcdef"} {
		if err := s.ReorderFolderProjectsFor(other, []string{mine.ID, id}, []int64{1, 1}); !errors.Is(err, ErrFolderProjectChanged) {
			t.Fatalf("other account %s: %v", id, err)
		}
	}
	if order() != "c,a,b" {
		t.Fatalf("%s", order())
	}
	// A new project lands at the end.
	d := mustFP(t, s, uid, fp("m", "/d"))
	if d.Position != 3 {
		t.Fatalf("%+v", d)
	}
	// Deleting leaves no gap that breaks the next create.
	if err := s.DeleteFolderProjectFor(uid, c.ID, 0); err != nil {
		t.Fatal(err)
	}
	e := mustFP(t, s, uid, fp("m", "/e"))
	if e.Position <= d.Position {
		t.Fatalf("%+v", e)
	}
}
