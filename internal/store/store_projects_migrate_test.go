package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// migrate runs the migration once and fails the test on an error.
func migrate(t *testing.T, s *Store) MigrationReport {
	t.Helper()
	rep, err := s.MigrateProjectsToFolders()
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

// byDir lists uid's folder projects keyed by host and folder.
func byDir(t *testing.T, s *Store, uid int64) map[string]FolderProject {
	t.Helper()
	list, err := s.ListFolderProjectsFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]FolderProject{}
	for _, p := range list {
		k := p.Host + " " + p.Dir
		if _, dup := out[k]; dup {
			t.Fatalf("two folder projects for %s", k)
		}
		out[k] = p
	}
	return out
}

func countRows(t *testing.T, s *Store, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// migratedEvents returns the details of uid's projects-migrated events, oldest first.
func migratedEvents(t *testing.T, s *Store, uid int64) []map[string]any {
	t.Helper()
	events, err := s.ListEventsFor(uid, 1000, "")
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Action != "projects-migrated" {
			continue
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(events[i].Detail), &d); err != nil {
			t.Fatal(err)
		}
		out = append(out, d)
	}
	return out
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

func project(name, prompt string, worktrees bool, members ...ProjectMember) Project {
	return Project{Name: name, DefaultTool: "claude", DefaultModel: "sonnet", SharedPrompt: prompt, UseWorktrees: worktrees, Members: members}
}

func TestMigrationDedupesByMachineAndFolder(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	studio, laptop := enrolledID(t, s, uid, "studio"), enrolledID(t, s, uid, "laptop")
	first := mustProject(t, s, uid, project("Shop", "", true, ProjectMember{Host: studio, Dir: "/w/reet"}, ProjectMember{Host: laptop, Dir: "/w/reet"}))
	second := mustProject(t, s, uid, project("Docs", "", true, ProjectMember{Host: studio, Dir: "/w/reet/"}, ProjectMember{Host: studio, Dir: "/w/docs"}))
	rep := migrate(t, s)
	got := byDir(t, s, uid)
	if len(got) != 3 || rep.Created != 3 || rep.Users != 1 || rep.Skipped != 0 {
		t.Fatalf("%+v %v", rep, got)
	}
	shared := got[studio+" /w/reet"]
	// The first (oldest) Project's member is recorded on the folder project.
	if shared.LegacyProjectID != first.ID+":0" || got[laptop+" /w/reet"].LegacyProjectID != first.ID+":1" || got[studio+" /w/docs"].LegacyProjectID != second.ID+":1" {
		t.Fatalf("%+v", got)
	}
	// Both Projects lead to the shared folder project.
	for _, id := range []string{first.ID, second.ID} {
		if p, err := s.FolderProjectForLegacyFor(uid, id); err != nil || p.ID != shared.ID {
			t.Fatalf("legacy %s: %v %+v", id, err, p)
		}
	}
	if _, err := s.FolderProjectForLegacyFor(uid, "nope"); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal(err)
	}
	if _, err := s.FolderProjectForLegacyFor(projectUser(t, s, "b"), first.ID); !errors.Is(err, ErrFolderProjectNotFound) {
		t.Fatal("another account's legacy id resolved", err)
	}
	// Position follows Project, then member, order.
	list, _ := s.ListFolderProjectsFor(uid)
	if list[0].ID != shared.ID || list[1].Dir != "/w/reet" || list[1].Host != laptop || list[2].Dir != "/w/docs" {
		t.Fatalf("%+v", list)
	}
}

