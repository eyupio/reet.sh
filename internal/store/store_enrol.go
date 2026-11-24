package store

import (
	"database/sql"
	"errors"
)

// ErrEnrolCodeInvalid is every reason a code cannot enrol a machine: unknown,
// expired, already used or its account disabled. Callers say one thing for all.
var ErrEnrolCodeInvalid = errors.New("enrolment code is not valid")

// ErrEnrolCodeLimit is returned when an account already holds its share of
// unused, unexpired codes.
var ErrEnrolCodeLimit = errors.New("too many unused enrolment codes")

// CreateEnrolCode stores the hash of a new single-use code, unless the account
// already holds max live ones. Codes that expired a day ago are dropped.
func (s *Store) CreateEnrolCode(uid int64, hash string, expiresAt int64, max int) error {
	return s.withTx(func(tx *sql.Tx) error {
		at := now()
		if _, err := tx.Exec(`DELETE FROM enrol_codes WHERE expires_at<?`, at-86_400_000); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM enrol_codes WHERE user_id=? AND used_at=0 AND expires_at>?`, uid, at).Scan(&n); err != nil {
			return err
		}
		if n >= max {
			return ErrEnrolCodeLimit
		}
		_, err := tx.Exec(`INSERT INTO enrol_codes(hash,user_id,created_at,expires_at) VALUES(?,?,?,?)`, hash, uid, at, expiresAt)
		return err
	})
}

// EnrolWithCode spends a code and enrols a machine for the code's account. The
// claim is one conditional UPDATE, so of two enrolments racing for a code only
// one gets it. An enrolment refused after the claim (a machine name already
// taken) puts the code back, so the person can try another name.
func (s *Store) EnrolWithCode(hash, host, version, commit string) (int64, SyncAgent, string, error) {
	at := now()
	res, err := s.db.Exec(`UPDATE enrol_codes SET used_at=? WHERE hash=? AND used_at=0 AND expires_at>?
		AND user_id IN (SELECT id FROM users WHERE disabled_at=0)`, at, hash, at)
	if err != nil {
		return 0, SyncAgent{}, "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return 0, SyncAgent{}, "", ErrEnrolCodeInvalid
	}
	var uid int64
	if err = s.db.QueryRow(`SELECT user_id FROM enrol_codes WHERE hash=?`, hash).Scan(&uid); err != nil {
		return 0, SyncAgent{}, "", err
	}
	a, secret, err := s.EnrollSyncAgentBuild(uid, host, version, commit)
	if err != nil {
		_, _ = s.db.Exec(`UPDATE enrol_codes SET used_at=0 WHERE hash=? AND agent_id=''`, hash)
		return 0, a, "", err
	}
	_, err = s.db.Exec(`UPDATE enrol_codes SET agent_id=? WHERE hash=?`, a.ID, hash)
	return uid, a, secret, err
}

// EnrolCodeAgent is the machine a spent code enrolled, or "".
func (s *Store) EnrolCodeAgent(hash string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT agent_id FROM enrol_codes WHERE hash=?`, hash).Scan(&id)
	return id, err
}
