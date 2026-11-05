package store

import (
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// WorktreeCandidate is a finished session's worktree that machine maintenance
// may look at. The worktree may already be gone; only the machine knows.
type WorktreeCandidate struct {
	SessionID string `json:"sessionId"`
	AgentID   string `json:"agentId"`
	Title     string `json:"title"`
	Path      string `json:"path"`
	Repo      string `json:"repo"`
	Branch    string `json:"branch"`
	EndedAt   int64  `json:"endedAt"`
}

// WorktreeCandidatesFor lists the account's finished sessions (archived ones
// included) that made a worktree not yet recorded as removed, and whose last
// change is at or before olderThan (unix ms). Live sessions are never listed.
func (s *Store) WorktreeCandidatesFor(uid int64, olderThan int64, limit int) ([]WorktreeCandidate, error) {
	rows, err := s.db.Query(`SELECT id,agent_id,title,worktree_path,worktree_repo,worktree_branch,updated_at FROM terminal_sessions t
		WHERE user_id=? AND worktree_path<>'' AND worktree_repo<>'' AND state IN ('ended','exited','lost') AND updated_at<=?
		AND NOT EXISTS(SELECT 1 FROM worktree_cleanups c WHERE c.user_id=t.user_id AND c.agent_id=t.agent_id AND c.path=t.worktree_path)
		ORDER BY updated_at,id LIMIT ?`, uid, olderThan, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorktreeCandidate{}
	for rows.Next() {
		var c WorktreeCandidate
		if err := rows.Scan(&c.SessionID, &c.AgentID, &c.Title, &c.Path, &c.Repo, &c.Branch, &c.EndedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// MarkWorktreeGone records that the worktree at this path on this machine no
// longer exists, so maintenance stops asking the machine about it.
func (s *Store) MarkWorktreeGone(uid int64, agent, path string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO worktree_cleanups(user_id,agent_id,path,removed_at) VALUES(?,?,?,?)`, uid, agent, path, time.Now().UnixMilli())
	return err
}

// MarkWorktreeGoneByRun does the same for the worktree of a run token, as a
// remove knows it: every session of this machine whose worktree folder is that
// run's. A token that is not a run's changes nothing.
func (s *Store) MarkWorktreeGoneByRun(uid int64, agent, run string) error {
	if !terminalproto.ValidRunToken(run) || agent == "" || strings.ContainsAny(run, "%_/") {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO worktree_cleanups(user_id,agent_id,path,removed_at)
		SELECT user_id,agent_id,worktree_path,? FROM terminal_sessions WHERE user_id=? AND agent_id=? AND worktree_path LIKE ?`,
		time.Now().UnixMilli(), uid, agent, "%/"+run)
	return err
}
