package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

var ErrGafferGoalRepeat = errors.New("Gaffer repeated the same worker instruction twice. Review the working conversation before continuing.")

// Configuration revisions change only with user controls. Runtime observations
// have their own timestamps and cannot silently reauthorise a queued instruction.
type GafferGoal struct {
	OwnerID          int64  `json:"-"`
	ID               string `json:"id"`
	ConversationID   string `json:"conversationId"`
	SessionID        string `json:"sessionId"`
	Revision         int64  `json:"revision"`
	Text             string `json:"text"`
	State            string `json:"state"`
	Note             string `json:"note"`
	ProviderID       string `json:"providerId"`
	ProviderRevision int64  `json:"providerRevision"`
	Model            string `json:"model"`
	ContextRevision  int64  `json:"contextRevision"`
	WorkerRevision   int64  `json:"workerRevision"`
	MaxReviews       int    `json:"maxReviews"`
	UsedReviews      int    `json:"usedReviews"`
	ExpiresAt        int64  `json:"expiresAt"`
	CreatedAt        int64  `json:"createdAt"`
	UpdatedAt        int64  `json:"updatedAt"`
	NextAt           int64  `json:"nextAt"`
	LastSeq          int64  `json:"lastSeq"`
	LastObservedAt   int64  `json:"lastObservedAt"`
	LastReviewedAt   int64  `json:"lastReviewedAt"`
	ActiveTurnID     string `json:"activeTurnId,omitempty"`
	PendingID        string `json:"pendingId,omitempty"`
	Summary          string `json:"summary,omitempty"`
	InputTokens      int    `json:"inputTokens"`
	OutputTokens     int    `json:"outputTokens"`
	UsageReported    bool   `json:"usageReported"`
	// WaitingOnWorker marks a goal that needs the person only because the
	// working model is waiting for their answer or approval. The supervisor
	// keeps looking at it and resumes it once that wait is over.
	WaitingOnWorker bool `json:"waitingOnWorker,omitempty"`
	// ResumedAt is when the person last resumed the goal. An idle working
	// model is measured from then, not from a review made before the pause.
	ResumedAt int64 `json:"resumedAt,omitempty"`
	// ReviewFailed marks a goal that needs the person because Gaffer's own
	// review did not complete. Resuming it tries the review again.
	ReviewFailed bool `json:"reviewFailed,omitempty"`
	// NoStep marks a goal that needs the person only because Gaffer's review
	// chose no next step: it neither instructed the working model, asked the
	// person something, nor handed the stage over.
	NoStep bool `json:"noStep,omitempty"`
}

