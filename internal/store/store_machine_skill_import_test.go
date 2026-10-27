package store

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"testing"
)

func machineImportFixture(t *testing.T) MachineSkillImport {
	t.Helper()
	base := ""
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte("---\r\nname: demo\r\ndescription: Machine instructions\r\n---\r\nUse this skill.\r\n")}, {Path: "bin/run", Data: []byte{0, 255, 1}, Executable: true}}
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	return MachineSkillImport{SchemaVersion: 1, RequestID: "machine_import_000001", ExpectedBase: &base, Agent: "agents", Name: "demo", Digest: m.Digest, Files: files}
}
func TestMachineSkillImportOwnedAtomicAndStableRetry(t *testing.T) {
	s, uid, a, _ := observationFixture(t)
	in := machineImportFixture(t)
	out, err := s.SaveMachineSkillImportFor(uid, a.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	full, err := s.PrivateSkillRevisionFor(uid, out.Bundle.ID, out.Revision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Files) != 2 || !full.Files[1].Executable || full.ManifestDigest != in.Digest || full.Origin.Kind != "import" {
		t.Fatal(full)
	}
	changed := in
	changed.Files = append([]skillbundle.ContentFile{}, in.Files...)
	changed.Files[1].Data = []byte("different")
	changedManifest, _ := skillbundle.FromContents(changed.Files)
	changed.Digest = changedManifest.Digest
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, changed); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatal("changed request ID reused", err)
	}
	// A later edit does not change the original request's receipt.
	next := in
	next.RequestID = "machine_import_000002"
	next.BundleID = out.Bundle.ID
	base := out.Revision.ID
	next.ExpectedBase = &base
	next.Files = append([]skillbundle.ContentFile{}, in.Files...)
	next.Files[1].Data = []byte("new")
	m, _ := skillbundle.FromContents(next.Files)
	next.Digest = m.Digest
	newer, err := s.SaveMachineSkillImportFor(uid, a.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.SaveMachineSkillImportFor(uid, a.ID, in)
	if err != nil || retry.Revision.ID != out.Revision.ID || retry.Bundle.Head != out.Revision.ID {
		t.Fatal(retry, err)
	}
	current, err := s.PrivateSkillBundleFor(uid, out.Bundle.ID)
	if err != nil || current.Head != newer.Revision.ID {
		t.Fatal("retry rewound head", current, err)
	}
	next.ExpectedBase = in.ExpectedBase
	next.RequestID = "machine_import_000003"
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, next); !errors.Is(err, ErrPrivateSkillConflict) {
		t.Fatal("stale base", err)
	}
	other, err := s.CreateUser("import-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveMachineSkillImportFor(other.ID, a.ID, in); err == nil {
		t.Fatal("cross-account import")
	}
	in.Digest = "changed"
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, in); !errors.Is(err, ErrMachineImportChanged) {
		t.Fatal(err)
	}
	in = machineImportFixture(t)
	in.Files = append(in.Files, skillbundle.ContentFile{Path: ".env", Data: []byte("PRIVATE")})
	m, _ = skillbundle.FromContents(in.Files)
	in.Digest = m.Digest
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, in); err == nil {
		t.Fatal("credential file imported")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, machineImportFixture(t)); err == nil {
		t.Fatal("paused import")
	}
	if err = s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveMachineSkillImportFor(uid, a.ID, machineImportFixture(t)); err == nil {
		t.Fatal("revoked import")
	}
}
func TestMachineSkillImportRollbackAndUnreportedScope(t *testing.T) {
	s, uid, a, _ := observationFixture(t)
	in := machineImportFixture(t)
	in.Project = "/not-reported"
	if _, err := s.SaveMachineSkillImportFor(uid, a.ID, in); err == nil {
		t.Fatal("unknown scope")
	}
	in.Project = ""
	if _, err := s.db.Exec(`CREATE TRIGGER fail_import BEFORE INSERT ON private_skill_revision_files BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveMachineSkillImportFor(uid, a.ID, in); err == nil {
		t.Fatal("partial write succeeded")
	}
	for _, table := range []string{"private_skill_bundles", "private_skill_revisions", "private_skill_blobs", "private_skill_revision_files"} {
		var n int
		if err := s.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatal(table, n, err)
		}
	}
}
