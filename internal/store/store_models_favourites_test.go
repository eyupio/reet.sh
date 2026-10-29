package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func favouriteModel(i int) string { return fmt.Sprintf("model-%02d", i) }

func mustFavourites(t *testing.T, s *Store, uid int64) ModelFavourites {
	t.Helper()
	out, err := s.ModelFavouritesFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestModelFavouritesMigrateAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid := projectUser(t, s, "a")
	if _, created, err := s.AddModelFavouriteFor(uid, "opencode", "anthropic/claude-sonnet-4-5"); err != nil || !created {
		t.Fatal(created, err)
	}
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal("reopen", err)
	}
	defer s.Close()
	out := mustFavourites(t, s, uid)
	if out.Version != 1 || len(out.Favourites) != 1 || out.Favourites[0].Tool != "opencode" || out.Favourites[0].Model != "anthropic/claude-sonnet-4-5" {
		t.Fatal(out)
	}
	var columns int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('model_favourites')`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 7 {
		t.Fatal("model_favourites columns", columns)
	}
}

func TestModelFavouritesAddAndRemoveAreIdempotent(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	out, created, err := s.AddModelFavouriteFor(uid, "opencode", "model-one")
	if err != nil || !created || out.Version != 1 || len(out.Favourites) != 1 {
		t.Fatal(created, out, err)
	}
	before := out.Version
	out, created, err = s.AddModelFavouriteFor(uid, "opencode", "model-one")
	if err != nil || created || out.Version != before || len(out.Favourites) != 1 {
		t.Fatal("a second star changed something", created, out, err)
	}
	out, removed, err := s.RemoveModelFavouriteFor(uid, "opencode", "model-one")
	if err != nil || !removed || out.Version != before+1 || len(out.Favourites) != 0 {
		t.Fatal(removed, out, err)
	}
	out, removed, err = s.RemoveModelFavouriteFor(uid, "opencode", "model-one")
	if err != nil || removed || out.Version != before+1 || len(out.Favourites) != 0 {
		t.Fatal("a second unstar changed something", removed, out, err)
	}
}

func TestModelFavouritesCap(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	for i := 0; i < MaxModelFavourites; i++ {
		if _, created, err := s.AddModelFavouriteFor(uid, "opencode", favouriteModel(i)); err != nil || !created {
			t.Fatal(i, created, err)
		}
	}
	before := mustFavourites(t, s, uid)
	if _, created, err := s.AddModelFavouriteFor(uid, "opencode", favouriteModel(MaxModelFavourites)); !errors.Is(err, ErrModelFavouritesFull) || created {
		t.Fatal(created, err)
	}
	after := mustFavourites(t, s, uid)
	if after.Version != before.Version || len(after.Favourites) != MaxModelFavourites {
		t.Fatal("a refused star changed the list", after)
	}
}

func TestModelFavouritesOrderChecksVersionAndExactSet(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	for _, model := range []string{"model-a", "model-b", "model-c"} {
		if _, _, err := s.AddModelFavouriteFor(uid, "opencode", model); err != nil {
			t.Fatal(err)
		}
	}
	base := mustFavourites(t, s, uid)
	order := []ModelFavouriteRef{
		{Tool: "opencode", Model: "model-c"},
		{Tool: "opencode", Model: "model-a"},
		{Tool: "opencode", Model: "model-b"},
	}
	for name, in := range map[string]struct {
		version int64
		items   []ModelFavouriteRef
	}{
		"stale version":    {base.Version + 1, order},
		"missing item":     {base.Version, order[:2]},
		"extra item":       {base.Version, append(append([]ModelFavouriteRef(nil), order...), ModelFavouriteRef{Tool: "opencode", Model: "model-d"})},
		"duplicate item":   {base.Version, []ModelFavouriteRef{order[0], order[0], order[1]}},
		"substituted item": {base.Version, []ModelFavouriteRef{order[0], order[1], {Tool: "opencode", Model: "model-d"}}},
	} {
		if _, err := s.ReorderModelFavouritesFor(uid, in.version, in.items); !errors.Is(err, ErrModelFavouritesStale) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got := mustFavourites(t, s, uid); got.Version != base.Version || got.Favourites[0].Model != "model-a" {
		t.Fatal("a refused reorder moved something", got)
	}
	out, err := s.ReorderModelFavouritesFor(uid, base.Version, order)
	if err != nil || out.Version != base.Version+1 || len(out.Favourites) != 3 ||
		out.Favourites[0].Model != "model-c" || out.Favourites[1].Model != "model-a" || out.Favourites[2].Model != "model-b" {
		t.Fatal(out, err)
	}
	// Asking for the order already stored changes nothing, version included.
	again, err := s.ReorderModelFavouritesFor(uid, out.Version, order)
	if err != nil || again.Version != out.Version {
		t.Fatal("same order moved the version", again, err)
	}
	// The old version is stale now.
	if _, err := s.ReorderModelFavouritesFor(uid, base.Version, order); !errors.Is(err, ErrModelFavouritesStale) {
		t.Fatal(err)
	}
}

func TestModelFavouriteDefaultOnePerTool(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	for _, fav := range []ModelFavouriteRef{{Tool: "opencode", Model: "model-a"}, {Tool: "opencode", Model: "model-b"}, {Tool: "claude", Model: "model-c"}} {
		if _, _, err := s.AddModelFavouriteFor(uid, fav.Tool, fav.Model); err != nil {
			t.Fatal(err)
		}
	}
	base := mustFavourites(t, s, uid)
	out, changed, err := s.SetModelFavouriteDefaultFor(uid, "opencode", "model-a")
	if err != nil || !changed || out.Version != base.Version+1 || !defaultOf(out, "opencode", "model-a") {
		t.Fatal(out, changed, err)
	}
	out, changed, err = s.SetModelFavouriteDefaultFor(uid, "opencode", "model-a")
	if err != nil || changed || out.Version != base.Version+1 {
		t.Fatal("same default changed something", out, changed, err)
	}
	out, changed, err = s.SetModelFavouriteDefaultFor(uid, "opencode", "model-b")
	if err != nil || !changed || defaultOf(out, "opencode", "model-a") || !defaultOf(out, "opencode", "model-b") {
		t.Fatal("the old default was not cleared", out, changed, err)
	}
	// Another tool keeps its own default.
	out, changed, err = s.SetModelFavouriteDefaultFor(uid, "claude", "model-c")
	if err != nil || !changed || !defaultOf(out, "claude", "model-c") || !defaultOf(out, "opencode", "model-b") {
		t.Fatal(out, changed, err)
	}
	out, changed, err = s.SetModelFavouriteDefaultFor(uid, "opencode", "")
	if err != nil || !changed || defaultOf(out, "opencode", "model-b") || !defaultOf(out, "claude", "model-c") {
		t.Fatal("clear", out, changed, err)
	}
	out, changed, err = s.SetModelFavouriteDefaultFor(uid, "opencode", "")
	if err != nil || changed {
		t.Fatal("clearing an empty default changed something", out, changed, err)
	}
	if _, _, err := s.SetModelFavouriteDefaultFor(uid, "opencode", "model-c"); !errors.Is(err, ErrModelFavouriteNotFound) {
		t.Fatal("a model starred for another tool became a default", err)
	}
}

func defaultOf(out ModelFavourites, tool, model string) bool {
	for _, f := range out.Favourites {
		if f.Tool == tool && f.Model == model {
			return f.AgentDefault
		}
	}
	return false
}

func TestModelFavouritesAreOwnerScoped(t *testing.T) {
	s := newStore(t)
	a, b := projectUser(t, s, "a"), projectUser(t, s, "b")
	if _, _, err := s.AddModelFavouriteFor(a, "opencode", "model-a"); err != nil {
		t.Fatal(err)
	}
	if out := mustFavourites(t, s, b); out.Version != 0 || len(out.Favourites) != 0 {
		t.Fatal("another account saw the star", out)
	}
	if out, removed, err := s.RemoveModelFavouriteFor(b, "opencode", "model-a"); err != nil || removed || len(out.Favourites) != 0 {
		t.Fatal(out, removed, err)
	}
	if _, _, err := s.SetModelFavouriteDefaultFor(b, "opencode", "model-a"); !errors.Is(err, ErrModelFavouriteNotFound) {
		t.Fatal(err)
	}
	if out := mustFavourites(t, s, a); out.Version != 1 || len(out.Favourites) != 1 {
		t.Fatal("another account's writes touched this one", out)
	}
}

func TestProfileDefaultsSurviveBackupAndRestore(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	p, err := s.ProfileFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	p.DefaultTool, p.DefaultModel, p.DefaultEffort = "cline", "model-one", "high"
	p.Theme = "dark"
	if err := s.SaveProfileFor(uid, p); err != nil {
		t.Fatal(err)
	}
	backupDir := t.TempDir()
	backup := filepath.Join(backupDir, "reet.db")
	if err := s.VacuumInto(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	if err := CheckDatabaseFile(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	restored, err := Open(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.ProfileFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultTool != "cline" || got.DefaultModel != "model-one" || got.DefaultEffort != "high" || got.Theme != "dark" {
		t.Fatal("profile defaults did not survive the backup", got)
	}
}

func TestFolderProjectEffortAndCline(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	saved, err := s.CreateFolderProjectFor(uid, FolderProject{
		Host: "laptop", Dir: "/home/tester/site", DefaultTool: "cline", DefaultModel: "model-one", DefaultEffort: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.DefaultTool != "cline" || saved.DefaultModel != "model-one" || saved.DefaultEffort != "medium" {
		t.Fatal(saved)
	}
	got, err := s.FolderProjectFor(uid, saved.ID)
	if err != nil || got.DefaultEffort != "medium" || got.DefaultTool != "cline" {
		t.Fatal(got, err)
	}
	for name, p := range map[string]FolderProject{
		"model without a tool":  {Host: "laptop", Dir: "/a", DefaultModel: "model-one"},
		"effort without a tool": {Host: "laptop", Dir: "/b", DefaultEffort: "high"},
		"effort on claude":      {Host: "laptop", Dir: "/c", DefaultTool: "claude", DefaultEffort: "high"},
		"unknown effort":        {Host: "laptop", Dir: "/d", DefaultTool: "cline", DefaultEffort: "max"},
		"unknown tool":          {Host: "laptop", Dir: "/e", DefaultTool: "bash"},
	} {
		if _, err := s.CreateFolderProjectFor(uid, p); !errors.Is(err, ErrFolderProjectInvalid) {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestFolderProjectExistingRowsKeepTheirDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid := projectUser(t, s, "a")
	// A row written before the effort column existed.
	if _, err := s.db.Exec(`ALTER TABLE folder_projects DROP COLUMN default_effort`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO folder_projects(id,user_id,host,dir,name,name_key,is_repo,base_branch,worktree_default,default_tool,default_model,starter_prompt,colour,position,legacy_project_id,note,created_at,updated_at,version)
		VALUES('old',?,'laptop','/old','old','old',0,'','inherit','claude','model-one','','',0,NULL,'',1,1,1)`, uid); err != nil {
		t.Fatal(err)
	}
	s.Close()
	for i := 0; i < 2; i++ {
		s, err = Open(dir)
		if err != nil {
			t.Fatal("reopen", i, err)
		}
		got, err := s.FolderProjectFor(uid, "old")
		if err != nil || got.DefaultTool != "claude" || got.DefaultModel != "model-one" || got.DefaultEffort != "" {
			t.Fatalf("open %d: %+v %v", i, got, err)
		}
		s.Close()
	}
}

