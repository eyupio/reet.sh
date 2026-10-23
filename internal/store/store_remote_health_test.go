package store

import (
	"errors"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestRemoteHealthRevisionGenerationRetentionAndIsolation(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	at := time.Now().Truncate(time.Minute).Add(20 * time.Second)
	in := RemoteHealthMinute{a.ID, at.Truncate(time.Minute).UnixMilli(), c.Generation, 1, terminalproto.HealthSummary{SchemaVersion: 1, Scope: "host", CPUPercent: terminalproto.Aggregate{Valid: 1, Min: 20, Max: 20, Sum: 20}, RTTMillis: []float64{42}}}
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); err != nil {
		t.Fatal(err)
	}
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); err != nil {
		t.Fatal("duplicate summary", err)
	}
	conflict := in
	conflict.Summary.InputBytes = 10
	if err := s.saveRemoteHealthMinuteAt(uid, conflict, at); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("same revision changed", err)
	}
	in.Revision = 2
	in.Summary.InputBytes = 100
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); err != nil {
		t.Fatal(err)
	}
	old := in
	old.Revision = 1
	if err := s.saveRemoteHealthMinuteAt(uid, old, at); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("older revision overwrote metrics", err)
	}
	for _, delta := range []time.Duration{-time.Minute, time.Minute} {
		bad := in
		bad.Minute += delta.Milliseconds()
		if err := s.saveRemoteHealthMinuteAt(uid, bad, at); err == nil {
			t.Fatal("manufactured historical/future metrics")
		}
	}
	admin, err := s.CreateUser("health-admin@example.com", "Admin", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.saveRemoteHealthMinuteAt(admin.ID, in, at); !errors.Is(err, ErrRemoteNotFound) {
		t.Fatal("cross-owner health write", err)
	}
	if got, err := s.RemoteHealthMinutesFor(admin.ID, a.ID); err != nil || len(got) != 0 {
		t.Fatal("cross-owner health read", err)
	}
	// A fresh connection replaces the current minute segment; counters are not
	// added to those of a dead process, and an old writer remains fenced out.
	c2, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), c.Capabilities)
	if err != nil {
		t.Fatal(err)
	}
	fresh := in
	fresh.Generation = c2.Generation
	fresh.Revision = 1
	fresh.Summary.InputBytes = 3
	if err := s.saveRemoteHealthMinuteAt(uid, fresh, at); err != nil {
		t.Fatal(err)
	}
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); !errors.Is(err, ErrTerminalConflict) {
		t.Fatal("old connection wrote health", err)
	}
	got, err := s.RemoteHealthMinutesFor(uid, a.ID)
	if err != nil || len(got) != 1 || got[0].Summary.InputBytes != 3 {
		t.Fatalf("bad segmented summary: %+v %v", got, err)
	}
	// At most one record per installation/minute, even across reconnects. Expired
	// rows are pruned on write and by the independent scheduler entry point.
	if _, err := s.db.Exec(`INSERT INTO remote_health_summaries(agent_id,user_id,minute,generation,revision,summary) VALUES(?,?,?,?,1,'{}')`, a.ID, uid, at.Add(-25*time.Hour).Truncate(time.Minute).UnixMilli(), c2.Generation); err != nil {
		t.Fatal(err)
	}
	fresh.Revision++
	if err := s.saveRemoteHealthMinuteAt(uid, fresh, at); err != nil {
		t.Fatal(err)
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM remote_health_summaries`).Scan(&count)
	if count != 1 {
		t.Fatal("expired metrics retained", count)
	}
	if _, err := s.db.Exec(`UPDATE remote_health_summaries SET minute=?`, time.Now().Add(-26*time.Hour).Truncate(time.Minute).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := s.PruneRemoteHealth(); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM remote_health_summaries`).Scan(&count)
	if count != 0 {
		t.Fatal("idle prune failed")
	}
	if err := s.saveRemoteHealthMinuteAt(uid, fresh, at); err != nil {
		t.Fatal(err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "revoke"); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM remote_health_summaries`).Scan(&count)
	if count != 0 {
		t.Fatal("revoked health retained")
	}
}

func TestHealthIndependentOfTerminalConsent(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	at := time.Now()
	caps := c.Capabilities
	caps.TerminalEnabled = false
	caps.Readiness = terminalproto.DisabledLocally
	c, err := s.BeginRemoteConnectionFor(uid, a.ID, syncID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	in := RemoteHealthMinute{a.ID, at.Truncate(time.Minute).UnixMilli(), c.Generation, 1, terminalproto.HealthSummary{SchemaVersion: 1, Scope: "host"}}
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); err != nil {
		t.Fatal("health required terminal opt-in", err)
	}
	if err := s.ControlSyncAgent(uid, a.ID, "pause"); err != nil {
		t.Fatal(err)
	}
	in.Revision++
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); err != nil {
		t.Fatal("sync pause disabled monitoring", err)
	}
	caps.HealthEnabled = false
	c, err = s.BeginRemoteConnectionFor(uid, a.ID, syncID(), caps)
	if err != nil {
		t.Fatal(err)
	}
	in.Generation = c.Generation
	in.Revision = 1
	if err := s.saveRemoteHealthMinuteAt(uid, in, at); !errors.Is(err, ErrTerminalNotReady) {
		t.Fatal("health opt-out ignored", err)
	}
}
