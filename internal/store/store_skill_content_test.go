package store

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func contentFiles(name, body string) []skillbundle.ContentFile {
	return []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\ndescription: Kept in reet\n---\n" + body + "\n")}}
}

func importedItem(t *testing.T, s *Store, uid int64, in string) skillStateRow {
	t.Helper()
	r := mustRef(t, in)
	if _, err := s.UpsertFor(uid, r, Patch{}); err != nil {
		t.Fatal(err)
	}
	return skillStateNamed(t, s, uid, r.Slug)
}

// localItem is a skill only a machine has: its shelf row is local/{name}, found
// in the agents folder of host.
func localItem(t *testing.T, s *Store, uid int64, host, name string) skillStateRow {
	t.Helper()
	if _, err := s.UpsertLocalFor(uid, name, Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(uid, "local/"+name, host, "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	return skillStateNamed(t, s, uid, name)
}

func TestCopyNeedsFetchForImported(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if item.origin != "imported" || item.revision != "" {
		t.Fatalf("fixture: %+v", item)
	}
	needs, err := s.SkillsNeedingCopyFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkillCopyNeed{{Item: item.id, Name: "find-skills", Kind: "fetch"}}
	if !reflect.DeepEqual(needs, want) {
		t.Fatalf("needs = %+v, want %+v", needs, want)
	}
	r, ok, err := s.SkillSourceFor(uid, item.id)
	if err != nil || !ok || r.ID != "vercel-labs/skills/find-skills" || r.SourceType != "github" {
		t.Fatalf("source = %+v %v %v", r, ok, err)
	}
}

func TestCopyNeedsCaptureForMachineOnly(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "x")
	agent := reportAgent(t, s, uid, "elitebook")
	if needs, err := s.SkillsNeedingCopyFor(uid); err != nil || len(needs) != 0 {
		t.Fatalf("a machine that has not reported is no source: %+v %v", needs, err)
	}
	reportOf(t, s, uid, agent, reportT0, installedAt("x", "d1"))
	needs, err := s.SkillsNeedingCopyFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	want := []SkillCopyNeed{{Item: item.id, Name: "x", Kind: "capture", AgentID: agent, Folder: "~/.agents/skills"}}
	if !reflect.DeepEqual(needs, want) {
		t.Fatalf("needs = %+v, want %+v", needs, want)
	}
}

func TestUnsourcedActiveSkillGetsAProblem(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "lonely")
	if n, err := s.MarkUnsourcedSkillsFor(uid); err != nil || n != 1 {
		t.Fatalf("marked %d, %v", n, err)
	}
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "copy-failed" || h.Reason != SkillNoSourceProblem {
		t.Fatalf("health: %+v", h)
	}
	if needs, _ := s.SkillsNeedingCopyFor(uid); len(needs) != 0 {
		t.Fatalf("a skill with a problem needs nothing until retried: %+v", needs)
	}
	// A machine that reports it later is a source once Retry clears the problem.
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, reportT0, installedAt("lonely", "d1"))
	if n, err := s.RetrySkillCopiesFor(uid, []string{item.id}); err != nil || n != 1 {
		t.Fatalf("retry cleared %d, %v", n, err)
	}
	needs, _ := s.SkillsNeedingCopyFor(uid)
	if len(needs) != 1 || needs[0].Kind != "capture" {
		t.Fatalf("needs after retry: %+v", needs)
	}
}

func TestUnsourcedArchivedSkillIsLeftAlone(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "gone")
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	if n, err := s.MarkUnsourcedSkillsFor(uid); err != nil || n != 0 {
		t.Fatalf("marked %d, %v", n, err)
	}
}

func TestArchiveWaitsForCopy(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	later := desiredT0.Add(time.Hour)
	if _, remove := desiredNames(desiredAt(t, s, uid, later)); len(remove) != 0 {
		t.Fatalf("removed before reet holds a copy: %v", remove)
	}
	_, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Find skills."), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills/tree/HEAD/skills/find-skills"})
	if err != nil {
		t.Fatal(err)
	}
	if _, remove := desiredNames(desiredAt(t, s, uid, later)); !reflect.DeepEqual(remove, []string{"find-skills"}) {
		t.Fatalf("remove after the copy landed: %v", remove)
	}
	row := skillStateNamed(t, s, uid, "find-skills")
	if row.id != item.id || row.revision == "" || row.status != SkillArchived {
		t.Fatalf("the copy must fill the same archived item: %+v", row)
	}
	var ref, origin string
	if err = s.db.QueryRow(`SELECT origin_ref,origin FROM library_items WHERE user_id=? AND id=?`, uid, item.id).Scan(&ref, &origin); err != nil || !strings.HasPrefix(ref, "https://github.com/") || origin != "imported" {
		t.Fatalf("origin_ref %q origin %q %v", ref, origin, err)
	}
}

