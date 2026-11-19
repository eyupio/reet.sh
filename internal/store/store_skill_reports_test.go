package store

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var reportT0 = time.UnixMilli(1_800_000_000_000)

// reportAgent enrolls a protocol-2 machine last seen at reportT0.
func reportAgent(t *testing.T, s *Store, uid int64, host string) string {
	t.Helper()
	a, _, err := s.EnrollSyncAgentBuild(uid, host, "1.0", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, a.ID, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE sync_agents SET last_seen=? WHERE id=?`, reportT0.UnixMilli(), a.ID); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

func reportOf(t *testing.T, s *Store, uid int64, agent string, at time.Time, entries ...SkillReportEntry) {
	t.Helper()
	if err := s.RecordSkillReportFor(uid, agent, SkillReport{Version: "v1", Entries: entries}, at); err != nil {
		t.Fatal(err)
	}
}

func installedAt(name, digest string) SkillReportEntry {
	return SkillReportEntry{Folder: "~/.agents/skills", Name: name, State: "installed", Digest: digest}
}

func healthOf(t *testing.T, s *Store, uid int64, id string, at time.Time) SkillHealth {
	t.Helper()
	m, err := s.SkillHealthFor(uid, at)
	if err != nil {
		t.Fatal(err)
	}
	h, ok := m[id]
	if !ok {
		t.Fatalf("no health for %s in %+v", id, m)
	}
	return h
}

func itemDigest(t *testing.T, s *Store, uid int64, id string) string {
	t.Helper()
	var d string
	if err := s.db.QueryRow(`SELECT digest FROM library_items WHERE user_id=? AND id=?`, uid, id).Scan(&d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestReportHealthAllInstalled(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	d := itemDigest(t, s, uid, item.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", d))
	reportOf(t, s, uid, b, reportT0, installedAt("SEO-Audit", d))
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "ok" || h.Total != 2 || h.Pending != 0 || len(h.Hosts) != 0 {
		t.Fatalf("health: %+v", h)
	}
	if len(h.Machines) != 2 {
		t.Fatalf("machines: %+v", h.Machines)
	}
}

func TestReportHealthIgnoresPausedAndOldAgents(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "off" || h.Total != 0 {
		t.Fatalf("no machines: %+v", h)
	}
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	if err := s.ControlSyncAgent(uid, b, "pause"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.EnrollSyncAgentBuild(uid, "studio", "0.9", ""); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", itemDigest(t, s, uid, item.id)))
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "ok" || h.Total != 1 {
		t.Fatalf("paused and old machines counted: %+v", h)
	}
}

func TestReportHealthInstallingAndFailed(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	d := itemDigest(t, s, uid, item.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", d))
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "installing" || h.Pending != 1 || h.Total != 2 {
		t.Fatalf("one missing: %+v", h)
	}
	reportOf(t, s, uid, b, reportT0, installedAt("seo-audit", "old-digest"))
	if h = healthOf(t, s, uid, item.id, reportT0); h.State != "installing" || h.Pending != 1 {
		t.Fatalf("installed at another digest: %+v", h)
	}
	reportOf(t, s, uid, b, reportT0, SkillReportEntry{Folder: "~/.agents/skills", Name: "seo-audit", State: "failed", Detail: "no write access"})
	h = healthOf(t, s, uid, item.id, reportT0)
	if h.State != "failed" || !reflect.DeepEqual(h.Hosts, []string{"thinkpad"}) || h.Reason != "no write access" {
		t.Fatalf("failed: %+v", h)
	}
}

func TestReportHealthInsideGraceShowsNoPendingWork(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", itemDigest(t, s, uid, item.id)))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	if h := healthOf(t, s, uid, item.id, reportT0.Add(3*time.Second)); h.State != "ok" || h.Pending != 0 {
		t.Fatalf("inside grace: %+v", h)
	}
	if h := healthOf(t, s, uid, item.id, reportT0.Add(SkillGrace)); h.State != "removing" || h.Pending != 1 {
		t.Fatalf("after grace: %+v", h)
	}
}

func TestReportHealthRemovingAndOffline(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	d := itemDigest(t, s, uid, item.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", d))
	reportOf(t, s, uid, b, reportT0, installedAt("seo-audit", d))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	later := reportT0.Add(time.Minute)
	if _, err := s.db.Exec(`UPDATE sync_agents SET last_seen=? WHERE id=?`, later.UnixMilli(), a); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, b, reportT0, SkillReportEntry{Folder: "~/.agents/skills", Name: "seo-audit", State: "removed"})
	h := healthOf(t, s, uid, item.id, later)
	if h.State != "removing" || h.Pending != 1 || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("removing: %+v", h)
	}
	h = healthOf(t, s, uid, item.id, later.Add(10*time.Minute))
	if h.State != "offline" || h.Pending != 1 || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("offline: %+v", h)
	}
}

func TestReportBlockedByAllowApply(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	a := reportAgent(t, s, uid, "elitebook")
	if err := s.RecordSkillReportFor(uid, a, SkillReport{Version: "v1", Blocked: "allow-apply"}, reportT0); err != nil {
		t.Fatal(err)
	}
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "blocked" || h.Pending != 1 || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("blocked: %+v", h)
	}
	reportOf(t, s, uid, a, reportT0.Add(time.Second), installedAt("seo-audit", itemDigest(t, s, uid, item.id)))
	if h = healthOf(t, s, uid, item.id, reportT0.Add(time.Second)); h.State != "ok" {
		t.Fatalf("a later report without the block: %+v", h)
	}
}

func TestReportContentStates(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	add := func(id, name, origin, problem string) {
		t.Helper()
		if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,copy_problem,created_at,updated_at) VALUES(?,?,?,?,?,?,?,1,1)`, uid, id, LibrarySkill, name, SkillActive, origin, problem); err != nil {
			t.Fatal(err)
		}
	}
	add("fetching", "fetching", "imported", "")
	add("copying", "copying", "machine", "")
	add("fetch-bad", "fetch-bad", "imported", "404 from github.com")
	add("copy-bad", "copy-bad", "machine", "folder is over 8 MiB")
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, SkillReportEntry{Folder: "~/.agents/skills", Name: "copying", State: "unknown", Digest: "d1"})
	m, err := s.SkillHealthFor(uid, reportT0)
	if err != nil {
		t.Fatal(err)
	}
	if h := m["fetching"]; h.State != "getting" || len(h.Hosts) != 0 {
		t.Fatalf("getting: %+v", h)
	}
	if h := m["copying"]; h.State != "copying" || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("copying: %+v", h)
	}
	if h := m["fetch-bad"]; h.State != "fetch-failed" || h.Reason != "404 from github.com" {
		t.Fatalf("fetch-failed: %+v", h)
	}
	if h := m["copy-bad"]; h.State != "copy-failed" || h.Reason != "folder is over 8 MiB" {
		t.Fatalf("copy-failed: %+v", h)
	}
}

