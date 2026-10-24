package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func contentTargetFixture(t *testing.T) (*Store, int64, SyncAgent, SkillContentTargetWrite, PrivateSkillResult) {
	t.Helper()
	s, uid, a, env := observationFixture(t)
	b, e := s.SavePrivateSkillRevisionFor(uid, "", privateWrite("content_target_bundle_01", "", "PRIVATE_PIN_BYTES"))
	if e != nil {
		t.Fatal(e)
	}
	return s, uid, a, SkillContentTargetWrite{EnvironmentID: env, AgentID: a.ID, BundleID: b.Bundle.ID, RevisionID: b.Revision.ID, Mode: "additive", ExpectedVersion: sp("")}, b
}
func TestSkillContentTargetPinsExactRevisionAndPreservesLegacy(t *testing.T) {
	s, uid, a, in, first := contentTargetFixture(t)
	// Seed an independently versioned legacy desired target. Content pins must not
	// alter its desired-set hash, baseline, acknowledgement or polling behaviour.
	if _, e := s.db.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, in.EnvironmentID, "shelf", "Existing", "[]", "legacy-version", now()); e != nil {
		t.Fatal(e)
	}
	before, e := s.SyncTargetsFor(uid)
	if e != nil {
		t.Fatal(e)
	}
	pin, e := s.SaveSkillContentTargetFor(uid, in)
	if e != nil {
		t.Fatal(e)
	}
	if pin.RevisionID != first.Revision.ID || pin.ManifestDigest != first.Revision.ManifestDigest || pin.Verification != "not_verified" || pin.Name != "demo" {
		t.Fatal(pin)
	}
	same, e := s.SaveSkillContentTargetFor(uid, in)
	if e != nil || !reflect.DeepEqual(same, pin) {
		t.Fatal("identical current intent", same, e)
	}
	next, e := s.SavePrivateSkillRevisionFor(uid, first.Bundle.ID, privateWrite("content_target_bundle_02", first.Revision.ID, "NEW_PRIVATE_BYTES"))
	if e != nil {
		t.Fatal(e)
	}
	got, e := s.SkillContentTargetFor(uid, pin.ID)
	if e != nil || got.RevisionID != first.Revision.ID {
		t.Fatal("pin followed head", got, e)
	}
	in.RevisionID = next.Revision.ID
	if _, e = s.SaveSkillContentTargetFor(uid, in); !errors.Is(e, ErrSkillContentTargetConflict) {
		t.Fatal("overwrote current pin", e)
	}
	in.ExpectedVersion = &pin.Version
	changed, e := s.SaveSkillContentTargetFor(uid, in)
	if e != nil || changed.ID != pin.ID || changed.Version == pin.Version || changed.RevisionID != next.Revision.ID {
		t.Fatal(changed, e)
	}
	in.RevisionID = first.Revision.ID
	if _, e = s.SaveSkillContentTargetFor(uid, in); !errors.Is(e, ErrSkillContentTargetConflict) {
		t.Fatal("stale request rewound pin", e)
	}
	if e = s.DeleteSkillContentTargetFor(uid, pin.ID, pin.Version); !errors.Is(e, ErrSkillContentTargetConflict) {
		t.Fatal("stale delete", e)
	}
	after, e := s.SyncTargetsFor(uid)
	if e != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("legacy desired changed", after, e)
	}
	if job, e := s.ClaimSyncJob(uid, a); e != nil || job != nil {
		t.Fatal("pin dispatched to legacy connector", job, e)
	}
	var jobs int
	_ = s.db.QueryRow(`SELECT count(*) FROM sync_jobs`).Scan(&jobs)
	if jobs != 0 {
		t.Fatal("pin queued execution")
	}
	if e = s.ControlSyncAgent(uid, a.ID, "pause"); e != nil {
		t.Fatal(e)
	}
	if all, e := s.SkillContentTargetsFor(uid); e != nil || len(all) != 1 {
		t.Fatal("paused pin not inspectable", all, e)
	}
	if e = s.DeleteSkillContentTargetFor(uid, changed.ID, changed.Version); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SkillContentTargetFor(uid, pin.ID); !errors.Is(e, ErrSkillContentTargetMissing) {
		t.Fatal(e)
	}
	if _, e = s.PrivateSkillRevisionFor(uid, first.Bundle.ID, first.Revision.ID); e != nil {
		t.Fatal("removing intent deleted private bytes", e)
	}
	if _, e = s.SaveSkillContentTargetFor(uid, in); e == nil {
		t.Fatal("recreated deleted pin using old update version")
	}
}
func TestSkillContentTargetOwnerScopeAndInputFences(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *Store, int64, SyncAgent, *SkillContentTargetWrite)
	}{
		{"missing version", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			in.ExpectedVersion = nil
		}},
		{"bad version", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			in.ExpectedVersion = sp("latest")
		}},
		{"digest as revision", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			in.RevisionID = strings.Repeat("a", 64)
		}},
		{"unknown revision", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			in.RevisionID = strings.Repeat("f", 32)
		}},
		{"legacy mirror mode", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) { in.Mode = "mirror" }},
		{"foreign bundle", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			other, e := s.CreateUser("pin-other@example.test", "Other", "member", "hash")
			if e != nil {
				t.Fatal(e)
			}
			b, e := s.SavePrivateSkillRevisionFor(other.ID, "", privateWrite("foreign_pin_bundle_01", "", "other"))
			if e != nil {
				t.Fatal(e)
			}
			in.BundleID = b.Bundle.ID
			in.RevisionID = b.Revision.ID
		}},
		{"foreign environment", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			other, e := s.CreateUser("pin-other@example.test", "Other", "member", "hash")
			if e != nil {
				t.Fatal(e)
			}
			_ = s.TouchEnvironmentFor(other.ID, a.Host, "agents", "", "")
			in.EnvironmentID = EnvironmentIDFor(other.ID, a.Host, "agents", "")
		}},
		{"wrong connector", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			b, _, e := s.EnrollSyncAgent(u, "another-host", "old")
			if e != nil {
				t.Fatal(e)
			}
			in.AgentID = b.ID
		}},
		{"paused", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			if e := s.ControlSyncAgent(u, a.ID, "pause"); e != nil {
				t.Fatal(e)
			}
		}},
		{"revoked", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			if e := s.ControlSyncAgent(u, a.ID, "revoke"); e != nil {
				t.Fatal(e)
			}
		}},
		{"reader", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, u)
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"disabled", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE users SET disabled_at=1 WHERE id=?`, u)
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"old inventory", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE environment_scans SET scanned_at=?`, time.Now().Add(-15*24*time.Hour).UnixMilli())
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"incomplete inventory", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE environment_scans SET complete=0`)
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"unsupported scope", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE environments SET agent='reet' WHERE id=?`, in.EnvironmentID)
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"legacy codex", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE environments SET agent='codex' WHERE id=?`, in.EnvironmentID)
			if e != nil {
				t.Fatal(e)
			}
		}},
		{"relative project", func(t *testing.T, s *Store, u int64, a SyncAgent, in *SkillContentTargetWrite) {
			_, e := s.db.Exec(`UPDATE environments SET project='relative' WHERE id=?`, in.EnvironmentID)
			if e != nil {
				t.Fatal(e)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, u, a, in, _ := contentTargetFixture(t)
			tc.change(t, s, u, a, &in)
			if _, e := s.SaveSkillContentTargetFor(u, in); e == nil {
				t.Fatal("unsafe pin accepted")
			}
			var n int
			_ = s.db.QueryRow(`SELECT count(*) FROM skill_content_targets`).Scan(&n)
			if n != 0 {
				t.Fatal("partial pin written")
			}
		})
	}
}
func TestSkillContentTargetIsolationCascadeAndReenrollment(t *testing.T) {
	for _, action := range []string{"scope", "enrollment", "account"} {
		t.Run(action, func(t *testing.T) {
			s, u, a, in, b := contentTargetFixture(t)
			p, e := s.SaveSkillContentTargetFor(u, in)
			if e != nil {
				t.Fatal(e)
			}
			other, e := s.CreateUser("pin-other@example.test", "Other", "member", "hash")
			if e != nil {
				t.Fatal(e)
			}
			if _, e = s.SkillContentTargetFor(other.ID, p.ID); !errors.Is(e, ErrSkillContentTargetMissing) {
				t.Fatal("foreign read", e)
			}
			if e = s.DeleteSkillContentTargetFor(other.ID, p.ID, p.Version); !errors.Is(e, ErrSkillContentTargetMissing) {
				t.Fatal("foreign delete", e)
			}
			if all, e := s.SkillContentTargetsFor(other.ID); e != nil || len(all) != 0 {
				t.Fatal("foreign list", all, e)
			}
			switch action {
			case "scope":
				e = s.ForgetEnvironmentFor(u, in.EnvironmentID)
			case "enrollment":
				e = s.ControlSyncAgent(u, a.ID, "revoke")
			case "account":
				if e = s.TransferOwner(u, other.ID); e != nil {
					t.Fatal(e)
				}
				e = s.DeleteUser(u)
			}
			if e != nil {
				t.Fatal(e)
			}
			var count int
			_ = s.db.QueryRow(`SELECT count(*) FROM skill_content_targets`).Scan(&count)
			if count != 0 {
				t.Fatal("orphaned intent")
			}
			if action == "enrollment" {
				replacement, _, e := s.EnrollSyncAgent(u, a.Host, "new")
				if e != nil || replacement.ID == a.ID {
					t.Fatal(replacement, e)
				}
				if _, e = s.SaveSkillContentTargetFor(u, in); e == nil {
					t.Fatal("old enrollment reused")
				}
				if _, e = s.PrivateSkillRevisionFor(u, b.Bundle.ID, b.Revision.ID); e != nil {
					t.Fatal("revocation deleted private bytes", e)
				}
			}
		})
	}
}
func TestSkillContentTargetTransactionRollbackAndConcurrentCAS(t *testing.T) {
	s, u, _, in, b := contentTargetFixture(t)
	p, e := s.SaveSkillContentTargetFor(u, in)
	if e != nil {
		t.Fatal(e)
	}
	next, e := s.SavePrivateSkillRevisionFor(u, b.Bundle.ID, privateWrite("pin_next_revision_01", b.Revision.ID, "next"))
	if e != nil {
		t.Fatal(e)
	}
	in.RevisionID = next.Revision.ID
	in.ExpectedVersion = &p.Version
	if _, e = s.db.Exec(`CREATE TRIGGER reject_pin BEFORE UPDATE ON skill_content_targets BEGIN SELECT RAISE(ABORT,'injected'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveSkillContentTargetFor(u, in); e == nil {
		t.Fatal("injected write succeeded")
	}
	actual, e := s.SkillContentTargetFor(u, p.ID)
	if e != nil || !reflect.DeepEqual(actual, p) {
		t.Fatal("rollback failed", actual, e)
	}
	if _, e = s.db.Exec(`DROP TRIGGER reject_pin`); e != nil {
		t.Fatal(e)
	}
	second, e := Open(s.dir)
	if e != nil {
		t.Fatal(e)
	}
	defer second.Close()
	barrier := make(chan struct{})
	result := make(chan error, 2)
	var wg sync.WaitGroup
	for i, st := range []*Store{s, second} {
		wg.Add(1)
		go func(index int, db *Store) {
			defer wg.Done()
			<-barrier
			copy := in
			if index == 1 {
				copy.Mode = "managed"
			}
			_, e := db.SaveSkillContentTargetFor(u, copy)
			result <- e
		}(i, st)
	}
	close(barrier)
	wg.Wait()
	close(result)
	success, conflicts := 0, 0
	for e := range result {
		if e == nil {
			success++
		} else if errors.Is(e, ErrSkillContentTargetConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("concurrent overwrite", success, conflicts)
	}
}
func TestSkillContentTargetMigrationReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "db")
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	// Recreate an old schema by dropping only the new optional table. Existing
	// private revisions and inventories must survive both migration and reopen.
	u, _ := s.DefaultOwner()
	b, e := s.SavePrivateSkillRevisionFor(u, "", privateWrite("pin_migration_bundle_01", "", "keep"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec(`DROP TABLE skill_content_targets`); e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if all, e := s.SkillContentTargetsFor(u); e != nil || len(all) != 0 {
		t.Fatal(all, e)
	}
	if _, e = s.PrivateSkillRevisionFor(u, b.Bundle.ID, b.Revision.ID); e != nil {
		t.Fatal("old revision lost", e)
	}
}
func TestSkillContentTargetCapacity(t *testing.T) {
	s, u, _, in, _ := contentTargetFixture(t)
	if e := s.withTx(func(tx *sql.Tx) error {
		for i := 0; i < MaxSkillContentTargets; i++ {
			_, e := tx.Exec(`INSERT INTO skill_content_targets(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,mode,version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("%048x", i+1), u, in.EnvironmentID, in.AgentID, fmt.Sprintf("fixture-%d", i), in.BundleID, in.RevisionID, "additive", strings.Repeat("c", 48), now())
			if e != nil {
				return e
			}
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveSkillContentTargetFor(u, in); !errors.Is(e, ErrSkillContentTargetCapacity) {
		t.Fatal("capacity not bounded", e)
	}
}

func TestSkillContentTargetExplicitRevisionRollbackAndNoOp(t *testing.T) {
	s, u, _, in, first := contentTargetFixture(t)
	second, e := s.SavePrivateSkillRevisionFor(u, first.Bundle.ID, privateWrite("pin_explicit_rollback_01", first.Revision.ID, "new"))
	if e != nil {
		t.Fatal(e)
	}
	in.RevisionID = second.Revision.ID
	p, e := s.SaveSkillContentTargetFor(u, in)
	if e != nil {
		t.Fatal(e)
	}
	in.RevisionID = first.Revision.ID
	in.ExpectedVersion = &p.Version
	rolled, e := s.SaveSkillContentTargetFor(u, in)
	if e != nil || rolled.Version == p.Version || rolled.RevisionID != first.Revision.ID {
		t.Fatal("explicit rollback unavailable", rolled, e)
	}
	in.ExpectedVersion = &rolled.Version
	same, e := s.SaveSkillContentTargetFor(u, in)
	if e != nil || !reflect.DeepEqual(same, rolled) {
		t.Fatal("unchanged intent invalidated previews", same, e)
	}
	// Rollback changes the desired pin, never the immutable library head.
	b, e := s.PrivateSkillBundleFor(u, first.Bundle.ID)
	if e != nil || b.Head != second.Revision.ID {
		t.Fatal("rollback rewound library", b, e)
	}
}
