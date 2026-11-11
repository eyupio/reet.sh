//go:build sync_contract

package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
)

// Stage 4 cross-stage scenarios (A05-A09, A11, A13, A15, A16, A19, A20) run
// against a real controller and separately enrolled connector processes with
// their own homes. Managed removals are switched on for these test controllers
// only; production controllers keep the internal rollout control off.

func (h *syncContractHarness) itemID(name string) string {
	h.t.Helper()
	items, err := h.st.LibraryFor(h.uid)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == syncContractName(name) {
			return item.ID
		}
	}
	h.t.Fatalf("no library item named %s", name)
	return ""
}

// selectFor replaces a machine's selected items with exactly these skills.
func (h *syncContractHarness) selectFor(m *syncContractMachine, names ...string) {
	h.t.Helper()
	ids := []string{}
	for _, name := range names {
		ids = append(ids, h.itemID(name))
	}
	target := store.EnvironmentIDFor(h.uid, m.name, "agents", "")
	// A later save names the version it replaced, as the web UI does.
	current, _ := h.st.LibrarySelectionFor(h.uid, target) // none yet on the first save
	h.mustRequest(http.MethodPut, "/api/v2/library/selections", store.LibrarySelection{TargetID: target, ItemIDs: ids, Version: current.Version})
}

func (m *syncContractMachine) present(name string) bool {
	_, err := os.Stat(m.path(name, "SKILL.md"))
	return err == nil
}

func (m *syncContractMachine) backupsOf(name string) []string {
	matches, _ := filepath.Glob(filepath.Join(m.home, ".agents", ".reet-sync-backups", "removed-*", syncContractName(name)))
	return matches
}

func (h *syncContractHarness) notifications() []store.Notification {
	h.t.Helper()
	var response struct{ Notifications []store.Notification }
	if err := json.Unmarshal([]byte(h.mustRequest(http.MethodGet, "/api/v2/notifications", nil)), &response); err != nil {
		h.t.Fatal(err)
	}
	return response.Notifications
}

func (h *syncContractHarness) requireRetained(name string) {
	h.t.Helper()
	item, err := h.st.LibraryItemFor(h.uid, h.itemID(name))
	if err != nil || item.ID == "" {
		h.t.Fatalf("the library entry for %s was lost: %v", name, err)
	}
}

func stage4Fleet(t *testing.T, name, text string, machines ...string) (*syncContractHarness, store.SyncSkill, []*syncContractMachine) {
	t.Helper()
	h := newSyncContractHarness(t)
	h.enableManagedRemovals()
	skill := h.hosted(name, text, false, true)
	ms := []*syncContractMachine{}
	for _, machine := range machines {
		ms = append(ms, h.machine(machine))
	}
	h.rounds(1, ms...)
	for _, m := range ms {
		h.selectFor(m, name)
	}
	h.rounds(5, ms...)
	for _, m := range ms {
		m.requireFile(name, "SKILL.md", syncContractContent(name, text))
	}
	return h, skill, ms
}

// A05 + A06: leaving In use removes online machines at once and an offline
// machine when it returns, keeping the library entry, history and selections;
// returning to In use restores the latest revision to the retained selections.
func TestStage4OfflineMachineRemovesOnReturnAndReactivationRestores(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "fleet", "first revision", "fleet-a", "fleet-b")
	a, b := ms[0], ms[1]
	b.link.offline.Store(true)
	h.used(skill.ID, false)
	h.rounds(8, a)
	if a.present("fleet") {
		t.Fatal("an online machine kept an inactive managed copy")
	}
	if !b.present("fleet") {
		t.Fatal("an offline machine changed before it reconnected")
	}
	b.link.offline.Store(false)
	h.rounds(8, a, b)
	if b.present("fleet") {
		t.Fatal("the returning machine did not apply the current intent")
	}
	if len(a.backupsOf("fleet")) != 1 || len(b.backupsOf("fleet")) != 1 {
		t.Fatal("removed copies were not kept recoverable", a.backupsOf("fleet"), b.backupsOf("fleet"))
	}
	h.requireRetained("fleet")
	for _, m := range ms {
		selection, err := h.st.LibrarySelectionFor(h.uid, store.EnvironmentIDFor(h.uid, m.name, "agents", ""))
		if err != nil || len(selection.ItemIDs) != 1 {
			t.Fatalf("deactivation changed %s's selection: %+v %v", m.name, selection, err)
		}
	}
	if len(h.notifications()) != 0 {
		t.Fatal("an authorised removal produced a deletion notification", h.notifications())
	}

	h.used(skill.ID, true)
	h.rounds(10, a, b)
	for _, m := range ms {
		m.requireFile("fleet", "SKILL.md", syncContractContent("fleet", "first revision"))
	}
	if len(h.notifications()) != 0 {
		t.Fatal("reactivation produced a deletion notification", h.notifications())
	}
}