func TestReportUnknownBecomesNew(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	unknown := func(name, digest string) SkillReportEntry {
		return SkillReportEntry{Folder: "~/.claude/skills", Name: name, State: "unknown", Digest: digest}
	}
	archived := savedWrittenSkill(t, s, uid, "old-one", false)
	if err := s.SetSkillStatusFor(uid, []string{archived.id}, SkillArchived, reportT0.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	savedWrittenSkill(t, s, uid, "live-one", false)
	reportOf(t, s, uid, a, reportT0, unknown("cq", "d1"), unknown("old-one", "x"), unknown("live-one", "y"))
	reportOf(t, s, uid, b, reportT0, unknown("CQ", "d2"))
	got, err := s.NewOnMachinesFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || strings.ToLower(got[0].Name) != "cq" || !got[0].DigestsDiffer || len(got[0].Copies) != 2 {
		t.Fatalf("new: %+v", got)
	}
	if got[0].Copies[0].Host != "elitebook" || got[0].Copies[0].AgentID != a || got[0].Copies[0].Folder != "~/.claude/skills" || got[0].Copies[0].Digest != "d1" || got[0].Copies[0].SeenAt != reportT0.UnixMilli() {
		t.Fatalf("copy: %+v", got[0].Copies[0])
	}
	reportOf(t, s, uid, b, reportT0, unknown("cq", "d1"))
	if got, _ = s.NewOnMachinesFor(uid); len(got) != 1 || got[0].DigestsDiffer {
		t.Fatalf("same digests: %+v", got)
	}
}

func TestReportConfirmsDeletionAndForgets(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	d := itemDigest(t, s, uid, item.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("demo", d))
	reportOf(t, s, uid, b, reportT0, installedAt("demo", d))
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, reportT0); err != nil {
		t.Fatal(err)
	}
	gone := SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "removed"}
	at := reportT0.Add(time.Minute)
	reportOf(t, s, uid, a, at, gone)
	if r := skillRemovals(t, s, uid); r["demo"].Reason != "deleted" {
		t.Fatalf("one machine confirmed, the name must stay: %+v", r)
	}
	reportOf(t, s, uid, b, at)
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("both confirmed: %+v", r)
	}
	unknown := SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "unknown", Digest: "z"}
	reportOf(t, s, uid, a, at, unknown)
	if got, _ := s.NewOnMachinesFor(uid); len(got) != 1 || got[0].Name != "demo" {
		t.Fatalf("a later unknown is new: %+v", got)
	}
}

