package store

import (
	"database/sql"
	"encoding/json"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// SetupGafferGoal commits the one reviewed browser action as one transaction.
// Provider maxima, this conversation's grants and its goal either all change or
// none change. Other conversations/clients receive no new grant.
func (s *Store) SetupGafferGoal(uid, conversationRevision, expected int64, g GafferGoal, resume bool) (GafferGoal, error) {
	err := s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE users SET id=id WHERE id=?`, uid); err != nil {
			return err
		}
		c, err := scanGafferConversation(tx.QueryRow(`SELECT `+gafferConversationColumns+` FROM gaffer_conversations WHERE user_id=? AND id=?`, uid, g.ConversationID))
		if err != nil {
			return err
		}
		if c.Revision != conversationRevision || c.ProviderID != g.ProviderID {
			return ErrGafferConflict
		}
		g.SessionID = c.SessionID
		if err := workerSubject(tx, uid, "gaffer", c.ID, c.SessionID, true); err != nil {
			return err
		}
		var tool, state, rawCaps, role string
		if err := tx.QueryRow(`SELECT t.tool,t.state,r.capabilities,u.role FROM terminal_sessions t JOIN remote_agent_capabilities r ON r.agent_id=t.agent_id AND r.user_id=t.user_id JOIN users u ON u.id=t.user_id WHERE t.user_id=? AND t.id=? AND u.disabled_at=0 AND u.role<>'viewer'`, uid, c.SessionID).Scan(&tool, &state, &rawCaps, &role); err != nil {
			return err
		}
		var caps terminalproto.Capabilities
		if json.Unmarshal([]byte(rawCaps), &caps) != nil {
			return ErrNotFound
		}
		_, available := caps.ChatAgent(tool)
		if !available || !caps.ACPLiveControls || state != string(terminalproto.Running) {
			return ErrNotFound
		}
		p, err := scanGaffer(tx.QueryRow(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? AND id=?`, uid, c.ProviderID))
		if err != nil {
			return err
		}
		model := c.Model
		if model == "" {
			model = p.Model
		}
		if !p.Enabled || p.Revision != g.ProviderRevision || model != g.Model || p.LocalOnly && role != "owner" && role != "admin" {
			return ErrGafferConflict
		}
		sharing, err := scanContextGrant(tx.QueryRow(`SELECT `+contextColumns+` FROM context_grants WHERE user_id=? AND kind='gaffer' AND subject=? AND session_id=?`, uid, c.ID, c.SessionID), c.SessionID)
		if err != nil {
			return err
		}
		worker, err := workerGrant(tx, uid, "gaffer", c.ID, c.SessionID)
		if err != nil {
			return err
		}
		if sharing.Revision != g.ContextRevision || worker.Revision != g.WorkerRevision {
			return ErrGafferConflict
		}
		if !p.AllowSessions || !p.AllowCode {
			p.Revision++
			if _, err := tx.Exec(`UPDATE gaffer_providers SET allow_sessions=1,allow_code=1,revision=?,updated_at=? WHERE user_id=? AND id=?`, p.Revision, now(), uid, p.ID); err != nil {
				return err
			}
		}
		changed := false
		if !sharing.History || !sharing.Text || sharing.ProviderID != p.ID || sharing.ProviderRevision != p.Revision || sharing.Model != model {
			sharing.Revision++
			changed = true
			if _, err := tx.Exec(`INSERT INTO context_grants(user_id,kind,subject,session_id,status,history,text,revision,provider_id,provider_revision,model,updated_at) VALUES(?,'gaffer',?,?,?,1,1,?,?,?,?,?) ON CONFLICT(user_id,kind,subject,session_id) DO UPDATE SET status=excluded.status,history=1,text=1,revision=excluded.revision,provider_id=excluded.provider_id,provider_revision=excluded.provider_revision,model=excluded.model,updated_at=excluded.updated_at`, uid, c.ID, c.SessionID, sharing.Status && p.AllowStatus, sharing.Revision, p.ID, p.Revision, model, now()); err != nil {
				return err
			}
		}
		if !worker.Enabled || worker.ProviderID != p.ID || worker.ProviderRevision != p.Revision || worker.Model != model {
			worker.Revision++
			changed = true
			if _, err := tx.Exec(`INSERT INTO worker_message_grants VALUES(?,'gaffer',?,?,1,?,?,?,?) ON CONFLICT(user_id,kind,subject,session_id) DO UPDATE SET enabled=1,revision=excluded.revision,provider_id=excluded.provider_id,provider_revision=excluded.provider_revision,model=excluded.model`, uid, c.ID, c.SessionID, worker.Revision, p.ID, p.Revision, model); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE terminal_chat_queue SET state='removed',text='',images='[]',note='Worker messaging permission reviewed.',revision=revision+1 WHERE user_id=? AND state='queued' AND origin='companion' AND id IN(SELECT queue_id FROM worker_message_receipts WHERE user_id=? AND kind='gaffer' AND subject=? AND session_id=?)`, uid, uid, c.ID, c.SessionID); err != nil {
				return err
			}
		}
		if changed {
			if _, err := tx.Exec(`UPDATE gaffer_conversations SET revision=revision+1,updated_at=? WHERE user_id=? AND id=?`, now(), uid, c.ID); err != nil {
				return err
			}
		}
		g.ProviderRevision = p.Revision
		g.ContextRevision = sharing.Revision
		g.WorkerRevision = worker.Revision
		if resume {
			if err := tx.QueryRow(`SELECT COALESCE(MAX(seq),0) FROM terminal_chat_events WHERE user_id=? AND session_id=? AND json_extract(event,'$.kind') IN ('agent','tool','status')`, uid, c.SessionID).Scan(&g.LastSeq); err != nil {
				return err
			}
			return controlGafferGoal(tx, uid, c.ID, expected, "resume", g, &g)
		}
		return saveGafferGoal(tx, uid, expected, &g)
	})
	return g, err
}
