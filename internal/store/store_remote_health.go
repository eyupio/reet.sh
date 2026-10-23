package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type RemoteHealthMinute struct {
	AgentID    string                      `json:"agentId"`
	Minute     int64                       `json:"minute"` // controller Unix milliseconds, exact minute
	Generation int64                       `json:"generation"`
	Revision   int64                       `json:"revision"`
	Summary    terminalproto.HealthSummary `json:"summary"`
}

// SaveRemoteHealthMinuteFor stores a controller-built cumulative summary. A
// retry is harmless, older revisions are rejected, and a new connection starts
// a new segment (replacing that minute instead of summing restarted counters).
// It accepts only the current controller minute; host timestamps cannot grow
// history or manufacture old observations. No raw report/terminal data is saved.
func (s *Store) SaveRemoteHealthMinuteFor(uid int64, in RemoteHealthMinute) error {
	return s.SaveRemoteHealthMinuteForContext(context.Background(), uid, in)
}
func (s *Store) SaveRemoteHealthMinuteForContext(ctx context.Context, uid int64, in RemoteHealthMinute) error {
	return s.saveRemoteHealthMinuteContext(ctx, uid, in, time.Now())
}
func (s *Store) saveRemoteHealthMinuteAt(uid int64, in RemoteHealthMinute, at time.Time) error {
	return s.saveRemoteHealthMinuteContext(context.Background(), uid, in, at)
}
func (s *Store) saveRemoteHealthMinuteContext(ctx context.Context, uid int64, in RemoteHealthMinute, at time.Time) error {
	if !terminalproto.ValidID(in.AgentID) || !terminalproto.ValidCounter(in.Generation) || !terminalproto.ValidCounter(in.Revision) || in.Minute != at.Truncate(time.Minute).UnixMilli() || in.Summary.Validate() != nil {
		return terminalproto.ErrInvalid
	}
	raw, _ := json.Marshal(in.Summary)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	err = func() error {
		if err := lockRemoteAgent(tx, uid, in.AgentID, false); err != nil {
			return err
		}
		caps, err := currentGeneration(tx, uid, in.AgentID, in.Generation)
		if err != nil {
			return err
		}
		if !caps.Capabilities.HealthEnabled {
			return ErrTerminalNotReady
		}
		var generation, revision int64
		var existing string
		err = tx.QueryRow(`SELECT generation,revision,summary FROM remote_health_summaries WHERE agent_id=? AND minute=?`, in.AgentID, in.Minute).Scan(&generation, &revision, &existing)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && generation == in.Generation {
			if in.Revision < revision || in.Revision == revision && existing != string(raw) {
				return ErrTerminalConflict
			}
			if in.Revision == revision {
				return nil
			}
		}
		if _, err := tx.Exec(`INSERT INTO remote_health_summaries(agent_id,user_id,minute,generation,revision,summary) VALUES(?,?,?,?,?,?) ON CONFLICT(agent_id,minute) DO UPDATE SET generation=excluded.generation,revision=excluded.revision,summary=excluded.summary`, in.AgentID, uid, in.Minute, in.Generation, in.Revision, string(raw)); err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM remote_health_summaries WHERE minute<?`, at.Add(-healthRetention).Truncate(time.Minute).UnixMilli())
		return err
	}()
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RemoteHealthMinutesFor returns bounded owner-only history from the last day.
// Freshness/live connectivity is a runtime observation and is never inferred
// from these retained summaries. A later scheduler must also prune while idle.
func (s *Store) RemoteHealthMinutesFor(uid int64, agentID string) ([]RemoteHealthMinute, error) {
	rows, err := s.db.Query(`SELECT agent_id,minute,generation,revision,summary FROM remote_health_summaries WHERE user_id=? AND agent_id=? AND minute>=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0) ORDER BY minute LIMIT 1441`, uid, agentID, time.Now().Add(-healthRetention).Truncate(time.Minute).UnixMilli(), uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RemoteHealthMinute{}
	for rows.Next() {
		var h RemoteHealthMinute
		var raw string
		if err := rows.Scan(&h.AgentID, &h.Minute, &h.Generation, &h.Revision, &raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &h.Summary) != nil || h.Summary.Validate() != nil {
			return nil, terminalproto.ErrInvalid
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// PruneRemoteHealth is called by the future monitoring scheduler even during
// inactivity. The hard 24-hour retention cannot be extended by a caller.
func (s *Store) PruneRemoteHealth() error {
	_, err := s.db.Exec(`DELETE FROM remote_health_summaries WHERE minute<?`, time.Now().Add(-healthRetention).Truncate(time.Minute).UnixMilli())
	return err
}