func TestCopyProblemKeepsSkillOnMachines(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "big")
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, reportT0, installedAt("big", "d1"))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSkillCopyProblemFor(uid, item.id, "too large"); err != nil {
		t.Fatal(err)
	}
	later := desiredT0.Add(time.Hour)
	if _, remove := desiredNames(desiredAt(t, s, uid, later)); len(remove) != 0 {
		t.Fatalf("removed with no copy: %v", remove)
	}
	if row := skillStateNamed(t, s, uid, "big"); row.status != SkillArchived {
		t.Fatalf("status: %+v", row)
	}
	if h := healthOf(t, s, uid, item.id, later); h.State != "copy-failed" || h.Reason != "too large" {
		t.Fatalf("health: %+v", h)
	}
}

func TestCopyingAndGettingStates(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	fetched := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	captured := localItem(t, s, uid, "elitebook", "kept")
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, reportT0, installedAt("kept", "d1"))
	if h := healthOf(t, s, uid, fetched.id, reportT0); h.State != "getting" {
		t.Fatalf("a fetch pending: %+v", h)
	}
	if h := healthOf(t, s, uid, captured.id, reportT0); h.State != "copying" || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("a capture pending: %+v", h)
	}
}

func TestStorageLimitsHoldALibrary(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	body := strings.Repeat("x", 4096)
	for i := 0; i < 139; i++ {
		name := fmt.Sprintf("skill-%03d", i)
		in := namedPrivateWrite(name, fmt.Sprintf("limits_request_%04d", i))
		in.Files[0].Data = append(in.Files[0].Data, body...)
		if _, err := s.SaveLibraryContentFor(uid, "", in); err != nil {
			if errors.Is(err, ErrPrivateSkillCapacity) {
				t.Fatalf("bundle %d hit the capacity limit", i)
			}
			t.Fatal(err)
		}
	}
}

func TestCaptureRefusesPathsOutsideManagedFolders(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "x")
	agent := reportAgent(t, s, uid, "elitebook")
	for _, p := range []string{"~/.ssh", "~/.ssh/id_rsa", "/etc/passwd", "~/.agents/skills/../../.ssh", "~/.agents/skills/..", "~/.agents", "~/.agentsx/skills/x", "/home/u/.agents/skills/x", ""} {
		if err := s.RequestSkillCaptureFor(uid, item.id, agent, p); !errors.Is(err, ErrSkillCapturePath) {
			t.Fatalf("%q: err = %v", p, err)
		}
	}
	for _, p := range []string{"~/.agents/skills/x", "~/.claude/skills", "~/.config/opencode/skills/x/.reet-backups/y", "~/.agents/.reet-backups/x.1"} {
		if _, _, ok := managedSkillPath(p); !ok {
			t.Fatalf("%q should be allowed", p)
		}
	}
	// A managed path passes the check; with no capability yet the machine is
	// simply not asked.
	if err := s.RequestSkillCaptureFor(uid, item.id, agent, "~/.agents/skills/x"); errors.Is(err, ErrSkillCapturePath) {
		t.Fatal(err)
	}
}

func TestEditedSinceImport(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "v1"), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills/tree/HEAD/skills/find-skills"}); err != nil {
		t.Fatal(err)
	}
	if edited, err := s.SkillEditedSinceImportFor(uid, item.id); err != nil || edited {
		t.Fatalf("fresh import: %v %v", edited, err)
	}
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "mine"), PrivateSkillOrigin{Kind: "edit"}); err != nil {
		t.Fatal(err)
	}
	if edited, err := s.SkillEditedSinceImportFor(uid, item.id); err != nil || !edited {
		t.Fatalf("after an edit: %v %v", edited, err)
	}
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "v2"), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills/tree/HEAD/skills/find-skills"}); err != nil {
		t.Fatal(err)
	}
	if edited, _ := s.SkillEditedSinceImportFor(uid, item.id); edited {
		t.Fatal("a newer fetched revision replaces the edits")
	}
	if rows := skillStatesNamed(t, s, uid, "find-skills"); len(rows) != 1 {
		t.Fatalf("content must stay on one item: %+v", rows)
	}
}