// A07: deselecting one machine removes only that machine's managed copy.
func TestStage4DeselectingOneMachineLeavesTheOthersCurrent(t *testing.T) {
	h, _, ms := stage4Fleet(t, "pair", "shared revision", "pair-a", "pair-b")
	a, b := ms[0], ms[1]
	h.selectFor(b)
	h.rounds(8, a, b)
	if b.present("pair") {
		t.Fatal("a deselected machine kept its managed copy")
	}
	a.requireFile("pair", "SKILL.md", syncContractContent("pair", "shared revision"))
	h.requireRetained("pair")
	if len(h.notifications()) != 0 {
		t.Fatal("deselecting produced a deletion notification", h.notifications())
	}
}

// A09: an editor's atomic save of identical bytes is not a deletion, an edit,
// a revision or a notification.
func TestStage4AtomicSaveIsNotADeletionOrAnEdit(t *testing.T) {
	h, _, ms := stage4Fleet(t, "atomic", "stable bytes", "atomic-a")
	m := ms[0]
	before, err := h.st.LibraryItemFor(h.uid, h.itemID("atomic"))
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		path := m.path("atomic", "SKILL.md")
		if err := os.WriteFile(path+".tmp", []byte(syncContractContent("atomic", "stable bytes")), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
		h.rounds(3, m)
	}
	after, err := h.st.LibraryItemFor(h.uid, h.itemID("atomic"))
	if err != nil || after.RevisionID != before.RevisionID {
		t.Fatalf("an atomic save of identical bytes created a revision: %v", err)
	}
	m.requireFile("atomic", "SKILL.md", syncContractContent("atomic", "stable bytes"))
	if len(h.notifications()) != 0 {
		t.Fatal("an atomic save produced a notification", h.notifications())
	}
}

// A11: a removal that is pending when both the controller and the connector
// restart still converges, with the same enrollment and no replay of the file work.
func TestStage4RemovalSurvivesControllerAndConnectorRestart(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "restart", "durable intent", "restart-a")
	m := ms[0]
	before, err := os.ReadFile(m.config)
	if err != nil {
		t.Fatal(err)
	}
	h.used(skill.ID, false)
	h.restartController()
	m.stop()
	m.start()
	h.rounds(10, m)
	if m.present("restart") {
		t.Fatal("restart lost the removal intent")
	}
	after, err := os.ReadFile(m.config)
	if err != nil || string(before) != string(after) {
		t.Fatalf("restart changed the enrollment: %v", err)
	}
	if len(m.backupsOf("restart")) != 1 {
		t.Fatal("restart duplicated or lost the recoverable copy", m.backupsOf("restart"))
	}
	h.requireRetained("restart")
}

