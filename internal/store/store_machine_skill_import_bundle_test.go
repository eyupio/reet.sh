package store

import (
	"errors"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// A machine's own secret may start a skill and add to the ones it started, but
// not add a revision to a bundle another machine, or the browser, made: its
// revisions are installed on the account's other machines.
func TestAMachineMayOnlyAddToBundlesItStarted(t *testing.T) {
	s, uid, a, _ := observationFixture(t)
	b, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "desktop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordInventoryFor(uid, EnvironmentIDFor(uid, "desktop", "agents", ""), true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}

	first, err := s.SaveMachineSkillImportFor(uid, a.ID, machineImportFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	next := func(request, content string, head string) MachineSkillImport {
		in := machineImportFixture(t)
		in.RequestID, in.BundleID, in.ExpectedBase = request, first.Bundle.ID, &head
		in.Files = []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Machine instructions\n---\n" + content + "\n")}}
		m, err := skillbundle.FromContents(in.Files)
		if err != nil {
			t.Fatal(err)
		}
		in.Digest = m.Digest
		return in
	}

	if _, err = s.SaveMachineSkillImportFor(uid, b.ID, next("machine_import_other_1", "from the desktop", first.Revision.ID)); !errors.Is(err, ErrMachineBundleNotOwned) {
		t.Fatalf("another machine added to the bundle: %v", err)
	}
	if head, e := s.PrivateSkillBundleFor(uid, first.Bundle.ID); e != nil || head.Head != first.Revision.ID {
		t.Fatalf("the refused write moved the head: %+v %v", head, e)
	}
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, next("machine_import_own_001", "from the laptop", first.Revision.ID)); err != nil {
		t.Fatalf("the machine that started the bundle may add to it: %v", err)
	}
	// A bundle that does not exist is still reported as missing, not as someone else's.
	missing := next("machine_import_miss_01", "nothing", "")
	missing.BundleID = "bundle_that_is_not_there"
	if _, err = s.SaveMachineSkillImportFor(uid, b.ID, missing); errors.Is(err, ErrMachineBundleNotOwned) || err == nil {
		t.Fatalf("a missing bundle: %v", err)
	}
}