func TestReportForgetsAfterThirtyDays(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, installedAt("demo", itemDigest(t, s, uid, item.id)))
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, reportT0); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, a, reportT0.Add(29*24*time.Hour), installedAt("demo", "x"))
	if r := skillRemovals(t, s, uid); len(r) != 1 {
		t.Fatalf("29 days: %+v", r)
	}
	reportOf(t, s, uid, a, reportT0.Add(31*24*time.Hour), installedAt("demo", "x"))
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("31 days: %+v", r)
	}
}

func TestReportArchivedNameIsNeverForgotten(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, a, reportT0.Add(40*24*time.Hour))
	if r := skillRemovals(t, s, uid); r["demo"].Reason != "archived" {
		t.Fatalf("archived name forgotten: %+v", r)
	}
}

func activityOf(t *testing.T, s *Store, uid int64) []SkillActivity {
	t.Helper()
	got, err := s.SkillActivityFor(uid, 50)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestReportArchivedReappearingIsRemovedAgain(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	d := itemDigest(t, s, uid, item.id)
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, installedAt("demo", d))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, a, reportT0.Add(time.Minute), SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "removed"})
	reportOf(t, s, uid, a, reportT0.Add(2*time.Minute), installedAt("demo", d))
	got := activityOf(t, s, uid)
	if len(got) == 0 || got[0].Action != "removed again" || got[0].Host != "elitebook" || got[0].Name != "demo" || got[0].At != reportT0.Add(2*time.Minute).UnixMilli() {
		t.Fatalf("activity: %+v", got)
	}
	reportOf(t, s, uid, a, reportT0.Add(3*time.Minute), installedAt("demo", d))
	if again := activityOf(t, s, uid); len(again) != len(got) {
		t.Fatalf("an unchanged report added activity: %+v", again)
	}
}

func TestReportEditedRecordsBackup(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, SkillReportEntry{Folder: "~/.claude/skills", Name: "demo", State: "edited", Digest: itemDigest(t, s, uid, item.id), Backup: "~/.claude/skills.backups/2027-01-15-demo"})
	var backup string
	if err := s.db.QueryRow(`SELECT backup FROM machine_skills WHERE user_id=? AND agent_id=?`, uid, a).Scan(&backup); err != nil || backup != "~/.claude/skills.backups/2027-01-15-demo" {
		t.Fatalf("backup %q, %v", backup, err)
	}
	got := activityOf(t, s, uid)
	if len(got) != 1 || got[0].Action != "edited" || got[0].Detail != "edited on elitebook; reet's copy put back" {
		t.Fatalf("activity: %+v", got)
	}
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "ok" {
		t.Fatalf("reet's copy was put back: %+v", h)
	}
}