func TestFolderProjectLegacyModelWithoutToolCanStillBeRenamed(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	if _, err := s.db.Exec(`INSERT INTO folder_projects(id,user_id,host,dir,name,name_key,is_repo,base_branch,worktree_default,default_tool,default_model,default_effort,starter_prompt,colour,position,legacy_project_id,note,created_at,updated_at,version)
		VALUES('legacy',?,'laptop','/legacy','legacy','legacy',0,'','inherit','','model-one','','','',0,NULL,'',1,1,1)`, uid); err != nil {
		t.Fatal(err)
	}
	got, err := s.FolderProjectFor(uid, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	got.Name = "renamed"
	if _, err := s.UpdateFolderProjectFor(uid, got, 1); err != nil {
		t.Fatalf("a rename was blocked by the legacy pairing: %v", err)
	}
	after, err := s.FolderProjectFor(uid, "legacy")
	if err != nil || after.Name != "renamed" || after.DefaultModel != "model-one" || after.DefaultTool != "" {
		t.Fatalf("legacy row: %+v %v", after, err)
	}
	after.DefaultModel = "other-model"
	if _, err := s.UpdateFolderProjectFor(uid, after, 2); !errors.Is(err, ErrFolderProjectInvalid) {
		t.Fatalf("a launch-default edit kept the legacy pairing: %v", err)
	}
}

func TestProfileModelWithoutToolStaysReadable(t *testing.T) {
	s := newStore(t)
	uid := projectUser(t, s, "a")
	if _, err := s.db.Exec(`INSERT INTO user_profiles(user_id,theme,timezone,date_format,default_model,updated_at) VALUES(?,'system','','locale','model-one',1)`, uid); err != nil {
		t.Fatal(err)
	}
	p, err := s.ProfileFor(uid)
	if err != nil || p.DefaultModel != "model-one" || p.DefaultTool != "" {
		t.Fatal("an existing row with a model and no tool was not readable", p, err)
	}
}
