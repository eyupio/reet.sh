package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	MaxGafferConversations     = 100
	MaxGafferTurns             = 100
	MaxGafferPromptBytes       = 8000
	MaxGafferAnswerBytes       = 64 << 10
	MaxGafferConversationBytes = 512 << 10
)

var (
	ErrGafferChatBusy = errors.New("Gaffer is already answering this conversation")
	ErrGafferChatFull = errors.New("this Gaffer conversation is full; remove it to start a fresh one")
)

type GafferConversation struct {
	ID         string `json:"id"`
	OwnerID    int64  `json:"-"`
	SessionID  string `json:"sessionId"`
	ProviderID string `json:"providerId"`
	Model      string `json:"model"`
	Revision   int64  `json:"revision"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type GafferTurn struct {
	GoalID              string      `json:"goalId,omitempty"`
	GoalRevision        int64       `json:"goalRevision,omitempty"`
	GoalNeedsUser       bool        `json:"goalNeedsUser,omitempty"`
	WorkerGrantRevision int64       `json:"workerGrantRevision,omitempty"`
	Context             *ContextUse `json:"context,omitempty"`
	ID                  string      `json:"id"`
	ConversationID      string      `json:"conversationId"`
	RequestRevision     int64       `json:"requestRevision"`
	Prompt              string      `json:"prompt"`
	Answer              string      `json:"answer"`
	State               string      `json:"state"`
	ProviderID          string      `json:"providerId"`
	ProviderName        string      `json:"providerName"`
	Model               string      `json:"model"`
	Error               string      `json:"error,omitempty"`
	InputTokens         int         `json:"inputTokens"`
	OutputTokens        int         `json:"outputTokens"`
	UsageReported       bool        `json:"usageReported"`
	Cut                 bool        `json:"cut"`
	StartedAt           int64       `json:"startedAt"`
	UpdatedAt           int64       `json:"updatedAt"`
	FinishedAt          int64       `json:"finishedAt"`
	// Tools are the reet tools Gaffer used or asked to use in this turn, in
	// order. One whose mode is "ask" waits as pending for the person.
	Tools []GafferTurnTool `json:"tools,omitempty"`
}

// MaxGafferToolResultBytes bounds what a turn keeps of one tool's answer.
const MaxGafferToolResultBytes = 6 << 10

// GafferTurnTool is one call of a reet tool by Gaffer. Args is the exact
// JSON the call was made, or will be made, with. State is "ran", "failed",
// "pending" (waiting for the person), "declined" or "expired".
type GafferTurnTool struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Args   string `json:"args"`
	Mode   string `json:"mode"`
	State  string `json:"state"`
	Result string `json:"result,omitempty"`
	At     int64  `json:"at"`
}

func validGafferID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func (s *Store) migrateGafferChats() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_conversations (
 id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 session_id TEXT NOT NULL DEFAULT '',provider_id TEXT NOT NULL,model TEXT NOT NULL DEFAULT '',
 revision INTEGER NOT NULL DEFAULT 1,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,
 UNIQUE(user_id,session_id));
 CREATE TABLE IF NOT EXISTS gaffer_turns (
 conversation_id TEXT NOT NULL REFERENCES gaffer_conversations(id) ON DELETE CASCADE,
 id TEXT NOT NULL,request_revision INTEGER NOT NULL,prompt TEXT NOT NULL,answer TEXT NOT NULL DEFAULT '',
 state TEXT NOT NULL,provider_id TEXT NOT NULL,provider_name TEXT NOT NULL,model TEXT NOT NULL,
 error TEXT NOT NULL DEFAULT '',input_tokens INTEGER NOT NULL DEFAULT 0,output_tokens INTEGER NOT NULL DEFAULT 0,
 usage_reported INTEGER NOT NULL DEFAULT 0,cut INTEGER NOT NULL DEFAULT 0,
 started_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,finished_at INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(conversation_id,id));
 CREATE UNIQUE INDEX IF NOT EXISTS gaffer_one_running_turn ON gaffer_turns(conversation_id) WHERE state='running';
 CREATE TABLE IF NOT EXISTS gaffer_tool_modes (
 user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,tool TEXT NOT NULL,mode TEXT NOT NULL,updated_at INTEGER NOT NULL,
 PRIMARY KEY(user_id,tool));`)
	return err
}

const gafferConversationColumns = `id,user_id,session_id,provider_id,model,revision,created_at,updated_at`
const gafferTurnColumns = `id,conversation_id,request_revision,prompt,answer,state,provider_id,provider_name,model,error,input_tokens,output_tokens,usage_reported,cut,started_at,updated_at,finished_at,context_json,worker_grant_revision,goal_id,goal_revision,goal_needs_user,tools_json`

