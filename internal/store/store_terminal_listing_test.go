package store

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A Factory stage session is listed only while an unarchived outcome still
// references it. Its goal survives an archived or deleted outcome, so the
// read-side rule, not the client, decides what Home and Sessions show.
func TestTerminalSessionsForListingHidesArchivedAndDeletedFactorySessions(t *testing.T) {
	st, uid, agent, caps := terminalFixture(t)
	n := 0
	next := func() string { n++; return fmt.Sprintf("%032x", n) }
	session := func() string {
		t.Helper()
		s, created, err := st.ReserveTerminalCreateFor(uid, agent.ID, next(), caps.Generation, terminalproto.Create{Title: "Session", Cols: 80, Rows: 24})
		if err != nil || !created {
			t.Fatal(created, err)
		}
		return s.ID
	}
	goal := func(sessionID, text string) {
		t.Helper()
		conversation := next()
		if _, err := st.CreateGafferConversationFor(uid, GafferConversation{ID: conversation, SessionID: sessionID, ProviderID: next()}); err != nil {
			t.Fatal(err)
		}
		if err := st.withTx(func(tx *sql.Tx) error {
			return saveGoal(tx, GafferGoal{OwnerID: uid, ID: next(), ConversationID: conversation, SessionID: sessionID, Text: text, State: "active"})
		}); err != nil {
			t.Fatal(err)
		}
	}
	outcome := func(sessionID, stageSessionID string) GafferOutcome {
		t.Helper()
		o, err := st.CreateGafferOutcomeFor(uid, GafferOutcome{ID: next(), Objective: "Ship it", Sessions: []GafferOutcomeSession{{Key: "a", Title: "Part", SessionID: sessionID, Stages: []GafferOutcomeStage{{Kind: "code", SessionID: stageSessionID}}}}})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	archive := func(o GafferOutcome, at int64) GafferOutcome {
		t.Helper()
		o, err := st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *GafferOutcome) bool {
			o.ArchivedAt = at
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	listed := func(id string) bool {
		t.Helper()
		list, err := st.TerminalSessionsForListing(uid)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == id {
				return true
			}
		}
		return false
	}
	unfiltered := func(id string) bool {
		t.Helper()
		list, err := st.TerminalSessionsFor(uid)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.ID == id {
				return true
			}
		}
		return false
	}

	live := session()
	goal(live, "Outcome: Ship it")
	outcome(live, "")

	archived := session()
	goal(archived, "Outcome: Ship it")
	archivedOutcome := archive(outcome(archived, ""), 1_800_000_000_000)

	deleted := session()
	goal(deleted, "Outcome: Ship it")
	gone := outcome(deleted, "")
	if _, err := st.UpdateGafferOutcomeFor(uid, gone.ID, gone.Revision, func(o *GafferOutcome) bool {
		o.State, o.Sessions[0].State = "settled", "done"
		return true
	}); err != nil {
		t.Fatal(err)
	}
	gone, err := st.GafferOutcomeFor(uid, gone.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteGafferOutcomeFor(uid, gone.ID, gone.Revision); err != nil {
		t.Fatal(err)
	}

	ordinary := session()
	plainGoal := session()
	goal(plainGoal, "Refactor: tidy it")
	stageReferenced := session()
	goal(stageReferenced, "Outcome: Ship it")
	outcome("", stageReferenced)

	if !listed(live) || !unfiltered(live) {
		t.Fatal("live outcome session hidden")
	}
	if listed(archived) || !unfiltered(archived) {
		t.Fatal("archived outcome session listed")
	}
	// The archived outcome still names the session after its goal is pruned.
	if _, err := st.db.Exec(`DELETE FROM gaffer_goals WHERE user_id=? AND json_extract(data,'$.sessionId')=?`, uid, archived); err != nil {
		t.Fatal(err)
	}
	if listed(archived) {
		t.Fatal("archived session listed after its goal was pruned")
	}
	if listed(deleted) || !unfiltered(deleted) {
		t.Fatal("deleted outcome session listed")
	}
	if !listed(ordinary) || !listed(plainGoal) {
		t.Fatal("ordinary or non-Factory session hidden")
	}
	if !listed(stageReferenced) {
		t.Fatal("stage-referenced session hidden")
	}

	// Restoring the outcome brings its session back.
	archive(archivedOutcome, 0)
	if !listed(archived) {
		t.Fatal("restored outcome session hidden")
	}
	if !listed(live) {
		t.Fatal("restoring one outcome hid another")
	}
}