func TestSkillContentFromRefusesAMovedHead(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	origin := PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/vercel-labs/skills/tree/HEAD/skills/find-skills"}
	v1, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "v1"), origin)
	if err != nil {
		t.Fatal(err)
	}
	mine, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "mine"), PrivateSkillOrigin{Kind: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSkillContentFromFor(uid, item.id, contentFiles("find-skills", "v2"), origin, v1.Revision.ID); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatalf("a head that moved must be refused: %v", err)
	}
	if edited, _ := s.SkillEditedSinceImportFor(uid, item.id); !edited {
		t.Fatal("a refused write must leave the edit as the head")
	}
	if _, err := s.SetSkillContentFromFor(uid, item.id, contentFiles("find-skills", "v2"), origin, mine.Revision.ID); err != nil {
		t.Fatalf("the current head is accepted: %v", err)
	}
}

func TestMachineCaptureRecordsWhereItCameFromAndFollowsArchive(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	item := localItem(t, s, uid, agent.Host, "demo")
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimRemoteImportFor(uid, agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestSkillCaptureFor(uid, item.id, agent.ID, "~/.agents/skills/demo"); err != nil {
		t.Fatal(err)
	}
	var id string
	if err := s.db.QueryRow(`SELECT id FROM machine_import_requests WHERE user_id=?`, uid).Scan(&id); err != nil {
		t.Fatal(err)
	}
	req, err := s.RemoteImportFor(uid, id)
	if err != nil {
		t.Fatal(err)
	}
	if done := finishMachineCopy(t, s, uid, agent, req, machineImportFixture(t).Files); done.State != "completed" {
		t.Fatalf("capture: %+v", done)
	}
	var ref string
	if err = s.db.QueryRow(`SELECT origin_ref FROM library_items WHERE user_id=? AND id=?`, uid, item.id).Scan(&ref); err != nil || ref != "machine:laptop:~/.agents/skills" {
		t.Fatalf("origin_ref %q %v", ref, err)
	}
	if _, remove := desiredNames(desiredAt(t, s, uid, desiredT0.Add(time.Hour))); !reflect.DeepEqual(remove, []string{"demo"}) {
		t.Fatalf("remove after capture: %v", remove)
	}
}

func TestCaptureNeedSkipsMachinesThatCannotBeAsked(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "thinkpad", "x") // only thinkpad has an environment
	elite := reportAgent(t, s, uid, "elitebook")
	think := reportAgent(t, s, uid, "thinkpad")
	// elitebook sorts first but reet has no environment for it; thinkpad's folder is not managed.
	reportOf(t, s, uid, elite, reportT0, installedAt("x", "d1"))
	reportOf(t, s, uid, think, reportT0, SkillReportEntry{Folder: "/tmp/elsewhere", Name: "x", State: "installed", Digest: "d1"})
	if needs, err := s.SkillsNeedingCopyFor(uid); err != nil || len(needs) != 0 {
		t.Fatalf("no machine can be asked: %+v %v", needs, err)
	}
	if n, err := s.MarkUnsourcedSkillsFor(uid); err != nil || n != 1 {
		t.Fatalf("marked %d, %v", n, err)
	}
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "copy-failed" || h.Reason != SkillNoSourceProblem {
		t.Fatalf("health: %+v", h)
	}
	// Once thinkpad reports it in a managed folder, it is the one asked.
	if _, err := s.RetrySkillCopiesFor(uid, []string{item.id}); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, think, reportT0, installedAt("x", "d1"))
	needs, _ := s.SkillsNeedingCopyFor(uid)
	if len(needs) != 1 || needs[0].AgentID != think {
		t.Fatalf("needs: %+v", needs)
	}
}

