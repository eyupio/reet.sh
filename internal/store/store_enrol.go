package store

import (
	"database/sql"
	"errors"
	"strings"
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

// EnrolWithCode spends a code and enrols a machine for the code's account.
// The claim is one conditional UPDATE in the same transaction as the
// enrolment, so of two enrolments racing for a code only one gets it, and a
// storage failure rolls the claim back. A machine of that name already enrolled
// is enrolled again in place: same id, a new secret (the old one stops
// working), and now by code. So the answer never says whether a name was
// taken, and the code is spent on every decision. Only a name that could never
// be valid is refused before the code is touched. replaced says the machine
// existed.
func (s *Store) EnrolWithCode(hash, host, version, commit string) (uid int64, a SyncAgent, secret string, replaced bool, err error) {
	host = strings.TrimSpace(host)
	if !validMachineName(host) {
		return 0, a, "", false, ErrMachineName
	}
	// A code that cannot be claimed is turned away on the read pool: this route needs no
	// credential, so a guess must not take the write connection from every agent poll.
	var live int
	if err = s.read.QueryRow(`SELECT COUNT(*) FROM enrol_codes WHERE hash=? AND used_at=0 AND expires_at>?
		AND user_id IN (SELECT id FROM users WHERE disabled_at=0)`, hash, now()).Scan(&live); err != nil {
		return 0, a, "", false, err
	}
	if live == 0 {
		return 0, a, "", false, ErrEnrolCodeInvalid
	}
	secret = syncID()
	err = s.withTx(func(tx *sql.Tx) error {
		at := now()
		res, err := tx.Exec(`UPDATE enrol_codes SET used_at=? WHERE hash=? AND used_at=0 AND expires_at>?
			AND user_id IN (SELECT id FROM users WHERE disabled_at=0)`, at, hash, at)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrEnrolCodeInvalid
		}
		if err = tx.QueryRow(`SELECT user_id FROM enrol_codes WHERE hash=?`, hash).Scan(&uid); err != nil {
			return err
		}
		canonical, err := resolveMachineHost(tx, uid, host)
		if err != nil {
			return err
		}
		existing, err := scanAgent(tx.QueryRow(`SELECT `+agentCols+` FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE`, uid, canonical))
		if errors.Is(err, sql.ErrNoRows) {
			a = SyncAgent{ID: syncID(), Host: canonical, Version: cleanAgentBuild(version), Commit: cleanAgentBuild(commit), LastSeen: now(), AutoUpdate: true}
			_, err = tx.Exec(`INSERT INTO sync_agents(id,user_id,host,secret_hash,version,source_commit,last_seen,paused,auto_update,enrolled_via) VALUES(?,?,?,?,?,?,?,0,1,?)`, a.ID, uid, a.Host, secretHash(secret), a.Version, a.Commit, a.LastSeen, EnrolledByCode)
		} else if err == nil {
			a, replaced = existing, true
			a.Version, a.Commit, a.LastSeen = cleanAgentBuild(version), cleanAgentBuild(commit), now()
			_, err = tx.Exec(`UPDATE sync_agents SET secret_hash=?,enrolled_via=?,version=?,source_commit=?,last_seen=? WHERE id=? AND user_id=?`, secretHash(secret), EnrolledByCode, a.Version, a.Commit, a.LastSeen, a.ID, uid)
		}
		if err != nil {
			return err
		}
		a.EnrolledVia, a.LegacyToken = EnrolledByCode, false
		_, err = tx.Exec(`UPDATE enrol_codes SET agent_id=? WHERE hash=?`, a.ID, hash)
		return err
	})
	if err != nil {
		return 0, a, "", false, err
	}
	return uid, a, secret, replaced, nil
}

// EnrolCodeAgent is the machine a spent code enrolled, or "".
func (s *Store) EnrolCodeAgent(hash string) (string, error) {
	var id string
	err := s.db.QueryRow(`SELECT agent_id FROM enrol_codes WHERE hash=?`, hash).Scan(&id)
	return id, err
}