func scanGafferConversation(row interface{ Scan(...any) error }) (GafferConversation, error) {
	var c GafferConversation
	err := row.Scan(&c.ID, &c.OwnerID, &c.SessionID, &c.ProviderID, &c.Model, &c.Revision, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}
func scanGafferTurn(row interface{ Scan(...any) error }) (GafferTurn, error) {
	var t GafferTurn
	var contextJSON, toolsJSON string
	err := row.Scan(&t.ID, &t.ConversationID, &t.RequestRevision, &t.Prompt, &t.Answer, &t.State, &t.ProviderID, &t.ProviderName, &t.Model, &t.Error, &t.InputTokens, &t.OutputTokens, &t.UsageReported, &t.Cut, &t.StartedAt, &t.UpdatedAt, &t.FinishedAt, &contextJSON, &t.WorkerGrantRevision, &t.GoalID, &t.GoalRevision, &t.GoalNeedsUser, &toolsJSON)
	if err == nil && contextJSON != "" {
		err = json.Unmarshal([]byte(contextJSON), &t.Context)
	}
	if err == nil && toolsJSON != "" {
		err = json.Unmarshal([]byte(toolsJSON), &t.Tools)
	}
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return t, err
}
func (s *Store) GafferConversationFor(uid int64, id string) (GafferConversation, error) {
	return scanGafferConversation(s.db.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, id))
}

// GafferConversationsFor lists only owned companion metadata, including scopes
// whose worker history was archived, so their separate history can be removed.
func (s *Store) GafferConversationsFor(uid int64) ([]GafferConversation, error) {
	rows, err := s.db.Query(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? ORDER BY updated_at DESC,id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferConversation{}
	for rows.Next() {
		c, err := scanGafferConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Store) GafferConversationScopeFor(uid int64, session string) (GafferConversation, error) {
	return scanGafferConversation(s.db.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND session_id=?`, uid, session))
}

// CreateGafferConversationFor is idempotent for an account/scope. An existing
// conversation is never reconfigured by a retried create request.
func (s *Store) CreateGafferConversationFor(uid int64, c GafferConversation) (GafferConversation, error) {
	if uid <= 0 || !validGafferID(c.ID) || !validGafferID(c.ProviderID) || len(c.SessionID) > 128 || len(c.Model) > 256 {
		return c, ErrGafferConflict
	}
	var result GafferConversation
	err := s.withTx(func(tx *sql.Tx) error {
		old, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND session_id=?`, uid, c.SessionID))
		if err == nil {
			result = old
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		var n int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_conversations WHERE user_id=?`, uid).Scan(&n); err != nil {
			return err
		}
		if n >= MaxGafferConversations {
			return ErrGafferChatFull
		}
		at := now()
		_, err = tx.Exec(`INSERT INTO gaffer_conversations(id,user_id,session_id,provider_id,model,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, c.ID, uid, c.SessionID, c.ProviderID, c.Model, at, at)
		if err == nil {
			result = GafferConversation{ID: c.ID, OwnerID: uid, SessionID: c.SessionID, ProviderID: c.ProviderID, Model: c.Model, Revision: 1, CreatedAt: at, UpdatedAt: at}
		}
		return err
	})
	return result, err
}
func (s *Store) ConfigureGafferConversationFor(uid int64, id string, revision int64, provider, model string) error {
	if !validGafferID(provider) || len(model) > 256 {
		return ErrGafferConflict
	}
	return s.withTx(func(tx *sql.Tx) error {
		c, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, id))
		if err != nil {
			return err
		}
		if c.Revision != revision {
			return ErrGafferConflict
		}
		var n int
		if err = tx.QueryRow(`SELECT COUNT(*) FROM gaffer_turns WHERE conversation_id=? AND state='running'`, id).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrGafferChatBusy
		}
		_, err = tx.Exec(`UPDATE gaffer_conversations SET provider_id=?,model=?,revision=revision+1,updated_at=? WHERE id=?`, provider, model, now(), id)
		return err
	})
}
func (s *Store) GafferTurnFor(uid int64, conversation, id string) (GafferTurn, error) {
	if _, err := s.GafferConversationFor(uid, conversation); err != nil {
		return GafferTurn{}, err
	}
	return scanGafferTurn(s.db.QueryRow(`SELECT `+gafferTurnColumns+` FROM gaffer_turns WHERE conversation_id=? AND id=?`, conversation, id))
}
func (s *Store) GafferTurnsFor(uid int64, conversation string) ([]GafferTurn, error) {
	if _, err := s.GafferConversationFor(uid, conversation); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+gafferTurnColumns+` FROM gaffer_turns WHERE conversation_id=? ORDER BY request_revision,id`, conversation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	turns := []GafferTurn{}
	for rows.Next() {
		t, err := scanGafferTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, t)
	}
	return turns, rows.Err()
}

