package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// historyFixture is a written skill edited twice: three versions, the last
// one current.
func historyFixture(t *testing.T) (*Store, int64, skillStateRow, []string) {
	t.Helper()
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "notes", false)
	revs := []string{item.revision}
	for _, body := range []string{"Second.", "Third."} {
		res, err := s.SetSkillContentFromFor(uid, item.id, contentFiles("notes", body), PrivateSkillOrigin{Kind: "edit"}, revs[len(revs)-1])
		if err != nil {
			t.Fatal(err)
		}
		revs = append(revs, res.Revision.ID)
	}
	return s, uid, skillStateNamed(t, s, uid, "notes"), revs
}

func TestSkillHistoryListsVersionsNewestFirst(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Revision != revs[2] || got[1].Revision != revs[1] || got[2].Revision != revs[0] {
		t.Fatalf("history order: %+v (want %v reversed)", got, revs)
	}
	if !got[0].Current || got[1].Current || got[2].Current {
		t.Fatalf("only the head is current: %+v", got)
	}
	for _, e := range got[:2] {
		if e.Kind != "edit" || e.Files != 1 || e.Digest == "" || e.SavedAt == 0 {
			t.Fatalf("edit entry: %+v", e)
		}
	}
	if got[2].Kind == "" || got[2].Files < 1 {
		t.Fatalf("first entry: %+v", got[2])
	}
}

// A skill kept from a machine names that machine, as the capture saved it
// (kind import, its enrollment in the reference): by the machine's current
// name, and by none once reet no longer knows the machine.
func TestSkillHistoryNamesTheMachineAKeptSkillCameFrom(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	item := localItem(t, s, uid, agent.Host, "demo")
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
	machine := func() SkillHistoryEntry {
		t.Helper()
		got, err := s.SkillHistoryFor(uid, item.id)
		if err != nil || len(got) != 1 {
			t.Fatalf("history: %+v %v", got, err)
		}
		return got[0]
	}
	if e := machine(); e.Kind != "import" || e.Machine != "laptop" {
		t.Fatalf("a kept skill's version: %+v", e)
	}
	if _, err = s.db.Exec(`UPDATE sync_agents SET host='studio' WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	if e := machine(); e.Machine != "studio" {
		t.Fatalf("after the machine was renamed: %+v", e)
	}
	if _, err = s.db.Exec(`DELETE FROM sync_agents WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	if e := machine(); e.Machine != "" {
		t.Fatalf("a machine reet no longer knows: %+v", e)
	}
}

// Use this version names the machine whose copy was taken, as it saved it
// (kind use-version, its host in the reference).
func TestSkillHistoryNamesTheMachineOfUseThisVersion(t *testing.T) {
	s, uid, agent, _ := observationFixture(t)
	if err := s.SetAgentSkillsProtocolFor(uid, agent.ID, CurrentSkillsProtocol); err != nil {
		t.Fatal(err)
	}
	item := savedWrittenSkill(t, s, uid, "notes", false)
	digest := ""
	for _, a := range desiredAt(t, s, uid, time.Now().Add(time.Hour)).Active {
		if a.Item == item.id {
			digest = a.Digest
		}
	}
	reportOf(t, s, uid, agent.ID, time.Now(), SkillReportEntry{Folder: "~/.agents/skills", Name: "notes", State: "edited", Digest: digest, Backup: "~/.agents/.reet-backups/2026-11-18/agents-notes"})
	if _, err := s.ClaimRemoteImportFor(uid, agent.ID, strings.Repeat("a", 48)); err != nil {
		t.Fatal(err)
	}
	asked, err := s.RequestSkillUseVersionFor(uid, item.id, agent.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	req, err := s.RemoteImportFor(uid, asked.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done := finishMachineCopy(t, s, uid, agent, req, contentFiles("notes", "Edited on the laptop.")); done.State != "completed" {
		t.Fatalf("use version: %+v", done)
	}
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil || len(got) != 2 || got[0].Kind != "use-version" || got[0].Machine != "laptop" || !got[0].Current {
		t.Fatalf("history after Use this version: %+v %v", got, err)
	}
	// The host it saved stands when reet no longer knows the machine.
	if _, err = s.db.Exec(`DELETE FROM sync_agents WHERE id=?`, agent.ID); err != nil {
		t.Fatal(err)
	}
	if got, err = s.SkillHistoryFor(uid, item.id); err != nil || got[0].Machine != "laptop" {
		t.Fatalf("after the machine was forgotten: %+v %v", got, err)
	}
}

func TestSkillHistoryIsTheOwnersOnly(t *testing.T) {
	s, _, item, _ := historyFixture(t)
	other, err := s.CreateUser("history-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SkillHistoryFor(other.ID, item.id); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another account read the history: %v", err)
	}
	if _, err = s.RestoreSkillRevisionFor(other.ID, item.id, item.revision, item.revision); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another account put a version back: %v", err)
	}
}

// Put back makes the old version's files a new head of kind restore, so the
// history only grows, and machines are given it like any edit.
func TestPutBackMakesANewRevisionThatMachinesGet(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	res, err := s.RestoreSkillRevisionFor(uid, item.id, revs[0], revs[2])
	if err != nil {
		t.Fatal(err)
	}
	now := skillStateNamed(t, s, uid, "notes")
	if now.revision != res.Revision.ID || now.revision == revs[0] || now.bundle != item.bundle {
		t.Fatalf("head after put back: %+v (result %s)", now, res.Revision.ID)
	}
	old, err := s.PrivateSkillRevisionFor(uid, item.bundle, revs[0])
	if err != nil {
		t.Fatal(err)
	}
	if res.Revision.ManifestDigest != old.ManifestDigest {
		t.Fatalf("put back saved other files: %s, want %s", res.Revision.ManifestDigest, old.ManifestDigest)
	}
	got, err := s.SkillHistoryFor(uid, item.id)
	if err != nil || len(got) != 4 || got[0].Kind != "restore" || got[0].DerivedFrom != revs[0] || !got[0].Current {
		t.Fatalf("history after put back: %+v %v", got, err)
	}
	found := false
	for _, a := range desiredAt(t, s, uid, time.Now().Add(time.Hour)).Active {
		if a.Item == item.id {
			found = a.Revision == res.Revision.ID && a.Digest == old.ManifestDigest
		}
	}
	if !found {
		t.Fatalf("machines are not given the version put back: %+v", desiredAt(t, s, uid, time.Now().Add(time.Hour)).Active)
	}
}

func TestPutBackRefusesAMovedHeadTheCurrentVersionAndAnotherSkills(t *testing.T) {
	s, uid, item, revs := historyFixture(t)
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, revs[0], revs[1]); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatalf("a stale head: %v", err)
	}
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, revs[2], revs[2]); !errors.Is(err, ErrSkillRevisionCurrent) {
		t.Fatalf("the current version: %v", err)
	}
	other := savedWrittenSkill(t, s, uid, "other", false)
	if _, err := s.RestoreSkillRevisionFor(uid, item.id, other.revision, revs[2]); !errors.Is(err, ErrPrivateSkillMissing) {
		t.Fatalf("another skill's version: %v", err)
	}
	if after := skillStateNamed(t, s, uid, "notes"); after.revision != revs[2] {
		t.Fatalf("a refused put back moved the head: %+v", after)
	}
}
