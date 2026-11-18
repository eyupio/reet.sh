package store

import (
	"reflect"
	"strings"
	"testing"
	"time"
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
