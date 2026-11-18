package store

import (
	"database/sql"
	"time"
)

// A dated record of what the agents reported using, for the Usage page.
//
// terminal_session_usage keeps one running total per session and goes when the
// session is archived. terminal_usage_events keeps each report that carried
// tokens, with the time it arrived and the agent, model and machine it came
// from, so totals over a period survive the session being archived and its
// machine being forgotten or revoked. terminal_usage_sessions keeps the
// session's latest title, machine name and project as of its latest report, for
// when the session itself is gone; while it exists, its own title is shown.
// Events older than usageEventsKeep are dropped as new ones are written.

const (
	usageEventsKeep = 90 * 24 * time.Hour
	// usagePruneBatch bounds what one write deletes, so a write after a long
	// quiet spell does not stall; the rest goes with the next writes.
	usagePruneBatch = 500
)

// UsageToolTotal is what one agent reported in a range: tokens summed, the
// sessions that reported, how many of those were Factory stages' sessions,
// and the time of its first report in the range.
type UsageToolTotal struct {
	Tool          string
	Input         int64
	Output        int64
	Cached        int64
	Sessions      int
	FactoryStages int
	FirstAt       int64
}

// UsageSession is one session's reports in a range. Archived is true for a
// session that has been removed from the history, or whose machine took it
// with it; State is "" then.
type UsageSession struct {
	ID          string
	Title       string
	Tool        string
	Model       string
	AgentID     string
	Host        string
	ProjectID   *string
	Structured  bool
	State       string
	Archived    bool
	Factory     bool
	Input       int64
	Output      int64
	Cached      int64
	LastAt      int64
	ContextUsed int64
	ContextSize int64
}

// UsageGaffer is what Gaffer's own provider reported for one model in a range.
type UsageGaffer struct {
	Provider string
	Model    string
	Input    int64
	Output   int64
	Replies  int
}

// UsageReport is an account's usage in a range. Since is the time of its
// earliest kept event at all (0 for none). More says the session list was cut.
type UsageReport struct {
	Since    int64
	Tools    []UsageToolTotal
	Sessions []UsageSession
	Gaffer   []UsageGaffer
	More     bool
}

