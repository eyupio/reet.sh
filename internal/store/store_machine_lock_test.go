package store

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A machine that says it is locked is refused at reservation as locked, and
// still as not ready for the callers that handle that.
func TestALockedMachineIsNotReservedAndSaysWhy(t *testing.T) {
	s, uid, a, c := terminalFixture(t)
	locked := c.Capabilities
	locked.TerminalEnabled, locked.Readiness, locked.Locked = false, terminalproto.DisabledLocally, true
	if err := s.UpdateRemoteCapabilitiesFor(uid, a.ID, c.InstanceID, c.Generation, locked); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.ReserveTerminalCreateFor(uid, a.ID, syncID(), c.Generation, terminalproto.Create{Title: "Shell", Cols: 80, Rows: 24})
	if !errors.Is(err, ErrMachineLocked) || !errors.Is(err, ErrTerminalNotReady) || err.Error() != terminalproto.LockedSentence {
		t.Fatalf("reservation on a locked machine: %v", err)
	}
}

// A skill report from a locked machine blocks its installs as allow-apply
// off does.
func TestReportBlockedByTheLock(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	item := savedWrittenSkill(t, s, uid, "seo-audit", false)
	a := reportAgent(t, s, uid, "elitebook")
	if err := s.RecordSkillReportFor(uid, a, SkillReport{Version: "v1", Blocked: "locked"}, reportT0); err != nil {
		t.Fatal(err)
	}
	h := healthOf(t, s, uid, item.id, reportT0)
	if h.State != "blocked" || h.Pending != 1 || !reflect.DeepEqual(h.Hosts, []string{"elitebook"}) {
		t.Fatalf("blocked: %+v", h)
	}
	if m := syncMachineOf(t, s, uid, "elitebook", reportT0.Add(time.Second)); m.State != "blocked" {
		t.Fatalf("machine: %+v", m)
	}
}

// A machine may refuse a remote import because it is locked; any other
// unknown reason is still refused.
func TestARemoteImportMayFailBecauseTheMachineIsLocked(t *testing.T) {
	s := newStore(t)
	uid, _ := s.DefaultOwner()
	if err := s.FailRemoteImportFor(uid, "agent", "missing", "instance", "machine_locked"); errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatal("machine_locked was refused as a reason")
	}
	if err := s.FailRemoteImportFor(uid, "agent", "missing", "instance", "bogus"); !errors.Is(err, ErrPrivateSkillInvalid) {
		t.Fatalf("an unknown reason was taken: %v", err)
	}
	if got := remoteImportFailure["machine_locked"]; got == "" {
		t.Fatal("machine_locked has no words")
	}
}
