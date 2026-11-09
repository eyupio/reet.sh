package store

import (
	"database/sql"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// retireAgents clears what an upgrade would otherwise leave naming an agent reet
// has retired: the default agent, model and effort of the account profile, of
// folder projects and of unstarted backlog drafts (a draft keeps its text), and
// the favourites and cached model lists that belong to it. Past sessions, run
// records, audit events and Setup history are history and are never touched.
//
// It runs every time the store opens and is idempotent: it only matches rows
// that still name a retired agent, so a second run changes nothing, not even a
// row's version.
func (s *Store) retireAgents() error {
	return s.withTx(func(tx *sql.Tx) error {
		at := now()
		for _, tool := range terminalproto.RetiredTools() {
			for _, step := range []struct {
				query string
				args  []any
			}{
				{`UPDATE user_profiles SET default_tool='', default_model='', default_effort='' WHERE default_tool=?`, []any{tool}},
				{`UPDATE folder_projects SET default_tool='', default_model='', default_effort='', version=version+1, updated_at=? WHERE default_tool=?`, []any{at, tool}},
				{`UPDATE session_backlog SET body=json_set(body,'$.tool','','$.model','','$.effort',''), version=version+1, updated_at=? WHERE json_extract(body,'$.tool')=?`, []any{at, tool}},
				{`DELETE FROM model_favourites WHERE tool=?`, []any{tool}},
				{`DELETE FROM machine_agent_models WHERE tool=?`, []any{tool}},
			} {
				if _, err := tx.Exec(step.query, step.args...); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
