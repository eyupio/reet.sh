package store

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

func libraryImportRecoveryFixture(t *testing.T) (*Store, int64, SyncAgent, SyncAgent, string, RemoteImport, []skillbundle.ContentFile) {
	t.Helper()
	s, uid, source, destination, target, previous, files := machineSyncFixture(t)
	if _, err := s.StopRemoteImportFor(uid, previous.ID); err != nil {
		t.Fatal(err)
	}
	r, err := s.RequestMachineSkillSync(uid, "local/demo", destination.Host, previous.EnvironmentID,
		"library_"+secretHash("local/demo"), target, true)
	if err != nil {
		t.Fatal(err)
	}
	return s, uid, source, destination, target, r, files
}

func TestExpiredLibraryImportGetsFreshCaptureAndFencesOldProcess(t *testing.T) {
	for _, transition := range []string{"restart", "lease expired", "already expired"} {
		t.Run(transition, func(t *testing.T) {
			s, uid, source, destination, target, old, files := libraryImportRecoveryFixture(t)
			old = reviewRemote(t, s, uid, source, old, files)
			instance := strings.Repeat("b", 48)
			if transition != "restart" {
				if transition == "lease expired" {
					instance = old.Instance
				}
				if err := s.withTx(func(tx *sql.Tx) error {
					r, err := readImport(tx, uid, old.ID)
					if err != nil {
						return err
					}
					r.ExpiresAt = 1
					if transition == "already expired" {
						r.State, r.Error = "expired", "preview_expired"
					}
					return writeImport(tx, uid, r)
				}); err != nil {
					t.Fatal(err)
				}
			}
			if work, err := s.ClaimRemoteImportFor(uid, source.ID, instance); err != nil || work != nil {
				t.Fatalf("stale preview was dispatched: %+v: %v", work, err)
			}
			expired, err := s.RemoteImportFor(uid, old.ID)
			if err != nil || expired.State != "expired" || expired.RequestKey != "retried_"+old.ID || expired.Receipt != nil {
				t.Fatalf("old capture was not retained and retired: %+v: %v", expired, err)
			}
			if _, err = s.ApproveRemoteImportFor(uid, old.ID, old.PreviewHash, nil); err == nil {
				t.Fatal("old approval was reused")
			}
			if _, err = s.CommitRemoteImportFor(uid, source.ID, old.ID, old.Instance, old.Preview.SourceDigest, files); err == nil {
				t.Fatal("old process uploaded into an expired capture")
			}
			problems, err := s.libraryCaptureProblems(uid)
			if err != nil || problems["local/demo"] != "" {
				t.Fatalf("recoverable expiry still hides the library item: %+v: %v", problems, err)
			}
			fresh, err := s.RequestMachineSkillSync(uid, "local/demo", destination.Host, old.EnvironmentID,
				old.RequestKey, target, true)
			if err != nil || fresh.ID == old.ID || fresh.Instance != instance || fresh.Preview != nil || fresh.PreviewHash != "" || len(fresh.Excluded) != 0 {
				t.Fatalf("replacement did not require a fresh preview: %+v: %v", fresh, err)
			}
			fresh = finishMachineCopy(t, s, uid, source, fresh, files)
			again, err := s.CommitRemoteImportFor(uid, source.ID, fresh.ID, fresh.Instance, fresh.Preview.SourceDigest, files)
			if err != nil || again.Revision.ID != fresh.Receipt.Revision.ID {
				t.Fatal("replacement receipt was not idempotent", err)
			}
			bundles, err := s.PrivateSkillBundlesFor(uid)
			if err != nil || len(bundles) != 1 {
				t.Fatalf("restart created duplicate content: %d: %v", len(bundles), err)
			}
			revisions, err := s.PrivateSkillRevisionsFor(uid, bundles[0].ID)
			if err != nil || len(revisions) != 1 {
				t.Fatalf("restart committed duplicate revisions: %d: %v", len(revisions), err)
			}
		})
	}
}

func TestLibraryRecoveryDoesNotRetryPermanentOrCancelledImports(t *testing.T) {
	for _, state := range []string{"cancelled", "failed", "completed"} {
		t.Run(state, func(t *testing.T) {
			s, uid, source, _, _, r, files := libraryImportRecoveryFixture(t)
			switch state {
			case "cancelled":
				if _, err := s.StopRemoteImportFor(uid, r.ID); err != nil {
					t.Fatal(err)
				}
			case "failed":
				if err := s.FailRemoteImportFor(uid, source.ID, r.ID, r.Instance, "capture_failed"); err != nil {
					t.Fatal(err)
				}
			case "completed":
				r = finishMachineCopy(t, s, uid, source, r, files)
			}
			if work, err := s.ClaimRemoteImportFor(uid, source.ID, strings.Repeat("b", 48)); err != nil || work != nil {
				t.Fatal(work, err)
			}
			after, err := s.RemoteImportFor(uid, r.ID)
			if err != nil || after.State != state || after.RequestKey != r.RequestKey {
				t.Fatalf("terminal capture was revived: %+v: %v", after, err)
			}
		})
	}
}

func TestLibraryRecoveryKeepsManualImportsAndAccountFences(t *testing.T) {
	t.Run("manual import", func(t *testing.T) {
		s, uid, source, r, files := remoteImportFixture(t)
		r = reviewRemote(t, s, uid, source, r, files)
		if _, err := s.ClaimRemoteImportFor(uid, source.ID, strings.Repeat("b", 48)); err != nil {
			t.Fatal(err)
		}
		after, err := s.RemoteImportFor(uid, r.ID)
		if err != nil || after.State != "expired" || after.RequestKey != r.RequestKey {
			t.Fatalf("manual consent was changed: %+v: %v", after, err)
		}
	})
	for _, change := range []string{"pause", "revoke", "viewer"} {
		t.Run(change, func(t *testing.T) {
			s, uid, source, _, _, r, files := libraryImportRecoveryFixture(t)
			r = reviewRemote(t, s, uid, source, r, files)
			if change == "viewer" {
				if _, err := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
					t.Fatal(err)
				}
			} else if err := s.ControlSyncAgent(uid, source.ID, change); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ClaimRemoteImportFor(uid, source.ID, strings.Repeat("b", 48)); err == nil {
				t.Fatal("unavailable account/enrollment claimed a capture")
			}
			var retired int
			if err := s.db.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=? AND request_key=?`, uid, "retried_"+r.ID).Scan(&retired); err != nil || retired != 0 {
				t.Fatalf("unauthorised claim retired the old key: %d: %v", retired, err)
			}
		})
	}
}
