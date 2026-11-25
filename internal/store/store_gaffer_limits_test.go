package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func limitID(n int) string { return fmt.Sprintf("%032x", n) }

// limitChat makes a conversation that last moved idle milliseconds ago. A state other than "" gives it a goal in that state.
func limitChat(t *testing.T, s *Store, uid int64, n int, state string, idle int64) {
	t.Helper()
	id := limitID(n)
	if _, err := s.CreateGafferConversationFor(uid, GafferConversation{ID: id, SessionID: fmt.Sprintf("session-%d", n), ProviderID: strings.Repeat("a", 32)}); err != nil {
		t.Fatal(err)
	}
	if state != "" {
		goal := GafferGoal{ID: id, ConversationID: id, OwnerID: uid, SessionID: fmt.Sprintf("session-%d", n), Revision: 1, State: state, MaxReviews: 12, ExpiresAt: now() + 3600000}
		if err := s.withTx(func(tx *sql.Tx) error { return saveGoal(tx, goal) }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.db.Exec(`UPDATE gaffer_conversations SET updated_at=? WHERE id=?`, now()-idle, id); err != nil {
		t.Fatal(err)
	}
	if state != "" {
		// The goal was last touched when the conversation was.
		if _, err := s.db.Exec(`UPDATE gaffer_goals SET data=json_set(data,'$.updatedAt',?) WHERE conversation_id=?`, now()-idle, id); err != nil {
			t.Fatal(err)
		}
	}
}

func limitUser(t *testing.T, s *Store, email string) int64 {
	t.Helper()
	u, err := s.CreateUser(email, "Owner", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// Each pipeline stage takes a conversation and only Remove gave one back, so a busy account filled up and its
// next stage started unfollowed. A full account now reuses the place of its oldest conversation whose goal is over
// for good and has been left alone for a day. Chats with no goal, goals that can still go on and recent ones stay.
func TestAFullAccountReusesTheOldestConversationWhoseGoalIsOver(t *testing.T) {
	s := newStore(t)
	uid := limitUser(t, s, "full@example.test")
	const day = 24 * 60 * 60 * 1000
	limitChat(t, s, uid, 1, "", 9*day)          // a plain chat, however old, is never removed
	limitChat(t, s, uid, 2, "stopped", 10*60e3) // over, but touched ten minutes ago
	limitChat(t, s, uid, 3, "limit", 9*day)     // out of reviews, the person may still renew it
	limitChat(t, s, uid, 4, "active", 9*day)
	limitChat(t, s, uid, 5, "completed", 5*day) // the oldest one that is over
	limitChat(t, s, uid, 6, "stopped", 2*day)
	// The conversation is the oldest of all, but its goal was stopped just now: the day starts from when it closed.
	limitChat(t, s, uid, 7, "stopped", 9*day)
	if _, err := s.db.Exec(`UPDATE gaffer_goals SET data=json_set(data,'$.updatedAt',?) WHERE conversation_id=?`, now(), limitID(7)); err != nil {
		t.Fatal(err)
	}
	for n := 8; n <= MaxGafferConversations; n++ {
		limitChat(t, s, uid, n, "", 0)
	}
	next := func(n int) error {
		_, err := s.CreateGafferConversationFor(uid, GafferConversation{ID: limitID(n), SessionID: fmt.Sprintf("session-%d", n), ProviderID: strings.Repeat("a", 32)})
		return err
	}
	if err := next(101); err != nil {
		t.Fatal("a full account could not start a conversation", err)
	}
	if _, err := s.GafferConversationFor(uid, limitID(5)); !errors.Is(err, ErrNotFound) {
		t.Fatal("the oldest finished conversation was not the one reused", err)
	}
	if _, err := s.GafferGoalFor(uid, limitID(5)); !errors.Is(err, ErrNotFound) {
		t.Fatal("its goal stayed behind", err)
	}
	if err := next(102); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GafferConversationFor(uid, limitID(6)); !errors.Is(err, ErrNotFound) {
		t.Fatal("the next finished conversation was not reused", err)
	}
	if err := next(103); !errors.Is(err, ErrGafferChatFull) {
		t.Fatal("nothing left to reuse must still say the account is full", err)
	}
	for _, n := range []int{1, 2, 3, 4, 7} {
		if _, err := s.GafferConversationFor(uid, limitID(n)); err != nil {
			t.Fatal("a conversation that must stay was removed", n, err)
		}
	}
}

// limitGoal saves a goal on conversation n (made if it is not there) after change has shaped it.
func limitGoal(t *testing.T, s *Store, uid int64, n int, state string, change func(*GafferGoal)) GafferGoal {
	t.Helper()
	limitChat(t, s, uid, n, state, 0)
	g, err := s.GafferGoalFor(uid, limitID(n))
	if err != nil {
		t.Fatal(err)
	}
	if change != nil {
		change(&g)
		if err = s.withTx(func(tx *sql.Tx) error { return saveGoal(tx, g) }); err != nil {
			t.Fatal(err)
		}
		if g, err = s.GafferGoalFor(uid, limitID(n)); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

// The limit of active goals has one error of its own, and every way a goal becomes active again honours it.
func TestTheActiveGoalLimitIsEnforcedWhereGoalsStartAndReturn(t *testing.T) {
	s := newStore(t)
	uid := limitUser(t, s, "goals@example.test")
	for n := 1; n <= MaxActiveGafferGoals; n++ {
		limitGoal(t, s, uid, n, "active", nil)
	}
	if n, err := s.ActiveGafferGoalsFor(uid); err != nil || n != MaxActiveGafferGoals {
		t.Fatal("active goals", n, err)
	}
	limitChat(t, s, uid, 11, "", 0)
	_, err := s.SaveGafferGoal(uid, 0, GafferGoal{ID: limitID(11), ConversationID: limitID(11), Text: "Ship it.", MaxReviews: 12, ExpiresAt: now() + 3600000})
	if !errors.Is(err, ErrGafferGoalLimit) {
		t.Fatal("an eleventh goal was started", err)
	}
	held := limitGoal(t, s, uid, 12, "needs_user", func(g *GafferGoal) { g.WaitingOnWorker = true })
	watched := limitGoal(t, s, uid, 13, "needs_user", nil)
	if err = s.ReleaseGafferGoalHold(held, "Back."); !errors.Is(err, ErrGafferGoalLimit) {
		t.Fatal("a held goal went active past the limit", err)
	}
	if err = s.ReleaseGafferGoalWatch(watched, "Back."); !errors.Is(err, ErrGafferGoalLimit) {
		t.Fatal("a watched goal went active past the limit", err)
	}
	// Room appears when one of the ten is over, and both then come back.
	stopped := limitGoal(t, s, uid, 1, "", func(g *GafferGoal) { g.State = "stopped" })
	if stopped.State != "stopped" {
		t.Fatal("setup", stopped)
	}
	if err = s.ReleaseGafferGoalHold(held, "Back."); err != nil {
		t.Fatal("room was free but a held goal could not return", err)
	}
	if err = s.ReleaseGafferGoalWatch(watched, "Back."); !errors.Is(err, ErrGafferGoalLimit) {
		t.Fatal("the room was already taken by the first one", err)
	}
}

// An unchanged "nothing new" observation must not make the goal look freshly updated, and a changed one must.
func TestAnUnchangedGoalObservationOnlyMovesTheNextLook(t *testing.T) {
	s := newStore(t)
	uid := limitUser(t, s, "observe@example.test")
	g := limitGoal(t, s, uid, 1, "active", nil)
	if err := s.ObserveGafferGoal(g, "active", "Waiting for new worker progress.", 100); err != nil {
		t.Fatal(err)
	}
	first, _ := s.GafferGoalFor(uid, g.ConversationID)
	time.Sleep(3 * time.Millisecond)
	if err := s.ObserveGafferGoal(g, "active", "Waiting for new worker progress.", 200); err != nil {
		t.Fatal(err)
	}
	second, _ := s.GafferGoalFor(uid, g.ConversationID)
	if second.NextAt != 200 || second.UpdatedAt != first.UpdatedAt || second.Revision != first.Revision {
		t.Fatal("an unchanged observation rewrote the goal", first, second)
	}
	time.Sleep(3 * time.Millisecond)
	if err := s.ObserveGafferGoal(g, "active", "The working model is busy.", 300); err != nil {
		t.Fatal(err)
	}
	third, _ := s.GafferGoalFor(uid, g.ConversationID)
	if third.NextAt != 300 || third.Note != "The working model is busy." || third.UpdatedAt <= second.UpdatedAt {
		t.Fatal("a changed observation was not saved as an update", second, third)
	}
}

// A goal that is over stays over: a stale tab's pause or complete cannot turn it into one that resume can revive.
func TestPauseAndCompleteRefuseAGoalThatIsOver(t *testing.T) {
	st, uid, session := queueFixture(t)
	cid := limitID(1)
	if _, err := st.CreateGafferConversationFor(uid, GafferConversation{ID: cid, SessionID: session, ProviderID: strings.Repeat("a", 32)}); err != nil {
		t.Fatal(err)
	}
	goal := GafferGoal{ID: cid, ConversationID: cid, OwnerID: uid, SessionID: session, Revision: 1, State: "stopped", MaxReviews: 12, ExpiresAt: now() + 3600000}
	if err := st.withTx(func(tx *sql.Tx) error { return saveGoal(tx, goal) }); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pause", "complete"} {
		if _, err := st.ControlGafferGoal(uid, cid, 1, action, GafferGoal{SessionID: session}); !errors.Is(err, ErrGafferConflict) {
			t.Fatal(action, "was accepted on a stopped goal", err)
		}
	}
	if g, err := st.GafferGoalFor(uid, cid); err != nil || g.State != "stopped" || g.Revision != 1 {
		t.Fatal("a refused control changed the goal", g, err)
	}
}

func limitOutcome(t *testing.T, s *Store, uid int64, n int, change func(*GafferOutcome)) {
	t.Helper()
	id := limitID(n)
	o, err := s.CreateGafferOutcomeFor(uid, GafferOutcome{ID: id, Objective: "Ship it.", Sessions: []GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it.", Stages: []GafferOutcomeStage{{Kind: "code", Tool: "claude", Backup: &GafferStageAgent{Tool: "codex"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateGafferOutcomeFor(uid, id, o.Revision, func(o *GafferOutcome) bool {
		o.State = "settled"
		change(o)
		return true
	}); err != nil {
		t.Fatal(err)
	}
}

func outcomeIDs(rows []GafferOutcome, err error) string {
	if err != nil {
		return err.Error()
	}
	ids := make([]string, 0, len(rows))
	for _, o := range rows {
		ids = append(ids, o.ID[len(o.ID)-1:])
	}
	return strings.Join(ids, ",")
}

// The loop that decides, rebalances and reports runs every ten seconds, so each list holds only what it can act on.
func TestTheOutcomeListsHoldOnlyWhatTheirLoopCanActOn(t *testing.T) {
	s := newStore(t)
	uid := limitUser(t, s, "lists@example.test")
	followed := func(o *GafferOutcome) { o.Sessions[0].State = "following" }
	finished := func(o *GafferOutcome) { o.Sessions[0].State = "done" }
	limitOutcome(t, s, uid, 1, func(o *GafferOutcome) { followed(o); o.Automation = true })                       // 1: automated, followed
	limitOutcome(t, s, uid, 2, func(o *GafferOutcome) { finished(o); o.Automation = true })                       // 2: automated, nothing to decide
	limitOutcome(t, s, uid, 3, func(o *GafferOutcome) { followed(o); o.Automation = true; o.ArchivedAt = now() }) // 3: put away
	limitOutcome(t, s, uid, 4, func(o *GafferOutcome) { finished(o); o.AutoReport = true })                       // 4: reports, finished
	limitOutcome(t, s, uid, 5, func(o *GafferOutcome) { finished(o); o.AutoReport = true; o.ArchivedAt = now() }) // 5: reports, put away
	if got := outcomeIDs(s.AutomatedGafferOutcomes()); got != "1" {
		t.Fatal("automated outcomes:", got)
	}
	if got := outcomeIDs(s.BackedGafferOutcomes()); got != "1" {
		t.Fatal("backed outcomes:", got)
	}
	if got := outcomeIDs(s.AutoReportGafferOutcomes()); got != "4" {
		t.Fatal("outcomes to report on:", got)
	}
}

// A request ID asked for twice is answered by the outcome it started, and one another account used is a conflict, not a database error.
func TestAReusedOutcomeRequestIDIsAnAnswerOrAConflict(t *testing.T) {
	s := newStore(t)
	mine, other := limitUser(t, s, "mine@example.test"), limitUser(t, s, "other@example.test")
	plan := GafferOutcome{ID: limitID(7), Objective: "Ship it.", Sessions: []GafferOutcomeSession{{Key: "a", Title: "Part", Prompt: "Do it."}}}
	first, err := s.CreateGafferOutcomeFor(mine, plan)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateGafferOutcomeFor(mine, plan)
	if err != nil || again.ID != first.ID || again.OwnerID != mine {
		t.Fatal("a repeat did not return the outcome it started", again, err)
	}
	if _, err = s.CreateGafferOutcomeFor(other, plan); !errors.Is(err, ErrGafferConflict) {
		t.Fatal("another account's id was not a conflict", err)
	}
	if _, err = s.GafferOutcomeFor(other, plan.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("the other account can see it", err)
	}
}
