package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func terminalFixture(t *testing.T) (*Store, int64, SyncAgent, RemoteAgentCapabilities) {
	t.Helper()
	s := newStore(t)
	uid, err := s.DefaultOwner()
	if err != nil {
		t.Fatal(err)
	}
	a, c := terminalAgent(t, s, uid, "laptop")
	return s, uid, a, c
}
func terminalAgent(t *testing.T, s *Store, uid int64, host string) (SyncAgent, RemoteAgentCapabilities) {
	t.Helper()
	a, _, err := s.EnrollSyncAgent(uid, host, "test")
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), terminalproto.Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, HealthEnabled: true, Readiness: terminalproto.Ready})
	if err != nil {
		t.Fatal(err)
	}
	return a, c
}
func createTerminal(t *testing.T, s *Store, uid int64, a SyncAgent, c RemoteAgentCapabilities) TerminalSession {
	t.Helper()
	got, created, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if err != nil || !created {
		t.Fatalf("reserve: created=%v err=%v", created, err)
	}
	return got
}
func TestTerminalIdempotencyRestartAndGenerationFencing(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	request := syncID()
	in := terminalproto.Create{Title: "Build", Cols: 80, Rows: 24}
	first, created, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in)
	if err != nil || !created {
		t.Fatal(err)
	}
	// Rename changes display metadata but not the original creation parameters.
	if err := s.RenameTerminalFor(uid, first.ID, "Renamed"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	again, created, err := other.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in)
	if err != nil || created || again.ID != first.ID || again.Title != "Renamed" {
		t.Fatalf("lost acknowledgment created another session: %+v %v %v", again, created, err)
	}
	changed := in
	changed.Cols++
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, changed); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("changed request accepted", err)
	}
	c2, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), c.Capabilities)
	if err != nil || c2.Generation != c.Generation+1 {
		t.Fatal(err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c.Generation, terminalproto.Running, nil); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("stale generation accepted", err)
	}
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in); err != nil {
		t.Fatal("metadata retry depends on obsolete transport", err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c2.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	endID := syncID()
	pending, err := s.RequestTerminalEndFor(uid, a.ID, first.ID, endID)
	if err != nil || pending.State != terminalproto.EndRequested {
		t.Fatal(err)
	}
	// A delayed create ACK cannot erase a durable end request.
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c2.Generation, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.TerminalSessionFor(uid, first.ID)
	if got.State != terminalproto.EndRequested {
		t.Fatal("end request lost")
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c2.Generation, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestTerminalEndFor(uid, a.ID, first.ID, endID); err != nil {
		t.Fatal("end retry", err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c2.Generation, terminalproto.Running, nil); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("resurrected final session", err)
	}
	ops, err := s.PendingTerminalOperationsFor(uid, a.ID, c2.Generation)
	if err != nil || len(ops) != 0 {
		t.Fatalf("completed work still pending: %+v %v", ops, err)
	}
	var sessions, operations int
	s.db.QueryRow(`SELECT COUNT(*) FROM terminal_sessions`).Scan(&sessions)
	s.db.QueryRow(`SELECT COUNT(*) FROM terminal_operations`).Scan(&operations)
	if sessions != 1 || operations != 2 {
		t.Fatalf("records: sessions=%d operations=%d", sessions, operations)
	}
}
func TestTerminalConcurrentReservationAcrossDatabaseHandles(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	other, err := Open(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	// Same logical request races through independent handles: exactly one row.
	var wg sync.WaitGroup
	results := make(chan error, 40)
	ids := make(chan string, 40)
	request := syncID()
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := s
			if i%2 == 1 {
				target = other
			}
			got, _, err := target.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in)
			results <- err
			ids <- got.ID
		}(i)
	}
	wg.Wait()
	var id string
	for i := 0; i < 20; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
		got := <-ids
		if id != "" && got != id {
			t.Fatal("duplicate shell reservation")
		}
		id = got
	}
	// Distinct requests race for the seven remaining installation slots.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			target := s
			if i%2 == 1 {
				target = other
			}
			_, _, err := target.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in)
			results <- err
		}(i)
	}
	wg.Wait()
	accepted, rejected := 0, 0
	for i := 0; i < 20; i++ {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrTerminalCapacity) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 7 || rejected != 13 {
		t.Fatalf("quota race: accepted=%d rejected=%d", accepted, rejected)
	}
	// Pending offline ends continue to consume quota, final observations release it.
	if _, err := s.RequestTerminalEndFor(uid, a.ID, id, syncID()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, in); !errors.Is(err, ErrTerminalCapacity) {
		t.Fatal("pending end released quota", err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, id, c.Generation, terminalproto.Lost, nil); err != nil {
		t.Fatal(err)
	}
	createTerminal(t, s, uid, a, c)
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM terminal_sessions WHERE state IN ('creating','running','end_requested')`).Scan(&count)
	if count != 8 {
		t.Fatal(count)
	}
}

func TestEndDuringCreationSuppressesCreateDispatch(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	first := createTerminal(t, s, uid, a, c)
	second := createTerminal(t, s, uid, a, c)
	endRequest := syncID()
	if _, err := s.RequestTerminalEndFor(uid, a.ID, first.ID, endRequest); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RequestTerminalEndFor(uid, a.ID, second.ID, endRequest); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("end key rebound to another session", err)
	}
	ops, err := s.PendingTerminalOperationsFor(uid, a.ID, c.Generation)
	if err != nil || len(ops) != 2 || ops[0].Kind != "end" || ops[0].SessionID != first.ID || ops[1].Kind != "create" || ops[1].SessionID != second.ID || ops[1].Create == nil {
		t.Fatalf("unsafe dispatch order: %+v %v", ops, err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c.Generation, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	var state, code string
	if err := s.db.QueryRow(`SELECT state,result_code FROM terminal_operations WHERE session_id=? AND kind='create'`, first.ID).Scan(&state, &code); err != nil || state != "failed" || code != "not_found" {
		t.Fatal("never-created shell incorrectly acknowledged", state, code, err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, second.ID, c.Generation, terminalproto.Exited, new(int)); err != nil {
		t.Fatal(err)
	}
	one := 1
	if err := s.ObserveTerminalStateFor(uid, a.ID, second.ID, c.Generation, terminalproto.Exited, &one); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("conflicting exit result accepted", err)
	}
}
func TestTerminalAccountQuotaAndRequestScope(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	request := syncID()
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}
	for i := 0; i < 4; i++ {
		agent, caps := a, c
		if i > 0 {
			agent, caps = terminalAgent(t, s, uid, fmt.Sprintf("host-%d", i))
		}
		for j := 0; j < 8; j++ {
			key := syncID()
			if j == 0 {
				key = request
			}
			if _, created, err := s.ReserveTerminalCreateFor(uid, agent.ID, key, caps.Generation, in); err != nil || !created {
				t.Fatal(err)
			}
		}
	}
	last, lastCaps := terminalAgent(t, s, uid, "last")
	if _, _, err := s.ReserveTerminalCreateFor(uid, last.ID, request, lastCaps.Generation, in); !errors.Is(err, ErrTerminalCapacity) {
		t.Fatal("account quota exceeded", err)
	}
	member, err := s.CreateUser("separate@example.com", "Separate", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	ma, mc := terminalAgent(t, s, member.ID, "last")
	if _, created, err := s.ReserveTerminalCreateFor(member.ID, ma.ID, request, mc.Generation, in); err != nil || !created {
		t.Fatal("another account shared quota/key", err)
	}
}

func TestDefinitiveCreateFailureIsDurableAndNotRetried(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	request := syncID()
	in := terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}
	first, _, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FailTerminalCreateFor(uid, a.ID, request, c.Generation, terminalproto.Capacity); err != nil {
		t.Fatal(err)
	}
	stored, err := s.TerminalOperationFor(uid, a.ID, "create", request)
	if err != nil || stored.Code != terminalproto.Capacity || stored.State != "failed" || stored.SessionID != first.ID {
		t.Fatal("failure outcome missing", err)
	}
	if err := s.FailTerminalCreateFor(uid, a.ID, request, c.Generation, terminalproto.Capacity); err != nil {
		t.Fatal("duplicate failure", err)
	}
	if err := s.FailTerminalCreateFor(uid, a.ID, request, c.Generation, terminalproto.Denied); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("failure receipt changed", err)
	}
	again, created, err := s.ReserveTerminalCreateFor(uid, a.ID, request, c.Generation, in)
	if err != nil || created || again.ID != first.ID || again.State != terminalproto.Lost {
		t.Fatal("failed request replayed", err)
	}
	if pending, err := s.PendingTerminalOperationsFor(uid, a.ID, c.Generation); err != nil || len(pending) != 0 {
		t.Fatal("failed operation dispatched", err)
	}
	if err := s.ObserveTerminalStateFor(uid, a.ID, first.ID, c.Generation, terminalproto.Running, nil); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("failed session resurrected", err)
	}
	for i := 0; i < 8; i++ {
		createTerminal(t, s, uid, a, c)
	}
}
func TestTerminalOwnershipConsentAndRevocation(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	session := createTerminal(t, s, uid, a, c)
	admin, err := s.CreateUser("admin@example.com", "Admin", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveTerminalCreateFor(admin.ID, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("admin inherited shell", err)
	}
	if _, err := s.TerminalSessionFor(admin.ID, session.ID); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("session leaked", err)
	}
	if err := s.RenameTerminalFor(admin.ID, session.ID, "Stolen"); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal(err)
	}
	if _, err := s.RequestTerminalEndFor(admin.ID, a.ID, session.ID, syncID()); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal(err)
	}
	if _, err := s.BeginRemoteConnectionFor(admin.ID, a.ID, syncID(), c.Capabilities); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal(err)
	}
	// Same-owner SQL cannot accidentally link a session to somebody else's agent.
	if _, err := s.db.Exec(`INSERT INTO terminal_sessions(id,user_id,agent_id,title,state,created_at,updated_at) VALUES(?,?,?,'Bad','creating',1,1)`, syncID(), admin.ID, a.ID); err == nil {
		t.Fatal("mismatched FK binding accepted")
	}
	disabled := c.Capabilities
	disabled.TerminalEnabled = false
	disabled.Readiness = terminalproto.DisabledLocally
	next, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), next.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrTerminalNotReady) {
		t.Fatal("local opt-out ignored", err)
	}
	// Disabling access cannot prevent an already requested end from being durable.
	if _, err := s.RequestTerminalEndFor(uid, a.ID, session.ID, syncID()); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"remote_agent_capabilities", "terminal_sessions", "terminal_operations"} {
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("revocation left %s rows=%d err=%v", table, n, err)
		}
	}
	replacement, _ := terminalAgent(t, s, uid, "laptop")
	if replacement.ID == a.ID {
		t.Fatal("enrollment identity reused")
	}
	if _, err := s.TerminalSessionFor(uid, session.ID); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("old enrollment adopted", err)
	}
}
func TestTerminalDisabledUsersAndViewers(t *testing.T) {
	s := newStore(t)
	s.DefaultOwner()
	user, err := s.CreateUser("member@example.com", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	a, c := terminalAgent(t, s, user.ID, "host")
	session := createTerminal(t, s, user.ID, a, c)
	if err := s.UpdateUserRole(user.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TerminalSessionFor(user.ID, session.ID); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("viewer read terminal")
	}
	if _, _, err := s.ReserveTerminalCreateFor(user.ID, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("viewer created terminal", err)
	}
	if err := s.SetUserDisabled(user.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RemoteCapabilitiesFor(user.ID, a.ID); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("disabled user read capabilities", err)
	}
	if err := s.DeleteUser(user.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM terminal_sessions`).Scan(&n)
	if n != 0 {
		t.Fatal("account deletion left terminal metadata")
	}
}