func (s *Store) migrateUsageEvents() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS terminal_usage_events(
		user_id INTEGER NOT NULL, session_id TEXT NOT NULL, agent_id TEXT NOT NULL,
		tool TEXT NOT NULL, model TEXT NOT NULL, structured INTEGER NOT NULL, at INTEGER NOT NULL,
		input_tokens INTEGER NOT NULL, output_tokens INTEGER NOT NULL, cached_tokens INTEGER NOT NULL);
	CREATE INDEX IF NOT EXISTS terminal_usage_events_when ON terminal_usage_events(user_id, at);
	CREATE TABLE IF NOT EXISTS terminal_usage_sessions(
		user_id INTEGER NOT NULL, session_id TEXT NOT NULL, title TEXT NOT NULL, host TEXT NOT NULL,
		project_id TEXT, updated_at INTEGER NOT NULL, PRIMARY KEY(user_id, session_id));
	CREATE TRIGGER IF NOT EXISTS terminal_usage_events_owner AFTER DELETE ON users BEGIN
		DELETE FROM terminal_usage_events WHERE user_id=old.id;
		DELETE FROM terminal_usage_sessions WHERE user_id=old.id; END;`)
	return err
}

// recordUsageEventTx dates one report that carried tokens, with what the
// session is now, and drops a bounded batch of the account's events that are
// past keeping, with the titles no kept event needs.
func recordUsageEventTx(tx *sql.Tx, uid int64, session TerminalSession, input, output, cached, at int64) error {
	var host string
	if err := tx.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, session.AgentID, uid).Scan(&host); err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO terminal_usage_events(user_id,session_id,agent_id,tool,model,structured,at,input_tokens,output_tokens,cached_tokens) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		uid, session.ID, session.AgentID, session.Tool, session.Model, session.Structured, at, input, output, cached); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO terminal_usage_sessions(user_id,session_id,title,host,project_id,updated_at) VALUES(?,?,?,?,?,?)
		ON CONFLICT(user_id,session_id) DO UPDATE SET title=excluded.title, host=CASE WHEN excluded.host<>'' THEN excluded.host ELSE host END,
			project_id=excluded.project_id, updated_at=MAX(updated_at, excluded.updated_at)`,
		uid, session.ID, session.Title, host, session.ProjectID, at); err != nil {
		return err
	}
	cutoff := at - usageEventsKeep.Milliseconds()
	if _, err := tx.Exec(`DELETE FROM terminal_usage_events WHERE rowid IN (SELECT rowid FROM terminal_usage_events WHERE user_id=? AND at<? LIMIT ?)`, uid, cutoff, usagePruneBatch); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM terminal_usage_sessions WHERE rowid IN (SELECT rowid FROM terminal_usage_sessions WHERE user_id=? AND updated_at<? LIMIT ?)`, uid, cutoff, usagePruneBatch)
	return err
}

// usageFactorySessions is the account's session ids that a Factory outcome
// names: each stage's session and a followed session's own. Only the outcomes
// the controller still keeps can say so.
const usageFactorySessions = `SELECT json_extract(st.value,'$.sessionId') FROM gaffer_outcomes o, json_each(o.data,'$.sessions') s, json_each(s.value,'$.stages') st WHERE o.user_id=?1 AND json_valid(o.data) AND COALESCE(json_extract(st.value,'$.sessionId'),'')<>''
	UNION SELECT json_extract(s.value,'$.sessionId') FROM gaffer_outcomes o, json_each(o.data,'$.sessions') s WHERE o.user_id=?1 AND json_valid(o.data) AND COALESCE(json_extract(s.value,'$.sessionId'),'')<>''`

// UsageFor reads an account's usage in [from, to): totals per agent, up to
// limit sessions (latest report first) and Gaffer's own replies. It reads only
// the account's own rows.
func (s *Store) UsageFor(uid, from, to int64, limit int) (UsageReport, error) {
	out := UsageReport{Tools: []UsageToolTotal{}, Sessions: []UsageSession{}, Gaffer: []UsageGaffer{}}
	var since sql.NullInt64
	if err := s.db.QueryRow(`SELECT MIN(at) FROM terminal_usage_events WHERE user_id=?`, uid).Scan(&since); err != nil {
		return out, err
	}
	out.Since = since.Int64

	rows, err := s.db.Query(`WITH factory(id) AS (`+usageFactorySessions+`),
		spans AS (SELECT session_id, tool FROM terminal_usage_events WHERE user_id=?1 AND at>=?2 AND at<?3 GROUP BY session_id, tool)
		SELECT e.tool, SUM(e.input_tokens), SUM(e.output_tokens), SUM(e.cached_tokens), COUNT(DISTINCT e.session_id), MIN(e.at),
			(SELECT COUNT(*) FROM spans p WHERE p.tool=e.tool AND p.session_id IN (SELECT id FROM factory))
		FROM terminal_usage_events e WHERE e.user_id=?1 AND e.at>=?2 AND e.at<?3 GROUP BY e.tool ORDER BY e.tool`, uid, from, to)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var t UsageToolTotal
		if err := rows.Scan(&t.Tool, &t.Input, &t.Output, &t.Cached, &t.Sessions, &t.FirstAt, &t.FactoryStages); err != nil {
			rows.Close()
			return out, err
		}
		out.Tools = append(out.Tools, t)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}

	// SQLite takes a bare column of a MAX() aggregate from the row that holds
	// the maximum, so tool, model and machine are those of the latest report.
	rows, err = s.db.Query(`WITH factory(id) AS (`+usageFactorySessions+`)
		SELECT e.session_id, e.tool, e.model, e.agent_id, e.structured, SUM(e.input_tokens), SUM(e.output_tokens), SUM(e.cached_tokens), MAX(e.at),
			COALESCE(t.title, k.title, ''), COALESCE(a.host, k.host, ''), COALESCE(t.project_id, k.project_id), COALESCE(t.state, ''),
			t.id IS NULL OR EXISTS(SELECT 1 FROM terminal_session_archives x WHERE x.session_id=e.session_id),
			e.session_id IN (SELECT id FROM factory), COALESCE(u.context_used, 0), COALESCE(u.context_size, 0)
		FROM terminal_usage_events e
		LEFT JOIN terminal_sessions t ON t.id=e.session_id AND t.user_id=e.user_id
		LEFT JOIN terminal_usage_sessions k ON k.user_id=e.user_id AND k.session_id=e.session_id
		LEFT JOIN sync_agents a ON a.id=t.agent_id AND a.user_id=e.user_id
		LEFT JOIN terminal_session_usage u ON u.user_id=e.user_id AND u.session_id=e.session_id
		WHERE e.user_id=?1 AND e.at>=?2 AND e.at<?3
		GROUP BY e.session_id ORDER BY MAX(e.at) DESC, e.session_id LIMIT ?4`, uid, from, to, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var r UsageSession
		var project sql.NullString
		if err := rows.Scan(&r.ID, &r.Tool, &r.Model, &r.AgentID, &r.Structured, &r.Input, &r.Output, &r.Cached, &r.LastAt, &r.Title, &r.Host, &project, &r.State, &r.Archived, &r.Factory, &r.ContextUsed, &r.ContextSize); err != nil {
			rows.Close()
			return out, err
		}
		if project.Valid {
			r.ProjectID = &project.String
		}
		if r.Archived {
			r.State = ""
		}
		out.Sessions = append(out.Sessions, r)
	}
	if err := rows.Close(); err != nil {
		return out, err
	}
	if len(out.Sessions) > limit {
		out.Sessions, out.More = out.Sessions[:limit], true
	}

	rows, err = s.db.Query(`SELECT t.provider_name, t.model, SUM(t.input_tokens), SUM(t.output_tokens), COUNT(*)
		FROM gaffer_turns t JOIN gaffer_conversations c ON c.id=t.conversation_id
		WHERE c.user_id=? AND t.usage_reported<>0 AND t.finished_at>=? AND t.finished_at<?
		GROUP BY t.provider_name, t.model ORDER BY SUM(t.input_tokens+t.output_tokens) DESC, t.provider_name, t.model`, uid, from, to)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var g UsageGaffer
		if err := rows.Scan(&g.Provider, &g.Model, &g.Input, &g.Output, &g.Replies); err != nil {
			return out, err
		}
		out.Gaffer = append(out.Gaffer, g)
	}
	return out, rows.Err()
}
