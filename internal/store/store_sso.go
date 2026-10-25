package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Single sign-on keeps three things: who an identity provider's subject is
// here (sso_identities), the sign-ins still under way (sso_logins), and which
// provider session a browser session began under, so a logout from the
// provider can end it (the sso_* columns of sessions).

var (
	// ErrSSOLinkedElsewhere is returned when a subject already belongs to
	// another account.
	ErrSSOLinkedElsewhere = errors.New("that identity is already linked to another account")
	// ErrSSOAlreadyLinked is returned when the account already has an
	// identity at that provider.
	ErrSSOAlreadyLinked = errors.New("this account is already linked to an identity at that provider")
	// ErrSSOBusy is returned when too many sign-ins are waiting to finish.
	ErrSSOBusy = errors.New("too many single sign-on attempts are waiting")
)

// SSOLoginCap bounds the sign-ins waiting to finish, so a flood of starts
// cannot grow the table without limit.
const SSOLoginCap = 1000

// SSOIdentity is one provider subject linked to an account.
type SSOIdentity struct {
	Issuer      string `json:"issuer"`
	Subject     string `json:"-"`
	UserID      int64  `json:"-"`
	CreatedAt   int64  `json:"linkedAt"`
	LastLoginAt int64  `json:"lastLoginAt,omitempty"`
}

// SSOLogin is a sign-in that has been started and not yet finished. Only
// hashes of the state and of the browser binding are kept.
type SSOLogin struct {
	StateHash   string
	BindingHash string
	Nonce       string
	Verifier    string
	Next        string
	Intent      string
	UserID      int64
	ExpiresAt   int64
}