func TestCapabilityRefreshCannotReenableFromStaleProcess(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	disabled := c.Capabilities
	disabled.TerminalEnabled = false
	disabled.Readiness = terminalproto.DisabledLocally
	if err := s.UpdateRemoteCapabilitiesFor(uid, a.ID, c.InstanceID, c.Generation, disabled); err != nil {
		t.Fatal(err)
	}
	got, err := s.RemoteCapabilitiesFor(uid, a.ID)
	if err != nil || got.Generation != c.Generation || got.Capabilities.TerminalEnabled {
		t.Fatal("capability refresh failed", err)
	}
	if _, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24}); !errors.Is(err, ErrTerminalNotReady) {
		t.Fatal("local disable ignored", err)
	}
	if err := s.UpdateRemoteCapabilitiesFor(uid, a.ID, syncID(), c.Generation, c.Capabilities); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("wrong process enabled access", err)
	}
	if err := s.UpdateRemoteCapabilitiesFor(uid, a.ID, c.InstanceID, c.Generation, c.Capabilities); err != nil {
		t.Fatal(err)
	}
	createTerminal(t, s, uid, a, c)
	next, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), disabled)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRemoteCapabilitiesFor(uid, a.ID, c.InstanceID, c.Generation, c.Capabilities); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("stale connection enabled access", err)
	}
	got, err = s.RemoteCapabilitiesFor(uid, a.ID)
	if err != nil || got.Generation != next.Generation || got.Capabilities.TerminalEnabled {
		t.Fatal("new process overwritten", err)
	}
}
func TestPrivateIdentityAndAdditiveMigration(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s.DefaultOwner()
	a, secret, err := s.EnrollSyncAgent(uid, "existing", "old-version")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("migration-preserved", "yes"); err != nil {
		t.Fatal(err)
	}
	// Simulate a database predating this stage, retaining all existing records.
	for _, table := range []string{"remote_health_summaries", "terminal_operations", "terminal_sessions", "remote_agent_capabilities", "private_transport_identity"} {
		if _, err := s.db.Exec(`DROP TABLE ` + table); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.AuthenticateSyncAgent(uid, a.ID, secret, "new-version"); err != nil {
		t.Fatal("enrollment migration", err)
	}
	if v, _ := s.GetSetting("migration-preserved"); v != "yes" {
		t.Fatal("setting lost")
	}
	if _, err := s.RemoteCapabilitiesFor(uid, a.ID); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("existing enrollment implicitly enabled")
	}
	if _, found, err := s.PrivateIdentity(); err != nil || found {
		t.Fatal("identity silently generated")
	}
	key, _ := sealed.NewSealer(strings.Repeat("a", 32), "test")
	cipher, err := key.SealSecret(`{"format":"reet-tailcat-v1","private":"capability-secret"}`)
	if err != nil {
		t.Fatal(err)
	}
	record := PrivateIdentityRecord{1, cipher}
	if err := s.SavePrivateIdentityIfAbsent(record); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePrivateIdentityIfAbsent(record); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePrivateIdentityIfAbsent(PrivateIdentityRecord{1, strings.Repeat("A", 64)}); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("identity was overwritten", err)
	}
	if err := s.SavePrivateIdentityIfAbsent(PrivateIdentityRecord{1, "plaintext-secret"}); err == nil {
		t.Fatal("plaintext accepted")
	}
	got, found, err := s.PrivateIdentity()
	if err != nil || !found || got != record {
		t.Fatal("identity restore", err)
	}
	wrong, _ := sealed.NewSealer(strings.Repeat("b", 32), "test")
	if _, err := wrong.OpenSecret(got.Ciphertext); err == nil {
		t.Fatal("wrong sealing key opened identity")
	}
	encoded, _ := json.Marshal(got)
	if string(encoded) != "{}" || strings.Contains(fmt.Sprintf("%+v %#v", got, got), cipher) {
		t.Fatal("identity exposed by formatting/JSON")
	}
}
