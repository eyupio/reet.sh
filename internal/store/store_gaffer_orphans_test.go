package store

import (
	"database/sql"
	"fmt"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Deleting an outcome removes the goal and saved chat of each ended stage
// session, so they stop showing as finished Other sessions. A session still
// running keeps its goal, which is what keeps it out of Home and Sessions.
func TestDeletingAnOutcomeRemovesTheGoalsOfItsEndedSessions(t *testing.T) {
	st, uid, agent, caps := terminalFixture(t)
	n := 0
	next := func() string { n++; return fmt.Sprintf("%032x", n) }
	session := func(state string) string {
		t.Helper()
		s, _, err := st.ReserveTerminalCreateFor(uid, agent.ID, next(), caps.Generation, terminalproto.Create{Title: "Session", Cols: 80, Rows: 24})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = st.db.Exec(`UPDATE terminal_sessions SET state=? WHERE id=?`, state, s.ID); err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	goal := func(sessionID, state string) string {
		t.Helper()
		conversation := next()
		if _, err := st.CreateGafferConversationFor(uid, GafferConversation{ID: conversation, SessionID: sessionID, ProviderID: next()}); err != nil {
			t.Fatal(err)
		}
		if err := st.withTx(func(tx *sql.Tx) error {
			return saveGoal(tx, GafferGoal{OwnerID: uid, ID: next(), ConversationID: conversation, SessionID: sessionID, Text: "Outcome: Ship it", State: state})
		}); err != nil {
			t.Fatal(err)
		}
		return conversation
	}
	outcome := func(sessionIDs ...string) GafferOutcome {
		t.Helper()
		var parts []GafferOutcomeSession
		for i, id := range sessionIDs {
			parts = append(parts, GafferOutcomeSession{Key: fmt.Sprint(i), Title: "Part", SessionID: id})
		}
		o, err := st.CreateGafferOutcomeFor(uid, GafferOutcome{ID: next(), Objective: "Ship it", Sessions: parts})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	settle := func(o GafferOutcome) GafferOutcome {
		t.Helper()
		if _, err := st.UpdateGafferOutcomeFor(uid, o.ID, o.Revision, func(o *GafferOutcome) bool {
			o.State = "settled"
			for i := range o.Sessions {
				o.Sessions[i].State = "done"
			}
			return true
		}); err != nil {
			t.Fatal(err)
		}
		o, err := st.GafferOutcomeFor(uid, o.ID)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	has := func(conversation string) bool {
		t.Helper()
		var c int
		if err := st.db.QueryRow(`SELECT (SELECT COUNT(*) FROM gaffer_goals WHERE conversation_id=?)+(SELECT COUNT(*) FROM gaffer_conversations WHERE id=?)`, conversation, conversation).Scan(&c); err != nil {
			t.Fatal(err)
		}
		return c > 0
	}
	archived := func(id string) bool {
		t.Helper()
		ok, err := st.TerminalArchivedFor(uid, id)
		if err != nil {
			t.Fatal(err)
		}
		return ok
	}

	ended, running, withWorktree, kept := session("ended"), session("running"), session("ended"), session("ended")
	if _, err := st.db.Exec(`UPDATE terminal_sessions SET worktree_path='/work/tree' WHERE id=?`, withWorktree); err != nil {
		t.Fatal(err)
	}
	endedGoal, runningGoal, worktreeGoal, keptGoal := goal(ended, "stopped"), goal(running, "stopped"), goal(withWorktree, "completed"), goal(kept, "stopped")
	other := outcome(kept)
	gone := settle(outcome(ended, running, withWorktree))
	if err := st.DeleteGafferOutcomeFor(uid, gone.ID, gone.Revision); err != nil {
		t.Fatal(err)
	}
	if has(endedGoal) || !archived(ended) {
		t.Fatal("an ended session's goal and chat should go, and the session be archived")
	}
	if !has(runningGoal) || archived(running) {
		t.Fatal("a running session keeps its goal and stays unarchived")
	}
	if !has(worktreeGoal) || archived(withWorktree) {
		t.Fatal("a session with a worktree to release keeps its goal")
	}
	if !has(keptGoal) || archived(kept) {
		t.Fatal("another outcome's session was touched")
	}

	// A goal left by an earlier delete goes at the next start, once its session has ended.
	if _, err := st.db.Exec(`UPDATE terminal_sessions SET state='ended' WHERE id=?`, running); err != nil {
		t.Fatal(err)
	}
	if err := st.pruneFactoryGoalsAtStart(); err != nil {
		t.Fatal(err)
	}
	if has(runningGoal) || !archived(running) {
		t.Fatal("the start-up sweep left an ended session's goal")
	}
	if !has(keptGoal) || !has(worktreeGoal) {
		t.Fatal("the start-up sweep removed a goal it should keep")
	}
	_ = other
}