// A13: one item whose content cannot be read completely is retained and
// explained; the healthy item beside it is still removed.
func TestStage4UnreadableItemDoesNotBlockHealthyRemoval(t *testing.T) {
	h := newSyncContractHarness(t)
	h.enableManagedRemovals()
	bad := h.hosted("bad-item", "unreadable soon", false, true)
	good := h.hosted("good-item", "healthy", false, true)
	m := h.machine("unreadable-a")
	h.rounds(1, m)
	h.selectFor(m, "bad-item", "good-item")
	h.rounds(5, m)
	if !m.present("bad-item") || !m.present("good-item") {
		t.Fatal("fixture did not install both items")
	}
	if err := os.Symlink("/etc/hostname", m.path("bad-item", "link")); err != nil {
		t.Fatal(err)
	}
	h.used(bad.ID, false)
	h.used(good.ID, false)
	h.rounds(10, m)
	if m.present("good-item") {
		t.Fatal("a healthy removal waited behind an unrelated unreadable item")
	}
	if !m.present("bad-item") {
		t.Fatal("an unreadable item's files were removed")
	}
	if _, err := os.Lstat(m.path("bad-item", "link")); err != nil {
		t.Fatal("foreign content inside the retained item changed", err)
	}
}

// A15: if the item becomes wanted again after a removal was planned and
// approved but before it ran, the stale removal must not take effect.
func TestStage4ReactivationBeforeTheRemovalRunsKeepsTheCopy(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "race", "stay put", "race-a")
	m := ms[0]
	h.used(skill.ID, false)
	reactivated := false
	for range 12 {
		if _, err := m.tick(); err != nil {
			t.Fatal(err)
		}
		jobs, err := h.st.SyncJobsFor(h.uid)
		if err != nil {
			t.Fatal(err)
		}
		for _, job := range jobs {
			if job.Kind == "apply" && job.Status == "queued" && strings.Contains(string(job.Plan), `"removalIntent"`) {
				h.used(skill.ID, true)
				reactivated = true
			}
		}
		if reactivated {
			break
		}
	}
	if !reactivated {
		t.Fatal("the fixture never saw an approved removal waiting to run")
	}
	h.rounds(10, m)
	m.requireFile("race", "SKILL.md", syncContractContent("race", "stay put"))
	if len(m.backupsOf("race")) != 0 {
		t.Fatal("a stale approved removal moved the copy", m.backupsOf("race"))
	}
	jobs, _ := h.st.SyncJobsFor(h.uid)
	for _, job := range jobs {
		if strings.Contains(string(job.Plan), `"removalIntent"`) && job.Status == "succeeded" {
			t.Fatalf("a superseded removal was acknowledged as complete: %+v", job)
		}
	}
}

// A16: an unseen local edit is preserved before intentional removal, and an
// unmanaged skill beside the managed one is never touched.
func TestStage4RemovalPreservesUnseenEditsAndForeignSkills(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "keeper", "original bytes", "keeper-a")
	m := ms[0]
	m.localSkill("foreign", "a skill reet never installed")
	edit := syncContractContent("keeper", "unseen local edit")
	if err := os.WriteFile(m.path("keeper", "SKILL.md"), []byte(edit), 0644); err != nil {
		t.Fatal(err)
	}
	h.used(skill.ID, false)
	h.rounds(10, m)
	if m.present("keeper") {
		t.Fatal("the managed copy was not removed")
	}
	if !m.present("foreign") {
		t.Fatal("an unmanaged skill beside the managed one was removed")
	}
	backups := m.backupsOf("keeper")
	if len(backups) != 1 {
		t.Fatal("no recoverable copy of the edited item", backups)
	}
	got, err := os.ReadFile(filepath.Join(backups[0], "SKILL.md"))
	if err != nil || string(got) != edit {
		t.Fatalf("the unseen edit was not preserved: %q %v", got, err)
	}
}

// A19: an older connector (no library protocol) is never sent removal
// semantics, even when the controller has the capability on, and is not rolled
// back onto the broad whole-library policy as a fallback.
func TestStage4OlderConnectorKeepsAdditiveBehaviour(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	h.enableManagedRemovals()
	skill := h.hosted("legacy", "old connector", false, true)
	m := h.machine("legacy-a")
	h.rounds(5, m)
	m.requireFile("legacy", "SKILL.md", syncContractContent("legacy", "old connector"))
	h.used(skill.ID, false)
	h.rounds(8, m)
	m.requireFile("legacy", "SKILL.md", syncContractContent("legacy", "old connector"))
	if len(m.backupsOf("legacy")) != 0 {
		t.Fatal("an older connector was made to remove a copy")
	}
}

