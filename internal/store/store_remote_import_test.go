package store

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

func remoteImportFixture(t *testing.T) (*Store, int64, SyncAgent, RemoteImport, []skillbundle.ContentFile) {
	t.Helper()
	s, uid, a, env := observationFixture(t)
	instance := strings.Repeat("a", 48)
	if _, err := s.ClaimRemoteImportFor(uid, a.ID, instance); err != nil {
		t.Fatal(err)
	}
	r, err := s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: "remote_preview_request_0001", AgentID: a.ID, EnvironmentID: env, Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, a, r, machineImportFixture(t).Files
}
func remotePreview(t *testing.T, files []skillbundle.ContentFile) skillimport.Preview {
	t.Helper()
	m, err := skillbundle.FromContents(files)
	if err != nil {
		t.Fatal(err)
	}
	p := skillimport.Preview{SchemaVersion: 1, SourceDigest: m.Digest, SkillValid: true, Files: []skillimport.PreviewFile{}}
	for _, f := range m.Files {
		for _, data := range files {
			if f.Path == data.Path {
				p.Files = append(p.Files, skillimport.PreviewFile{File: f, Blocked: skillimport.BlockedReason(data)})
			}
		}
	}
	return p
}
func reviewRemote(t *testing.T, s *Store, uid int64, a SyncAgent, r RemoteImport, files []skillbundle.ContentFile) RemoteImport {
	t.Helper()
	work, err := s.ClaimRemoteImportFor(uid, a.ID, r.Instance)
	if err != nil || work == nil || work.State != "previewing" {
		t.Fatal(work, err)
	}
	if err = s.RecordRemotePreviewFor(uid, a.ID, r.ID, r.Instance, remotePreview(t, files)); err != nil {
		t.Fatal(err)
	}
	result, err := s.RemoteImportFor(uid, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestRemoteImportReviewExclusionsAndAtomicReceipt(t *testing.T) {
	s, uid, a, r, files := remoteImportFixture(t)
	// The new channel must never dispatch an import to a legacy sync poll.
	if old, err := s.ClaimSyncJob(uid, a); err != nil || old != nil {
		t.Fatal("import leaked into legacy queue", old, err)
	}
	unsafe := append(append([]skillbundle.ContentFile{}, files...), skillbundle.ContentFile{Path: ".env", Data: []byte("PRIVATE_TOKEN=not-for-upload")})
	r = reviewRemote(t, s, uid, a, r, unsafe)
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "PRIVATE_TOKEN") || strings.Contains(string(raw), `"data":`) {
		t.Fatal("preview leaked bytes")
	}
	if _, err := s.ApproveRemoteImportFor(uid, r.ID, r.PreviewHash, nil); err == nil {
		t.Fatal("hazard approved")
	}
	if _, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files); err == nil {
		t.Fatal("unapproved transfer")
	}
	approved, err := s.ApproveRemoteImportFor(uid, r.ID, r.PreviewHash, []string{".env"})
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := skillbundle.FromContents(files)
	if approved.Digest != selected.Digest {
		t.Fatal("selection digest mismatch")
	}
	work, err := s.ClaimRemoteImportFor(uid, a.ID, r.Instance)
	if err != nil || work == nil || work.State != "uploading" {
		t.Fatal(work, err)
	}
	// Abort after private bytes have been staged: request receipt and revision
	// must roll back together, leaving an identical retry recoverable.
	if _, err = s.db.Exec(`CREATE TRIGGER fail_remote_receipt BEFORE UPDATE ON machine_import_requests WHEN NEW.state='completed' BEGIN SELECT RAISE(ABORT,'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files); err == nil {
		t.Fatal("injected receipt failure succeeded")
	}
	bundles, err := s.PrivateSkillBundlesFor(uid)
	if err != nil || len(bundles) != 0 {
		t.Fatal("partial private write", bundles, err)
	}
	if _, err = s.db.Exec(`DROP TRIGGER fail_remote_receipt`); err != nil {
		t.Fatal(err)
	}
	result, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files)
	if err != nil || again.Revision.ID != result.Revision.ID {
		t.Fatal("lost-response retry duplicated revision", again, err)
	}
	all, _, err := s.RemoteImportsFor(uid)
	if err != nil || all[0].State != "completed" || all[0].Receipt.Revision.ID != result.Revision.ID {
		t.Fatal(all, err)
	}
	if work, err = s.ClaimRemoteImportFor(uid, a.ID, r.Instance); err != nil || work != nil {
		t.Fatal("completed upload replayed", work, err)
	}
	if err = s.CancelRemoteImportFor(uid, r.ID); err != nil {
		t.Fatal(err)
	}
	bundles, err = s.PrivateSkillBundlesFor(uid)
	if err != nil || len(bundles) != 1 {
		t.Fatal("history removal deleted private copy", bundles, err)
	}
}
func TestRemoteImportCancellationExpiryGenerationAndOwners(t *testing.T) {
	for _, action := range []string{"cancel", "restart", "expire", "pause", "revoke", "role", "different bytes"} {
		t.Run(action, func(t *testing.T) {
			s, uid, a, r, files := remoteImportFixture(t)
			r = reviewRemote(t, s, uid, a, r, files)
			if _, err := s.ApproveRemoteImportFor(uid, r.ID, r.PreviewHash, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ClaimRemoteImportFor(uid, a.ID, r.Instance); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "cancel":
				if err := s.CancelRemoteImportFor(uid, r.ID); err != nil {
					t.Fatal(err)
				}
			case "restart":
				if _, err := s.ClaimRemoteImportFor(uid, a.ID, strings.Repeat("b", 48)); err != nil {
					t.Fatal(err)
				}
			case "expire":
				if err := s.withTx(func(tx *sql.Tx) error {
					r, err := readImport(tx, uid, r.ID)
					if err != nil {
						return err
					}
					r.ExpiresAt = 1
					return writeImport(tx, uid, r)
				}); err != nil {
					t.Fatal(err)
				}
			case "pause":
				if err := s.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
					t.Fatal(err)
				}
			case "revoke":
				if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
					t.Fatal(err)
				}
			case "role":
				if _, err := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
					t.Fatal(err)
				}
			case "different bytes":
				files[1].Data = []byte("changed")
			}
			if _, err := s.CommitRemoteImportFor(uid, a.ID, r.ID, r.Instance, r.Preview.SourceDigest, files); err == nil {
				t.Fatal("unapproved outcome committed")
			}
			var n int
			if err := s.db.QueryRow(`SELECT count(*) FROM private_skill_revisions`).Scan(&n); err != nil || n != 0 {
				t.Fatal(n, err)
			}
		})
	}
	s, uid, a, r, files := remoteImportFixture(t)
	other, err := s.CreateUser("remote-other@example.test", "Other", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ApproveRemoteImportFor(other.ID, r.ID, "", nil); err == nil {
		t.Fatal("foreign approval")
	}
	if err = s.RecordRemotePreviewFor(other.ID, a.ID, r.ID, r.Instance, remotePreview(t, files)); err == nil {
		t.Fatal("foreign preview")
	}
	if err = s.CancelRemoteImportFor(other.ID, r.ID); err == nil {
		t.Fatal("foreign cancellation")
	}
	if all, cap, err := s.RemoteImportsFor(other.ID); err != nil || len(all) != 0 || len(cap) != 0 {
		t.Fatal(all, cap, err)
	}
	original, err := s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: r.RequestKey, AgentID: a.ID, EnvironmentID: r.EnvironmentID, Name: r.Name})
	if err != nil || original.ID != r.ID {
		t.Fatal("create retry", original, err)
	}
	if _, err = s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: r.RequestKey, AgentID: a.ID, EnvironmentID: r.EnvironmentID, Name: "changed"}); err == nil {
		t.Fatal("changed create retry")
	}
}
