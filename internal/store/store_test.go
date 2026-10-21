package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/ref"
)

func bp(b bool) *bool     { return &b }
func sp(s string) *string { return &s }

func mustRef(t *testing.T, in string) ref.Ref {
	t.Helper()
	r, err := ref.Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestUpsertIsIdempotentAndKeepsFlags(t *testing.T) {
	s := newStore(t)
	r := mustRef(t, "vercel-labs/skills/find-skills")
	if _, err := s.Upsert(r, Patch{Liked: bp(true)}); err != nil {
		t.Fatal(err)
	}
	sk, err := s.Upsert(r, Patch{Used: bp(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !sk.Liked || !sk.Used || sk.Bookmarked {
		t.Fatalf("flags after second upsert: %+v", sk)
	}
	sk, _ = s.Upsert(r, Patch{})
	if !sk.Liked {
		t.Fatal("empty upsert cleared liked")
	}
	if sk.Name != "find-skills" {
		t.Fatalf("name should default to slug, got %q", sk.Name)
	}
}

func TestListFilterAndCounts(t *testing.T) {
	s := newStore(t)
	s.Upsert(mustRef(t, "a/b/one"), Patch{Liked: bp(true)})
	s.Upsert(mustRef(t, "a/b/two"), Patch{Bookmarked: bp(true)})
	s.Upsert(mustRef(t, "a/b/three"), Patch{Used: bp(true), Liked: bp(true)})
	got, counts, err := s.List(Filter{Tab: "liked"})
	if err != nil || len(got) != 2 {
		t.Fatalf("liked: %v %v", got, err)
	}
	if counts != (Counts{All: 3, Liked: 2, Bookmarked: 1, Used: 1}) {
		t.Fatalf("counts: %+v", counts)
	}
	got, _, _ = s.List(Filter{Tab: "used"})
	if len(got) != 1 || got[0].Slug != "three" {
		t.Fatalf("used: %+v", got)
	}
}

func TestListSearchMatchesNameSourceNotes(t *testing.T) {
	s := newStore(t)
	a, _ := s.Upsert(mustRef(t, "acme/tools/alpha"), Patch{})
	s.Upsert(mustRef(t, "zed/other/beta"), Patch{Notes: sp("great for databases")})
	for q, want := range map[string]string{"alpha": "alpha", "acme": "alpha", "databases": "beta"} {
		got, _, err := s.List(Filter{Query: q})
		if err != nil || len(got) != 1 || got[0].Slug != want {
			t.Errorf("query %q: %+v %v", q, got, err)
		}
	}
	_ = a
}

func TestResolveMovesFlags(t *testing.T) {
	s := newStore(t)
	local, err := s.UpsertLocal("my-skill", Patch{Used: bp(true), Notes: sp("mine")})
	if err != nil || local.ID != "local/my-skill" || local.SourceType != "local" {
		t.Fatalf("local: %+v %v", local, err)
	}
	s.Patch(local.ID, Patch{Liked: bp(true)})
	sk, err := s.Resolve(local.ID, mustRef(t, "owner/repo/my-skill"))
	if err != nil {
		t.Fatal(err)
	}
	if sk.ID != "owner/repo/my-skill" || !sk.Used || !sk.Liked || sk.Notes != "mine" {
		t.Fatalf("resolved: %+v", sk)
	}
	if _, ok, _ := s.Get("local/my-skill"); ok {
		t.Fatal("old local row still present")
	}
}

func TestOpenMigratesOlderSchema(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "reet.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE skills (
		id TEXT PRIMARY KEY, source TEXT NOT NULL DEFAULT '', slug TEXT NOT NULL DEFAULT '',
		source_type TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '', notes TEXT NOT NULL DEFAULT '',
		installs INTEGER NOT NULL DEFAULT 0,
		liked INTEGER NOT NULL DEFAULT 0, bookmarked INTEGER NOT NULL DEFAULT 0, used INTEGER NOT NULL DEFAULT 0,
		created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO skills(id,created_at,updated_at) VALUES('a/b/old',1,1)`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sk, ok, err := s.Get("a/b/old")
	if err != nil || !ok || sk.InstallsWeek != 0 || len(sk.Trend) != 0 {
		t.Fatalf("old row: %+v %v %v", sk, ok, err)
	}
	if err := s.SetMeta("a/b/old", "Old", "desc", 10, 3, []int64{1, 3}); err != nil {
		t.Fatal(err)
	}
	sk, _, _ = s.Get("a/b/old")
	if sk.InstallsWeek != 3 || len(sk.Trend) != 2 {
		t.Fatalf("migrated row: %+v", sk)
	}
}

func TestExportRestoreRoundTrip(t *testing.T) {
	a := newStore(t)
	r := mustRef(t, "a/b/one")
	a.Upsert(r, Patch{Liked: bp(true), Notes: sp("n")})
	a.SetMeta(r.ID, "One", "desc", 42, 7, []int64{1, 2, 7})
	a.UpsertLocal("two", Patch{Used: bp(true)})
	out, err := a.Export()
	if err != nil || len(out) != 2 {
		t.Fatalf("export: %v %v", out, err)
	}
	b := newStore(t)
	if err := b.Restore(out); err != nil {
		t.Fatal(err)
	}
	got, counts, _ := b.List(Filter{})
	if len(got) != 2 || counts.Liked != 1 || counts.Used != 1 {
		t.Fatalf("restored: %+v %+v", got, counts)
	}
	for _, sk := range got {
		if sk.ID == r.ID && (sk.Installs != 42 || sk.InstallsWeek != 7 || len(sk.Trend) != 3) {
			t.Fatalf("metadata did not round-trip: %+v", sk)
		}
	}
}

func TestUpsertLocal(t *testing.T) {
	s := newStore(t)
	sk, err := s.UpsertLocal("thing", Patch{Used: bp(true)})
	if err != nil || sk.ID != "local/thing" || sk.Name != "thing" || !sk.Used {
		t.Fatalf("%+v %v", sk, err)
	}
	if sk.URL != "" {
		t.Fatal("local skills have no url")
	}
}

func TestSetMetaAndDelete(t *testing.T) {
	s := newStore(t)
	r := mustRef(t, "a/b/one")
	s.Upsert(r, Patch{})
	if err := s.SetMeta(r.ID, "One", "desc", 42, 7, []int64{1, 2, 7}); err != nil {
		t.Fatal(err)
	}
	sk, _, _ := s.Get(r.ID)
	if sk.Name != "One" || sk.Description != "desc" || sk.Installs != 42 || sk.InstallsWeek != 7 || len(sk.Trend) != 3 {
		t.Fatalf("%+v", sk)
	}
	if err := s.Delete(r.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Get(r.ID); ok {
		t.Fatal("still present")
	}
}

func TestRestoreRederivesLinkAndRejectsBadIDs(t *testing.T) {
	s := newStore(t)
	err := s.Restore([]Skill{
		{ID: "a/b/c", URL: "javascript:alert(1)", Source: "evil", SourceType: "github", Name: "C", Liked: true},
		{ID: "local/mine", URL: "https://evil.example/x", Source: "x", SourceType: "github", Name: "mine"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := s.Get("a/b/c")
	if c.URL != "https://www.skills.sh/a/b/c" || c.Source != "a/b" || c.SourceType != "github" || !c.Liked || c.Name != "C" {
		t.Fatalf("github skill not re-derived: %+v", c)
	}
	m, _, _ := s.Get("local/mine")
	if m.URL != "" || m.SourceType != "local" || m.Source != "local" || m.Slug != "mine" {
		t.Fatalf("local skill not re-derived: %+v", m)
	}
	for _, bad := range []string{"", "nonsense", "a/b/c/d", "local/", "local/x/y"} {
		if err := s.Restore([]Skill{{ID: bad}}); err == nil {
			t.Errorf("Restore accepted id %q", bad)
		}
	}
}

func TestFlagChangeBumpsRecencyButReimportDoesNot(t *testing.T) {
	s := newStore(t)
	a, b := mustRef(t, "o/r/aaa"), mustRef(t, "o/r/bbb")
	s.Upsert(a, Patch{})
	time.Sleep(3 * time.Millisecond)
	s.Upsert(b, Patch{})
	time.Sleep(3 * time.Millisecond)
	first := func() string {
		got, _, _ := s.List(Filter{})
		return got[0].Slug
	}
	if first() != "bbb" {
		t.Fatalf("newest first expected, got %s", first())
	}
	s.Upsert(a, Patch{Liked: bp(true)})
	if first() != "aaa" {
		t.Fatal("liking an old skill should move it to the top of Recent")
	}
	time.Sleep(3 * time.Millisecond)
	s.Upsert(b, Patch{})
	s.Upsert(a, Patch{Liked: bp(true)})
	if first() != "aaa" {
		t.Fatal("re-importing with no change must not reorder")
	}
}

func TestEnvironmentsAndFilters(t *testing.T) {
	s := newStore(t)
	a := mustRef(t, "a/b/alpha")
	b := mustRef(t, "a/b/beta")
	c := mustRef(t, "a/b/gamma")
	for _, r := range []ref.Ref{a, b, c} {
		if _, err := s.Upsert(r, Patch{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range []struct{ id, host, agent, project string }{
		{a.ID, "laptop", "claude", ""},
		{a.ID, "desktop", "cursor", ""},
		{b.ID, "laptop", "cursor", ""},
		{c.ID, "laptop", "claude", ""},
		{c.ID, "laptop", "cursor", ""},
	} {
		if err := s.RecordEnvironment(e.id, e.host, e.agent, e.project, ".skills", ".skills/"+e.id); err != nil {
			t.Fatal(err)
		}
	}
	envs, err := s.ListEnvironments()
	if err != nil || len(envs) != 3 {
		t.Fatalf("envs: %+v %v", envs, err)
	}
	if got := map[string]int{}; true {
		for _, e := range envs {
			got[e.ID] = e.SkillCount
		}
		if got["laptop/claude"] != 2 || got["laptop/cursor"] != 2 || got["desktop/cursor"] != 1 {
			t.Fatalf("counts: %+v", got)
		}
	}
	got, _, err := s.List(Filter{Env: "laptop/claude"})
	if err != nil || len(got) != 2 {
		t.Fatalf("env filter: %+v %v", got, err)
	}
	got, _, err = s.List(Filter{Only: "one-machine"})
	if err != nil || len(got) != 2 {
		t.Fatalf("one machine: %+v %v", got, err)
	}
	sk, ok, _ := s.Get(a.ID)
	if !ok || len(sk.Environments) != 2 {
		t.Fatalf("attached envs: %+v", sk)
	}
	if n, err := s.PruneEnvironment("laptop/cursor", []string{b.ID}); err != nil || n != 1 {
		t.Fatalf("prune: %d %v", n, err)
	}
}

func TestTagsTopicsAndFilters(t *testing.T) {
	s := newStore(t)
	a := mustRef(t, "a/b/alpha")
	b := mustRef(t, "a/b/beta")
	s.Upsert(a, Patch{})
	s.Upsert(b, Patch{})
	if err := s.AddSkillTags(a.ID, []string{"client work", "testing"}); err != nil {
		t.Fatal(err)
	}
	tags, err := s.ListTags()
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Tag{}
	for _, tag := range tags {
		byName[tag.Name] = tag
	}
	if byName["client work"].Count != 1 || byName["testing"].Count != 1 {
		t.Fatalf("tags: %+v", tags)
	}
	got, _, err := s.List(Filter{Tags: []int64{byName["client work"].ID}})
	if err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("tag filter: %+v %v", got, err)
	}
	got, _, err = s.List(Filter{Query: `tag:"client work"`})
	if err != nil || len(got) != 1 || got[0].ID != a.ID {
		t.Fatalf("tag query: %+v %v", got, err)
	}
	if err := s.RemoveSkillTag(a.ID, byName["client work"].ID); err != nil {
		t.Fatal(err)
	}
	sk, _, _ := s.Get(a.ID)
	for _, tag := range sk.Tags {
		if tag.Name == "client work" {
			t.Fatal("user tag was not removed")
		}
	}

	if err := s.ReplaceTopics([]Topic{{Slug: "design", Name: "Design & UI", Skills: []string{a.ID}}}); err != nil {
		t.Fatal(err)
	}
	sk, _, _ = s.Get(a.ID)
	found := false
	for _, tag := range sk.Tags {
		if tag.Name == "Design & UI" && tag.Source == "topic" {
			found = true
		}
	}
	if !found {
		t.Fatalf("topic tag not attached: %+v", sk.Tags)
	}
	if _, err := s.Upsert(b, Patch{}); err != nil {
		t.Fatal(err)
	}
	sk, _, _ = s.Get(b.ID)
	for _, tag := range sk.Tags {
		if tag.Name == "Design & UI" {
			t.Fatal("topic tag leaked to a skill outside the topic")
		}
	}
	tags, _ = s.ListTags()
	byName = map[string]Tag{}
	for _, tag := range tags {
		byName[tag.Name] = tag
	}
	if err := s.RenameTag(byName["testing"].ID, "quality"); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeTags(byName["quality"].ID, byName["Design & UI"].ID); err != nil {
		t.Fatal(err)
	}
}

func TestEventsAndIgnored(t *testing.T) {
	s := newStore(t)
	r := mustRef(t, "a/b/alpha")
	s.Upsert(r, Patch{})
	if err := s.AddEvent("ui", "add", r.ID, map[string]string{"name": "Alpha"}); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListEvents(10, r.ID)
	if err != nil || len(events) != 1 || events[0].Action != "add" || events[0].SkillName != "alpha" {
		t.Fatalf("events: %+v %v", events, err)
	}
	if err := s.Ignore(r.ID, "alpha", "removed"); err != nil {
		t.Fatal(err)
	}
	if ignored, err := s.IsIgnored(r.ID); err != nil || !ignored {
		t.Fatalf("ignored: %v %v", ignored, err)
	}
	if _, ok, _ := s.Get(r.ID); ok {
		t.Fatal("ignored skill still exists")
	}
	if err := s.Unignore(r.ID); err != nil {
		t.Fatal(err)
	}
	if ignored, _ := s.IsIgnored(r.ID); ignored {
		t.Fatal("still ignored")
	}
}

func TestExportRestoreSnapshot(t *testing.T) {
	a := newStore(t)
	r := mustRef(t, "a/b/alpha")
	a.Upsert(r, Patch{})
	if err := a.RecordEnvironment(r.ID, "laptop", "claude", "", ".claude/skills", ".claude/skills/alpha"); err != nil {
		t.Fatal(err)
	}
	if err := a.AddSkillTags(r.ID, []string{"client"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ReplaceTopics([]Topic{{Slug: "design", Name: "Design & UI", Skills: []string{r.ID}}}); err != nil {
		t.Fatal(err)
	}
	tags, _ := a.ListTags()
	var designID int64
	for _, tag := range tags {
		if tag.Name == "Design & UI" {
			designID = tag.ID
		}
	}
	if err := a.RemoveSkillTag(r.ID, designID); err != nil {
		t.Fatal(err)
	}
	if err := a.Ignore("local/old", "old", "removed"); err != nil {
		t.Fatal(err)
	}
	snap, err := a.ExportSnapshot(true)
	if err != nil || len(snap.Skills) != 1 || len(snap.Ignored) != 1 || len(snap.Skills[0].HiddenTags) != 1 {
		t.Fatalf("snapshot: %+v %v", snap, err)
	}
	b := newStore(t)
	if err := b.RestoreSnapshot(snap); err != nil {
		t.Fatal(err)
	}
	sk, ok, _ := b.Get(r.ID)
	if !ok || len(sk.Tags) != 1 || len(sk.Environments) != 1 {
		t.Fatalf("restored: %+v %v", sk, ok)
	}
	if ignored, _ := b.IsIgnored("local/old"); !ignored {
		t.Fatal("ignored id not restored")
	}
	if err := b.ReplaceTopics([]Topic{{Slug: "design", Name: "Design & UI", Skills: []string{r.ID}}}); err != nil {
		t.Fatal(err)
	}
	sk, _, _ = b.Get(r.ID)
	for _, tag := range sk.Tags {
		if tag.Name == "Design & UI" {
			t.Fatal("hidden topic tag was re-added visibly")
		}
	}
}

func TestRefreshQueueUsesOldestFetched(t *testing.T) {
	s := newStore(t)
	a := mustRef(t, "a/b/aaa")
	b := mustRef(t, "a/b/bbb")
	c := mustRef(t, "a/b/ccc")
	for _, r := range []ref.Ref{a, b, c} {
		s.Upsert(r, Patch{})
	}
	if err := s.SetMeta(b.ID, "B", "", 1, 0, nil); err != nil {
		t.Fatal(err)
	}
	queue, err := s.RefreshQueue(2)
	if err != nil || len(queue) != 2 {
		t.Fatalf("queue: %+v %v", queue, err)
	}
	if queue[0] != a.ID || queue[1] != c.ID {
		t.Fatalf("unfetched first: %+v", queue)
	}
	time.Sleep(2 * time.Millisecond)
	if err := s.TouchFetched(a.ID); err != nil {
		t.Fatal(err)
	}
	queue, _ = s.RefreshQueue(2)
	if queue[0] != c.ID || queue[1] != b.ID {
		t.Fatalf("touched order: %+v", queue)
	}
}

func TestPerUserShelvesAndScoping(t *testing.T) {
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	memberUser, err := s.CreateUser("member@example.com", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	member := memberUser.ID
	a := mustRef(t, "a/b/alpha")
	b := mustRef(t, "a/b/beta")
	if _, err := s.UpsertFor(owner, a, Patch{Liked: bp(true), Notes: sp("owner note")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(member, a, Patch{Used: bp(true), Notes: sp("member note")}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpsertFor(owner, b, Patch{}); err != nil {
		t.Fatal(err)
	}
	ownerList, _, err := s.ListFor(owner, Filter{})
	if err != nil || len(ownerList) != 2 {
		t.Fatalf("owner list: %+v %v", ownerList, err)
	}
	memberList, _, err := s.ListFor(member, Filter{})
	if err != nil || len(memberList) != 1 {
		t.Fatalf("member list: %+v %v", memberList, err)
	}
	sk, ok, _ := s.GetFor(member, a.ID)
	if !ok || !sk.Used || sk.Liked || sk.Notes != "member note" {
		t.Fatalf("member skill: %+v", sk)
	}
	if err := s.AddSkillTagsFor(owner, a.ID, []string{"owner-tag"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSkillTagsFor(member, a.ID, []string{"member-tag"}); err != nil {
		t.Fatal(err)
	}
	ownerTags, _ := s.ListTagsFor(owner)
	memberTags, _ := s.ListTagsFor(member)
	ownerCounts, memberCounts := map[string]int{}, map[string]int{}
	for _, tag := range ownerTags {
		ownerCounts[tag.Name] = tag.Count
	}
	for _, tag := range memberTags {
		memberCounts[tag.Name] = tag.Count
	}
	if ownerCounts["owner-tag"] != 1 || ownerCounts["member-tag"] != 0 || memberCounts["member-tag"] != 1 || memberCounts["owner-tag"] != 0 {
		t.Fatalf("tags: %+v %+v", ownerCounts, memberCounts)
	}
	if err := s.RecordEnvironmentFor(owner, a.ID, "laptop", "claude", "", ".claude", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(member, a.ID, "desktop", "cursor", "", ".cursor", ""); err != nil {
		t.Fatal(err)
	}
	ownerEnvs, _ := s.ListEnvironmentsFor(owner)
	memberEnvs, _ := s.ListEnvironmentsFor(member)
	if len(ownerEnvs) != 1 || len(memberEnvs) != 1 || ownerEnvs[0].Host != "laptop" || memberEnvs[0].Host != "desktop" {
		t.Fatalf("envs: %+v %+v", ownerEnvs, memberEnvs)
	}
	if err := s.IgnoreFor(member, b.ID, "beta", "test"); err != nil {
		t.Fatal(err)
	}
	if ignored, _ := s.IsIgnoredFor(owner, b.ID); ignored {
		t.Fatal("owner ignored by member")
	}
	if err := s.AddEventFor(member, "ui", "test", a.ID, nil); err != nil {
		t.Fatal(err)
	}
	ownerEvents, _ := s.ListEventsFor(owner, 10, "")
	memberEvents, _ := s.ListEventsFor(member, 10, "")
	if len(ownerEvents) != 0 || len(memberEvents) != 1 {
		t.Fatalf("events: %+v %+v", ownerEvents, memberEvents)
	}
	if err := s.RemoveFor(member, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.GetFor(owner, a.ID); !ok {
		t.Fatal("owner lost a skill when member removed it")
	}
}

func TestMigrateLegacyShelfToOwner(t *testing.T) {
	s := newStore(t)
	r := mustRef(t, "a/b/legacy")
	if _, err := s.Upsert(r, Patch{Liked: bp(true), Notes: sp("legacy note")}); err != nil {
		t.Fatal(err)
	}
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	sk, ok, err := s.GetFor(owner, r.ID)
	if err != nil || !ok || !sk.Liked || sk.Notes != "legacy note" {
		t.Fatalf("migration: %+v %v %v", sk, ok, err)
	}
}

func TestMigrateLegacyEnvironment(t *testing.T) {
	s := newStore(t)
	r := mustRef(t, "a/b/with-env")
	if _, err := s.Upsert(r, Patch{Used: bp(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO environments(id,user_id,name,kind,agent,host,path,project,first_seen,last_seen)
		VALUES('laptop/claude',0,'laptop · Claude Code','machine','claude','laptop','.claude/skills','',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
		VALUES(?,'laptop/claude',1,1,'.claude/skills/with-env')`, r.ID); err != nil {
		t.Fatal(err)
	}
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	sk, ok, err := s.GetFor(owner, r.ID)
	if err != nil || !ok || len(sk.Environments) != 1 {
		t.Fatalf("migrated envs: %+v %v %v", sk, ok, err)
	}
	if sk.Environments[0].ID != EnvironmentIDFor(owner, "laptop", "claude", "") {
		t.Fatalf("env id: %s", sk.Environments[0].ID)
	}
}
