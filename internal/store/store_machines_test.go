package store

import (
	"errors"
	"slices"
	"testing"
)

func TestMachineMergePreservesInventoriesReferencesAndAliases(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	owner, _ := s.DefaultOwner()
	member, err := s.CreateUser("other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	a, b := mustRef(t, "o/r/alpha"), mustRef(t, "o/r/beta")
	for _, r := range []struct {
		id   string
		slug string
	}{{a.ID, "alpha"}, {b.ID, "beta"}} {
		_, err := s.UpsertFor(owner, mustRef(t, "o/r/"+r.slug), Patch{})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, host := range []string{"ns3142913", "PROD_Snicket", "reference", "follower"} {
		if err := s.TouchEnvironmentFor(owner, host, "agents", "", ".agents/skills"); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordInventoryFor(owner, EnvironmentIDFor(owner, host, "agents", ""), true, MachineInfo{OS: "linux"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RecordEnvironmentFor(owner, a.ID, "ns3142913", "agents", "", "old-path", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnvironmentFor(owner, b.ID, "PROD_Snicket", "agents", "", "new-path", ""); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"ns3142913", "PROD_Snicket"} {
		if err := s.TouchEnvironmentFor(owner, host, "claude", "/work/project", "project-path"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.TouchEnvironmentFor(owner, "ns3142913", "opencode", "", "extra-path"); err != nil {
		t.Fatal(err)
	}
	oldID, newID := EnvironmentIDFor(owner, "ns3142913", "agents", ""), EnvironmentIDFor(owner, "PROD_Snicket", "agents", "")
	if _, err := s.db.Exec(`UPDATE environment_scans SET scanned_at=1000 WHERE env_id=?`, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE environment_scans SET scanned_at=2000 WHERE env_id=?`, newID); err != nil {
		t.Fatal(err)
	}
	// Reference snapshots are independent of the latest inventory timestamps.
	if _, err := s.db.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, owner, oldID, EnvironmentIDFor(owner, "reference", "agents", ""), "Reference", "[]", "saved-revision", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, owner, EnvironmentIDFor(owner, "follower", "agents", ""), oldID, "Old name", "[]", "follower-revision", 4); err != nil {
		t.Fatal(err)
	}
	if err := s.TouchEnvironmentFor(member.ID, "ns3142913", "agents", "", "other-path"); err != nil {
		t.Fatal(err)
	}
	before, err := s.ExportFor(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.MergeMachinesFor(owner, "ns3142913", "PROD_Snicket"); err != nil {
		t.Fatal(err)
	}
	envs, err := s.SyncEnvironmentsFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	machines, err := s.GroupMachinesFor(owner, envs)
	if err != nil {
		t.Fatal(err)
	}
	var prod *Machine
	for i := range machines {
		if machines[i].Host == "ns3142913" {
			t.Fatal("duplicate host remains")
		}
		if machines[i].Host == "PROD_Snicket" {
			prod = &machines[i]
		}
	}
	if prod == nil || len(prod.Groups) != 3 || prod.SkillCount != 1 || !slices.Contains(prod.Aliases, "ns3142913") {
		t.Fatalf("merged groups/counts: %+v", prod)
	}
	installed, err := s.EnvironmentSkillsFor(owner, newID)
	if err != nil || len(installed) != 1 || installed[0].ID != b.ID {
		t.Fatalf("inventories were unioned or newest was lost: %+v %v", installed, err)
	}
	targets, err := s.SyncTargetsFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("references lost: %+v", targets)
	}
	for _, target := range targets {
		if target.Revision == "saved-revision" && (!slices.Contains(target.TargetAliases, oldID) || !slices.Contains(target.HostAliases, "ns3142913") || target.Target.ID != newID) {
			t.Fatalf("legacy command aliases lost: %+v", target)
		}
		if target.Revision == "follower-revision" && target.SourceID != newID {
			t.Fatal("source reference not retargeted")
		}
	}
	others, _ := s.ListEnvironmentsFor(member.ID)
	if len(others) != 1 || others[0].Host != "ns3142913" {
		t.Fatal("cross-account modification")
	}
	// A later old-name scan and restore reuse the canonical namespace.
	if err := s.RecordEnvironmentFor(owner, a.ID, "ns3142913", "agents", "", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := s.EnvironmentSkillsFor(owner, newID)
	if len(got) != 2 {
		t.Fatalf("old-name report not redirected: %+v", got)
	}
	if err := s.RestoreSnapshotFor(owner, before); err != nil {
		t.Fatal(err)
	}
	restoredEnvs, _ := s.ListEnvironmentsFor(owner)
	for _, env := range restoredEnvs {
		if env.Host == "ns3142913" {
			t.Fatal("restore resurrected the merged hostname")
		}
	}
	filtered, _, err := s.ListFor(owner, Filter{Machine: "ns3142913"})
	if err != nil || len(filtered) != 2 {
		t.Fatalf("legacy filter did not resolve: %v %v", filtered, err)
	}
	if err := s.MergeMachinesFor(owner, "PROD_Snicket", "Preferred"); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeMachinesFor(owner, "Preferred", "ns3142913"); err != nil {
		t.Fatal(err)
	}
	for _, alias := range []string{"PROD_Snicket", "preferred", "ns3142913"} {
		canonical, err := s.ResolveMachineHostFor(owner, alias)
		if err != nil || canonical != "ns3142913" {
			t.Fatalf("alias chain not flat: %s %s %v", alias, canonical, err)
		}
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := s.ResolveMachineHostFor(owner, "PROD_Snicket")
	if err != nil || canonical != "ns3142913" {
		t.Fatal("aliases did not survive restart")
	}
	if err := s.ForgetMachineFor(owner, "PROD_Snicket"); err != nil {
		t.Fatal(err)
	}
	envs, _ = s.SyncEnvironmentsFor(owner)
	for _, env := range envs {
		if env.Host == "ns3142913" {
			t.Fatal("forget left machine groups")
		}
	}
	skill, ok, _ := s.GetFor(owner, a.ID)
	if !ok || skill.ID != a.ID {
		t.Fatal("forget removed shelf entries")
	}
	if err := s.RegisterMachineFor(owner, MachineInfo{Host: "PROD_Snicket", Server: true}); err != nil {
		t.Fatal(err)
	}
	envs, _ = s.SyncEnvironmentsFor(owner)
	for _, env := range envs {
		if env.Agent == "reet-server" && env.Host != "ns3142913" {
			t.Fatal("heartbeat resurrected old name")
		}
	}
}

func TestMachineMergeConflictingReferencesRollsBack(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	owner, _ := s.DefaultOwner()
	for _, host := range []string{"old", "preferred"} {
		if err := s.TouchEnvironmentFor(owner, host, "agents", "", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, owner, EnvironmentIDFor(owner, host, "agents", ""), host, "Reference", "[]", host, 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MergeMachinesFor(owner, "old", "preferred"); !errors.Is(err, ErrMachineConflict) {
		t.Fatalf("wanted conflict, got %v", err)
	}
	envs, _ := s.ListEnvironmentsFor(owner)
	aliases, _ := s.MachineAliasesFor(owner)
	targets, _ := s.SyncTargetsFor(owner)
	if len(envs) != 2 || len(aliases) != 0 || len(targets) != 2 {
		t.Fatal("failed merge partially changed data")
	}
}
