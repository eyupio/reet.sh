package store

import (
	"testing"
)

func TestPruneEmptyServerEnvironmentsOnlyRemovesOldEmptyReports(t *testing.T) {
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"old", "fresh", "busy"} {
		if err := s.TouchEnvironmentFor(owner, host, "reet-server", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.UpsertLocalFor(owner, "installed", Patch{}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(owner, "local/installed", "busy", "reet-server", "", "", ""); err != nil {
		t.Fatal(err)
	}
	cutoff := now() - 10*24*60*60*1000
	if _, err := s.db.Exec(`UPDATE environments SET last_seen=? WHERE id IN (?,?)`,
		cutoff, EnvironmentIDFor(owner, "old", "reet-server", ""), EnvironmentIDFor(owner, "busy", "reet-server", "")); err != nil {
		t.Fatal(err)
	}
	deleted, err := s.PruneEmptyServerEnvironmentsFor(owner, 7)
	if err != nil || deleted != 1 {
		t.Fatalf("prune: %d %v", deleted, err)
	}
	envs, err := s.ListEnvironmentsFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, env := range envs {
		seen[env.Host] = true
	}
	if seen["old"] {
		t.Fatal("old empty report survived")
	}
	if !seen["fresh"] || !seen["busy"] {
		t.Fatalf("prune removed the wrong reports: %+v", seen)
	}
}

func TestAutomaticPruneSettingRunsOnHeartbeat(t *testing.T) {
	s := newStore(t)
	owner, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.TouchEnvironmentFor(owner, "stale", "reet-server", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE environments SET last_seen=? WHERE id=?`,
		now()-30*24*60*60*1000, EnvironmentIDFor(owner, "stale", "reet-server", "")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("prune_server_environments_days", "7"); err != nil {
		t.Fatal(err)
	}
	if err := s.RegisterMachineFor(owner, MachineInfo{Host: "live", Server: true}); err != nil {
		t.Fatal(err)
	}
	envs, err := s.ListEnvironmentsFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range envs {
		if env.Host == "stale" {
			t.Fatal("heartbeat did not apply the prune setting")
		}
	}
}