func TestOriginRefBackfillIsTheOriginalLink(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	gh := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	wk := importedItem(t, s, uid, "example.com/foo")
	if _, err := s.db.Exec(`UPDATE library_items SET origin_ref='https://www.skills.sh/vercel-labs/skills/find-skills' WHERE id=?`, gh.id); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateSkillContent(); err != nil {
		t.Fatal(err)
	}
	get := func(id string) string {
		_, ref, err := s.SkillOriginFor(uid, id)
		if err != nil {
			t.Fatal(err)
		}
		return ref
	}
	if got := get(gh.id); got != "https://github.com/vercel-labs/skills" {
		t.Fatalf("github origin_ref = %q", got)
	}
	if got := get(wk.id); got != "" {
		t.Fatalf("well-known origin_ref = %q, want empty until content lands", got)
	}
}

// A capture is the reet copy's starting point, like a fetched original: an edit
// after it is an edit, and a newer capture replaces the edits. Revisions saved
// before captures had their own kind (import) count the same way.
func TestEditedSinceImportCountsACaptureAsTheStartingPoint(t *testing.T) {
	for _, kind := range []string{"capture", "import"} {
		t.Run(kind, func(t *testing.T) {
			s := newStore(t)
			uid, _ := s.DefaultOwner()
			item := localItem(t, s, uid, "elitebook", "x")
			if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("x", "v1"), PrivateSkillOrigin{Kind: kind, Reference: "machine:elitebook:~/.agents/skills"}); err != nil {
				t.Fatal(err)
			}
			if edited, err := s.SkillEditedSinceImportFor(uid, item.id); err != nil || edited {
				t.Fatalf("fresh capture: %v %v", edited, err)
			}
			if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("x", "mine"), PrivateSkillOrigin{Kind: "edit"}); err != nil {
				t.Fatal(err)
			}
			if edited, err := s.SkillEditedSinceImportFor(uid, item.id); err != nil || !edited {
				t.Fatalf("after an edit: %v %v", edited, err)
			}
			if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("x", "v2"), PrivateSkillOrigin{Kind: kind, Reference: "machine:elitebook:~/.agents/skills"}); err != nil {
				t.Fatal(err)
			}
			if edited, _ := s.SkillEditedSinceImportFor(uid, item.id); edited {
				t.Fatal("a newer capture replaces the edits")
			}
		})
	}
}

// "No machine has a copy" only holds until a machine that can be asked reports
// the skill; any other problem stays until Retry.
func TestNoSourceProblemClearsWhenASourceAppears(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := localItem(t, s, uid, "elitebook", "lonely")
	other := localItem(t, s, uid, "elitebook", "broken")
	if n, err := s.MarkUnsourcedSkillsFor(uid); err != nil || n != 2 {
		t.Fatalf("marked %d, %v", n, err)
	}
	if _, err := s.db.Exec(`UPDATE library_items SET copy_problem='folder is over 8 MiB' WHERE user_id=? AND id=?`, uid, other.id); err != nil {
		t.Fatal(err)
	}
	agent := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, agent, reportT0, installedAt("lonely", "d1"), installedAt("broken", "d2"))
	needs, err := s.SkillsNeedingCopyFor(uid)
	if err != nil || len(needs) != 1 || needs[0].Item != item.id || needs[0].Kind != "capture" || needs[0].AgentID != agent {
		t.Fatalf("needs: %+v %v", needs, err)
	}
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "copying" {
		t.Fatalf("the cleared skill: %+v", h)
	}
	if h := healthOf(t, s, uid, other.id, reportT0); h.State != "copy-failed" || h.Reason != "folder is over 8 MiB" {
		t.Fatalf("another problem was cleared: %+v", h)
	}
}

// Only an item's first content puts an Archived skill's name on the removal
// list; a later write of any kind (edit, use-version, original, capture) must
// not bring back a purged row.
func TestEditOfAnArchivedSkillDoesNotReAddItsRemovalRow(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM skill_removals WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"edit", "use-version"} {
		if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("seo-audit", kind), PrivateSkillOrigin{Kind: kind}); err != nil {
			t.Fatal(err)
		}
		if r := skillRemovals(t, s, uid); len(r) != 0 {
			t.Fatalf("a %s re-added the removal row: %+v", kind, r)
		}
	}
	for _, kind := range []string{"original", "capture"} {
		if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("seo-audit", kind), PrivateSkillOrigin{Kind: kind, Reference: "https://github.com/a/b"}); err != nil {
			t.Fatal(err)
		}
		if r := skillRemovals(t, s, uid); len(r) != 0 {
			t.Fatalf("a %s landing on an item that had content re-added the removal row: %+v", kind, r)
		}
	}
}