func TestReportActivityKeepsNewest500(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	for i := 0; i < 520; i++ {
		d := "a"
		if i%2 == 1 {
			d = "b"
		}
		reportOf(t, s, uid, a, reportT0.Add(time.Duration(i)*time.Second), SkillReportEntry{Folder: "f", Name: "x", State: "unknown", Digest: d})
	}
	var n int
	if err := s.db.QueryRow(`SELECT count(*) FROM skill_activity WHERE user_id=?`, uid).Scan(&n); err != nil || n != 500 {
		t.Fatalf("rows %d, %v", n, err)
	}
	got, _ := s.SkillActivityFor(uid, 10)
	if len(got) != 10 || got[0].At != reportT0.Add(519*time.Second).UnixMilli() {
		t.Fatalf("newest first: %+v", got)
	}
}

func TestReportDuplicateNamesInOneFolderWriteOneActivityRow(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	// Names that differ only by case are one row in the table, so they are one
	// entry: the last one in the report is kept.
	reportOf(t, s, uid, a, reportT0,
		SkillReportEntry{Folder: "~/.agents/skills", Name: "Demo", State: "installed", Digest: "d1"},
		SkillReportEntry{Folder: "~/.agents/skills", Name: "demo", State: "failed", Detail: "no write access"},
		SkillReportEntry{Folder: "~/.claude/skills", Name: "demo", State: "installed", Digest: "d1"})
	activity, err := s.SkillActivityFor(uid, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 2 {
		t.Fatalf("want one row per folder and name, got %+v", activity)
	}
	var rows int
	if err = s.db.QueryRow(`SELECT count(*) FROM machine_skills WHERE user_id=? AND agent_id=? AND folder='~/.agents/skills'`, uid, a).Scan(&rows); err != nil || rows != 1 {
		t.Fatal("the table keeps one row", rows, err)
	}
	var state string
	if err = s.db.QueryRow(`SELECT state FROM machine_skills WHERE user_id=? AND agent_id=? AND folder='~/.agents/skills'`, uid, a).Scan(&state); err != nil || state != "failed" {
		t.Fatalf("the last entry is the one kept: %s %v", state, err)
	}
}

func TestCleanReportTextTruncatesOnARuneBoundary(t *testing.T) {
	got := cleanReportText("a"+strings.Repeat("é", 10), 4)
	if !utf8.ValidString(got) || len(got) > 4 || got != "aé" {
		t.Fatalf("%q", got)
	}
	if got = cleanReportText("  short  ", 20); got != "short" {
		t.Fatalf("%q", got)
	}
	if got = cleanReportText(strings.Repeat("日", 5), 9); got != strings.Repeat("日", 3) {
		t.Fatalf("%q", got)
	}
}

func TestReportNewOnMachinesListsOldestSightingFirst(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	late, early := reportAgent(t, s, uid, "alpha"), reportAgent(t, s, uid, "zeta")
	unknown := SkillReportEntry{Folder: "~/.claude/skills", Name: "cq", State: "unknown", Digest: "d1"}
	reportOf(t, s, uid, early, reportT0, unknown)
	reportOf(t, s, uid, late, reportT0.Add(time.Minute), unknown)
	got, err := s.NewOnMachinesFor(uid)
	if err != nil || len(got) != 1 || len(got[0].Copies) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	if got[0].Copies[0].Host != "zeta" || got[0].Copies[1].Host != "alpha" {
		t.Fatalf("not oldest first: %+v", got[0].Copies)
	}
}

func TestReportCopyingNamesOnlyCountedMachines(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,1,1)`, uid, "copying", LibrarySkill, "copying", SkillActive, "machine"); err != nil {
		t.Fatal(err)
	}
	counted, paused := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	old, _, err := s.EnrollSyncAgentBuild(uid, "studio", "0.9", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ControlSyncAgent(uid, paused, "pause"); err != nil {
		t.Fatal(err)
	}
	unknown := SkillReportEntry{Folder: "~/.agents/skills", Name: "copying", State: "unknown", Digest: "d1"}
	for _, id := range []string{counted, paused, old.ID} {
		reportOf(t, s, uid, id, reportT0, unknown)
	}
	h := healthOf(t, s, uid, "copying", reportT0)
	if h.State != "copying" || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("copying: %+v", h)
	}
}

func TestReportAgentUnseenForExactlyFiveMinutesIsNotOffline(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	reportAgent(t, s, uid, "elitebook") // last seen at reportT0, never reported
	if h := healthOf(t, s, uid, item.id, reportT0.Add(skillOfflineAfter)); h.State != "installing" {
		t.Fatalf("five minutes exactly: %+v", h)
	}
	if h := healthOf(t, s, uid, item.id, reportT0.Add(skillOfflineAfter+time.Millisecond)); h.State != "offline" {
		t.Fatalf("a millisecond past five minutes: %+v", h)
	}
}

func TestReportNameInstalledInOneFolderAndFailedInAnotherReadsFailed(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", itemDigest(t, s, uid, item.id)),
		SkillReportEntry{Folder: "~/.claude/skills", Name: "seo-audit", State: "failed", Detail: "read-only folder"})
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "failed" || h.Reason != "read-only folder" || h.Pending != 1 {
		t.Fatalf("failed: %+v", h)
	}
}

func TestReportArchivedItemWithNoMachinesIsOff(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	if h := healthOf(t, s, uid, item.id, reportT0.Add(time.Minute)); h.State != "off" || h.Total != 0 || h.Pending != 0 {
		t.Fatalf("archived, no machines: %+v", h)
	}
}

// A machine that never reported has no rows, so it has nothing reet knows to
// remove; an install it still owes reads installing, then offline when unseen.
func TestReportMachineThatNeverReportedHasNothingToRemove(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	reportAgent(t, s, uid, "elitebook")
	if h := healthOf(t, s, uid, item.id, reportT0); h.State != "installing" || h.Pending != 1 {
		t.Fatalf("never reported, active: %+v", h)
	}
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	if h := healthOf(t, s, uid, item.id, reportT0.Add(time.Minute)); h.State != "off" || h.Pending != 0 || h.Total != 1 {
		t.Fatalf("never reported, archived: %+v", h)
	}
}

// An Archived skill every machine has taken off is off, not "on all N
// machines".
func TestReportArchivedItemOffEveryMachineIsOff(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	d := itemDigest(t, s, uid, item.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("seo-audit", d))
	reportOf(t, s, uid, b, reportT0, installedAt("seo-audit", d))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	removed := SkillReportEntry{Folder: "~/.agents/skills", Name: "seo-audit", State: "removed"}
	later := reportT0.Add(time.Minute)
	reportOf(t, s, uid, a, later, removed)
	reportOf(t, s, uid, b, later, removed)
	if h := healthOf(t, s, uid, item.id, later); h.State != "off" || h.Pending != 0 || h.Total != 2 {
		t.Fatalf("archived, off every machine: %+v", h)
	}
}

// An Archived imported skill reet has no copy of yet, still on a machine and
// not on the removal list, is waiting on its fetch: getting, not ok or off.
func TestReportArchivedImportedSkillWaitingOnItsFetchIsGetting(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if _, err := s.db.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,origin,created_at,updated_at) VALUES(?,?,?,?,?,?,1,1)`, uid, "imp", LibrarySkill, "fetched", SkillArchived, "imported"); err != nil {
		t.Fatal(err)
	}
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, SkillReportEntry{Folder: "~/.agents/skills", Name: "fetched", State: "unknown", Digest: "d1"})
	if h := healthOf(t, s, uid, "imp", reportT0); h.State != "getting" {
		t.Fatalf("archived imported, waiting on its fetch: %+v", h)
	}
}

