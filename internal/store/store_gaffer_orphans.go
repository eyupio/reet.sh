package store

import (
	"database/sql"
	"errors"
)

// pruneFactoryGoals removes the Gaffer goal and saved chat that a deleted or
// pruned outcome left behind for each of its finished stage sessions, which
// otherwise show on the Factory page as Other sessions that are Finished. uid 0
// means every account.
//
// The goal is also what hides the session from Home and Sessions once its
// outcome is gone, so a goal goes only when the session is gone, is already
// archived, or has ended with no worktree to release (it is archived here, as
// removing it from history would). A running session, or one whose worktree a
// person must still decide about, keeps its goal and stays hidden as before.
// Nothing on any machine is touched.
func pruneFactoryGoals(tx *sql.Tx, uid int64) error {
	rows, err := tx.Query(`SELECT g.conversation_id,g.user_id,COALESCE(json_extract(g.data,'$.sessionId'),'') FROM gaffer_goals g
 WHERE (?=0 OR g.user_id=?) AND substr(json_extract(g.data,'$.text'),1,9)='Outcome: '
 AND json_extract(g.data,'$.state') IN ('completed','stopped','limit')
 AND NOT EXISTS(SELECT 1 FROM gaffer_outcomes o, json_each(o.data,'$.sessions') p WHERE o.user_id=g.user_id AND (
  json_extract(p.value,'$.sessionId')=json_extract(g.data,'$.sessionId') OR json_extract(p.value,'$.conversationId')=g.conversation_id
  OR EXISTS(SELECT 1 FROM json_each(p.value,'$.retired') r WHERE r.value=g.conversation_id)
  OR EXISTS(SELECT 1 FROM json_each(p.value,'$.stages') st WHERE json_extract(st.value,'$.sessionId')=json_extract(g.data,'$.sessionId') OR json_extract(st.value,'$.conversationId')=g.conversation_id)))`, uid, uid)
	if err != nil {
		return err
	}
	type orphan struct {
		conversation, session string
		uid                   int64
	}
	var found []orphan
	for rows.Next() {
		var o orphan
		if err = rows.Scan(&o.conversation, &o.uid, &o.session); err != nil {
			rows.Close()
			return err
		}
		found = append(found, o)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, o := range found {
		var free bool
		err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM terminal_session_archives WHERE session_id=? AND user_id=?)
 OR NOT EXISTS(SELECT 1 FROM terminal_sessions WHERE id=? AND user_id=?)
 OR EXISTS(SELECT 1 FROM terminal_sessions t WHERE t.id=? AND t.user_id=? AND (t.worktree_path='' OR EXISTS(SELECT 1 FROM worktree_cleanups c WHERE c.user_id=t.user_id AND c.agent_id=t.agent_id AND c.path=t.worktree_path)))`,
			o.session, o.uid, o.session, o.uid, o.session, o.uid).Scan(&free)
		if err != nil {
			return err
		}
		if !free {
			continue
		}
		if o.session != "" {
			// A running session cannot be archived: its goal stays, as the marker that hides it.
			switch err = archiveTerminalTx(tx, o.uid, o.session); {
			case errors.Is(err, ErrTerminalConflict):
				continue
			case err != nil && !errors.Is(err, ErrRemoteNotFound):
				return err
			}
		}
		if _, err = tx.Exec(`DELETE FROM gaffer_conversations WHERE id=? AND user_id=?`, o.conversation, o.uid); err != nil {
			return err
		}
	}
	return nil
}

// pruneFactoryGoalsAtStart clears what outcomes deleted before goals were
// removed with them left behind. It is safe to run on every start.
func (s *Store) pruneFactoryGoalsAtStart() error {
	return s.withTx(func(tx *sql.Tx) error { return pruneFactoryGoals(tx, 0) })
}
