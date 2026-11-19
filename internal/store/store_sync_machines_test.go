package store

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func syncMachineOf(t *testing.T, s *Store, uid int64, host string, at time.Time) SyncMachine {
	t.Helper()
	ms, err := s.SyncMachinesFor(uid, at)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms {
		if m.Host == host {
			return m
		}
	}
	t.Fatalf("no machine %s in %+v", host, ms)
	return SyncMachine{}
}

// Each machine's Sync row: what it is doing, which skills failed there and
// why, and what it has still to do.
func TestSyncMachinesStates(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	audit := savedWrittenSkill(t, s, uid, "seo-audit", false)
	cluster := savedWrittenSkill(t, s, uid, "seo-cluster", false)
	gone := savedWrittenSkill(t, s, uid, "seo-bing", false)
	if err := s.SetSkillStatusFor(uid, []string{gone.id}, SkillArchived, reportT0.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	da, dc := itemDigest(t, s, uid, audit.id), itemDigest(t, s, uid, cluster.id)
	elitebook, thinkpad, studio := reportAgent(t, s, uid, "elitebook"), reportAgent(t, s, uid, "thinkpad"), reportAgent(t, s, uid, "studio")
	buildBox, blocked := reportAgent(t, s, uid, "build-box"), reportAgent(t, s, uid, "lab")
	old, _, err := s.EnrollSyncAgentBuild(uid, "old-mac", "0.9", "")
	if err != nil {
		t.Fatal(err)
	}
	newer := reportAgent(t, s, uid, "future")
	if err = s.SetAgentSkillsProtocolFor(uid, newer, CurrentSkillsProtocol+1); err != nil {
		t.Fatal(err)
	}
	if err = s.ControlSyncAgent(uid, buildBox, "pause"); err != nil {
		t.Fatal(err)
	}
	reportOf(t, s, uid, elitebook, reportT0, installedAt("seo-audit", da), installedAt("seo-cluster", dc), SkillReportEntry{Folder: "~/.agents/skills", Name: "seo-bing", State: "removed"})
	reportOf(t, s, uid, thinkpad, reportT0, installedAt("seo-audit", da),
		SkillReportEntry{Folder: "~/.claude/skills", Name: "seo-cluster", State: "failed", Detail: "no write access"},
		SkillReportEntry{Folder: "~/.agents/skills", Name: "seo-bing", State: "removed"})
	reportOf(t, s, uid, studio, reportT0, installedAt("seo-audit", da), installedAt("seo-cluster", dc), installedAt("seo-bing", "x"))
	if err = s.RecordSkillReportFor(uid, blocked, SkillReport{Version: "v1", Blocked: "allow-apply"}, reportT0); err != nil {
		t.Fatal(err)
	}
	at := reportT0.Add(11 * time.Minute) // studio, last seen at reportT0, is offline by now
	if _, err = s.db.Exec(`UPDATE sync_agents SET last_seen=? WHERE id IN (?,?,?)`, at.UnixMilli(), elitebook, thinkpad, blocked); err != nil {
		t.Fatal(err)
	}

	ms, err := s.SyncMachinesFor(uid, at)
	if err != nil {
		t.Fatal(err)
	}
	hosts := []string{}
	for _, m := range ms {
		hosts = append(hosts, m.Host)
	}
	if !reflect.DeepEqual(hosts, []string{"build-box", "elitebook", "future", "lab", "old-mac", "studio", "thinkpad"}) {
		t.Fatalf("machines: %v", hosts)
	}
	for _, c := range []struct {
		host, state string
		online      bool
		protocol    int
		failed      []SyncFailure
		waiting     []string
	}{
		{"elitebook", "ok", true, 2, nil, nil},
		{"thinkpad", "failed", true, 2, []SyncFailure{{Name: "seo-cluster", Detail: "no write access"}}, nil},
		{"studio", "offline", false, 2, nil, []string{"seo-bing"}},
		{"build-box", "paused", false, 2, nil, nil},
		{"lab", "blocked", true, 2, nil, []string{"seo-audit", "seo-cluster"}},
		{"old-mac", "needs-update", false, 0, nil, nil},
		{"future", "needs-newer-controller", false, CurrentSkillsProtocol + 1, nil, nil},
	} {
		m := syncMachineOf(t, s, uid, c.host, at)
		if m.State != c.state || m.Online != c.online || m.Protocol != c.protocol || m.Paused != (c.host == "build-box") || m.AgentID == "" {
			t.Errorf("%s: %+v", c.host, m)
		}
		if len(m.Failed) != len(c.failed) || len(c.failed) > 0 && !reflect.DeepEqual(m.Failed, c.failed) {
			t.Errorf("%s failed: %+v", c.host, m.Failed)
		}
		if len(m.Waiting) != len(c.waiting) || len(c.waiting) > 0 && !reflect.DeepEqual(m.Waiting, c.waiting) {
			t.Errorf("%s waiting: %+v", c.host, m.Waiting)
		}
	}
	if m := syncMachineOf(t, s, uid, "old-mac", at); m.AgentID != old.ID || m.Failed == nil || m.Waiting == nil {
		t.Errorf("old-mac lists are null, or the id is wrong: %+v", m)
	}
	if m := syncMachineOf(t, s, uid, "elitebook", at); m.LastSeen != at.UnixMilli() {
		t.Errorf("last seen: %+v", m)
	}
	// Installing and removing at once is updating.
	if _, err = s.db.Exec(`DELETE FROM machine_skills WHERE agent_id=? AND name='seo-cluster'`, studio); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE sync_agents SET last_seen=? WHERE id=?`, at.UnixMilli(), studio); err != nil {
		t.Fatal(err)
	}
	if m := syncMachineOf(t, s, uid, "studio", at); m.State != "updating" || !reflect.DeepEqual(m.Waiting, []string{"seo-bing", "seo-cluster"}) {
		t.Errorf("studio installing and removing: %+v", m)
	}
}

// Retry clears a machine's failure: the skill reads as being installed again,
// the request waits for the machine's next poll, and is handed over once.
func TestSkillRetryClearsTheFailureUntilTheNextReport(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	cluster := savedWrittenSkill(t, s, uid, "seo-cluster", false)
	gone := savedWrittenSkill(t, s, uid, "seo-bing", false)
	if err := s.SetSkillStatusFor(uid, []string{gone.id}, SkillArchived, reportT0.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	thinkpad := reportAgent(t, s, uid, "thinkpad")
	failed := []SkillReportEntry{
		{Folder: "~/.claude/skills", Name: "seo-cluster", State: "failed", Detail: "no write access"},
		{Folder: "~/.agents/skills", Name: "seo-bing", State: "failed", Detail: "busy"},
	}
	reportOf(t, s, uid, thinkpad, reportT0, failed...)
	if h := healthOf(t, s, uid, cluster.id, reportT0); h.State != "failed" {
		t.Fatalf("before: %+v", h)
	}
	names, err := s.RequestSkillRetryFor(uid, thinkpad, []string{"SEO-Cluster", "seo-bing", " ", "seo-cluster"}, reportT0)
	if err != nil || !reflect.DeepEqual(names, []string{"SEO-Cluster", "seo-bing"}) {
		t.Fatalf("retry: %v %v", names, err)
	}
	if h := healthOf(t, s, uid, cluster.id, reportT0); h.State != "installing" || h.Pending != 1 {
		t.Fatalf("a retried failure still reads failed: %+v", h)
	}
	if m := syncMachineOf(t, s, uid, "thinkpad", reportT0); m.State != "updating" || len(m.Failed) != 0 || !reflect.DeepEqual(m.Waiting, []string{"seo-bing", "seo-cluster"}) {
		t.Fatalf("machine after the retry: %+v", m)
	}
	// A removal asked again is not taken for done.
	var confirmed int
	if err = s.db.QueryRow(`SELECT count(*) FROM skill_removal_confirms WHERE agent_id=?`, thinkpad).Scan(&confirmed); err != nil || confirmed != 0 {
		t.Fatalf("confirms: %d %v", confirmed, err)
	}
	took, err := s.TakeSkillRetriesFor(uid, thinkpad)
	if err != nil || !reflect.DeepEqual(took, []string{"SEO-Cluster", "seo-bing"}) {
		t.Fatalf("take: %v %v", took, err)
	}
	if again, err := s.TakeSkillRetriesFor(uid, thinkpad); err != nil || len(again) != 0 {
		t.Fatalf("handed over twice: %v %v", again, err)
	}
	// The activity says it was asked for, and a second failure is a new line.
	activity, err := s.SkillActivityFor(uid, 10)
	if err != nil || len(activity) == 0 || activity[0].Action != "retrying" || activity[0].Host != "thinkpad" {
		t.Fatalf("activity: %+v %v", activity, err)
	}
	reportOf(t, s, uid, thinkpad, reportT0.Add(time.Second), failed...)
	if h := healthOf(t, s, uid, cluster.id, reportT0.Add(time.Second)); h.State != "failed" {
		t.Fatalf("failing again: %+v", h)
	}
	if activity, _ = s.SkillActivityFor(uid, 10); activity[0].Action != "failed" {
		t.Fatalf("failing again wrote no activity: %+v", activity)
	}
}

func TestSkillRetryIsRefusedForAnotherAccountOrAMachineThatCannotTry(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	thinkpad := reportAgent(t, s, uid, "thinkpad")
	other, err := s.CreateUser("other@retry.invalid", "Other", "member", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestSkillRetryFor(other.ID, thinkpad, []string{"a"}, reportT0); !errors.Is(err, ErrNoSuchMachine) {
		t.Fatalf("another account: %v", err)
	}
	if took, _ := s.TakeSkillRetriesFor(uid, thinkpad); len(took) != 0 {
		t.Fatalf("recorded for another account: %v", took)
	}
	if _, err = s.RequestSkillRetryFor(uid, thinkpad, nil, reportT0); err == nil {
		t.Fatal("no names was taken")
	}
	if err = s.ControlSyncAgent(uid, thinkpad, "pause"); err != nil {
		t.Fatal(err)
	}
	var refused SkillRetryRefused
	if _, err = s.RequestSkillRetryFor(uid, thinkpad, []string{"a"}, reportT0); !errors.As(err, &refused) {
		t.Fatalf("paused: %v", err)
	}
	if err = s.ControlSyncAgent(uid, thinkpad, "resume"); err != nil {
		t.Fatal(err)
	}
	if err = s.SetAgentSkillsProtocolFor(uid, thinkpad, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestSkillRetryFor(uid, thinkpad, []string{"a"}, reportT0); !errors.As(err, &refused) {
		t.Fatalf("below the protocol: %v", err)
	}
}
