package store

import (
	"database/sql"
	"errors"
	"strings"
)

// SetSyncStage accepts bounded, typed progress only from the agent executing
// this job. It is informational and never approves or changes the plan.
func (s *Store) SetSyncStage(uid int64, agentID, jobID, stage, detail string) error {
	if len([]rune(detail)) > 240 {
		detail = string([]rune(detail)[:240])
	}
	return s.withTx(func(tx *sql.Tx) error {
		var kind, status, raw, current string
		if err := tx.QueryRow(`SELECT kind,status,stages,stage FROM sync_jobs WHERE user_id=? AND agent_id=? AND id=?`, uid, agentID, jobID).Scan(&kind, &status, &raw, &current); err != nil {
			return ErrNotFound
		}
		if kind == "update" || status != "running" {
			return errors.New("sync job is not running")
		}
		stages := decodeStages(raw)
		if len(stages) == 0 {
			stages = []UpdateStage{{Key: "read", Label: "Read machine", Status: "pending"}, {Key: "plan", Label: "Plan changes", Status: "pending"}, {Key: "apply", Label: "Apply changes", Status: "pending"}, {Key: "verify", Label: "Verify installation", Status: "pending"}}
		}
		idx := stageIndex(stages, stage)
		if idx < 0 || (stageIndex(stages, current) > idx) {
			return errors.New("invalid sync progress stage")
		}
		advanceStages(stages, stage, strings.TrimSpace(detail))
		_, err := tx.Exec(`UPDATE sync_jobs SET stage=?,stages=?,updated_at=? WHERE id=?`, stage, encodeStages(stages), now(), jobID)
		return err
	})
}