func (s *Store) migrateSSO() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS sso_identities (
			issuer TEXT NOT NULL, subject TEXT NOT NULL,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL, last_login_at INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(issuer, subject))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_sso_identities_user ON sso_identities(user_id, issuer)`,
		`CREATE TABLE IF NOT EXISTS sso_logins (
			state_hash TEXT PRIMARY KEY, binding_hash TEXT NOT NULL, nonce TEXT NOT NULL, verifier TEXT NOT NULL,
			next TEXT NOT NULL DEFAULT '', intent TEXT NOT NULL DEFAULT 'signin',
			user_id INTEGER NOT NULL DEFAULT 0, expires_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_sso_logins_expires ON sso_logins(expires_at)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	for _, col := range []string{"sso_issuer", "sso_sid", "sso_subject"} {
		ok, err := hasColumn(s.db, "sessions", col)
		if err != nil {
			return err
		}
		if !ok {
			if _, err := s.db.Exec(`ALTER TABLE sessions ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_sso ON sessions(sso_issuer, sso_subject)`)
	return err
}

// SSOIdentityUser finds the account a provider subject is linked to.
func (s *Store) SSOIdentityUser(issuer, subject string) (User, bool, error) {
	return s.scanUser(s.db.QueryRow(`SELECT u.id,u.email,u.name,u.role,u.password_hash,u.created_at,u.disabled_at
		FROM sso_identities i JOIN users u ON u.id=i.user_id WHERE i.issuer=? AND i.subject=?`, issuer, subject))
}

// LinkSSOIdentity ties a provider subject to an account. Linking the same
// pair again does nothing.
func (s *Store) LinkSSOIdentity(issuer, subject string, userID int64) error {
	return s.withTx(func(tx *sql.Tx) error { return linkSSO(tx, issuer, subject, userID) })
}

func linkSSO(tx *sql.Tx, issuer, subject string, userID int64) error {
	var owner int64
	err := tx.QueryRow(`SELECT user_id FROM sso_identities WHERE issuer=? AND subject=?`, issuer, subject).Scan(&owner)
	switch {
	case err == nil && owner == userID:
		return nil
	case err == nil:
		return ErrSSOLinkedElsewhere
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	var have int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sso_identities WHERE issuer=? AND user_id=?`, issuer, userID).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return ErrSSOAlreadyLinked
	}
	_, err = tx.Exec(`INSERT INTO sso_identities(issuer,subject,user_id,created_at) VALUES(?,?,?,?)`, issuer, subject, userID, now())
	return err
}

// UnlinkSSOIdentity removes an account's link at one provider.
func (s *Store) UnlinkSSOIdentity(userID int64, issuer string) error {
	res, err := s.db.Exec(`DELETE FROM sso_identities WHERE user_id=? AND issuer=?`, userID, issuer)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SSOIdentityFor is an account's link at a provider, if it has one.
func (s *Store) SSOIdentityFor(userID int64, issuer string) (SSOIdentity, bool, error) {
	var i SSOIdentity
	err := s.db.QueryRow(`SELECT issuer,subject,user_id,created_at,last_login_at FROM sso_identities WHERE user_id=? AND issuer=?`, userID, issuer).
		Scan(&i.Issuer, &i.Subject, &i.UserID, &i.CreatedAt, &i.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SSOIdentity{}, false, nil
	}
	return i, err == nil, err
}

// TouchSSOIdentity records a sign-in through an identity.
func (s *Store) TouchSSOIdentity(issuer, subject string) {
	_, _ = s.db.Exec(`UPDATE sso_identities SET last_login_at=? WHERE issuer=? AND subject=?`, now(), issuer, subject)
}

// SSOIdentityCount is how many accounts are linked at a provider.
func (s *Store) SSOIdentityCount(issuer string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM sso_identities WHERE issuer=?`, issuer).Scan(&n)
	return n, err
}

// CreateSSOUser makes an account for a provider subject and links it, in one
// step. The account has no password, and is never the owner: ownership comes
// only from setup and from transfer.
func (s *Store) CreateSSOUser(issuer, subject, email, name, role string) (User, error) {
	if role != "admin" && role != "member" && role != "viewer" {
		return User{}, errors.New("invalid role for a single sign-on account")
	}
	email = normalizeEmail(email)
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	var id int64
	err := s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO users(email,name,role,password_hash,created_at) VALUES(?,?,?,?,?)`, email, name, role, "", now())
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return ErrUserExists
			}
			return err
		}
		id, _ = res.LastInsertId()
		return linkSSO(tx, issuer, subject, id)
	})
	if err != nil {
		return User{}, err
	}
	u, _, err := s.UserByID(id)
	return u, err
}

// CreateSSOLogin records a started sign-in. Expired attempts are cleared
// first; if the table is still at its cap the attempt is refused.
func (s *Store) CreateSSOLogin(l SSOLogin, at int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM sso_logins WHERE expires_at<=?`, at); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sso_logins`).Scan(&n); err != nil {
			return err
		}
		if n >= SSOLoginCap {
			return ErrSSOBusy
		}
		_, err := tx.Exec(`INSERT INTO sso_logins(state_hash,binding_hash,nonce,verifier,next,intent,user_id,expires_at) VALUES(?,?,?,?,?,?,?,?)`,
			l.StateHash, l.BindingHash, l.Nonce, l.Verifier, l.Next, l.Intent, l.UserID, l.ExpiresAt)
		return err
	})
}

// ConsumeSSOLogin takes a started sign-in out of the table and returns it. A
// state works once: whatever happens next, the attempt is spent.
func (s *Store) ConsumeSSOLogin(stateHash string, at int64) (SSOLogin, bool, error) {
	var l SSOLogin
	err := s.withTx(func(tx *sql.Tx) error {
		err := tx.QueryRow(`SELECT state_hash,binding_hash,nonce,verifier,next,intent,user_id,expires_at FROM sso_logins WHERE state_hash=?`, stateHash).
			Scan(&l.StateHash, &l.BindingHash, &l.Nonce, &l.Verifier, &l.Next, &l.Intent, &l.UserID, &l.ExpiresAt)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`DELETE FROM sso_logins WHERE state_hash=?`, stateHash)
		return err
	})
	if errors.Is(err, sql.ErrNoRows) {
		return SSOLogin{}, false, nil
	}
	if err != nil {
		return SSOLogin{}, false, err
	}
	if l.ExpiresAt <= at {
		return SSOLogin{}, false, nil
	}
	return l, true, nil
}

// CreateSSOSession starts a session and remembers which provider session it
// began under.
func (s *Store) CreateSSOSession(id string, userID, expiresAt int64, ua, issuer, sid, subject string) error {
	_, err := s.db.Exec(`INSERT INTO sessions(id,user_id,expires_at,ua,created_at,last_seen,sso_issuer,sso_sid,sso_subject) VALUES(?,?,?,?,?,?,?,?,?)`,
		id, userID, expiresAt, ua, now(), now(), issuer, sid, subject)
	return err
}

// DeleteSSOSessions ends the sessions a provider's logout names and returns
// their ids. A session id and a subject together name one session of one
// person; either alone names every session it matches; neither names none.
func (s *Store) DeleteSSOSessions(issuer, subject, sid string) ([]string, error) {
	if issuer == "" || (subject == "" && sid == "") {
		return nil, nil
	}
	q := `SELECT id FROM sessions WHERE sso_issuer=?`
	args := []any{issuer}
	if sid != "" {
		q += ` AND sso_sid=?`
		args = append(args, sid)
	}
	if subject != "" {
		q += ` AND sso_subject=?`
		args = append(args, subject)
	}
	var ids []string
	err := s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(q, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			if _, err := tx.Exec(`DELETE FROM sessions WHERE id=?`, id); err != nil {
				return err
			}
		}
		return nil
	})
	return ids, err
}

// UserSessionIDs lists an account's sessions, so what hangs off them (an open
// terminal) can be closed when the account is disabled.
func (s *Store) UserSessionIDs(userID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT id FROM sessions WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
