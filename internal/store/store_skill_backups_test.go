package store

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func editedAt(folder, name, digest, backup string) SkillReportEntry {
	return SkillReportEntry{Folder: folder, Name: name, State: "edited", Digest: digest, Backup: backup}
}

func backupsOf(t *testing.T, s *Store, uid int64, name string, at time.Time) []SkillBackup {
	t.Helper()
	got, err := s.SkillBackupsFor(uid, name, at)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// The machine's rows are replaced on every report, so an edited copy's backup
// would be gone a poll later; Use this version still needs to find it.
func TestEditedBackupOutlivesTheNextReport(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	d := itemDigest(t, s, uid, item.id)
	a := reportAgent(t, s, uid, "elitebook")
	backup := "~/.agents/.reet-backups/2027-01-15/agents-demo"
	reportOf(t, s, uid, a, reportT0, editedAt("~/.agents/skills", "demo", d, backup))
	reportOf(t, s, uid, a, reportT0.Add(5*time.Second), installedAt("demo", d))
	want := []SkillBackup{{AgentID: a, Host: "elitebook", Folder: "~/.agents/skills", Name: "demo", Backup: backup, At: reportT0.UnixMilli()}}
	if got := backupsOf(t, s, uid, "DEMO", reportT0.Add(time.Minute)); !reflect.DeepEqual(got, want) {
		t.Fatalf("backups = %+v, want %+v", got, want)
	}
	var machineBackup string
	if err := s.db.QueryRow(`SELECT backup FROM machine_skills WHERE user_id=? AND agent_id=?`, uid, a).Scan(&machineBackup); err != nil || machineBackup != "" {
		t.Fatalf("fixture: the machine's row no longer names the backup: %q %v", machineBackup, err)
	}
}

// Two copies whose names differ only by case are one row of machine_skills,
// but each was someone's work: both backups are kept. The report is shaped as
// an agent sends it: each copy's own entry, then the skill's entry, which
// names the first copy's backup under the skill's name.
func TestEditedCaseVariantsKeepEveryBackup(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "demo", false)
	d := itemDigest(t, s, uid, item.id)
	a := reportAgent(t, s, uid, "elitebook")
	first := "~/.agents/.reet-backups/2027-01-15/agents-Demo"
	second := "~/.agents/.reet-backups/2027-01-15/agents-demo"
	reportOf(t, s, uid, a, reportT0,
		editedAt("~/.agents/skills", "Demo", d, first),
		editedAt("~/.agents/skills", "demo", d, second),
		editedAt("~/.agents/skills", "demo", d, first))
	got := backupsOf(t, s, uid, "demo", reportT0)
	paths := map[string]string{}
	for _, b := range got {
		paths[b.Name] = b.Backup
	}
	if want := map[string]string{"Demo": first, "demo": second}; len(got) != 2 || !reflect.DeepEqual(paths, want) {
		t.Fatalf("backups: %+v", got)
	}
	// The skill's entry alone, in a later report, names a backup already
	// kept under its copy's own name: nothing changes.
	reportOf(t, s, uid, a, reportT0.Add(time.Second), editedAt("~/.agents/skills", "demo", d, first))
	if again := backupsOf(t, s, uid, "demo", reportT0.Add(time.Second)); !reflect.DeepEqual(again, got) {
		t.Fatalf("backups after the skill's entry again: %+v", again)
	}
}