func (s *Store) migrateGafferGoals() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_goals(
 conversation_id TEXT PRIMARY KEY REFERENCES gaffer_conversations(id) ON DELETE CASCADE,
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,data TEXT NOT NULL CHECK(json_valid(data)));
 CREATE INDEX IF NOT EXISTS gaffer_goals_due ON gaffer_goals(json_extract(data,'$.state'),json_extract(data,'$.nextAt'));`)
	return err
}
func goalRow(q contextQuerier, uid int64, conversation string) (GafferGoal, error) {
	var g GafferGoal
	var raw string
	err := q.QueryRow(`SELECT data FROM gaffer_goals WHERE user_id=? AND conversation_id=?`, uid, conversation).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrNotFound
	}
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(raw), &g)
	g.OwnerID = uid
	return g, err
}
func saveGoal(tx *sql.Tx, g GafferGoal) error {
	g.UpdatedAt = now()
	raw, err := json.Marshal(g)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO gaffer_goals(conversation_id,user_id,data) VALUES(?,?,?) ON CONFLICT(conversation_id) DO UPDATE SET user_id=excluded.user_id,data=excluded.data`, g.ConversationID, g.OwnerID, string(raw))
	return err
}
func (s *Store) GafferGoalFor(uid int64, c string) (GafferGoal, error) {
	if _, err := s.GafferConversationFor(uid, c); err != nil {
		return GafferGoal{}, err
	}
	return goalRow(s.db, uid, c)
}
func goalAuthority(q contextQuerier, g GafferGoal) error {
	if g.State != "active" || g.ExpiresAt <= now() {
		return ErrNotFound
	}
	if err := workerAllowed(q, g.OwnerID, "gaffer", g.ConversationID, g.SessionID, g.WorkerRevision); err != nil {
		return err
	}
	w, err := workerGrant(q, g.OwnerID, "gaffer", g.ConversationID, g.SessionID)
	if err != nil {
		return err
	}
	if w.ProviderID != g.ProviderID || w.ProviderRevision != g.ProviderRevision || w.Model != g.Model {
		return ErrNotFound
	}
	grant, err := scanContextGrant(q.QueryRow(`SELECT `+contextColumns+` FROM context_grants WHERE user_id=? AND kind='gaffer' AND subject=? AND session_id=?`, g.OwnerID, g.ConversationID, g.SessionID), g.SessionID)
	if err != nil {
		return err
	}
	if grant.Revision != g.ContextRevision || !grant.History || !grant.Text || grant.ProviderID != g.ProviderID || grant.ProviderRevision != g.ProviderRevision || grant.Model != g.Model {
		return ErrNotFound
	}
	var allowed bool
	err = q.QueryRow(`SELECT EXISTS(SELECT 1 FROM gaffer_providers WHERE user_id=? AND id=? AND revision=? AND enabled=1 AND allow_sessions=1 AND allow_code=1)`, g.OwnerID, g.ProviderID, g.ProviderRevision).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrNotFound
	}
	return nil
}
func checkGoal(q contextQuerier, uid int64, c, id string, rev int64, turn string) error {
	g, err := goalRow(q, uid, c)
	if err != nil {
		return err
	}
	if g.ID != id || g.Revision != rev || turn != "" && g.ActiveTurnID != turn {
		return ErrNotFound
	}
	return goalAuthority(q, g)
}
func (s *Store) CheckGafferGoal(uid int64, c, id string, rev int64, turn string) error {
	return checkGoal(s.db, uid, c, id, rev, turn)
}
func removeGoalQueue(tx *sql.Tx, g GafferGoal) error {
	_, err := tx.Exec(`UPDATE terminal_chat_queue SET state='removed',text='',images='[]',note='Gaffer follow-up stopped or changed.',revision=revision+1 WHERE user_id=? AND state='queued' AND origin='companion' AND id IN(SELECT r.queue_id FROM worker_message_receipts r JOIN gaffer_turns t ON t.conversation_id=r.subject AND t.id=r.turn_id WHERE r.user_id=? AND r.kind='gaffer' AND r.subject=? AND t.goal_id=?)`, g.OwnerID, g.OwnerID, g.ConversationID, g.ID)
	return err
}
func (s *Store) SaveGafferGoal(uid int64, expected int64, g GafferGoal) (GafferGoal, error) {

	err := s.withTx(func(tx *sql.Tx) error { return saveGafferGoal(tx, uid, expected, &g) })
	return g, err
}
func saveGafferGoal(tx *sql.Tx, uid int64, expected int64, g *GafferGoal) error {
	if !validGafferID(g.ID) || len(g.Text) > 2000 || strings.TrimSpace(g.Text) == "" || !utf8.ValidString(g.Text) || strings.ContainsRune(g.Text, 0) || g.MaxReviews < 1 || g.MaxReviews > 48 || g.ExpiresAt <= now() || g.ExpiresAt > now()+7*24*60*60*1000 {
		return ErrGafferConflict
	}
	if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
		return err
	}
	c, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, g.ConversationID))
	if err != nil {
		return err
	}
	g.OwnerID = uid
	g.SessionID = c.SessionID
	old, err := goalRow(tx, uid, c.ID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if old.Revision != expected {
		return ErrGafferConflict
	}
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_goals WHERE user_id=? AND json_extract(data,'$.state')='active' AND conversation_id!=?`, uid, c.ID).Scan(&count); err != nil {
		return err
	}
	if count >= 10 {
		return ErrGafferChatBusy
	}
	if old.ID != "" {
		if err = removeGoalQueue(tx, old); err != nil {
			return err
		}
	}
	g.Revision = old.Revision + 1
	g.State = "active"
	g.CreatedAt = now()
	g.NextAt = now()
	g.UsageReported = true
	if err = goalAuthority(tx, *g); err != nil {
		return err
	}
	return saveGoal(tx, *g)
}
func (s *Store) ControlGafferGoal(uid int64, c string, expected int64, action string, pins GafferGoal) (GafferGoal, error) {
	var g GafferGoal
	err := s.withTx(func(tx *sql.Tx) error { return controlGafferGoal(tx, uid, c, expected, action, pins, &g) })
	return g, err
}
func controlGafferGoal(tx *sql.Tx, uid int64, c string, expected int64, action string, pins GafferGoal, g *GafferGoal) error {
	if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
		return err
	}
	if err := workerSubject(tx, uid, "gaffer", c, pins.SessionID, false); err != nil {
		return err
	}
	var err error
	*g, err = goalRow(tx, uid, c)
	if err != nil {
		return err
	}
	if g.Revision != expected {
		return ErrGafferConflict
	}
	if err = removeGoalQueue(tx, *g); err != nil {
		return err
	}
	g.Revision++
	g.ActiveTurnID = ""
	g.PendingID = ""
	g.Note = ""
	g.WaitingOnWorker = false
	g.ReviewFailed = false
	g.NoStep = false
	switch action {
	case "pause":
		g.State = "paused"
		g.Note = "You paused Gaffer follow-up. The working model keeps running."
	case "stop":
		g.State = "stopped"
		g.Note = "You stopped this goal."
	case "complete":
		g.State = "completed"
		g.Note = "You marked this goal complete."
	case "resume":
		if g.State == "stopped" || g.State == "completed" || g.ExpiresAt <= now() || g.UsedReviews >= g.MaxReviews {
			return ErrGafferConflict
		}
		var active int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM gaffer_goals WHERE user_id=? AND conversation_id!=? AND json_extract(data,'$.state')='active'`, uid, c).Scan(&active); err != nil {
			return err
		}
		if active >= 10 {
			return ErrGafferChatBusy
		}
		g.State = "active"
		g.ProviderID = pins.ProviderID
		g.ProviderRevision = pins.ProviderRevision
		g.Model = pins.Model
		g.ContextRevision = pins.ContextRevision
		g.WorkerRevision = pins.WorkerRevision
		g.LastSeq = pins.LastSeq
		g.NextAt = now()
		g.ResumedAt = now()
		if err = goalAuthority(tx, *g); err != nil {
			return err
		}
	default:
		return ErrGafferConflict
	}
	return saveGoal(tx, *g)
}
func (s *Store) DueGafferGoals(at int64) ([]GafferGoal, error) {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_goals WHERE (json_extract(data,'$.state')='active' OR (json_extract(data,'$.state')='needs_user' AND json_extract(data,'$.waitingOnWorker'))) AND json_extract(data,'$.nextAt')<=? ORDER BY json_extract(data,'$.nextAt'),conversation_id LIMIT 50`, at)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferGoal{}
	for rows.Next() {
		var uid int64
		var raw string
		if err = rows.Scan(&uid, &raw); err != nil {
			return nil, err
		}
		var g GafferGoal
		if err = json.Unmarshal([]byte(raw), &g); err != nil {
			return nil, err
		}
		g.OwnerID = uid
		out = append(out, g)
	}
	return out, rows.Err()
}
func (s *Store) ObserveGafferGoal(g GafferGoal, state, note string, next int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, g.OwnerID); err != nil {
			return err
		}
		cur, err := goalRow(tx, g.OwnerID, g.ConversationID)
		if err != nil {
			return err
		}
		if cur.ID != g.ID || cur.Revision != g.Revision || cur.State != "active" || cur.ActiveTurnID != "" {
			return ErrGafferConflict
		}
		cur.State = state
		cur.Note = note
		cur.NextAt = next
		cur.WaitingOnWorker = false
		if state != "active" {
			if err = removeGoalQueue(tx, cur); err != nil {
				return err
			}
		}
		return saveGoal(tx, cur)
	})
}

// HoldGafferGoalForWorker records that the working model is waiting for the
// person. The goal needs them, and stays on the supervisor's list so it can
// carry on once they have answered.
func (s *Store) HoldGafferGoalForWorker(g GafferGoal, note string, next int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, g.OwnerID); err != nil {
			return err
		}
		cur, err := goalRow(tx, g.OwnerID, g.ConversationID)
		if err != nil {
			return err
		}
		held := cur.State == "needs_user" && cur.WaitingOnWorker
		if cur.ID != g.ID || cur.Revision != g.Revision || cur.ActiveTurnID != "" || cur.State != "active" && !held {
			return ErrGafferConflict
		}
		if !held {
			if err = removeGoalQueue(tx, cur); err != nil {
				return err
			}
		}
		cur.State, cur.Note, cur.NextAt, cur.WaitingOnWorker = "needs_user", note, next, true
		return saveGoal(tx, cur)
	})
}

// EndGafferGoalHold stops the supervisor looking at a held goal that now needs
// the person for another reason.
func (s *Store) EndGafferGoalHold(g GafferGoal, note string) error {
	return s.withTx(func(tx *sql.Tx) error {
		cur, err := goalRow(tx, g.OwnerID, g.ConversationID)
		if err != nil {
			return err
		}
		if cur.ID != g.ID || cur.Revision != g.Revision || cur.State != "needs_user" || !cur.WaitingOnWorker {
			return ErrGafferConflict
		}
		cur.Note, cur.WaitingOnWorker = note, false
		return saveGoal(tx, cur)
	})
}

// ReleaseGafferGoalHold makes a goal held for the working model active again,
// within the account's limit of active goals. Nothing else about it changes:
// what Gaffer has already reviewed stays reviewed.
func (s *Store) ReleaseGafferGoalHold(g GafferGoal, note string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, g.OwnerID); err != nil {
			return err
		}
		cur, err := goalRow(tx, g.OwnerID, g.ConversationID)
		if err != nil {
			return err
		}
		if cur.ID != g.ID || cur.Revision != g.Revision || cur.State != "needs_user" || !cur.WaitingOnWorker {
			return ErrGafferConflict
		}
		var active int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_goals WHERE user_id=? AND conversation_id!=? AND json_extract(data,'$.state')='active'`, g.OwnerID, g.ConversationID).Scan(&active); err != nil {
			return err
		}
		if active >= 10 {
			return ErrGafferChatBusy
		}
		cur.State, cur.Note, cur.NextAt, cur.WaitingOnWorker = "active", note, now(), false
		return saveGoal(tx, cur)
	})
}

