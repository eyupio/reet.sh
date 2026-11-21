package store

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

// The list read in one pass must be what asking about each environment in
// turn gives.
func TestSyncEnvironmentsMatchAskingEachEnvironment(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	other, err := s.CreateUser("sync-other@example.com", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	record := func(uid int64, id, host, agent, project string) {
		t.Helper()
		r := mustRef(t, id)
		if _, err := s.UpsertFor(uid, r, Patch{}); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordEnvironmentFor(uid, r.ID, host, agent, project, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	record(owner, "a/b/zeta", "laptop", "claude", "")
	record(owner, "a/b/alpha", "laptop", "claude", "")
	record(owner, "a/b/alpha", "laptop", "agents", "")
	record(owner, "a/b/mid", "desktop", "claude", "/work/project")
	record(other.ID, "a/b/alpha", "laptop", "claude", "")
	// A machine with no skills, one scanned completely, one with an error.
	if err := s.RegisterMachineFor(owner, MachineInfo{Host: "empty", OS: "linux", Arch: "arm64", Version: "1.2.3"}); err != nil {
		t.Fatal(err)
	}
	laptop := EnvironmentIDFor(owner, "laptop", "claude", "")
	agents := EnvironmentIDFor(owner, "laptop", "agents", "")
	if err := s.RecordInventoryFor(owner, laptop, true, MachineInfo{OS: "darwin", Arch: "arm64", Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordInventoryFor(owner, agents, false, MachineInfo{OS: "darwin", Arch: "arm64", Version: "2.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordScanErrorFor(owner, agents, "the folder could not be read"); err != nil {
		t.Fatal(err)
	}
	// Observed content: current for one environment, out of date for another.
	if _, err := s.db.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,last_seen) VALUES('agent-1',?,'LAPTOP','h','1',?)`, owner, now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO content_observation_scopes(agent_id,user_id,agent,project,inventory_revision,complete,observed_at,received_at,skills)
 SELECT 'agent-1',?,'claude','',inventory_revision,1,?,?,'[]' FROM environment_scans WHERE env_id=?`, owner, now(), now(), laptop); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO content_observation_scopes(agent_id,user_id,agent,project,inventory_revision,complete,observed_at,received_at,skills)
 SELECT 'agent-1',?,'agents','',inventory_revision+1,1,?,?,'[]' FROM environment_scans WHERE env_id=?`, owner, now(), now(), agents); err != nil {
		t.Fatal(err)
	}

	for _, uid := range []int64{owner, other.ID} {
		got, err := s.SyncEnvironmentsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		envs, err := s.ListEnvironmentsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]SyncEnvironment, 0, len(envs))
		for _, env := range envs {
			item := SyncEnvironment{Environment: env}
			err := s.db.QueryRow(`SELECT scanned_at,complete,os,arch,version FROM environment_scans WHERE env_id=?`, env.ID).Scan(&item.ScannedAt, &item.Complete, &item.OS, &item.Arch, &item.Version)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				t.Fatal(err)
			}
			_ = s.db.QueryRow(`SELECT error FROM scan_errors WHERE env_id=?`, env.ID).Scan(&item.Error)
			if item.Skills, err = s.EnvironmentSkillsFor(uid, env.ID); err != nil {
				t.Fatal(err)
			}
			if item.Content, err = s.ContentObservationFor(uid, env.ID); err != nil {
				t.Fatal(err)
			}
			want = append(want, item)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("user %d:\n got %+v\nwant %+v", uid, got, want)
		}
	}

	got, _ := s.SyncEnvironmentsFor(owner)
	if len(got) < 4 {
		t.Fatalf("environments: %d", len(got))
	}
	seen := map[string]SyncEnvironment{}
	for _, e := range got {
		seen[e.ID] = e
	}
	if e := seen[laptop]; len(e.Skills) != 2 || e.Skills[0].Slug != "alpha" || !e.Complete || e.OS != "darwin" || e.Content.State != "observed" {
		t.Fatalf("laptop: %+v content %+v", e, e.Content)
	}
	if e := seen[agents]; e.Error == "" || e.Complete || e.Content.State != "not_reported" {
		t.Fatalf("agents: %+v content %+v", e, e.Content)
	}
	if e := seen[EnvironmentIDFor(owner, "empty", "reet", "")]; e.Skills == nil || len(e.Skills) != 0 || e.Version != "1.2.3" {
		t.Fatalf("empty: %+v", e)
	}
}