// A20: a revoked connector cannot act and keeps its local state; the other
// machine converges and its enrollment is unaffected.
func TestStage4RevokedConnectorCannotRemoveAndDoesNotBlockOthers(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "revoked", "shared", "revoked-a", "revoked-b")
	a, b := ms[0], ms[1]
	before, err := os.ReadFile(b.config)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.st.RevokeAPIToken(h.uid, b.tokenID); err != nil {
		t.Fatal(err)
	}
	h.used(skill.ID, false)
	h.rounds(8, a)
	if a.present("revoked") {
		t.Fatal("the healthy machine did not converge")
	}
	if _, err = b.tick(); err == nil {
		t.Fatal("a revoked connector kept controller access")
	}
	if !b.present("revoked") {
		t.Fatal("a revoked connector changed local files")
	}
	after, err := os.ReadFile(b.config)
	if err != nil || string(before) != string(after) {
		t.Fatalf("revocation changed the local enrollment: %v", err)
	}
	agents, err := h.st.SyncAgentsFor(h.uid)
	if err != nil || len(agents) != 2 {
		t.Fatalf("revocation deleted an enrollment: %d %v", len(agents), err)
	}
}

func restoreNotices(h *syncContractHarness, name string) int {
	count := 0
	for _, notice := range h.notifications() {
		if notice.Kind == "library_restore" && strings.Contains(notice.Title, syncContractName(name)) {
			count++
		}
	}
	return count
}

// A08 (support file, then whole folder): each distinct deletion is restored
// from canonical bytes and announced exactly once, however many cycles follow.
func TestStage4EachDeletionEpisodeIsRestoredAndAnnouncedOnce(t *testing.T) {
	h, _, ms := stage4Fleet(t, "episodes", "canonical", "episodes-a")
	m := ms[0]
	if err := os.Remove(m.path("episodes", "reference.txt")); err != nil {
		t.Fatal(err)
	}
	h.rounds(2, m)
	time.Sleep(450 * time.Millisecond)
	h.rounds(8, m)
	got, err := os.ReadFile(m.path("episodes", "reference.txt"))
	if err != nil || string(got) != "supporting fixture bytes\n" {
		t.Fatalf("the deleted support file was not restored from canonical bytes: %q %v", got, err)
	}
	m.requireFile("episodes", "SKILL.md", syncContractContent("episodes", "canonical"))
	if n := restoreNotices(h, "episodes"); n != 1 {
		t.Fatalf("want one notice for the first deletion, got %d", n)
	}

	if err := os.RemoveAll(filepath.Dir(m.path("episodes", "SKILL.md"))); err != nil {
		t.Fatal(err)
	}
	h.rounds(2, m)
	time.Sleep(450 * time.Millisecond)
	h.rounds(8, m)
	m.requireFile("episodes", "SKILL.md", syncContractContent("episodes", "canonical"))
	if n := restoreNotices(h, "episodes"); n != 2 {
		t.Fatalf("want one notice per deletion episode, got %d", n)
	}
	h.rounds(10, m)
	if n := restoreNotices(h, "episodes"); n != 2 {
		t.Fatalf("later cycles repeated a notification: %d", n)
	}
}

// A deletion followed at once by deactivation is an authorised removal, not an
// accident: the folder is not restored and nobody is told it was.
func TestStage4DeletionThenDeactivationIsNeitherRestoredNorAnnounced(t *testing.T) {
	h, skill, ms := stage4Fleet(t, "doomed", "canonical", "doomed-a")
	m := ms[0]
	if err := os.RemoveAll(filepath.Dir(m.path("doomed", "SKILL.md"))); err != nil {
		t.Fatal(err)
	}
	h.used(skill.ID, false)
	time.Sleep(450 * time.Millisecond)
	h.rounds(10, m)
	if m.present("doomed") {
		t.Fatal("an item that left In use was restored after a local deletion")
	}
	if n := restoreNotices(h, "doomed"); n != 0 {
		t.Fatalf("an authorised removal was announced as an accidental deletion: %d", n)
	}
	h.requireRetained("doomed")
}