// StartGafferTurnFor stores the request receipt before any provider call. The
// same ID/body/revision returns its original outcome, even after a restart.
func (s *Store) StartGafferTurnFor(uid int64, t GafferTurn) (GafferTurn, bool, error) {
	if !validGafferID(t.ID) || !validGafferID(t.ConversationID) || !validGafferID(t.ProviderID) || t.RequestRevision < 1 || len(t.Prompt) == 0 || len(t.Prompt) > MaxGafferPromptBytes || !utf8.ValidString(t.Prompt) || strings.ContainsRune(t.Prompt, 0) {
		return t, false, ErrGafferConflict
	}
	created := false
	err := s.withTx(func(tx *sql.Tx) error {
		if t.GoalID != "" {
			if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
				return err
			}
		}
		c, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, t.ConversationID))
		if err != nil {
			return err
		}
		old, err := scanGafferTurn(tx.QueryRow(`SELECT `+gafferTurnColumns+` FROM gaffer_turns WHERE conversation_id=? AND id=?`, c.ID, t.ID))
		if err == nil {
			if old.Prompt != t.Prompt || old.RequestRevision != t.RequestRevision || (old.WorkerGrantRevision > 0) != (t.WorkerGrantRevision > 0) || old.GoalID != t.GoalID || old.GoalRevision != t.GoalRevision {
				return ErrGafferConflict
			}
			t = old
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		if c.Revision != t.RequestRevision || c.ProviderID != t.ProviderID {
			return ErrGafferConflict
		}
		var count, bytes, running int
		if err = tx.QueryRow(`SELECT COUNT(*),COALESCE(SUM(length(CAST(prompt AS BLOB))+length(CAST(answer AS BLOB))),0),COALESCE(SUM(state='running'),0) FROM gaffer_turns WHERE conversation_id=?`, c.ID).Scan(&count, &bytes, &running); err != nil {
			return err
		}
		if running > 0 {
			return ErrGafferChatBusy
		}
		if count >= MaxGafferTurns || bytes+len(t.Prompt)+MaxGafferAnswerBytes > MaxGafferConversationBytes {
			return ErrGafferChatFull
		}
		if t.GoalID != "" {
			if err = admitGoalReview(tx, uid, t); err != nil {
				return err
			}
		}
		at := now()
		t.State = "running"
		t.StartedAt = at
		t.UpdatedAt = at
		contextJSON := ""
		if t.Context != nil {
			raw, err := json.Marshal(t.Context)
			if err != nil {
				return err
			}
			contextJSON = string(raw)
		}
		_, err = tx.Exec(`INSERT INTO gaffer_turns(conversation_id,id,request_revision,prompt,state,provider_id,provider_name,model,started_at,updated_at,context_json,worker_grant_revision,goal_id,goal_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, c.ID, t.ID, t.RequestRevision, t.Prompt, t.State, t.ProviderID, t.ProviderName, t.Model, at, at, contextJSON, t.WorkerGrantRevision, t.GoalID, t.GoalRevision)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE id=?`, at, c.ID)
		created = err == nil
		return err
	})
	return t, created, err
}