// A newer backup of the same copy replaces the older one, the same one
// reported again keeps its time, and none is kept past 30 days.
func TestEditedBackupIsSupersededAndExpires(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	b := reportAgent(t, s, uid, "thinkpad")
	reportOf(t, s, uid, a, reportT0, editedAt("~/.claude/skills", "demo", "d1", "~/.claude/.reet-backups/2027-01-15/claude-demo"))
	reportOf(t, s, uid, b, reportT0, editedAt("~/.claude/skills", "demo", "d1", "~/.claude/.reet-backups/2027-01-15/claude-demo"))
	later := reportT0.Add(time.Hour)
	reportOf(t, s, uid, a, later, editedAt("~/.claude/skills", "demo", "d2", "~/.claude/.reet-backups/2027-01-15/claude-demo-2"))
	reportOf(t, s, uid, b, later, editedAt("~/.claude/skills", "demo", "d1", "~/.claude/.reet-backups/2027-01-15/claude-demo"))
	got := backupsOf(t, s, uid, "demo", later)
	if len(got) != 2 {
		t.Fatalf("one backup per machine and copy: %+v", got)
	}
	byHost := map[string]SkillBackup{}
	for _, g := range got {
		byHost[g.Host] = g
	}
	if g := byHost["elitebook"]; g.Backup != "~/.claude/.reet-backups/2027-01-15/claude-demo-2" || g.At != later.UnixMilli() {
		t.Fatalf("the newer backup supersedes: %+v", g)
	}
	if g := byHost["thinkpad"]; g.Backup != "~/.claude/.reet-backups/2027-01-15/claude-demo" || g.At != reportT0.UnixMilli() {
		t.Fatalf("the same backup reported again keeps its time: %+v", g)
	}
	if got[0].At < got[1].At {
		t.Fatalf("newest first: %+v", got)
	}
	month := 30 * 24 * time.Hour
	if got := backupsOf(t, s, uid, "demo", reportT0.Add(month-time.Millisecond)); len(got) != 2 {
		t.Fatalf("kept for 30 days: %+v", got)
	}
	if got := backupsOf(t, s, uid, "demo", reportT0.Add(month)); len(got) != 1 || got[0].Host != "elitebook" {
		t.Fatalf("a backup 30 days old is not offered: %+v", got)
	}
	// A report prunes what is past 30 days.
	reportOf(t, s, uid, b, later.Add(month), installedAt("demo", "d2"))
	var rows int
	if err := s.db.QueryRow(`SELECT count(*) FROM skill_edit_backups WHERE user_id=?`, uid).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rows past 30 days: %d %v", rows, err)
	}
	if got := backupsOf(t, s, uid, "", later); len(got) != 0 {
		t.Fatalf("all backups: %+v", got)
	}
}

// Only an edited entry with a backup is a backup; a report naming one with no
// folder, as an agent does for a name it refused, is kept as text.
func TestEditedBackupNeedsAPathAndTakesAnyFolder(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0,
		editedAt("~/.agents/skills", "nobackup", "d1", ""),
		SkillReportEntry{Folder: "~/.agents/skills", Name: "installed", State: "installed", Digest: "d1", Backup: "~/.agents/.reet-backups/2027-01-15/agents-installed"},
		editedAt("", "refused/name", "d1", "~/.agents/.reet-backups/2027-01-15/agents-x"))
	got := backupsOf(t, s, uid, "", reportT0)
	if len(got) != 1 || got[0].Name != "refused/name" || got[0].Folder != "" {
		t.Fatalf("backups: %+v", got)
	}
}

// Each machine keeps at most skillBackupsKept backups, dropping the oldest.
func TestEditedBackupsAreBoundedPerMachine(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	entries := make([]SkillReportEntry, 0, skillBackupsKept)
	for i := range skillBackupsKept {
		name := fmt.Sprintf("s%04d", i)
		entries = append(entries, editedAt("~/.agents/skills", name, "d1", "~/.agents/.reet-backups/2027-01-15/agents-"+name))
	}
	reportOf(t, s, uid, a, reportT0, entries...)
	reportOf(t, s, uid, a, reportT0.Add(time.Second), editedAt("~/.claude/skills", "newest", "d1", "~/.claude/.reet-backups/2027-01-15/claude-newest"))
	got := backupsOf(t, s, uid, "", reportT0.Add(time.Second))
	if len(got) != skillBackupsKept || got[0].Name != "newest" {
		t.Fatalf("kept %d, want %d with the newest first", len(got), skillBackupsKept)
	}
}

// After an edited cycle the next report says installed at the same revision:
// reet's copy went back in the edited cycle, so it is not a second install.
func TestEditedThenInstalledIsNotANewInstall(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	a := reportAgent(t, s, uid, "elitebook")
	reportOf(t, s, uid, a, reportT0, editedAt("~/.agents/skills", "demo", "d1", "~/.agents/.reet-backups/2027-01-15/agents-demo"))
	reportOf(t, s, uid, a, reportT0.Add(time.Second), installedAt("demo", "d1"))
	got := activityOf(t, s, uid)
	if len(got) != 1 || got[0].Action != "edited" {
		t.Fatalf("activity: %+v", got)
	}
	// A new revision after the edit is an update, as from any install.
	reportOf(t, s, uid, a, reportT0.Add(2*time.Second), editedAt("~/.agents/skills", "demo", "d1", "~/.agents/.reet-backups/2027-01-15/agents-demo-2"))
	reportOf(t, s, uid, a, reportT0.Add(3*time.Second), installedAt("demo", "d2"))
	got = activityOf(t, s, uid)
	if len(got) != 3 || got[0].Action != "updated" || got[0].Detail != "d2" || got[1].Action != "edited" {
		t.Fatalf("activity: %+v", got)
	}
}