func TestReportBlockedOutranksRemoving(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	d := itemDigest(t, s, uid, item.id)
	blocked, other := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	if err := s.RecordSkillReportFor(uid, blocked, SkillReport{Version: "v1", Blocked: "allow-apply", Entries: []SkillReportEntry{installedAt("seo-audit", d)}}, reportT0); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, other, reportT0, installedAt("seo-audit", d))
	if err := s.SetSkillStatusFor(uid, []string{item.id}, SkillArchived, reportT0); err != nil {
		t.Fatal(err)
	}
	h := healthOf(t, s, uid, item.id, reportT0.Add(time.Minute))
	if h.State != "blocked" || h.Pending != 2 || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("blocked and removing: %+v", h)
	}
	states := map[string]string{}
	for _, m := range h.Machines {
		states[m.Host] = m.State
	}
	if states["elitebook"] != "blocked" || states["thinkpad"] != "removing" {
		t.Fatalf("machines: %+v", h.Machines)
	}
}

func columnsOf(t *testing.T, s *Store, table string) map[string]bool {
	t.Helper()
	rows, err := s.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		out[name] = true
	}
	return out
}

// A report's version lives on machine_skill_reports only, and nothing keeps a
// report time there: the rows of machine_skills carry the time of the sighting.
func TestReportTablesKeepNoUnusedColumns(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	check := func() {
		t.Helper()
		if c := columnsOf(t, s, "machine_skills"); c["version"] || !c["at"] {
			t.Fatalf("machine_skills columns: %v", c)
		}
		if c := columnsOf(t, s, "machine_skill_reports"); c["at"] || !c["version"] || !c["blocked"] {
			t.Fatalf("machine_skill_reports columns: %v", c)
		}
	}
	check()
	// The tables as earlier builds made them are brought to the same shape.
	for _, stmt := range []string{
		`DROP TABLE machine_skills`, `DROP TABLE machine_skill_reports`,
		`CREATE TABLE machine_skills(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,folder TEXT NOT NULL,name TEXT NOT NULL COLLATE NOCASE,state TEXT NOT NULL,digest TEXT NOT NULL DEFAULT '',detail TEXT NOT NULL DEFAULT '',backup TEXT NOT NULL DEFAULT '',version TEXT NOT NULL DEFAULT '',at INTEGER NOT NULL,PRIMARY KEY(user_id,agent_id,folder,name))`,
		`CREATE TABLE machine_skill_reports(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL PRIMARY KEY REFERENCES sync_agents(id) ON DELETE CASCADE,version TEXT NOT NULL DEFAULT '',blocked TEXT NOT NULL DEFAULT '',at INTEGER NOT NULL)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	a := reportAgent(t, s, uid, "elitebook")
	if _, err := s.db.Exec(`INSERT INTO machine_skills(user_id,agent_id,folder,name,state,version,at) VALUES(?,?,'f','kept','installed','v1',5)`, uid, a); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateSkillReports(); err != nil {
		t.Fatal(err)
	}
	check()
	var state string
	if err := s.db.QueryRow(`SELECT state FROM machine_skills WHERE name='kept'`).Scan(&state); err != nil || state != "installed" {
		t.Fatal("the migration lost rows", err)
	}
	if err := s.migrateSkillReports(); err != nil {
		t.Fatal("not repeatable", err)
	}
}

func TestReportNewOnMachinesKeepsFirstSightingAcrossReports(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	older, newer := reportAgent(t, s, uid, "zeta"), reportAgent(t, s, uid, "alpha")
	unknown := SkillReportEntry{Folder: "~/.claude/skills", Name: "cq", State: "unknown", Digest: "d1"}
	reportOf(t, s, uid, older, reportT0, unknown)
	reportOf(t, s, uid, newer, reportT0.Add(time.Minute), unknown)
	// Both report again, the first machine last.
	reportOf(t, s, uid, newer, reportT0.Add(2*time.Minute), unknown)
	reportOf(t, s, uid, older, reportT0.Add(3*time.Minute), unknown)
	got, err := s.NewOnMachinesFor(uid)
	if err != nil || len(got) != 1 || len(got[0].Copies) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	c := got[0].Copies
	if c[0].Host != "zeta" || c[0].SeenAt != reportT0.UnixMilli() || c[1].Host != "alpha" || c[1].SeenAt != reportT0.Add(time.Minute).UnixMilli() {
		t.Fatalf("a later report must not move the first sighting: %+v", c)
	}
	// A skill that goes away and comes back is a new sighting.
	reportOf(t, s, uid, older, reportT0.Add(4*time.Minute))
	reportOf(t, s, uid, older, reportT0.Add(5*time.Minute), unknown)
	got, _ = s.NewOnMachinesFor(uid)
	if len(got) != 1 || got[0].Copies[0].Host != "alpha" || got[0].Copies[1].SeenAt != reportT0.Add(5*time.Minute).UnixMilli() {
		t.Fatalf("after coming back: %+v", got)
	}
}

// A report the agent cut short does not confirm a removal by leaving a name
// out: the name may be among the entries it did not send.
func TestCutReportConfirmsNoRemoval(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	a := reportAgent(t, s, uid, "elitebook")
	if err := s.DeleteSkillsFor(uid, []string{item.id}, false, reportT0); err != nil {
		t.Fatal(err)
	}
	cut := SkillReportEntry{Name: SkillReportCutName, State: "failed", Detail: SkillReportCutDetail}
	reportOf(t, s, uid, a, reportT0.Add(time.Minute), installedAt("other", "d1"), cut)
	if r := skillRemovals(t, s, uid); r["demo"].Reason != "deleted" {
		t.Fatalf("a cut report confirmed the removal: %+v", r)
	}
	if !(SkillReport{Entries: []SkillReportEntry{cut}}).Cut() || (SkillReport{Entries: []SkillReportEntry{installedAt("x", "d")}}).Cut() {
		t.Fatal("Cut misreads a report")
	}
	// The same report, whole, confirms it.
	reportOf(t, s, uid, a, reportT0.Add(2*time.Minute), installedAt("other", "d1"))
	if r := skillRemovals(t, s, uid); len(r) != 0 {
		t.Fatalf("a whole report did not confirm: %+v", r)
	}
}

// The page says what the machines are doing over every skill, deleted ones still coming off included, each machine
// counted once: a failure first, then installing, removing or both.
func TestSkillsPageWorkCountsEveryMachineOnce(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	kept := savedWrittenSkill(t, s, uid, "kept", false)
	gone := savedWrittenSkill(t, s, uid, "gone", false)
	fresh := savedWrittenSkill(t, s, uid, "fresh", false)
	dk, dg, df := itemDigest(t, s, uid, kept.id), itemDigest(t, s, uid, gone.id), itemDigest(t, s, uid, fresh.id)
	a, b := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, installedAt("kept", dk), installedAt("gone", dg), installedAt("fresh", df))
	reportOf(t, s, uid, b, reportT0, installedAt("kept", dk), installedAt("gone", dg))
	work := func(at time.Time) SkillWork {
		t.Helper()
		page, err := s.SkillsPageFor(uid, SkillActive, at)
		if err != nil {
			t.Fatal(err)
		}
		return page.Work
	}
	if w := work(reportT0); w != (SkillWork{Installing: 1, Updating: 1}) {
		t.Fatalf("thinkpad has fresh to install: %+v", w)
	}
	if err := s.DeleteSkillsFor(uid, []string{gone.id}, false, reportT0); err != nil {
		t.Fatal(err)
	}
	later := reportT0.Add(time.Minute)
	page, err := s.SkillsPageFor(uid, SkillActive, later)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range page.Skills {
		if r.ID == gone.id {
			t.Fatalf("a deleted skill is not on the page: %+v", page.Skills)
		}
	}
	if page.Work != (SkillWork{Installing: 1, Removing: 2, Updating: 2}) {
		t.Fatalf("both machines still have the deleted skill to take off: %+v", page.Work)
	}
	removed := SkillReportEntry{Folder: "~/.agents/skills", Name: "gone", State: "removed"}
	reportOf(t, s, uid, a, later, installedAt("kept", dk), installedAt("fresh", df), removed)
	reportOf(t, s, uid, b, later, installedAt("kept", dk), installedAt("gone", dg), SkillReportEntry{Folder: "~/.agents/skills", Name: "fresh", State: "failed", Detail: "no write access"})
	if w := work(later); w != (SkillWork{Failed: 1}) {
		t.Fatalf("thinkpad needs a look, once, and elitebook is done: %+v", w)
	}
}

// A machine below the skills protocol gets no skill work, so it is counted apart: it needs reet updated. A paused one
// is left alone, and the host is named when there is just one.
func TestSkillsPageWorkCountsMachinesThatNeedAnUpdate(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "kept", false)
	reportAgent(t, s, uid, "elitebook")
	work := func() SkillWork {
		t.Helper()
		page, err := s.SkillsPageFor(uid, SkillActive, reportT0)
		if err != nil {
			t.Fatal(err)
		}
		return page.Work
	}
	old, _, err := s.EnrollSyncAgentBuild(uid, "thinkpad", "0.9", "")
	if err != nil {
		t.Fatal(err)
	}
	if w := work(); w.NeedsUpdate != 1 || w.NeedsUpdateHost != "thinkpad" {
		t.Fatalf("thinkpad needs reet updated: %+v", w)
	}
	if _, _, err = s.EnrollSyncAgentBuild(uid, "studio", "0.9", ""); err != nil {
		t.Fatal(err)
	}
	if w := work(); w.NeedsUpdate != 2 || w.NeedsUpdateHost != "" {
		t.Fatalf("two machines need reet updated, and neither is named: %+v", w)
	}
	if _, err = s.db.Exec(`UPDATE sync_agents SET paused=1 WHERE id=?`, old.ID); err != nil {
		t.Fatal(err)
	}
	if w := work(); w.NeedsUpdate != 1 || w.NeedsUpdateHost != "studio" {
		t.Fatalf("a paused machine is left alone: %+v", w)
	}
}

// A machine on a newer skills protocol than this controller's gets no skill work either, so it is counted apart too:
// the controller needs updating. A paused one is left alone, the host is named when there is just one, and neither
// counts as one that needs reet updated.
func TestSkillsPageWorkCountsMachinesNewerThanTheController(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	savedWrittenSkill(t, s, uid, "kept", false)
	reportAgent(t, s, uid, "elitebook")
	work := func() SkillWork {
		t.Helper()
		page, err := s.SkillsPageFor(uid, SkillActive, reportT0)
		if err != nil {
			t.Fatal(err)
		}
		return page.Work
	}
	newer := func(host string) string {
		t.Helper()
		a, _, err := s.EnrollSyncAgentBuild(uid, host, "9.0", "")
		if err != nil {
			t.Fatal(err)
		}
		if err = s.SetAgentSkillsProtocolFor(uid, a.ID, CurrentSkillsProtocol+1); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	if w := work(); w.NeedsNewerController != 0 {
		t.Fatalf("no machine is newer yet: %+v", w)
	}
	thinkpad := newer("thinkpad")
	if w := work(); w.NeedsNewerController != 1 || w.NeedsNewerControllerHost != "thinkpad" || w.NeedsUpdate != 0 {
		t.Fatalf("thinkpad runs a newer reet than the controller: %+v", w)
	}
	newer("studio")
	if w := work(); w.NeedsNewerController != 2 || w.NeedsNewerControllerHost != "" {
		t.Fatalf("two newer machines, and neither is named: %+v", w)
	}
	if _, err := s.db.Exec(`UPDATE sync_agents SET paused=1 WHERE id=?`, thinkpad); err != nil {
		t.Fatal(err)
	}
	if w := work(); w.NeedsNewerController != 1 || w.NeedsNewerControllerHost != "studio" {
		t.Fatalf("a paused machine is left alone: %+v", w)
	}
}

// A paused machine, or one that no longer follows the library (another skills
// protocol), puts nothing under New on a machine: its old report is stale.
func TestReportNewOnMachinesSkipsPausedAndOffProtocolMachines(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	live, paused, old := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad"), reportAgent(t, s, uid, "studio")
	unknown := func(name string) SkillReportEntry {
		return SkillReportEntry{Folder: "~/.claude/skills", Name: name, State: "unknown", Digest: "d"}
	}
	reportOf(t, s, uid, live, reportT0, unknown("seen"))
	reportOf(t, s, uid, paused, reportT0, unknown("seen"), unknown("paused-only"))
	reportOf(t, s, uid, old, reportT0, unknown("old-only"))
	if err := s.ControlSyncAgent(uid, paused, "pause"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAgentSkillsProtocolFor(uid, old, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.NewOnMachinesFor(uid)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "seen" || len(got[0].Copies) != 1 || got[0].Copies[0].Host != "elitebook" {
		t.Fatalf("new: %+v", got)
	}
	kept, err := s.KeepNewSkillsFor(uid, []string{"paused-only"}, SkillActive, "", reportT0)
	if err == nil {
		t.Fatalf("kept a skill only a paused machine reported: %+v", kept)
	}
}
