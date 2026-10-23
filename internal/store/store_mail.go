package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

// The SQL behind reet's email (internal/mail): the instance's mail server, the
// outbox, team notification settings and password-reset links. The rules
// about what is sent, when and to whom are internal/mail's.

const (
	MailQueued    = "queued"
	MailSending   = "sending"
	MailDelivered = "delivered"
	MailFailed    = "failed"
	MailBounced   = "bounced"
	MailHeld      = "held"
)

// RedactedBody replaces a sign-in message's body once it no longer needs to be
// sent, so a copy of the database holds no live link.
const RedactedBody = "(removed after sending: this message carried a sign-in link)"

var (
	ErrMailWaiting = errors.New("that message is already waiting to be sent")
	ErrMailSpent   = errors.New("that message carried a sign-in link that is spent; send a new one instead")
)

// MailRetention is how long a delivered or given-up message is kept.
const MailRetention = 90 * 24 * time.Hour

func (s *Store) migrateMail() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS mail_settings (
			id INTEGER PRIMARY KEY CHECK(id=1),
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			tls TEXT NOT NULL DEFAULT 'starttls',
			username TEXT NOT NULL DEFAULT '',
			password_sealed TEXT NOT NULL DEFAULT '',
			from_addr TEXT NOT NULL DEFAULT '',
			from_name TEXT NOT NULL DEFAULT 'reet.sh',
			reply_to TEXT NOT NULL DEFAULT '',
			base_url TEXT NOT NULL DEFAULT '',
			dkim_selector TEXT NOT NULL DEFAULT '',
			updated_at INTEGER NOT NULL DEFAULT 0,
			updated_by TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS mail_outbox (
			id TEXT PRIMARY KEY,
			idem_key TEXT NOT NULL DEFAULT '',
			template TEXT NOT NULL DEFAULT '',
			to_addr TEXT NOT NULL,
			team_id INTEGER NOT NULL DEFAULT 0,
			subject TEXT NOT NULL DEFAULT '',
			text_body TEXT NOT NULL DEFAULT '',
			html_body TEXT NOT NULL DEFAULT '',
			secret INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL DEFAULT 'queued',
			attempts INTEGER NOT NULL DEFAULT 0,
			next_attempt_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			via TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			delivered_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mail_outbox_key ON mail_outbox(idem_key) WHERE idem_key<>''`,
		`CREATE INDEX IF NOT EXISTS idx_mail_outbox_due ON mail_outbox(state,next_attempt_at)`,
		`CREATE INDEX IF NOT EXISTS idx_mail_outbox_created ON mail_outbox(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS team_notify (
			group_id INTEGER PRIMARY KEY REFERENCES audience_groups(id) ON DELETE CASCADE,
			enabled INTEGER NOT NULL DEFAULT 1,
			events TEXT NOT NULL DEFAULT '[]',
			recipients TEXT NOT NULL DEFAULT 'managers',
			extra_emails TEXT NOT NULL DEFAULT '[]',
			updated_at INTEGER NOT NULL DEFAULT 0,
			updated_by INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS password_resets (
			token_hash TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			used_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_resets_expiry ON password_resets(expires_at)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// MailSettings is the single saved mail server, with the password sealed.
type MailSettings struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	TLS            string `json:"tls"`
	User           string `json:"user"`
	PasswordSealed string `json:"-"`
	PasswordSet    bool   `json:"passwordSet"`
	From           string `json:"from"`
	FromName       string `json:"fromName"`
	ReplyTo        string `json:"replyTo"`
	BaseURL        string `json:"baseUrl"`
	DKIMSelector   string `json:"dkimSelector"`
	UpdatedAt      int64  `json:"updatedAt,omitempty"`
	UpdatedBy      string `json:"updatedBy,omitempty"`
}

// GetMailSettings reads the saved server. The second result says whether one
// has ever been saved; an empty row is not the same as no row.
func (s *Store) GetMailSettings() (MailSettings, bool, error) {
	var m MailSettings
	var sealed string
	err := s.db.QueryRow(`SELECT host,port,tls,username,password_sealed,from_addr,from_name,reply_to,base_url,dkim_selector,updated_at,updated_by
		FROM mail_settings WHERE id=1`).Scan(&m.Host, &m.Port, &m.TLS, &m.User, &sealed, &m.From, &m.FromName, &m.ReplyTo, &m.BaseURL, &m.DKIMSelector, &m.UpdatedAt, &m.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return MailSettings{TLS: "starttls", Port: 587, FromName: "reet.sh"}, false, nil
	}
	if err != nil {
		return MailSettings{}, false, err
	}
	m.PasswordSealed = sealed
	m.PasswordSet = sealed != ""
	return m, true, nil
}

// SaveMailSettings stores the single server. The next message uses it.
func (s *Store) SaveMailSettings(m MailSettings, by string) error {
	_, err := s.db.Exec(`INSERT INTO mail_settings(id,host,port,tls,username,password_sealed,from_addr,from_name,reply_to,base_url,dkim_selector,updated_at,updated_by)
		VALUES(1,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET host=excluded.host,port=excluded.port,tls=excluded.tls,username=excluded.username,
			password_sealed=excluded.password_sealed,from_addr=excluded.from_addr,from_name=excluded.from_name,
			reply_to=excluded.reply_to,base_url=excluded.base_url,dkim_selector=excluded.dkim_selector,
			updated_at=excluded.updated_at,updated_by=excluded.updated_by`,
		m.Host, m.Port, m.TLS, m.User, m.PasswordSealed, m.From, m.FromName, m.ReplyTo, m.BaseURL, m.DKIMSelector, now(), by)
	return err
}

// Mail is one message in the outbox.
type Mail struct {
	ID            string `json:"id"`
	Key           string `json:"-"`
	Template      string `json:"template"`
	To            string `json:"to"`
	TeamID        int64  `json:"teamId,omitempty"`
	Subject       string `json:"subject"`
	Text          string `json:"-"`
	HTML          string `json:"-"`
	Secret        bool   `json:"secret"`
	State         string `json:"state"`
	Attempts      int    `json:"attempts"`
	NextAttemptAt int64  `json:"nextAttemptAt,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	Via           string `json:"via,omitempty"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
	DeliveredAt   int64  `json:"deliveredAt,omitempty"`
	// BodyRemoved is set by MailListing when a sign-in message's body is gone.
	BodyRemoved bool `json:"-"`
}

// Spent says a sign-in message's body has been removed, now that it was sent
// or given up: the link it carried is no longer live, and it cannot be sent
// again.
func (m Mail) Spent() bool { return m.Secret && (m.BodyRemoved || m.Text == RedactedBody) }

const mailCols = `id,idem_key,template,to_addr,team_id,subject,text_body,html_body,secret,state,attempts,next_attempt_at,last_error,via,created_at,updated_at,delivered_at`

func scanMail(row interface{ Scan(...any) error }) (Mail, error) {
	var m Mail
	var secret int
	if err := row.Scan(&m.ID, &m.Key, &m.Template, &m.To, &m.TeamID, &m.Subject, &m.Text, &m.HTML, &secret, &m.State,
		&m.Attempts, &m.NextAttemptAt, &m.LastError, &m.Via, &m.CreatedAt, &m.UpdatedAt, &m.DeliveredAt); err != nil {
		return Mail{}, err
	}
	m.Secret = secret != 0
	m.BodyRemoved = m.Secret && m.Text == RedactedBody
	return m, nil
}

func newMailID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "mail_" + hex.EncodeToString(b[:])
}

// EnqueueMail adds a message unless one with the same key is already there, in
// which case that one is returned and created is false. The key is what makes
// a notice sent once however often the thing that causes it happens.
func (s *Store) EnqueueMail(m Mail) (Mail, bool, error) {
	if m.Key == "" || m.To == "" {
		return Mail{}, false, errors.New("a message needs a key and a recipient")
	}
	if m.ID == "" {
		m.ID = newMailID()
	}
	if m.State == "" {
		m.State = MailQueued
	}
	t := now()
	secret := 0
	if m.Secret {
		secret = 1
	}
	_, err := s.db.Exec(`INSERT INTO mail_outbox(id,idem_key,template,to_addr,team_id,subject,text_body,html_body,secret,state,attempts,next_attempt_at,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,0,?,?,?)`,
		m.ID, m.Key, m.Template, m.To, m.TeamID, m.Subject, m.Text, m.HTML, secret, m.State, t, t, t)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		existing, gerr := scanMail(s.db.QueryRow(`SELECT `+mailCols+` FROM mail_outbox WHERE idem_key=?`, m.Key))
		if gerr != nil {
			return Mail{}, false, gerr
		}
		return existing, false, nil
	}
	if err != nil {
		return Mail{}, false, fmt.Errorf("adding a message to the outbox: %w", err)
	}
	got, err := s.GetMail(m.ID)
	return got, err == nil, err
}

// GetMail reads one message, bodies included.
func (s *Store) GetMail(id string) (Mail, error) {
	m, err := scanMail(s.db.QueryRow(`SELECT `+mailCols+` FROM mail_outbox WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Mail{}, ErrNotFound
	}
	return m, err
}

// MailListing is the outbox without the bodies, for the admin list that draws
// neither.
func (s *Store) MailListing(state string, limit int) ([]Mail, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT id,idem_key,template,to_addr,team_id,subject,secret,state,attempts,next_attempt_at,last_error,via,created_at,updated_at,delivered_at,text_body=?
		FROM mail_outbox`
	args := []any{RedactedBody}
	if state != "" {
		q += ` WHERE state=?`
		args = append(args, state)
	}
	q += ` ORDER BY created_at DESC,id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Mail{}
	for rows.Next() {
		var m Mail
		var secret, removed int
		if err := rows.Scan(&m.ID, &m.Key, &m.Template, &m.To, &m.TeamID, &m.Subject, &secret, &m.State,
			&m.Attempts, &m.NextAttemptAt, &m.LastError, &m.Via, &m.CreatedAt, &m.UpdatedAt, &m.DeliveredAt, &removed); err != nil {
			return nil, err
		}
		m.Secret, m.BodyRemoved = secret != 0, removed != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// MailCounts is how many messages are in each state.
func (s *Store) MailCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT state,COUNT(*) FROM mail_outbox GROUP BY state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// ClaimMail takes up to limit due messages and marks them sending, so two
// workers never send one message twice.
func (s *Store) ClaimMail(limit int, at int64) ([]Mail, error) {
	if limit <= 0 {
		limit = 20
	}
	t := at
	rows, err := s.db.Query(`UPDATE mail_outbox SET state=?, updated_at=?
		WHERE id IN (SELECT id FROM mail_outbox WHERE state=? AND next_attempt_at<=? ORDER BY next_attempt_at,created_at LIMIT ?)
		RETURNING `+mailCols, MailSending, t, MailQueued, t, limit)
	if err != nil {
		return nil, fmt.Errorf("claiming messages to send: %w", err)
	}
	defer rows.Close()
	out := []Mail{}
	for rows.Next() {
		m, err := scanMail(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MailSent records a delivery. A sign-in message's body is blanked now that it
// has done its job.
func (s *Store) MailSent(id, via string) error {
	t := now()
	_, err := s.db.Exec(`UPDATE mail_outbox SET state=?,via=?,attempts=attempts+1,last_error='',delivered_at=?,updated_at=?,
		text_body=CASE secret WHEN 1 THEN ? ELSE text_body END,
		html_body=CASE secret WHEN 1 THEN ? ELSE html_body END
		WHERE id=?`, MailDelivered, via, t, t, RedactedBody, RedactedBody, id)
	return err
}

// MailRetry records a failed attempt and when to try again.
func (s *Store) MailRetry(id, why string, next int64) error {
	_, err := s.db.Exec(`UPDATE mail_outbox SET state=?,attempts=attempts+1,last_error=?,next_attempt_at=?,updated_at=? WHERE id=?`,
		MailQueued, clip(why), next, now(), id)
	return err
}

// MailGiveUp records the last attempt: failed (out of retries) or bounced (the
// server refused the address for good).
func (s *Store) MailGiveUp(id, state, why string) error {
	if state != MailFailed && state != MailBounced {
		return errors.New("a message is given up as failed or bounced")
	}
	t := now()
	_, err := s.db.Exec(`UPDATE mail_outbox SET state=?,attempts=attempts+1,last_error=?,updated_at=?,
		text_body=CASE secret WHEN 1 THEN ? ELSE text_body END,
		html_body=CASE secret WHEN 1 THEN ? ELSE html_body END
		WHERE id=?`, state, clip(why), t, RedactedBody, RedactedBody, id)
	return err
}

// RequeueMail puts a message back to be sent now, for admin resend. A sign-in
// message whose body has been blanked cannot be: the link it carried is spent.
func (s *Store) RequeueMail(id string) (Mail, error) {
	m, err := s.GetMail(id)
	if err != nil {
		return Mail{}, err
	}
	if m.State == MailSending || m.State == MailQueued {
		return Mail{}, ErrMailWaiting
	}
	if m.Spent() {
		return Mail{}, ErrMailSpent
	}
	t := now()
	if _, err := s.db.Exec(`UPDATE mail_outbox SET state=?,next_attempt_at=?,last_error='',updated_at=? WHERE id=?`,
		MailQueued, t, t, id); err != nil {
		return Mail{}, err
	}
	return s.GetMail(id)
}

// ReleaseMail puts claimed messages that were never tried back in the queue.
func (s *Store) ReleaseMail(ids []string) error {
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE mail_outbox SET state=?,updated_at=? WHERE id=? AND state=?`,
			MailQueued, now(), id, MailSending); err != nil {
			return err
		}
	}
	return nil
}

// RecoverSendingMail puts back messages left marked sending by an instance
// that stopped mid-send. They may have gone; sending again is the better
// mistake, and the Message-ID lets a mail client fold the two into one.
func (s *Store) RecoverSendingMail() (int, error) {
	res, err := s.db.Exec(`UPDATE mail_outbox SET state=?,updated_at=? WHERE state=?`, MailQueued, now(), MailSending)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// PruneMail deletes finished outbox rows older than MailRetention and password
// reset links that have expired. A queued, sending or held message is never
// removed: somebody may still need it sent.
func (s *Store) PruneMail() (int, error) {
	t := now() - int64(MailRetention/time.Millisecond)
	res, err := s.db.Exec(`DELETE FROM mail_outbox WHERE state IN (?,?,?) AND updated_at<?`,
		MailDelivered, MailFailed, MailBounced, t)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`DELETE FROM password_resets WHERE expires_at<=?`, now()); err != nil {
		return int(n), err
	}
	return int(n), nil
}

func clip(s string) string {
	if len(s) > 500 {
		return strings.ToValidUTF8(s[:500], "")
	}
	return s
}

// ---------------------------------------------------------------------------
// Password resets

// CreatePasswordReset issues a reset link, once: only its hash is kept. Any
// earlier unused link for the account stops working, so the newest email is
// the only one that does anything.
func (s *Store) CreatePasswordReset(tokenHash string, userID, expiresAt int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM password_resets WHERE user_id=?`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO password_resets(token_hash,user_id,created_at,expires_at) VALUES(?,?,?,?)`,
			tokenHash, userID, now(), expiresAt)
		return err
	})
}

