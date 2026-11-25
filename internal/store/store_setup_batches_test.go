package store

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const setupSecretText = "SETUP-TEXT-THAT-MUST-NOT-REACH-A-BACKUP"

func setupMember(position int, a SyncAgent) SetupBatchMemberInput {
	return SetupBatchMemberInput{Position: position, AgentID: a.ID, Host: a.Host, Fingerprint: "f", Files: []SetupBatchFile{{Agent: "claude", Kind: "instructions", Name: "CLAUDE.md", State: "merge"}}}
}

func setupBatch(t *testing.T, s *Store, uid int64, request string, members ...SetupBatchMemberInput) SetupBatch {
	t.Helper()
	b, _, err := s.CreateSetupBatchFor(uid, request, SetupBatchSource{AgentID: "source", Host: "source-host", ReadAt: 42}, []byte(`[{"content":"`+setupSecretText+`"}]`), members)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSetupBatchCreateReplayAndActiveGuard(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	widening := setupMember(1, b)
	widening.Widens = true
	batch := setupBatch(t, s, uid, "setup_request_0001", setupMember(0, a), widening)
	if batch.State != SoftwareBatchPending || !batch.Widens || !batch.ContentHeld || batch.SourceHost != "source-host" || batch.ReadAt != 42 || len(batch.Members[0].Files) != 1 {
		t.Fatalf("batch: %+v", batch)
	}
	again, created, err := s.CreateSetupBatchFor(uid, "setup_request_0001", SetupBatchSource{}, nil, []SetupBatchMemberInput{setupMember(0, a)})
	if err != nil || created || again.ID != batch.ID {
		t.Fatalf("replay: %+v %v %v", again, created, err)
	}
	if _, _, err := s.CreateSetupBatchFor(uid, "setup_request_0002", SetupBatchSource{}, []byte("[]"), []SetupBatchMemberInput{setupMember(0, a)}); !errors.Is(err, ErrSetupBatchInProgress) {
		t.Fatalf("second active batch: %v", err)
	}
	// A software batch is separate: it is not held up by a setup batch.
	if _, _, err := s.CreateSoftwareBatchFor(uid, "software_request_1", "opencode", []SoftwareBatchMemberInput{pendingMember(0, a)}); err != nil {
		t.Fatalf("software batch beside a setup batch: %v", err)
	}
}

func TestSetupBatchRunsStopsSkipsAndDeletesContentWhenDone(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch := setupBatch(t, s, uid, "setup_request_0003", setupMember(0, a), setupMember(1, b))
	if items, err := s.SetupBatchItems(uid, batch.ID); err != nil || !bytes.Contains(items, []byte(setupSecretText)) {
		t.Fatalf("items: %s %v", items, err)
	}
	if _, err := s.SetupBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetupBatchMemberState(uid, batch.ID, 0, SoftwareMemberFailed, "changed", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.SetupBatchFor(uid, batch.ID); got.State != SoftwareBatchStopped || !got.ContentHeld {
		t.Fatalf("stopped: %+v", got)
	}
	if _, err := s.RetrySetupBatchMember(uid, batch.ID, 1); !errors.Is(err, ErrSetupBatchMemberNotFailed) {
		t.Fatalf("retry of a pending member: %v", err)
	}
	if err := s.SkipSetupBatchMember(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetupBatchMemberStart(uid, batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetupBatchMemberState(uid, batch.ID, 1, SoftwareMemberApplied, "", "restore-fleet-x-1"); err != nil {
		t.Fatal(err)
	}
	done, _ := s.SetupBatchFor(uid, batch.ID)
	if done.State != SoftwareBatchDone || done.ContentHeld || done.Members[0].State != SoftwareMemberSkipped || done.Members[0].Code != "changed" || done.Members[1].Journal != "restore-fleet-x-1" {
		t.Fatalf("done: %+v", done)
	}
	if _, err := s.SetupBatchItems(uid, batch.ID); !errors.Is(err, ErrSetupBatchContentGone) {
		t.Fatalf("content after done: %v", err)
	}
	// Undo is allowed once, on an applied member only, after the batch ended.
	if err := s.SetSetupBatchMemberUndone(uid, batch.ID, 0); !errors.Is(err, ErrSetupBatchNotUndoable) {
		t.Fatalf("undo a skipped member: %v", err)
	}
	if err := s.SetSetupBatchMemberUndone(uid, batch.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetupBatchMemberUndone(uid, batch.ID, 1); !errors.Is(err, ErrSetupBatchNotUndoable) {
		t.Fatalf("second undo: %v", err)
	}
}

func TestSetupBatchAbandonAndResume(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch := setupBatch(t, s, uid, "setup_request_0004", setupMember(0, a), setupMember(1, b))
	if _, err := s.SetupBatchMemberStart(uid, batch.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AbandonSetupBatch(uid, batch.ID); !errors.Is(err, ErrSetupBatchRunning) {
		t.Fatalf("abandon while applying: %v", err)
	}
	// A controller stop leaves member 0 running; resume puts it back.
	pending, err := s.ResumeSetupBatches()
	if err != nil || len(pending) != 1 || pending[0].BatchID != batch.ID {
		t.Fatalf("resume: %+v %v", pending, err)
	}
	if got, _ := s.SetupBatchFor(uid, batch.ID); got.Members[0].State != SoftwareMemberPending {
		t.Fatalf("after resume: %+v", got.Members[0])
	}
	abandoned, err := s.AbandonSetupBatch(uid, batch.ID)
	if err != nil || abandoned.State != SoftwareBatchAbandoned || abandoned.ContentHeld || abandoned.Members[1].Code != "abandoned" {
		t.Fatalf("abandon: %+v %v", abandoned, err)
	}
}

func TestSetupBatchContentIsDeletedAfterADay(t *testing.T) {
	s, uid, a, b := softwareBatchFixture(t)
	batch := setupBatch(t, s, uid, "setup_request_0005", setupMember(0, a), setupMember(1, b))
	if n, err := s.PurgeSetupBatchContent(now()); err != nil || n != 0 {
		t.Fatalf("a fresh batch lost its content: %d %v", n, err)
	}
	if n, err := s.PurgeSetupBatchContent(now() + SetupBatchContentTTL + 1); err != nil || n != 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
	got, _ := s.SetupBatchFor(uid, batch.ID)
	if got.State != SoftwareBatchAbandoned || got.ContentHeld || got.Members[0].Code != "expired" {
		t.Fatalf("expired: %+v", got)
	}
	// While a machine applies, only the text goes: its outcome still settles the batch.
	other := setupBatch(t, s, uid, "setup_request_0006", setupMember(0, a), setupMember(1, b))
	if _, err := s.SetupBatchMemberStart(uid, other.ID, 0); err != nil {
		t.Fatal(err)
	}
	if n, err := s.PurgeSetupBatchContent(now() + SetupBatchContentTTL + 1); err != nil || n != 1 {
		t.Fatalf("purge while applying: %d %v", n, err)
	}
	if err := s.SetSetupBatchMemberState(uid, other.ID, 0, SoftwareMemberApplied, "", "restore-fleet-y-1"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.SetupBatchFor(uid, other.ID)
	if got.State != SoftwareBatchDone || got.ContentHeld || got.Members[1].Code != "expired" {
		t.Fatalf("expired while applying: %+v", got)
	}
}

// A controller backup is a VACUUM INTO copy; the copied setup text must not
// be in it, not even in freed pages.
func TestBackupCopyHoldsNoSetupText(t *testing.T) {
	s, uid, a, _ := softwareBatchFixture(t)
	batch := setupBatch(t, s, uid, "setup_request_0007", setupMember(0, a))
	dest := filepath.Join(t.TempDir(), "copy.db")
	if err := s.VacuumInto(context.Background(), dest); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(setupSecretText)) {
		t.Fatal("the backup copy holds setup text")
	}
	db, err := sql.Open("sqlite", "file:"+dest+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var batches, content int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM setup_batches),(SELECT COUNT(*) FROM setup_batch_content)`).Scan(&batches, &content); err != nil {
		t.Fatal(err)
	}
	if batches != 1 || content != 0 {
		t.Fatalf("copy: %d batches, %d content rows", batches, content)
	}
	// The live database still holds it until the batch settles.
	if _, err := s.SetupBatchItems(uid, batch.ID); err != nil {
		t.Fatal(err)
	}
}

// A restored controller has no setup text (backups never hold it), so an
// unfinished setup batch is abandoned and the report says so.
func TestRestoringAbandonsUnfinishedSetupBatches(t *testing.T) {
	path := movingFixture(t)
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	var owner int64
	if err := db.QueryRow(`SELECT id FROM users WHERE email='owner@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO setup_batches(id,user_id,source_agent_id,source_host,read_at,state,created_at,updated_at) VALUES('setup1',?,'s','studio',1,'stopped',1,1)`,
		`INSERT INTO setup_batch_members(batch_id,user_id,position,agent_id,host,state,updated_at) VALUES('setup1',?,0,'a','laptop','applied',1),('setup1',?,1,'b','desk','failed',1),('setup1',?,2,'c','den','pending',1)`,
	} {
		args := []any{owner}
		if strings.Count(q, "?") == 3 {
			args = []any{owner, owner, owner}
		}
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	ad, err := AdaptDatabaseFile(context.Background(), path, AdaptOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, path, `SELECT state FROM setup_batches`); got != "abandoned" {
		t.Fatalf("batch: %s", got)
	}
	if got := scalar(t, path, `SELECT group_concat(state||'/'||code,',') FROM (SELECT * FROM setup_batch_members ORDER BY position)`); got != "applied/,skipped/paused_by_restore,skipped/paused_by_restore" {
		t.Fatalf("members: %s", got)
	}
	if !strings.Contains(strings.Join(ad.Paused, "\n"), "setup batch setup1 (from studio): abandoned with 2 machine(s)") {
		t.Fatalf("report: %v", ad.Paused)
	}
}
