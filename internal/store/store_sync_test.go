package store

import (
	"testing"
)

func TestProjectEnvironmentIDsDoNotCollide(t *testing.T) {
	a := EnvironmentIDFor(1, "machine", "claude", "/work/a-b/c")
	b := EnvironmentIDFor(1, "machine", "claude", "/work/a/b-c")
	if a == b {
		t.Fatal("different project paths share an environment")
	}
	if a != EnvironmentIDFor(1, "machine", "claude", "/work/a-b/c") {
		t.Fatal("project ID is unstable")
	}
}

func TestMachinesGroupAgentsDeduplicateSkillsAndScopeFilters(t *testing.T) {
	s := newStore(t)
	owner, _ := s.DefaultOwner()
	other, err := s.CreateUser("machine-other@example.com", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	r := mustRef(t, "a/b/common")
	s.UpsertFor(owner, r, Patch{})
	s.UpsertFor(other.ID, r, Patch{})
	for _, agent := range []string{"claude", "agents"} {
		if err = s.RecordEnvironmentFor(owner, r.ID, "laptop", agent, "", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	s.RegisterMachineFor(owner, MachineInfo{Host: "laptop", Server: true})
	s.RecordEnvironmentFor(other.ID, r.ID, "server", "claude", "", "", "")
	envs, err := s.SyncEnvironmentsFor(owner)
	if err != nil {
		t.Fatal(err)
	}
	machines := GroupMachines(envs)
	if len(machines) != 1 || machines[0].Host != "laptop" || len(machines[0].Groups) != 3 || machines[0].SkillCount != 1 {
		t.Fatalf("groups %+v", machines)
	}
	skills, _, err := s.ListFor(owner, Filter{Machine: "laptop"})
	if err != nil || len(skills) != 1 {
		t.Fatalf("machine filter %v %v", skills, err)
	}
	skills, _, err = s.ListFor(owner, Filter{Machine: "server"})
	if err != nil || len(skills) != 0 {
		t.Fatal("another account's machine leaked")
	}
	skills, _, err = s.ListFor(owner, Filter{Machine: "laptop", Only: "one-machine"})
	if err != nil || len(skills) != 1 {
		t.Fatal("agent groups counted as different machines")
	}
}
