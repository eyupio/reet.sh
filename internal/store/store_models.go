package store

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The cache's own states. The API adds offline and update, which are
// connectivity/negotiation facts and are never written here.
const (
	MachineModelsListed      = "listed"
	MachineModelsUnsupported = "unsupported"
	MachineModelsFailed      = "failed"
	MachineModelsOff         = "off"
)

// MachineModels is one machine's cached model list for one tool. Models,
// More, Source, AgentVersion and FetchedAt describe the last successful list;
// State, Reason and CheckedAt describe the latest attempt, which may have
// failed and left the last good list in place.
type MachineModels struct {
	AgentID      string
	Tool         string
	State        string
	Reason       terminalproto.ModelsReason
	Models       []terminalproto.ModelEntry
	More         bool
	Source       string
	AgentVersion string
	FetchedAt    int64
	CheckedAt    int64
}

func (s *Store) migrateMachineModels() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS machine_agent_models (
				user_id INTEGER NOT NULL,
				agent_id TEXT NOT NULL,
				tool TEXT NOT NULL,
				state TEXT NOT NULL,
				reason TEXT NOT NULL DEFAULT '',
				models TEXT NOT NULL DEFAULT '[]',
				more INTEGER NOT NULL DEFAULT 0,
				source TEXT NOT NULL DEFAULT '',
				agent_version TEXT NOT NULL DEFAULT '',
				fetched_at INTEGER NOT NULL DEFAULT 0,
				checked_at INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY(user_id,agent_id,tool),
				FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS machine_agent_models_user_tool ON machine_agent_models(user_id,tool)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

func validMachineModels(row MachineModels) error {
	if !terminalproto.ValidID(row.AgentID) || !terminalproto.ValidToolV2(row.Tool) {
		return terminalproto.ErrInvalid
	}
	switch row.State {
	case MachineModelsListed, MachineModelsUnsupported, MachineModelsFailed, MachineModelsOff:
	default:
		return terminalproto.ErrInvalid
	}
	if row.Reason != "" && !row.Reason.Valid() {
		return terminalproto.ErrInvalid
	}
	if row.State == MachineModelsListed && row.Reason != "" {
		return terminalproto.ErrInvalid
	}
	if len(row.Models) > terminalproto.MaxModels ||
		!terminalproto.ValidPlainText(row.Source, 64) || !terminalproto.ValidPlainText(row.AgentVersion, 64) {
		return terminalproto.ErrInvalid
	}
	return nil
}

// UpsertMachineModelsFor records one attempt. A listed answer replaces the
// models and everything that describes them. Any other answer changes only
// the state, reason and check time, so a failed refresh keeps the last good
// list and its fetched time. The whole upsert runs in one transaction, so two
// attempts cannot lose a good list to a failure.
func (s *Store) UpsertMachineModelsFor(uid int64, row MachineModels) error {
	if err := validMachineModels(row); err != nil {
		return err
	}
	models := row.Models
	if models == nil {
		models = []terminalproto.ModelEntry{}
	}
	raw, err := json.Marshal(models)
	if err != nil {
		return err
	}
	return s.withTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO machine_agent_models(user_id,agent_id,tool,state,reason,models,more,source,agent_version,fetched_at,checked_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(user_id,agent_id,tool) DO UPDATE SET
				state=excluded.state,
				reason=excluded.reason,
				checked_at=excluded.checked_at,
				models=CASE WHEN excluded.state='listed' THEN excluded.models ELSE machine_agent_models.models END,
				more=CASE WHEN excluded.state='listed' THEN excluded.more ELSE machine_agent_models.more END,
				source=CASE WHEN excluded.state='listed' THEN excluded.source ELSE machine_agent_models.source END,
				agent_version=CASE WHEN excluded.state='listed' THEN excluded.agent_version ELSE machine_agent_models.agent_version END,
				fetched_at=CASE WHEN excluded.state='listed' THEN excluded.fetched_at ELSE machine_agent_models.fetched_at END`,
			uid, row.AgentID, row.Tool, row.State, string(row.Reason), string(raw), row.More, row.Source, row.AgentVersion, row.FetchedAt, row.CheckedAt)
		return err
	})
}

func scanMachineModels(row interface{ Scan(...any) error }) (MachineModels, error) {
	var out MachineModels
	var reason, raw string
	if err := row.Scan(&out.AgentID, &out.Tool, &out.State, &reason, &raw, &out.More, &out.Source, &out.AgentVersion, &out.FetchedAt, &out.CheckedAt); err != nil {
		return out, err
	}
	out.Reason = terminalproto.ModelsReason(reason)
	if out.Reason != "" && !out.Reason.Valid() {
		return out, terminalproto.ErrInvalid
	}
	if raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &out.Models); err != nil || len(out.Models) > terminalproto.MaxModels {
			return out, terminalproto.ErrInvalid
		}
	}
	if out.Models == nil {
		out.Models = []terminalproto.ModelEntry{}
	}
	return out, nil
}

const machineModelsCols = `agent_id,tool,state,reason,models,more,source,agent_version,fetched_at,checked_at`

// MachineModelsFor returns the account's cached list for one machine and tool.
func (s *Store) MachineModelsFor(uid int64, agentID, tool string) (MachineModels, bool, error) {
	if !terminalproto.ValidID(agentID) || !terminalproto.ValidToolV2(tool) {
		return MachineModels{}, false, terminalproto.ErrInvalid
	}
	out, err := scanMachineModels(s.db.QueryRow(`SELECT `+machineModelsCols+` FROM machine_agent_models WHERE user_id=? AND agent_id=? AND tool=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0)`, uid, agentID, tool, uid))
	if errors.Is(err, sql.ErrNoRows) {
		return MachineModels{}, false, nil
	}
	if err != nil {
		return MachineModels{}, false, err
	}
	return out, true, nil
}

// MachineModelsAllFor returns every cached list for one account and tool.
func (s *Store) MachineModelsAllFor(uid int64, tool string) ([]MachineModels, error) {
	if !terminalproto.ValidToolV2(tool) {
		return nil, terminalproto.ErrInvalid
	}
	rows, err := s.db.Query(`SELECT `+machineModelsCols+` FROM machine_agent_models WHERE user_id=? AND tool=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0) ORDER BY agent_id`, uid, tool, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MachineModels{}
	for rows.Next() {
		row, err := scanMachineModels(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// deleteMachineModelsTx removes the lists of every enrollment on host. It is
// used by ForgetMachineFor in the same transaction as the other cleanup.
func deleteMachineModelsTx(tx *sql.Tx, uid int64, host string) error {
	_, err := tx.Exec(`DELETE FROM machine_agent_models WHERE user_id=? AND agent_id IN (SELECT id FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE)`, uid, uid, host)
	return err
}
