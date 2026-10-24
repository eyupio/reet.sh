package store

import (
	"errors"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestTerminalArchiveClosedStatesAndAuthorization(t *testing.T) {
	for _, state := range []terminalproto.SessionState{terminalproto.Creating, terminalproto.Running, terminalproto.EndRequested, terminalproto.Ended, terminalproto.Exited, terminalproto.Lost} {
		t.Run(string(state), func(t *testing.T) {
			s, uid, a, c := terminalFixture(t)
			session := createTerminal(t, s, uid, a, c)
			if state != terminalproto.Creating {
				if err := s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Running, nil); err != nil {
					t.Fatal(err)
				}
				if state == terminalproto.EndRequested || state == terminalproto.Ended {
					if _, err := s.RequestTerminalEndFor(uid, a.ID, session.ID, syncID()); err != nil {
						t.Fatal(err)
					}
				}
				if !state.ReservesQuota() {
					var exit *int
					if state == terminalproto.Exited {
						n := 0
						exit = &n
					}
					if err := s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, state, exit); err != nil {
						t.Fatal(err)
					}
				}
			}
			other, err := s.CreateUser("archive-other@example.test", "Other", "admin", "hash")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{session.ID, syncID()} {
				if err := s.ArchiveTerminalFor(other.ID, id); !errors.Is(err, ErrRemoteNotFound) {
					t.Fatal("foreign archive", err)
				}
			}
			if _, err := s.db.Exec(`UPDATE users SET role='viewer' WHERE id=?`, uid); err != nil {
				t.Fatal(err)
			}
			if err := s.ArchiveTerminalFor(uid, session.ID); !errors.Is(err, ErrRemoteNotFound) {
				t.Fatal("viewer archive", err)
			}
			if _, err := s.db.Exec(`UPDATE users SET role='owner',disabled_at=1 WHERE id=?`, uid); err != nil {
				t.Fatal(err)
			}
			if err := s.ArchiveTerminalFor(uid, session.ID); !errors.Is(err, ErrRemoteNotFound) {
				t.Fatal("disabled archive", err)
			}
			if _, err := s.db.Exec(`UPDATE users SET disabled_at=0 WHERE id=?`, uid); err != nil {
				t.Fatal(err)
			}
			err = s.ArchiveTerminalFor(uid, session.ID)
			if state.ReservesQuota() {
				if !errors.Is(err, ErrTerminalConflict) {
					t.Fatal("active archive", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			rows, err := s.TerminalSessionsFor(uid)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if state.ReservesQuota() {
				want = 1
			}
			if len(rows) != want {
				t.Fatalf("visible sessions %d, want %d", len(rows), want)
			}
			retained, err := s.TerminalSessionFor(uid, session.ID)
			if err != nil || retained.State != state {
				t.Fatalf("identity/state changed: %+v %v", retained, err)
			}
		})
	}
}

func TestTerminalArchivePersistsWithoutDeletingRetryReceipts(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	createID, endID := syncID(), syncID()
	in := terminalproto.Create{Title: "Original", Cols: 80, Rows: 24}
	session, _, err := s.ReserveTerminalCreateFor(uid, a.ID, createID, c.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RequestTerminalEndFor(uid, a.ID, session.ID, endID); err != nil {
		t.Fatal(err)
	}
	if err = s.ArchiveTerminalFor(uid, session.ID); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("pending end archived", err)
	}
	if err = s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	if err = s.ArchiveTerminalFor(uid, session.ID); err != nil {
		t.Fatal(err)
	}
	var originalTime int64
	if err = s.db.QueryRow(`SELECT archived_at FROM terminal_session_archives WHERE session_id=?`, session.ID).Scan(&originalTime); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err = reopened.ArchiveTerminalFor(uid, session.ID); err != nil {
		t.Fatal("repeat archive", err)
	}
	var archiveTime int64
	if err = reopened.db.QueryRow(`SELECT archived_at FROM terminal_session_archives WHERE session_id=?`, session.ID).Scan(&archiveTime); err != nil || archiveTime != originalTime {
		t.Fatalf("archive time changed: %d %d %v", originalTime, archiveTime, err)
	}
	again, created, err := reopened.ReserveTerminalCreateFor(uid, a.ID, createID, c.Generation, in)
	if err != nil || created || again.ID != session.ID || again.State != terminalproto.Ended {
		t.Fatalf("create retry replaced archived session: %+v %v %v", again, created, err)
	}
	again, err = reopened.RequestTerminalEndFor(uid, a.ID, session.ID, endID)
	if err != nil || again.ID != session.ID || again.State != terminalproto.Ended {
		t.Fatalf("end retry changed identity: %+v %v", again, err)
	}
	if err = reopened.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Running, nil); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("archived shell resurrected", err)
	}
	rows, err := reopened.TerminalSessionsFor(uid)
	if err != nil || len(rows) != 0 {
		t.Fatal("archived session returned to history", rows, err)
	}
	ops, err := reopened.PendingTerminalOperationsFor(uid, a.ID, c.Generation)
	if err != nil || len(ops) != 0 {
		t.Fatal("archived work requeued", ops, err)
	}
	var sessions, receipts int
	if err = reopened.db.QueryRow(`SELECT COUNT(*) FROM terminal_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err = reopened.db.QueryRow(`SELECT COUNT(*) FROM terminal_operations`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || receipts != 2 {
		t.Fatalf("deleted durable records: sessions=%d receipts=%d", sessions, receipts)
	}
}

func TestTerminalArchiveMigrationAndConcurrentEnd(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	session := createTerminal(t, s, uid, a, c)
	if err := s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	// Model a database created before the archive table existed.
	if _, err := s.db.Exec(`DROP TABLE terminal_session_archives`); err != nil {
		t.Fatal(err)
	}
	second, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	rows, err := second.TerminalSessionsFor(uid)
	if err != nil || len(rows) != 1 {
		t.Fatal("migration hid an existing session", rows, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		_, err := s.RequestTerminalEndFor(uid, a.ID, session.ID, syncID())
		if err == nil {
			err = s.ObserveTerminalStateFor(uid, a.ID, session.ID, c.Generation, terminalproto.Ended, nil)
		}
		results <- err
	}()
	go func() {
		defer workers.Done()
		<-start
		err := second.ArchiveTerminalFor(uid, session.ID)
		if errors.Is(err, ErrTerminalConflict) {
			err = nil
		}
		results <- err
	}()
	close(start)
	workers.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal("concurrent end/archive", err)
		}
	}
	if err := second.ArchiveTerminalFor(uid, session.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.TerminalSessionsFor(uid); err != nil || len(rows) != 0 {
		t.Fatal("concurrent archive not durable", rows, err)
	}
}

func TestTerminalArchiveCannotReleaseActiveQuota(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	for i := 0; i < terminalproto.MaxSessionsPerAgent; i++ {
		session := createTerminal(t, s, uid, a, c)
		if err := s.ArchiveTerminalFor(uid, session.ID); !errors.Is(err, ErrTerminalConflict) {
			t.Fatal(err)
		}
	}
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Extra", Cols: 80, Rows: 24}); !errors.Is(err, ErrTerminalCapacity) {
		t.Fatal("archive freed active quota", err)
	}
}