// A19, whole path: an account that was synced by an older connector (whole
// library, additive) is upgraded to the library protocol and then rolled back.
// Ownership is only ever claimed from a recorded install, and an older connector
// records none, so after the upgrade nothing the old sync wrote is removed,
// replaced or duplicated, whether or not it is still selected, and a skill the
// person made themselves is untouched. Rolling back makes no removals either.
func TestStage4LegacyAccountUpgradeKeepsEverythingTheOldSyncWroteAndRollbackRemovesNothing(t *testing.T) {
	h := newLegacySyncContractHarness(t)
	alpha := h.hosted("alpha", "legacy alpha", false, true)
	h.hosted("beta", "legacy beta", false, true)
	m := h.machine("legacy-upgrade")
	m.localSkill("mine", "written by the person, never by reet")
	h.rounds(6, m)
	for _, name := range []string{"alpha", "beta"} {
		m.requireFile(name, "SKILL.md", syncContractContent(name, "legacy "+name))
	}

	// Upgrade: the controller offers the capability and the connector speaks the library protocol.
	h.enableManagedRemovals()
	h.legacyProtocol = false
	h.rounds(2, m)
	h.selectFor(m, "alpha")
	h.rounds(15, m)
	m.requireFile("alpha", "SKILL.md", syncContractContent("alpha", "legacy alpha"))
	if len(m.backupsOf("alpha")) != 0 {
		t.Fatal("the selected legacy copy was replaced or moved aside by the upgrade")
	}
	// The old sync left no record that proves reet installed beta, so it is not reet's to remove.
	m.requireFile("beta", "SKILL.md", syncContractContent("beta", "legacy beta"))
	if len(m.backupsOf("beta")) != 0 {
		t.Fatal("a copy with no recorded install was moved aside")
	}
	h.requireRetained("beta")
	if !m.present("mine") {
		t.Fatal("a skill the person made themselves was touched by the upgrade")
	}

	// Rollback: the older protocol again. Nothing is removed, whatever the library says.
	h.legacyProtocol = true
	h.used(alpha.ID, false)
	h.rounds(8, m)
	m.requireFile("alpha", "SKILL.md", syncContractContent("alpha", "legacy alpha"))
	if !m.present("mine") || !m.present("beta") || len(m.backupsOf("alpha")) != 0 {
		t.Fatal("rolling back to the older protocol removed something")
	}
}

// A14 (destination collision): an unmanaged folder that already has the name of
// a selected item is not overwritten, the collision is explained, and the
// healthy item beside it still installs.
func TestStage4UnmanagedFolderWithTheSameNameIsKeptAndExplained(t *testing.T) {
	h := newSyncContractHarness(t)
	h.enableManagedRemovals()
	h.hosted("clash", "library version", false, true)
	h.hosted("fine", "healthy", false, true)
	m := h.machine("clash-a")
	h.rounds(1, m)
	m.localSkill("clash", "the person's own, unrelated skill with the same name")
	h.selectFor(m, "clash", "fine")
	h.rounds(10, m)
	m.requireFile("fine", "SKILL.md", syncContractContent("fine", "healthy"))
	m.requireFile("clash", "SKILL.md", syncContractContent("clash", "the person's own, unrelated skill with the same name"))
	if len(m.backupsOf("clash")) != 0 {
		t.Fatal("an unrelated folder was moved aside")
	}
	state := h.mustRequest(http.MethodGet, "/api/v2/sync", nil)
	if !strings.Contains(state, `"`+syncContractName("clash")+`":"Local content differs; import this folder to preserve it before syncing this item."`) {
		t.Fatal("the collision was not explained to the person")
	}
}