// Budget and review reservation commit in the same transaction as the turn.
func admitGoalReview(tx *sql.Tx, uid int64, t GafferTurn) error {
	g, err := goalRow(tx, uid, t.ConversationID)
	if err != nil {
		return err
	}
	if g.ID != t.GoalID || g.Revision != t.GoalRevision || g.ActiveTurnID != "" || g.UsedReviews >= g.MaxReviews {
		return ErrGafferConflict
	}
	if err = goalAuthority(tx, g); err != nil {
		return err
	}
	if t.WorkerGrantRevision != g.WorkerRevision || t.Context == nil || t.Context.GrantRevision != g.ContextRevision {
		return ErrGafferConflict
	}
	g.ActiveTurnID = t.ID
	g.UsedReviews++
	g.LastSeq = t.Context.LastSeq
	g.LastObservedAt = t.Context.ReadAt
	g.NextAt = now() + 60_000
	return saveGoal(tx, g)
}
func goalTurnAllowed(q contextQuerier, uid int64, c, turn string, active bool) error {
	var id string
	var rev int64
	if err := q.QueryRow(`SELECT goal_id,goal_revision FROM gaffer_turns WHERE conversation_id=? AND id=?`, c, turn).Scan(&id, &rev); err != nil {
		return err
	}
	if id == "" {
		return nil
	}
	required := ""
	if active {
		required = turn
	}
	return checkGoal(q, uid, c, id, rev, required)
}
func goalSummary(text string) string {
	if len(text) > 2000 {
		text = text[:2000]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return text
}
func (s *Store) FinishGafferGoal(uid int64, c, id string, rev int64, turnID string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
			return err
		}
		g, err := goalRow(tx, uid, c)
		if err != nil {
			return err
		}
		if g.ID != id || g.Revision != rev || g.ActiveTurnID != turnID {
			return ErrGafferConflict
		}
		t, err := scanGafferTurn(tx.QueryRow(`SELECT `+gafferTurnColumns+` FROM gaffer_turns WHERE conversation_id=? AND id=?`, c, turnID))
		if err != nil {
			return err
		}
		g.ActiveTurnID = ""
		g.Summary = goalSummary(t.Answer)
		g.LastReviewedAt = t.FinishedAt
		g.InputTokens += t.InputTokens
		g.OutputTokens += t.OutputTokens
		g.UsageReported = g.UsageReported && t.UsageReported
		g.NextAt = now() + 60_000
		switch {
		case g.ExpiresAt <= now():
			g.State = "limit"
			g.Note = "The follow-up time limit was reached."
		case t.State != "completed":
			g.State = "needs_user"
			g.ReviewFailed = true
			g.Note = "The Gaffer review was interrupted, cancelled or failed. It will not be repeated automatically."
			if reason := strings.TrimSpace(t.Error); reason != "" {
				if len(reason) > 300 {
					reason = strings.ToValidUTF8(reason[:300], "") + "…"
				}
				g.Note = "Gaffer’s review did not complete (" + t.State + "): " + reason + " It will not be repeated automatically; resume the goal to try again."
			}
		case t.GoalNeedsUser:
			g.State = "needs_user"
			g.Note = "Gaffer needs your decision. Read its latest review."
		default:
			m, err := scanWorkerMessage(tx.QueryRow(`SELECT `+workerMessageSelect+` FROM worker_message_receipts r LEFT JOIN terminal_chat_queue q ON q.user_id=r.user_id AND q.session_id=r.session_id AND q.id=r.queue_id WHERE r.user_id=? AND r.kind='gaffer' AND r.subject=? AND r.session_id=? AND r.id=?`, uid, c, g.SessionID, t.ID))
			if err == nil {
				g.PendingID = m.ID
			} else if !errors.Is(err, ErrNotFound) {
				return err
			}
			g.Note = "Waiting for new worker progress."
		}
		if g.State != "active" {
			if err = removeGoalQueue(tx, g); err != nil {
				return err
			}
		}
		return saveGoal(tx, g)
	})
}