// UpdateGafferTurnFor only changes a running row. A late callback cannot undo
// cancellation, restart reconciliation, deletion or a completed answer.
func (s *Store) UpdateGafferTurnFor(uid int64, t GafferTurn) error {
	switch t.State {
	case "running", "completed", "failed", "cancelled", "interrupted":
	default:
		return ErrGafferConflict
	}
	if len(t.Answer) > MaxGafferAnswerBytes || len(t.Error) > 1200 || !utf8.ValidString(t.Answer) {
		return ErrGafferConflict
	}
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, t.ConversationID)); err != nil {
			return err
		}
		at := now()
		finished := int64(0)
		if t.State != "running" {
			finished = at
		}
		toolsJSON, err := gafferToolsJSON(t.Tools)
		if err != nil {
			return err
		}
		r, err := tx.Exec(`UPDATE gaffer_turns SET answer=?,state=?,error=?,input_tokens=?,output_tokens=?,usage_reported=?,cut=?,updated_at=?,finished_at=?,goal_needs_user=?,tools_json=? WHERE conversation_id=? AND id=? AND state='running'`, t.Answer, t.State, t.Error, t.InputTokens, t.OutputTokens, t.UsageReported, t.Cut, at, finished, t.GoalNeedsUser, toolsJSON, t.ConversationID, t.ID)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrGafferConflict
		}
		if finished != 0 {
			_, err = tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE id=?`, at, t.ConversationID)
		}
		return err
	})
}
func (s *Store) DeleteGafferConversationFor(uid int64, id string, revision int64) error {
	r, err := s.db.Exec(`DELETE FROM gaffer_conversations WHERE user_id=? AND id=? AND revision=?`, uid, id, revision)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		if _, err = s.GafferConversationFor(uid, id); err != nil {
			return err
		}
		return ErrGafferConflict
	}
	return nil
}

// RecoverGafferTurns runs once when the controller service starts. It never
// calls a provider or replays an interrupted request. One controller owns a DB.
func (s *Store) RecoverGafferTurns() error {
	return s.withTx(func(tx *sql.Tx) error {
		at := now()
		if _, err := tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE id IN (SELECT conversation_id FROM gaffer_turns WHERE state='running')`, at); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE gaffer_turns SET state='interrupted',error='The controller stopped before this answer finished. It was not sent again.',updated_at=?,finished_at=? WHERE state='running'`, at, at)
		return err
	})
}

func gafferToolsJSON(tools []GafferTurnTool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	if len(tools) > 16 {
		return "", ErrGafferConflict
	}
	for i := range tools {
		if len(tools[i].Result) > MaxGafferToolResultBytes {
			tools[i].Result = strings.ToValidUTF8(tools[i].Result[:MaxGafferToolResultBytes], "") + "…"
		}
	}
	raw, err := json.Marshal(tools)
	return string(raw), err
}

// PendingGafferTurnToolFor returns the tool call of a finished turn that
// still waits for the person's answer.
func (s *Store) PendingGafferTurnToolFor(uid int64, conversation, turn, tool string) (GafferTurnTool, error) {
	t, err := s.GafferTurnFor(uid, conversation, turn)
	if err != nil {
		return GafferTurnTool{}, err
	}
	for _, item := range t.Tools {
		if item.ID == tool && item.State == "pending" && t.State != "running" {
			return item, nil
		}
	}
	return GafferTurnTool{}, ErrGafferConflict
}

// ResolveGafferTurnToolFor records what became of a pending tool call: it is
// taken from pending exactly once, so an approval cannot run a call twice.
// claim moves it to "approved" before the call is made; the result follows.
func (s *Store) ResolveGafferTurnToolFor(uid int64, conversation, turn, tool, from, state, result string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, conversation)); err != nil {
			return err
		}
		t, err := scanGafferTurn(tx.QueryRow(`SELECT `+gafferTurnColumns+` FROM gaffer_turns WHERE conversation_id=? AND id=?`, conversation, turn))
		if err != nil {
			return err
		}
		found := false
		for i := range t.Tools {
			if t.Tools[i].ID == tool && t.Tools[i].State == from && t.State != "running" {
				t.Tools[i].State, t.Tools[i].Result, t.Tools[i].At = state, result, now()
				found = true
			}
		}
		if !found {
			return ErrGafferConflict
		}
		raw, err := gafferToolsJSON(t.Tools)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE gaffer_turns SET tools_json=?,updated_at=? WHERE conversation_id=? AND id=?`, raw, now(), conversation, turn); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE id=?`, now(), conversation)
		return err
	})
}

// GafferToolModesFor returns the reet tools the person let Gaffer use: tool
// name to "ask" or "allow". A tool that is not listed is off.
func (s *Store) GafferToolModesFor(uid int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT tool,mode FROM gaffer_tool_modes WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var tool, mode string
		if err = rows.Scan(&tool, &mode); err != nil {
			return nil, err
		}
		out[tool] = mode
	}
	return out, rows.Err()
}

// SetGafferToolModesFor replaces the person's whole choice. Only "ask" and
// "allow" are stored; anything else switches the tool off.
func (s *Store) SetGafferToolModesFor(uid int64, modes map[string]string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM gaffer_tool_modes WHERE user_id=?`, uid); err != nil {
			return err
		}
		for tool, mode := range modes {
			if mode != "ask" && mode != "allow" || tool == "" || len(tool) > 100 {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO gaffer_tool_modes(user_id,tool,mode,updated_at) VALUES(?,?,?,?)`, uid, tool, mode, now()); err != nil {
				return err
			}
		}
		return nil
	})
}