// CF2: the name joins the removal list once reet holds a copy of an Archived
// item, whatever kind the first content is.
func TestArchiveRemovalRowFollowsTheFirstContentOfAnyKind(t *testing.T) {
	for _, kind := range []string{"edit", "use-version", "original", "capture"} {
		s := newStore(t)
		uid, _ := s.DefaultOwner()
		item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
		if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
			t.Fatal(err)
		}
		if r := skillRemovals(t, s, uid); len(r) != 0 {
			t.Fatalf("%s: before any content: %+v", kind, r)
		}
		if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Find skills."), PrivateSkillOrigin{Kind: kind, Reference: "x"}); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if r := skillRemovals(t, s, uid); r["find-skills"].Reason != "archived" {
			t.Fatalf("%s: the first content of an Archived item must put its name on the list: %+v", kind, r)
		}
	}
}

func TestEditedArchivedItemThatHadContentAddsNoRemovalRow(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := importedItem(t, s, uid, "vercel-labs/skills/find-skills")
	if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Find skills."), PrivateSkillOrigin{Kind: "original", Reference: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, desiredT0); err != nil {
		t.Fatal(err)
	}
	// The row was purged (every machine confirmed it); an edit must not bring it back.
	if _, err := s.db.Exec(`DELETE FROM skill_removals WHERE user_id=?`, uid); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"edit", "original"} {
		if _, err := s.SetSkillContentFor(uid, item.id, contentFiles("find-skills", "Changed by "+kind), PrivateSkillOrigin{Kind: kind, Reference: "x"}); err != nil {
			t.Fatal(err)
		}
		if r := skillRemovals(t, s, uid); len(r) != 0 {
			t.Fatalf("%s of an item that already had content: %+v", kind, r)
		}
	}
}

// A library copy takes any name a machine can hold a skill under (the agent's
// rule), not only the lowercase-hyphen names hosting needs; its SKILL.md may
// name it loosely (case, spaces, hyphens and underscores aside).
func TestLibraryCopyTakesAnyNameAMachineAccepts(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	for _, c := range []struct{ name, mdName string }{
		{"my_skill", "my_skill"},
		{"Skill-Creator", "skill-creator"},
		{"Skill Creator+", "Skill Creator+"},
	} {
		id := "item-" + strings.NewReplacer(" ", "-", "+", "p").Replace(strings.ToLower(c.name))
		if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,1,1)`, uid, id, LibrarySkill, c.name, SkillActive, "imported"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.SetSkillContentFor(uid, id, contentFiles(c.mdName, "Body."), PrivateSkillOrigin{Kind: "original", Reference: "https://github.com/o/r"}); err != nil {
			t.Fatalf("copy of %q: %v", c.name, err)
		}
		if r := skillStateNamed(t, s, uid, c.name); r.revision == "" {
			t.Fatalf("copy of %q left no revision: %+v", c.name, r)
		}
	}
	// SKILL.md naming another skill is still refused.
	if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,1,1)`, uid, "other", LibrarySkill, "my_other", SkillActive, "imported"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetSkillContentFor(uid, "other", contentFiles("something-else", "Body."), PrivateSkillOrigin{Kind: "original"}); !errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatalf("SKILL.md naming another skill: %v", err)
	}
}

func TestValidLibrarySkillName(t *testing.T) {
	for name, want := range map[string]bool{
		"seo-audit": true, "my_skill": true, "Skill-Creator": true, "with space": true, "café": true,
		"": false, ".": false, "..": false, ".hidden": false, "a/b": false, `a\b`: false, "a\x00b": false,
		strings.Repeat("a", 255): true, strings.Repeat("a", 256): false,
	} {
		if got := ValidLibrarySkillName(name); got != want {
			t.Errorf("ValidLibrarySkillName(%q) = %v, want %v", name, got, want)
		}
	}
	// Hosting still needs a hosted name.
	if ValidHostedName("my_skill") {
		t.Fatal("my_skill became a hosted name")
	}
}
