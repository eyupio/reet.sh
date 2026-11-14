package store

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MachineAccounts is the last answer a machine gave about its agents'
// sign-ins: who is signed in, on which plan and, when the account asked for
// them, the plan's allowance figures. It holds no credential; the machine
// never sends one. CheckedAt is when the controller received the answer, in
// milliseconds.
type MachineAccounts struct {
	AgentID   string
	Accounts  []terminalproto.AgentAccount
	CheckedAt int64
}

func (s *Store) migrateMachineAccounts() error {
	// One row per machine: an answer is kept whole and replaced whole, so an
	// account that signed out never keeps a name from the answer before. The
	// row goes with the enrollment.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS machine_agent_accounts (
		user_id INTEGER NOT NULL,
		agent_id TEXT NOT NULL,
		accounts TEXT NOT NULL DEFAULT '[]',
		checked_at INTEGER NOT NULL,
		PRIMARY KEY(user_id,agent_id),
		FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`)
	return err
}

// SaveMachineAccountsFor keeps a machine's answer, replacing the one before.
// It refuses an answer a connector may not give, and a machine the account
// does not own.
func (s *Store) SaveMachineAccountsFor(uid int64, agentID string, accounts []terminalproto.AgentAccount, checkedAt int64) error {
	if !terminalproto.ValidID(agentID) || checkedAt <= 0 || !terminalproto.ValidAgentAccounts(accounts) {
		return terminalproto.ErrInvalid
	}
	if accounts == nil {
		accounts = []terminalproto.AgentAccount{}
	}
	raw, err := json.Marshal(accounts)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO machine_agent_accounts(user_id,agent_id,accounts,checked_at) VALUES(?,?,?,?)
		ON CONFLICT(user_id,agent_id) DO UPDATE SET accounts=excluded.accounts, checked_at=excluded.checked_at`, uid, agentID, string(raw), checkedAt)
	return err
}

// MachineAccountsFor returns the account's kept answer for one machine. ok is
// false for a machine that has never answered.
func (s *Store) MachineAccountsFor(uid int64, agentID string) (MachineAccounts, bool, error) {
	if !terminalproto.ValidID(agentID) {
		return MachineAccounts{}, false, terminalproto.ErrInvalid
	}
	out := MachineAccounts{AgentID: agentID}
	var raw string
	err := s.db.QueryRow(`SELECT accounts,checked_at FROM machine_agent_accounts WHERE user_id=? AND agent_id=? AND EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0)`, uid, agentID, uid).
		Scan(&raw, &out.CheckedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return MachineAccounts{}, false, nil
	}
	if err != nil {
		return MachineAccounts{}, false, err
	}
	if err := json.Unmarshal([]byte(raw), &out.Accounts); err != nil || !terminalproto.ValidAgentAccounts(out.Accounts) {
		return MachineAccounts{}, false, terminalproto.ErrInvalid
	}
	if out.Accounts == nil {
		out.Accounts = []terminalproto.AgentAccount{}
	}
	return out, true, nil
}

// deleteMachineAccountsTx removes the answers of every enrollment on host. It
// is used by ForgetMachineFor in the same transaction as the other cleanup.
func deleteMachineAccountsTx(tx *sql.Tx, uid int64, host string) error {
	_, err := tx.Exec(`DELETE FROM machine_agent_accounts WHERE user_id=? AND agent_id IN (SELECT id FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE)`, uid, uid, host)
	return err
}
