package store

import (
	"errors"
	"strings"
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

// A person who reviews and approves an import in the browser may choose any of their bundles to add to, whichever machine
// started it: their approval, not the machine's secret, authorises the write.
func TestAnApprovedImportMayAddToABundleAnotherMachineStarted(t *testing.T) {
	s, uid, a, env := observationFixture(t)
	other, _, err := s.EnrollSyncAgent(uid, "desktop", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TouchEnvironmentFor(uid, "desktop", "agents", "", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordInventoryFor(uid, EnvironmentIDFor(uid, "desktop", "agents", ""), true, MachineInfo{}); err != nil {
		t.Fatal(err)
	}
	first, err := s.SaveMachineSkillImportFor(uid, other.ID, machineImportFixture(t))
	if err != nil {
		t.Fatal(err)
	}

	instance := strings.Repeat("a", 48)
	if _, err = s.ClaimRemoteImportFor(uid, a.ID, instance); err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: "remote_preview_request_0002", AgentID: a.ID, EnvironmentID: env, Name: "demo", BundleID: first.Bundle.ID, ExpectedBase: first.Revision.ID})
	if err != nil {
		t.Fatal(err)
	}
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\nname: demo\ndescription: Machine instructions\n---\nChanged on another machine.\n")}}
	r = reviewRemote(t, s, uid, a, r, files)
	if _, err = s.ApproveRemoteImportFor(uid, r.ID, r.PreviewHash, nil); err != nil {
		t.Fatal(err)
	}
	if work, e := s.ClaimRemoteImportFor(uid, a.ID, r.Instance); e != nil || work == nil || work.State != "uploading" {
		t.Fatal(work, e)
	}
	got, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files)
	if err != nil {
		t.Fatalf("an approved import into another machine's bundle was refused: %v", err)
	}
	if got.Bundle.ID != first.Bundle.ID || got.Bundle.Head == first.Revision.ID {
		t.Fatalf("the revision was not added to the chosen bundle: %+v", got)
	}
}