// ResetPasswordWithToken spends a reset link and sets the password in one
// transaction. It returns the account, so the caller can send a security
// notice; a used, unknown, disabled or expired link returns ok false.
func (s *Store) ResetPasswordWithToken(tokenHash, passwordHash string, at int64) (User, bool, error) {
	var u User
	ok := false
	err := s.withTx(func(tx *sql.Tx) error {
		err := tx.QueryRow(`SELECT u.id,u.email,u.name,u.role,u.password_hash,u.created_at,u.disabled_at
			FROM password_resets r JOIN users u ON u.id=r.user_id
			WHERE r.token_hash=? AND r.used_at=0 AND r.expires_at>? AND u.disabled_at=0`, tokenHash, at).
			Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.PasswordHash, &u.CreatedAt, &u.DisabledAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE password_resets SET used_at=? WHERE token_hash=?`, at, tokenHash); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET password_hash=? WHERE id=?`, passwordHash, u.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id=?`, u.ID); err != nil {
			return err
		}
		ok = true
		return nil
	})
	return u, ok, err
}

// ---------------------------------------------------------------------------
// Team notifications

// TeamNotifyEvents are the kinds a team's administrators may turn on.
var TeamNotifyEvents = []string{"member_joined", "member_left", "member_role", "list_shared"}

// MailRecipient is somebody a message goes to.
type MailRecipient struct {
	Email string `json:"email"`
	Name  string `json:"name,omitempty"`
}

// TeamNotify is one group's notification choice.
type TeamNotify struct {
	GroupID     int64    `json:"groupId"`
	Enabled     bool     `json:"enabled"`
	Events      []string `json:"events"`
	Recipients  string   `json:"recipients"`
	ExtraEmails []string `json:"extraEmails"`
	UpdatedAt   int64    `json:"updatedAt,omitempty"`
	UpdatedBy   int64    `json:"updatedBy,omitempty"`
}

// DefaultTeamNotify is what a group that has never changed the settings gets:
// every kind on, to its owner and managers.
func DefaultTeamNotify(groupID int64) TeamNotify {
	return TeamNotify{GroupID: groupID, Enabled: true, Events: append([]string(nil), TeamNotifyEvents...), Recipients: "managers", ExtraEmails: []string{}}
}

func validTeamNotifyEvent(event string) bool {
	for _, e := range TeamNotifyEvents {
		if e == event {
			return true
		}
	}
	return false
}

// TeamNotifyFor reads a group's choice, or its defaults when it has none.
func (s *Store) TeamNotifyFor(groupID int64) (TeamNotify, error) {
	n := DefaultTeamNotify(groupID)
	var events, extras string
	var enabled int
	err := s.db.QueryRow(`SELECT enabled,events,recipients,extra_emails,updated_at,updated_by FROM team_notify WHERE group_id=?`, groupID).
		Scan(&enabled, &events, &n.Recipients, &extras, &n.UpdatedAt, &n.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return n, nil
	}
	if err != nil {
		return n, err
	}
	n.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(events), &n.Events); err != nil {
		n.Events = []string{}
	}
	if err := json.Unmarshal([]byte(extras), &n.ExtraEmails); err != nil {
		n.ExtraEmails = []string{}
	}
	if n.Events == nil {
		n.Events = []string{}
	}
	if n.ExtraEmails == nil {
		n.ExtraEmails = []string{}
	}
	return n, nil
}

// SaveTeamNotify validates and stores one group's choice.
func (s *Store) SaveTeamNotify(n TeamNotify, by int64) error {
	if n.Recipients != "managers" && n.Recipients != "members" {
		return errors.New("choose managers or every member")
	}
	seen := map[string]bool{}
	events := []string{}
	for _, e := range n.Events {
		if !validTeamNotifyEvent(e) {
			return errors.New("unknown team notification")
		}
		if !seen[e] {
			seen[e] = true
			events = append(events, e)
		}
	}
	extras := []string{}
	seenEmail := map[string]bool{}
	for _, raw := range n.ExtraEmails {
		raw = strings.ToLower(strings.TrimSpace(raw))
		if raw == "" {
			continue
		}
		a, err := mail.ParseAddress(raw)
		if err != nil || a.Name != "" || a.Address != raw || !strings.Contains(raw, "@") {
			return fmt.Errorf("%q is not an email address", raw)
		}
		if seenEmail[raw] {
			continue
		}
		if len(extras) >= 20 {
			return errors.New("add at most 20 extra addresses")
		}
		seenEmail[raw] = true
		extras = append(extras, raw)
	}
	eventsJSON, _ := json.Marshal(events)
	extrasJSON, _ := json.Marshal(extras)
	enabled := 0
	if n.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`INSERT INTO team_notify(group_id,enabled,events,recipients,extra_emails,updated_at,updated_by)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(group_id) DO UPDATE SET enabled=excluded.enabled,events=excluded.events,recipients=excluded.recipients,
			extra_emails=excluded.extra_emails,updated_at=excluded.updated_at,updated_by=excluded.updated_by`,
		n.GroupID, enabled, string(eventsJSON), n.Recipients, string(extrasJSON), now(), by)
	return err
}

// GroupName is a group's name for a message, without asking who is reading.
func (s *Store) GroupName(groupID int64) (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT name FROM audience_groups WHERE id=?`, groupID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return name, err
}

// TeamNotifyRecipients is who a team notice goes to: its owner and managers,
// or every active member, plus any extra addresses the team added. Each
// address appears once and disabled accounts are left out.
func (s *Store) TeamNotifyRecipients(groupID int64, audience string, extra []string) ([]MailRecipient, error) {
	query := `SELECT u.name,u.email FROM audience_groups g JOIN users u ON u.id=g.owner_id
		WHERE g.id=? AND u.disabled_at=0
		UNION
		SELECT u.name,u.email FROM audience_group_members m JOIN users u ON u.id=m.user_id
		WHERE m.group_id=? AND u.disabled_at=0`
	if audience != "members" {
		query = `SELECT u.name,u.email FROM audience_groups g JOIN users u ON u.id=g.owner_id
			WHERE g.id=? AND u.disabled_at=0
			UNION
			SELECT u.name,u.email FROM audience_group_members m JOIN users u ON u.id=m.user_id
			WHERE m.group_id=? AND m.role='manager' AND u.disabled_at=0`
	}
	rows, err := s.db.Query(query, groupID, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MailRecipient{}
	seen := map[string]bool{}
	for rows.Next() {
		var r MailRecipient
		if err := rows.Scan(&r.Name, &r.Email); err != nil {
			return nil, err
		}
		e := strings.ToLower(strings.TrimSpace(r.Email))
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, MailRecipient{Email: r.Email, Name: r.Name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, raw := range extra {
		e := strings.ToLower(strings.TrimSpace(raw))
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, MailRecipient{Email: e})
	}
	return out, nil
}
