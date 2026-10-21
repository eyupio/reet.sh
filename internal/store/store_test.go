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
