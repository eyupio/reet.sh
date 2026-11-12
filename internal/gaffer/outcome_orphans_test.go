package gaffer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// followedOutcome saves a settled outcome whose one pipeline is following the
// given session, the state a Factory outcome sits in while its agent works.
func (r *outcomeRig) followedOutcome(t *testing.T, id, session, state string) store.GafferOutcome {
	t.Helper()
	uid := r.p.UserID
	o, err := r.st.CreateGafferOutcomeFor(uid, store.GafferOutcome{ID: strings.Repeat(id, 32), Objective: "Ship it.", Sessions: []store.GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it. Do not merge.", RequestID: strings.Repeat("8", 32)}}})
	if err != nil {
		t.Fatal(err)
	}
	o, err = r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool {
		o.State, o.Sessions[0].State, o.Sessions[0].SessionID = "settled", state, session
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// liveSession starts a real running session row on the rig's machine.
func (r *outcomeRig) liveSession(t *testing.T, request string) string {
	t.Helper()
	id, _, err := r.starter.StartOutcomeSession(context.Background(), r.p.UserID, OutcomeLaunch{AgentID: r.starter.agent, RequestID: strings.Repeat(request, 32), Title: "Work", Tool: "opencode", Dir: "/work/billing", Prompt: "Do it."})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// endSession ends a session the way the person does: asks the controller to
// end it, then the machine confirms. With removeHistory the session is also
// archived as it closes, so it leaves the session list altogether.
func (r *outcomeRig) endSession(t *testing.T, session string, removeHistory bool) {
	t.Helper()
	uid := r.p.UserID
	if _, err := r.st.RequestTerminalEndFor(uid, r.starter.agent, session, session[:32], removeHistory); err != nil {
		t.Fatal(err)
	}
	if err := r.st.ObserveTerminalStateFor(uid, r.starter.agent, session, 1, terminalproto.Ended, nil); err != nil {
		t.Fatal(err)
	}
	archived, err := r.st.TerminalArchivedFor(uid, session)
	if err != nil || archived != removeHistory {
		t.Fatalf("archived=%v want %v: %v", archived, removeHistory, err)
	}
}

func TestAPipelineWhoseSessionIsGoneIsNoLongerFollowedAndItsOutcomeCanBeDeleted(t *testing.T) {
	r := outcomeFixture(t)
	uid := r.p.UserID
	missing := r.followedOutcome(t, "1", strings.Repeat("a", 48), "following")
	archivedSession := r.liveSession(t, "2")
	archived := r.followedOutcome(t, "3", archivedSession, "following")
	r.endSession(t, archivedSession, true)
	alive := r.followedOutcome(t, "4", r.liveSession(t, "5"), "following")
	ended := r.liveSession(t, "6")
	endedOutcome := r.followedOutcome(t, "7", ended, "following")
	r.endSession(t, ended, false)

	if err := r.s.DeleteOutcome(uid, missing.ID, missing.Revision); !errors.Is(err, store.ErrGafferOutcomeActive) {
		t.Fatalf("before the controller looks, the outcome is stuck: %v", err)
	}
	r.s.releaseGoneSessions()

	for _, gone := range []store.GafferOutcome{missing, archived} {
		o, err := r.s.Outcome(uid, gone.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := o.Sessions[0]; got.State != "unfollowed" || !strings.Contains(got.Note, "no longer exists") {
			t.Fatalf("a pipeline whose session is gone must stop being followed: %+v", got)
		}
		if err := r.s.DeleteOutcome(uid, o.ID, o.Revision); err != nil {
			t.Fatalf("the outcome of a gone session must be deletable: %v", err)
		}
	}
	for _, kept := range []store.GafferOutcome{alive, endedOutcome} {
		// A running session, and an ended one that can still be looked at or moved
		// on from, are the person's to decide: the controller leaves them alone.
		o, err := r.s.Outcome(uid, kept.ID)
		if err != nil || o.Sessions[0].State != "following" || o.Revision != kept.Revision {
			t.Fatalf("a session that still exists must stay followed: %+v %v", o, err)
		}
	}
}

func TestStoppingFollowingEndsAStuckOutcomeSoItCanBeArchivedAndDeleted(t *testing.T) {
	r := outcomeFixture(t)
	uid := r.p.UserID
	stuck := r.followedOutcome(t, "1", r.liveSession(t, "2"), "following")
	if _, err := r.s.StopFollowingOutcome(uid, stuck.ID, stuck.Revision-1); !errors.Is(err, store.ErrGafferOutcomeBusy) {
		t.Fatalf("stop from a stale revision: %v", err)
	}
	o, err := r.s.StopFollowingOutcome(uid, stuck.ID, stuck.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if got := o.Sessions[0]; got.State != "unfollowed" || got.SessionID != stuck.Sessions[0].SessionID || !strings.Contains(got.Note, "You stopped following") {
		t.Fatalf("pipeline after the stop: %+v", got)
	}
	if !o.Finished() {
		t.Fatalf("a stopped outcome must be finished: %+v", o)
	}
	// Stopping again changes nothing and fails nothing.
	again, err := r.s.StopFollowingOutcome(uid, o.ID, o.Revision)
	if err != nil || again.Revision != o.Revision {
		t.Fatalf("stopping twice: %+v %v", again, err)
	}
	if _, err := r.s.ArchiveOutcome(uid, o.ID, o.Revision, true); err != nil {
		t.Fatalf("archive after the stop: %v", err)
	}
	archived, _ := r.s.Outcome(uid, o.ID)
	if err := r.s.DeleteOutcome(uid, o.ID, archived.Revision); err != nil {
		t.Fatalf("delete after the stop: %v", err)
	}
}

func TestStoppingFollowingCancelsPipelinesNotYetStartedAndOutcomesStillStarting(t *testing.T) {
	r := outcomeFixture(t)
	uid := r.p.UserID
	o := r.followedOutcome(t, "1", "", "waiting")
	o, err := r.st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *store.GafferOutcome) bool { o.State = "starting"; return true })
	if err != nil {
		t.Fatal(err)
	}
	o, err = r.s.StopFollowingOutcome(uid, o.ID, o.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if o.State != "stopped" || o.Sessions[0].State != "cancelled" || !o.Finished() {
		t.Fatalf("a starting outcome stopped by the person: %+v", o)
	}
}