// Only metadata is read here; projected text is assembled later under its grant.
func (s *Store) GafferWorkerProgress(uid int64, session string) (int64, error) {
	if err := contextSession(s.db, uid, session); err != nil {
		return 0, err
	}
	var seq int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM terminal_chat_events WHERE user_id=? AND session_id=? AND json_extract(event,'$.kind') IN('agent','tool','status')`, uid, session).Scan(&seq)
	return seq, err
}

func (s *Store) GafferWorkerHalted(uid int64, session string) (bool, error) {
	var state string
	err := s.db.QueryRow(`SELECT COALESCE(json_extract(event,'$.state'),'') FROM terminal_chat_events WHERE user_id=? AND session_id=? AND json_extract(event,'$.kind')='status' ORDER BY seq DESC LIMIT 1`, uid, session).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	switch state {
	case "cancelled", "stopped", "error", "failed", "ended":
		return true, nil
	}
	return false, nil
}

func (s *Store) RecoverGafferGoals() error {
	rows, err := s.db.Query(`SELECT user_id,data FROM gaffer_goals WHERE json_extract(data,'$.activeTurnId') IS NOT NULL`)
	if err != nil {
		return err
	}
	var goals []GafferGoal
	for rows.Next() {
		var uid int64
		var raw string
		if err = rows.Scan(&uid, &raw); err != nil {
			break
		}
		var g GafferGoal
		if err = json.Unmarshal([]byte(raw), &g); err != nil {
			break
		}
		g.OwnerID = uid
		goals = append(goals, g)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, g := range goals {
		if err = s.FinishGafferGoal(g.OwnerID, g.ConversationID, g.ID, g.Revision, g.ActiveTurnID); err != nil && !errors.Is(err, ErrGafferConflict) {
			return err
		}
	}
	return nil
}

// MarkGafferGoalNoStep rewords a goal that needs the person because its
// review ended without a next step. It changes nothing else, and nothing at
// all when the goal has moved on since.
func (s *Store) MarkGafferGoalNoStep(uid int64, c, id, note string) error {
	return s.withTx(func(tx *sql.Tx) error {
		g, err := goalRow(tx, uid, c)
		if err != nil {
			return err
		}
		if g.ID != id || g.State != "needs_user" || g.ActiveTurnID != "" || g.WaitingOnWorker || g.ReviewFailed {
			return ErrGafferConflict
		}
		g.NoStep, g.Note = true, note
		return saveGoal(tx, g)
	})
}