// A plain Project id as the legacy key would make every member after the first
// collide on the unique legacy index and be dropped as "already there".
func TestMigrationKeepsEveryMemberWhenASecondMemberSharesAFolder(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	a := mustProject(t, s, uid, project("A", "", true, ProjectMember{Host: m, Dir: "/w/one"}, ProjectMember{Host: m, Dir: "/w/shared"}, ProjectMember{Host: m, Dir: "/w/three"}))
	b := mustProject(t, s, uid, project("B", "", true, ProjectMember{Host: m, Dir: "/w/two"}, ProjectMember{Host: m, Dir: "/w/shared"}))
	rep := migrate(t, s)
	got := byDir(t, s, uid)
	want := map[string]string{"/w/one": a.ID + ":0", "/w/shared": a.ID + ":1", "/w/three": a.ID + ":2", "/w/two": b.ID + ":0"}
	if len(got) != 4 || rep.Created != 4 || rep.Skipped != 0 {
		t.Fatalf("%+v %+v", rep, got)
	}
	for dir, legacy := range want {
		if got[m+" "+dir].LegacyProjectID != legacy {
			t.Fatalf("%s: %+v", dir, got[m+" "+dir])
		}
	}
	var mapped int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM folder_project_legacy WHERE user_id=? AND folder_project_id<>''`, uid).Scan(&mapped); err != nil || mapped != 5 {
		t.Fatal("every member is mapped", mapped, err)
	}
}

func TestMigrationNamesAndDefaults(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	mustFP(t, s, uid, FolderProject{Host: m, Dir: "/elsewhere", Name: "Taken"})
	multi := Project{Name: "Many", DefaultTool: "claude", DefaultModel: "sonnet", UseWorktrees: true, Members: []ProjectMember{
		{Host: m, Dir: "/a/site"},                               // Project defaults
		{Host: m, Dir: "/b/site", Tool: "codex"},                // member tool, no model: the agent's own default
		{Host: m, Dir: "/c/api", Tool: "codex", Model: "gpt-5"}, // member tool and model
		{Host: m, Dir: "/d/web", Model: "opus"},                 // member model with the Project's tool
	}}
	mustProject(t, s, uid, multi)
	mustProject(t, s, uid, Project{Name: "Solo", DefaultTool: "gemini", UseWorktrees: false, Members: []ProjectMember{{Host: m, Dir: "/e/solo-folder"}}})
	mustProject(t, s, uid, Project{Name: "taken", DefaultTool: "claude", Members: []ProjectMember{{Host: m, Dir: "/f/site"}}})
	migrate(t, s)
	got := byDir(t, s, uid)
	for dir, want := range map[string]FolderProject{
		"/a/site":        {Name: "site", DefaultTool: "claude", DefaultModel: "sonnet", WorktreeDefault: "yes"},
		"/b/site":        {Name: "site (2)", DefaultTool: "codex", DefaultModel: "", WorktreeDefault: "yes"},
		"/c/api":         {Name: "api", DefaultTool: "codex", DefaultModel: "gpt-5", WorktreeDefault: "yes"},
		"/d/web":         {Name: "web", DefaultTool: "claude", DefaultModel: "opus", WorktreeDefault: "yes"},
		"/e/solo-folder": {Name: "Solo", DefaultTool: "gemini", DefaultModel: "", WorktreeDefault: "no"},
		// A one-repository Project whose name another project has falls back to the folder's name.
		"/f/site": {Name: "site (3)", DefaultTool: "claude", DefaultModel: "", WorktreeDefault: "no"},
	} {
		p := got[m+" "+dir]
		if p.Name != want.Name || p.DefaultTool != want.DefaultTool || p.DefaultModel != want.DefaultModel || p.WorktreeDefault != want.WorktreeDefault || p.Host != m || p.BaseBranch != "" || p.IsRepo {
			t.Fatalf("%s: got %+v", dir, p)
		}
	}
}

func TestMigrationCarriesStarterPrompts(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	oldest := mustProject(t, s, uid, project("Oldest", "fix the build", true, ProjectMember{Host: m, Dir: "/a"}, ProjectMember{Host: m, Dir: "/b"}))
	mustProject(t, s, uid, project("Later", "write the docs", true, ProjectMember{Host: m, Dir: "/a"}, ProjectMember{Host: m, Dir: "/c"}))
	mustProject(t, s, uid, project("Same prompt", "fix the build", true, ProjectMember{Host: m, Dir: "/b"}))
	mustProject(t, s, uid, project("No prompt", "", true, ProjectMember{Host: m, Dir: "/a"}))
	mustProject(t, s, uid, project("Also later", "ship it today", true, ProjectMember{Host: m, Dir: "/a"}))
	migrate(t, s)
	got := byDir(t, s, uid)
	a, b, c := got[m+" /a"], got[m+" /b"], got[m+" /c"]
	if a.StarterPrompt != "fix the build" || b.StarterPrompt != "fix the build" || c.StarterPrompt != "write the docs" {
		t.Fatalf("prompts: %q %q %q", a.StarterPrompt, b.StarterPrompt, c.StarterPrompt)
	}
	if a.Note != "Later, Also later" || b.Note != "" || c.Note != "" {
		t.Fatalf("notes: %q %q %q", a.Note, b.Note, c.Note)
	}
	if a.LegacyProjectID != oldest.ID+":0" {
		t.Fatal(a.LegacyProjectID)
	}
	// The note can be cleared, and an edit otherwise keeps it.
	kept, err := s.UpdateFolderProjectFor(uid, a, a.Version)
	if err != nil || kept.Note != a.Note {
		t.Fatal(err, kept.Note)
	}
	kept.Note = ""
	cleared, err := s.UpdateFolderProjectFor(uid, kept, kept.Version)
	if err != nil || cleared.Note != "" {
		t.Fatal(err, cleared)
	}
	if read, _ := s.FolderProjectFor(uid, a.ID); read.Note != "" || read.StarterPrompt != "fix the build" {
		t.Fatalf("%+v", read)
	}
}

func TestMigrationSkipsRegisteredFolder(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	mine := mustFP(t, s, uid, FolderProject{Host: m, Dir: "/w/reet", Name: "Mine", StarterPrompt: "my own words", WorktreeDefault: "no", DefaultTool: "gemini"})
	p := mustProject(t, s, uid, project("Shop", "their words here", true, ProjectMember{Host: m, Dir: "/w/reet"}, ProjectMember{Host: m, Dir: "/w/new"}))
	mustProject(t, s, uid, project("Later", "other words here", true, ProjectMember{Host: m, Dir: "/w/reet"}))
	rep := migrate(t, s)
	if rep.Created != 1 || rep.Skipped != 2 {
		t.Fatalf("%+v", rep)
	}
	after, err := s.FolderProjectFor(uid, mine.ID)
	if err != nil || after != mine {
		t.Fatalf("the registered folder changed:\n%+v\n%+v", after, mine)
	}
	// Its Project still leads somewhere: to the registered folder project.
	if got, err := s.FolderProjectForLegacyFor(uid, p.ID); err != nil || got.ID != mine.ID {
		t.Fatal(err, got)
	}
}

func TestMigrationIsIdempotent(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	mustProject(t, s, uid, project("Shop", "fix the build", true, ProjectMember{Host: m, Dir: "/a"}, ProjectMember{Host: m, Dir: "/b"}))
	mustProject(t, s, uid, project("Empty", "", true))
	if rep := migrate(t, s); rep.Created != 2 {
		t.Fatalf("%+v", rep)
	}
	before, _ := s.ListFolderProjectsFor(uid)
	events := len(migratedEvents(t, s, uid))
	if events != 2 {
		t.Fatal("one event per Project, also one without repositories", events)
	}
	if rep := migrate(t, s); rep != (MigrationReport{}) {
		t.Fatalf("second run: %+v", rep)
	}
	// Without the setting the mapping alone keeps it from copying again.
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key='projects_migrated'`); err != nil {
		t.Fatal(err)
	}
	if rep := migrate(t, s); rep.Created != 0 || rep.Users != 0 {
		t.Fatalf("third run: %+v", rep)
	}
	// Removing a migrated folder project does not bring it back.
	if err := s.DeleteFolderProjectFor(uid, before[0].ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM settings WHERE key='projects_migrated'`); err != nil {
		t.Fatal(err)
	}
	if rep := migrate(t, s); rep.Created != 0 {
		t.Fatalf("fourth run: %+v", rep)
	}
	after, _ := s.ListFolderProjectsFor(uid)
	if len(after) != 1 || after[0] != before[1] || len(migratedEvents(t, s, uid)) != events {
		t.Fatalf("%+v %d", after, len(migratedEvents(t, s, uid)))
	}
}

func TestMigrationRunTwiceConcurrently(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	for i := 0; i < 5; i++ {
		mustProject(t, s, uid, project(fmt.Sprintf("P%d", i), "", true, ProjectMember{Host: m, Dir: fmt.Sprintf("/w/%d", i)}, ProjectMember{Host: m, Dir: "/w/shared"}))
	}
	var wg sync.WaitGroup
	reps := make([]MigrationReport, 4)
	errs := make([]error, 4)
	for i := range reps {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reps[i], errs[i] = s.MigrateProjectsToFolders()
		}(i)
	}
	wg.Wait()
	total := 0
	for i, rep := range reps {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		total += rep.Created
	}
	if list, _ := s.ListFolderProjectsFor(uid); total != 6 || len(list) != 6 || len(migratedEvents(t, s, uid)) != 5 {
		t.Fatal(total, len(list), len(migratedEvents(t, s, uid)))
	}
}

func TestMigrationKeepsRuns(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	p := mustProject(t, s, uid, project("Shop", "fix the build", true, ProjectMember{Host: m, Dir: "/w/site"}, ProjectMember{Host: m, Dir: "/w/api", Tool: "codex"}))
	run, err := s.CreateRunFor(uid, p.ID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	const tok = "aaaaaaaaaaaaaaaaaaaaaaaa"
	if err := s.SetRunMemberWorktreeToken(uid, run.ID, 0, tok); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunMemberState(uid, run.ID, 0, RunMemberStarted, "", "session-0000000000000001", "/w/.reet/worktrees/shop/site/"+tok, "site", "reet/shop-"+tok, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRunMemberState(uid, run.ID, 1, RunMemberFailed, "tool_not_installed", "", "", "", "", 1); err != nil {
		t.Fatal(err)
	}
	beforeProject, _ := s.ProjectFor(uid, p.ID)
	beforeRun, _ := s.RunFor(uid, run.ID)
	tables := []string{"projects", "project_members", "project_runs", "project_run_members"}
	counts := map[string]int{}
	for _, table := range tables {
		counts[table] = countRows(t, s, table)
	}
	if rep := migrate(t, s); rep.Created != 2 || len(byDir(t, s, uid)) != 2 {
		t.Fatalf("%+v", rep)
	}
	for _, table := range tables {
		if n := countRows(t, s, table); n != counts[table] {
			t.Fatalf("%s: %d rows, was %d", table, n, counts[table])
		}
	}
	if got, err := s.ProjectFor(uid, p.ID); err != nil || !reflect.DeepEqual(got, beforeProject) {
		t.Fatalf("the Project changed: %v\n%+v\n%+v", err, got, beforeProject)
	}
	if got, err := s.RunFor(uid, run.ID); err != nil || !reflect.DeepEqual(got, beforeRun) || got.Members[0].WorktreePath == "" {
		t.Fatalf("the run changed: %v %+v", err, got)
	}
	if runs, err := s.RunsFor(uid, p.ID, MaxRunsListed); err != nil || len(runs) != 1 {
		t.Fatal(err, runs)
	}
	if has, err := s.ProjectMayHaveWorktrees(uid, p.ID); err != nil || !has {
		t.Fatal(err, has)
	}
	// Removing the run's worktree is still recorded.
	if err := s.SetRunMemberRemovedFor(uid, m, tok); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RunFor(uid, run.ID); got.Members[0].State != RunMemberRemoved {
		t.Fatal(got.Members[0].State)
	}
	// A new run of the old Project still starts.
	if _, _, err := s.StartRunFor(uid, p.ID, "start-after-migration-0001", []int{1}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationAuditHasNoPrompt(t *testing.T) {
	logs := captureLog(t)
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	const secret = "zebra-sentinel prompt words"
	p := mustProject(t, s, uid, project("Shop", secret, true, ProjectMember{Host: m, Dir: "/w/secret-folder"}, ProjectMember{Host: m, Dir: "/w/other"}))
	mustProject(t, s, uid, project("Later", secret+" changed", true, ProjectMember{Host: m, Dir: "/w/secret-folder"}))
	// One bad row, so the skip is logged.
	mustProject(t, s, uid, project("Bad", secret, true, ProjectMember{Host: strings.Repeat("h", 200), Dir: "/w/bad-folder"}))
	rep := migrate(t, s)
	if rep.Skipped != 1 || rep.Created != 2 {
		t.Fatalf("%+v", rep)
	}
	got := byDir(t, s, uid)
	events := migratedEvents(t, s, uid)
	want := map[string]any{"project": "Shop", "folderProjects": []any{got[m+" /w/secret-folder"].ID, got[m+" /w/other"].ID}}
	if len(events) != 3 || !reflect.DeepEqual(events[0], want) {
		t.Fatalf("%v", events)
	}
	if !reflect.DeepEqual(events[1], map[string]any{"project": "Later", "folderProjects": []any{got[m+" /w/secret-folder"].ID}}) ||
		!reflect.DeepEqual(events[2], map[string]any{"project": "Bad", "folderProjects": []any{}}) {
		t.Fatalf("%v", events[1:])
	}
	all, _ := s.ListEventsFor(uid, 1000, "")
	raw, _ := json.Marshal(all)
	for _, leak := range []string{"zebra", "folder", "/w/"} {
		if strings.Contains(strings.ReplaceAll(string(raw), "folderProjects", ""), leak) {
			t.Fatalf("audit has %q: %s", leak, raw)
		}
	}
	for _, e := range all {
		if e.Action == "projects-migrated" && e.Source != "migration" {
			t.Fatal(e.Source)
		}
	}
	if logs.Len() == 0 {
		t.Fatal("a skipped row was not logged")
	}
	for _, leak := range []string{"zebra", "folder", "/w/", "Shop", "Bad", "Later", p.ID, "hhhh"} {
		if strings.Contains(logs.String(), leak) {
			t.Fatalf("log has %q: %s", leak, logs)
		}
	}
}

func TestMigrationRevokedMachine(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	gone := enrolledID(t, s, uid, "old-desk")
	mustProject(t, s, uid, project("Shop", "", true, ProjectMember{Host: gone, Dir: "/w/site"}))
	if err := s.ControlSyncAgent(uid, gone, "revoke"); err != nil {
		t.Fatal(err)
	}
	if rep := migrate(t, s); rep.Created != 1 || rep.Skipped != 0 {
		t.Fatalf("%+v", rep)
	}
	if got := byDir(t, s, uid)[gone+" /w/site"]; got.ID == "" || got.Host != gone {
		t.Fatalf("%+v", byDir(t, s, uid))
	}
}

func TestMigrationSkipsABadRowAndKeepsGoing(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	mustProject(t, s, uid, project("Shop", "", true, ProjectMember{Host: strings.Repeat("h", 200), Dir: "/w/a"}, ProjectMember{Host: m, Dir: "/w/b"}))
	// A row written before today's rules: a folder the folder-project rules refuse.
	bad := mustProject(t, s, uid, project("Old", "", true, ProjectMember{Host: m, Dir: "/w/c"}))
	if _, err := s.db.Exec(`UPDATE project_members SET dir='relative' WHERE project_id=?`, bad.ID); err != nil {
		t.Fatal(err)
	}
	mustProject(t, s, uid, project("Fine", "", true, ProjectMember{Host: m, Dir: "/w/d"}))
	rep := migrate(t, s)
	if rep.Created != 2 || rep.Skipped != 2 {
		t.Fatalf("%+v", rep)
	}
	if got := byDir(t, s, uid); len(got) != 2 || got[m+" /w/b"].ID == "" || got[m+" /w/d"].ID == "" {
		t.Fatalf("%+v", got)
	}
	// Bad rows do not hold the migration open.
	if v, _ := s.GetSetting("projects_migrated"); v == "" {
		t.Fatal("not marked done")
	}
}

func TestMigrationLimitSkipsAndALaterRunContinues(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	for i := 0; i < MaxFolderProjects-1; i++ {
		mustFP(t, s, uid, fp(m, fmt.Sprintf("/full/%d", i)))
	}
	first := mustProject(t, s, uid, project("First", "", true, ProjectMember{Host: m, Dir: "/w/a"}, ProjectMember{Host: m, Dir: "/w/b"}))
	second := mustProject(t, s, uid, project("Second", "", true, ProjectMember{Host: m, Dir: "/w/c"}))
	rep := migrate(t, s)
	if rep.Created != 1 || rep.Skipped != 2 {
		t.Fatalf("%+v", rep)
	}
	if v, _ := s.GetSetting("projects_migrated"); v != "" {
		t.Fatal("marked done with Projects left over")
	}
	// The person makes room; the next start carries on where it stopped.
	list, _ := s.ListFolderProjectsFor(uid)
	for _, p := range list[:3] {
		if err := s.DeleteFolderProjectFor(uid, p.ID, 0); err != nil {
			t.Fatal(err)
		}
	}
	rep = migrate(t, s)
	if rep.Created != 2 || rep.Skipped != 0 {
		t.Fatalf("%+v", rep)
	}
	got := byDir(t, s, uid)
	if got[m+" /w/a"].LegacyProjectID != first.ID+":0" || got[m+" /w/b"].LegacyProjectID != first.ID+":1" || got[m+" /w/c"].LegacyProjectID != second.ID+":0" {
		t.Fatalf("%+v %+v %+v", got[m+" /w/a"], got[m+" /w/b"], got[m+" /w/c"])
	}
	if v, _ := s.GetSetting("projects_migrated"); v == "" {
		t.Fatal("not marked done")
	}
	var names []string
	for _, e := range migratedEvents(t, s, uid) {
		names = append(names, e["project"].(string))
	}
	sort.Strings(names)
	// First on both runs (it gained a folder on the second), Second once.
	if !reflect.DeepEqual(names, []string{"First", "First", "Second"}) {
		t.Fatal(names)
	}
}

func TestMigrationOnAnOldDatabaseWithoutTheMappingTable(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid := projectUser(t, s, "a")
	m := enrolledID(t, s, uid, "studio")
	p := mustProject(t, s, uid, project("Shop", "fix the build", true, ProjectMember{Host: m, Dir: "/w/a"}, ProjectMember{Host: m, Dir: "/w/b"}))
	// A database from before this change: no mapping table, no note column.
	for _, stmt := range []string{`DROP TABLE folder_project_legacy`, `ALTER TABLE folder_projects DROP COLUMN note`} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(stmt, err)
		}
	}
	s.Close()
	if s, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	if rep := migrate(t, s); rep.Created != 2 {
		t.Fatalf("%+v", rep)
	}
	// The person points one at another folder since.
	moved := byDir(t, s, uid)[m+" /w/b"]
	moved.Dir = "/w/b-moved"
	if _, err := s.UpdateFolderProjectFor(uid, moved, 0); err != nil {
		t.Fatal(err)
	}
	// The mapping table lost again, and the setting with it: the legacy
	// column alone keeps the copies from being made twice.
	for _, stmt := range []string{`DROP TABLE folder_project_legacy`, `DELETE FROM settings WHERE key='projects_migrated'`} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(stmt, err)
		}
	}
	s.Close()
	if s, err = Open(dir); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if rep := migrate(t, s); rep.Created != 0 || rep.Skipped != 0 {
		t.Fatalf("%+v", rep)
	}
	if got := byDir(t, s, uid); len(got) != 2 || got[m+" /w/b-moved"].ID != moved.ID {
		t.Fatalf("%+v", got)
	}
	if got, err := s.FolderProjectForLegacyFor(uid, p.ID); err != nil || got.Dir != "/w/a" {
		t.Fatal(err, got)
	}
}
