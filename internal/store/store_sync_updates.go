package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

// UpdateStage is one step of an agent update, shown to the person who asked
// for it and to anyone watching the machine.
type UpdateStage struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

const (
	UpdateStageQueued   = "queued"
	UpdateStageDownload = "download"
	UpdateStageVerify   = "verify"
	UpdateStagePreserve = "preserve"
	UpdateStageReplace  = "replace"
	UpdateStageRestart  = "restart"
	UpdateStageCheck    = "check"
	UpdateStageRollback = "rollback"
)

// UpdateStageKeys is the order the agent reports an update in. The controller
// refuses a stage it does not know, so a newer agent cannot write words an
// older controller would show as blank.
var UpdateStageKeys = []string{
	UpdateStageQueued,
	UpdateStageDownload,
	UpdateStageVerify,
	UpdateStagePreserve,
	UpdateStageReplace,
	UpdateStageRestart,
	UpdateStageCheck,
}

// NewUpdateStages is the timeline a queued update starts with.
func NewUpdateStages() []UpdateStage {
	return []UpdateStage{
		{Key: UpdateStageQueued, Label: "Queued for the machine", Status: "running", Detail: "Waiting for the agent to collect it."},
		{Key: UpdateStageDownload, Label: "Downloading the build", Status: "pending"},
		{Key: UpdateStageVerify, Label: "Verifying the checksum and build", Status: "pending"},
		{Key: UpdateStagePreserve, Label: "Keeping the current binary", Status: "pending"},
		{Key: UpdateStageReplace, Label: "Installing the new binary", Status: "pending"},
		{Key: UpdateStageRestart, Label: "Restarting the agent", Status: "pending"},
		{Key: UpdateStageCheck, Label: "Waiting for the new agent to report", Status: "pending"},
	}
}

func decodeStages(raw string) []UpdateStage {
	var stages []UpdateStage
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &stages) != nil {
		return nil
	}
	return stages
}

func encodeStages(stages []UpdateStage) string {
	if len(stages) == 0 {
		return "[]"
	}
	b, err := json.Marshal(stages)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func stageIndex(stages []UpdateStage, key string) int {
	for i := range stages {
		if stages[i].Key == key {
			return i
		}
	}
	return -1
}

// advanceStages moves the timeline to key, marking everything before it done.
func advanceStages(stages []UpdateStage, key, detail string) int {
	idx := stageIndex(stages, key)
	if idx < 0 {
		idx = len(stages) - 1
	}
	for i := range stages {
		switch {
		case i < idx:
			if stages[i].Status != "failed" {
				stages[i].Status = "done"
			}
		case i == idx:
			// A repeated report refreshes the words, never rewinds the step.
			if stages[i].Status != "done" && stages[i].Status != "failed" {
				stages[i].Status = "running"
			}
			if detail != "" {
				stages[i].Detail = detail
			}
		default:
			if stages[i].Status == "running" {
				stages[i].Status = "pending"
			}
		}
	}
	return idx
}

func finishStages(stages []UpdateStage, status, message string) (string, []UpdateStage) {
	if len(stages) == 0 {
		return "", nil
	}
	switch status {
	case "succeeded":
		for i := range stages {
			if stages[i].Status == "pending" || stages[i].Status == "running" {
				stages[i].Status = "done"
			}
		}
		return stages[len(stages)-1].Key, stages
	case "failed":
		for i := range stages {
			if stages[i].Status == "running" || stages[i].Status == "pending" {
				stages[i].Status = "failed"
				if message != "" {
					stages[i].Detail = message
				}
				return stages[i].Key, stages
			}
		}
		return stages[len(stages)-1].Key, stages
	case "rolled_back":
		for i := range stages {
			if stages[i].Status == "running" || stages[i].Status == "pending" {
				stages[i].Status = "failed"
				if message != "" {
					stages[i].Detail = message
				}
				break
			}
		}
		if stageIndex(stages, UpdateStageRollback) < 0 {
			stages = append(stages, UpdateStage{Key: UpdateStageRollback, Label: "Restoring the previous binary", Status: "done", Detail: "The previous build is running again."})
		}
		return UpdateStageRollback, stages
	}
	return "", stages
}

// SetAgentUpdateStage records what the agent is doing right now. Only the
// agent's own running update may write, and a stage never moves backwards.
func (s *Store) SetAgentUpdateStage(uid int64, agentID, jobID, stage, detail string) error {
	detail = strings.TrimSpace(detail)
	if utf8.RuneCountInString(detail) > 240 {
		detail = string([]rune(detail)[:240])
	}
	return s.withTx(func(tx *sql.Tx) error {
		var kind, status, stagesJSON, current string
		err := tx.QueryRow(`SELECT kind,status,stages,stage FROM sync_jobs WHERE user_id=? AND agent_id=? AND id=?`, uid, agentID, jobID).Scan(&kind, &status, &stagesJSON, &current)
		if err != nil {
			return ErrNotFound
		}
		if kind != "update" {
			return errors.New("not an update job")
		}
		if status != "running" {
			return errors.New("update is not running")
		}
		stages := decodeStages(stagesJSON)
		idx := stageIndex(stages, stage)
		if idx < 0 {
			return errors.New("unknown update stage")
		}
		if cur := stageIndex(stages, current); cur >= 0 && idx < cur {
			return errors.New("update already passed that stage")
		}
		advanceStages(stages, stage, detail)
		_, err = tx.Exec(`UPDATE sync_jobs SET stage=?,stages=?,updated_at=? WHERE id=?`, stage, encodeStages(stages), now(), jobID)
		return err
	})
}

// AgentUpdateSettledSince reports whether this agent's last update attempt
// ended recently. A successful update is included on purpose: if the agent
// still reports an older build afterwards, retrying at once would loop rather
// than help. A queued or running attempt is not counted; the one-job gate
// already holds a second request off.
func (s *Store) AgentUpdateSettledSince(uid int64, agentID string, since int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sync_jobs WHERE user_id=? AND agent_id=? AND kind='update' AND status IN ('succeeded','failed','rolled_back','cancelled') AND updated_at>=?`, uid, agentID, since).Scan(&n)
	return n > 0, err
}

// LatestUpdateJobsFor returns the newest update job for each of the account's
// enrolled machines. The general job list is bounded so a busy machine's scans
// cannot push its update out of sight.
func (s *Store) LatestUpdateJobsFor(uid int64) ([]SyncJob, error) {
	rows, err := s.db.Query(`SELECT `+jobCols+` FROM sync_jobs WHERE user_id=? AND kind='update' ORDER BY created_at DESC LIMIT 500`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := []SyncJob{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		if seen[j.AgentID] {
			continue
		}
		seen[j.AgentID] = true
		out = append(out, j)
	}
	return out, rows.Err()
}
