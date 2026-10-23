package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Support tickets live here, with the rest of the SQL. A person opens one for
// themselves or for a team they belong to; the team's owner and managers can
// read and answer its tickets, and instance administrators see every ticket.
// Internal notes are only ever returned when staff ask for them.

const (
	TicketOpen    = "open"
	TicketWaiting = "waiting"
	TicketClosed  = "closed"
)

const (
	MaxTicketSubject = 200
	MaxTicketBody    = 20000
)

func ValidTicketStatus(status string) bool {
	return status == TicketOpen || status == TicketWaiting || status == TicketClosed
}

// Ticket is one support conversation.
type Ticket struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"userId,omitempty"`
	Opener      string `json:"opener"`
	OpenerEmail string `json:"-"`
	GroupID     int64  `json:"groupId,omitempty"`
	GroupName   string `json:"groupName,omitempty"`
	Subject     string `json:"subject"`
	Status      string `json:"status"`
	AssigneeID  int64  `json:"assigneeId,omitempty"`
	Assignee    string `json:"assignee,omitempty"`
	Messages    int    `json:"messages"`
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// TicketMessage is one reply or internal note.
type TicketMessage struct {
	ID         int64  `json:"id"`
	TicketID   int64  `json:"ticketId"`
	AuthorID   int64  `json:"authorId,omitempty"`
	AuthorKind string `json:"authorKind"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	Internal   bool   `json:"internal,omitempty"`
	CreatedAt  int64  `json:"createdAt"`
}

// TicketFilter narrows a ticket list.
type TicketFilter struct {
	Status   string
	Assignee int64
	Limit    int
}

func (s *Store) migrateTickets() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS tickets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			opener TEXT NOT NULL DEFAULT '',
			opener_email TEXT NOT NULL DEFAULT '',
			group_id INTEGER REFERENCES audience_groups(id) ON DELETE SET NULL,
			group_name TEXT NOT NULL DEFAULT '',
			subject TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open','waiting','closed')),
			assignee_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS ticket_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			ticket_id INTEGER NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
			author_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
			author_kind TEXT NOT NULL CHECK(author_kind IN ('user','team','admin')),
			author TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL,
			internal INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_updated ON tickets(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_user ON tickets(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_group ON tickets(group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket ON ticket_messages(ticket_id, created_at)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func ticketText(value string, max int) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return "", errors.New("write what the message says")
	case utf8.RuneCountInString(value) > max:
		return "", fmt.Errorf("a message is at most %d characters", max)
	}
	return value, nil
}

// OpenTicket starts a ticket with its first message. groupID is 0 for a
// personal ticket; otherwise groupName is kept as a snapshot so the ticket
// still says what it was about after the group is deleted.
func (s *Store) OpenTicket(userID int64, opener, openerEmail string, groupID int64, groupName, subject, body string) (Ticket, error) {
	subject = strings.Join(strings.Fields(subject), " ")
	if subject == "" || utf8.RuneCountInString(subject) > MaxTicketSubject {
		return Ticket{}, fmt.Errorf("a ticket needs a subject of up to %d characters", MaxTicketSubject)
	}
	body, err := ticketText(body, MaxTicketBody)
	if err != nil {
		return Ticket{}, err
	}
	t := now()
	var group any
	if groupID != 0 {
		group = groupID
	}
	var id int64
	err = s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO tickets(user_id,opener,opener_email,group_id,group_name,subject,status,created_at,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?)`, userID, strings.TrimSpace(opener), strings.TrimSpace(openerEmail), group,
			strings.TrimSpace(groupName), subject, TicketOpen, t, t)
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO ticket_messages(ticket_id,author_id,author_kind,author,body,internal,created_at)
			VALUES(?,?,?,?,?,0,?)`, id, userID, "user", strings.TrimSpace(opener), body, t)
		return err
	})
	if err != nil {
		return Ticket{}, err
	}
	return s.GetTicket(id)
}

const ticketCols = `t.id,COALESCE(t.user_id,0),t.opener,t.opener_email,COALESCE(t.group_id,0),t.group_name,t.subject,t.status,
	COALESCE(t.assignee_id,0),COALESCE(au.name,''),t.created_at,t.updated_at,
	(SELECT COUNT(*) FROM ticket_messages m WHERE m.ticket_id=t.id AND m.internal=0)`

func scanTicket(row interface{ Scan(...any) error }) (Ticket, error) {
	var t Ticket
	if err := row.Scan(&t.ID, &t.UserID, &t.Opener, &t.OpenerEmail, &t.GroupID, &t.GroupName, &t.Subject, &t.Status,
		&t.AssigneeID, &t.Assignee, &t.CreatedAt, &t.UpdatedAt, &t.Messages); err != nil {
		return Ticket{}, err
	}
	return t, nil
}

// GetTicket reads one ticket.
func (s *Store) GetTicket(id int64) (Ticket, error) {
	t, err := scanTicket(s.db.QueryRow(`SELECT `+ticketCols+` FROM tickets t LEFT JOIN users au ON au.id=t.assignee_id WHERE t.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	return t, err
}

func (s *Store) ticketQuery(filter TicketFilter, userID int64, admin bool) (string, []any) {
	q := `SELECT ` + ticketCols + ` FROM tickets t LEFT JOIN users au ON au.id=t.assignee_id WHERE 1=1`
	args := []any{}
	if !admin {
		q += ` AND (t.user_id=? OR (t.group_id IS NOT NULL AND t.group_id IN (
			SELECT id FROM audience_groups WHERE owner_id=?
			UNION SELECT group_id FROM audience_group_members WHERE user_id=? AND role IN ('owner','manager'))))`
		args = append(args, userID, userID, userID)
	}
	if filter.Status != "" {
		q += ` AND t.status=?`
		args = append(args, filter.Status)
	}
	if filter.Assignee != 0 {
		q += ` AND t.assignee_id=?`
		args = append(args, filter.Assignee)
	}
	return q, args
}

// TicketsFor lists the tickets a person can see: their own, and the tickets
// of teams they own or manage. An instance administrator sees them all.
func (s *Store) TicketsFor(userID int64, role string, filter TicketFilter) ([]Ticket, error) {
	admin := role == "owner" || role == "admin"
	q, args := s.ticketQuery(filter, userID, admin)
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	args = append(args, limit)
	rows, err := s.db.Query(q+` ORDER BY t.updated_at DESC,t.id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Ticket{}
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TicketCounts is how many tickets are in each status.
func (s *Store) TicketCounts() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT status,COUNT(*) FROM tickets GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{TicketOpen: 0, TicketWaiting: 0, TicketClosed: 0}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out[status] = n
	}
	return out, rows.Err()
}

// TicketMessages lists a thread oldest first. Internal notes are returned
// only to a caller that asks for them.
func (s *Store) TicketMessages(ticketID int64, includeInternal bool) ([]TicketMessage, error) {
	q := `SELECT id,ticket_id,COALESCE(author_id,0),author_kind,author,body,internal,created_at
		FROM ticket_messages WHERE ticket_id=?`
	if !includeInternal {
		q += ` AND internal=0`
	}
	rows, err := s.db.Query(q+` ORDER BY created_at,id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TicketMessage{}
	for rows.Next() {
		var m TicketMessage
		var internal int
		if err := rows.Scan(&m.ID, &m.TicketID, &m.AuthorID, &m.AuthorKind, &m.Author, &m.Body, &internal, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Internal = internal != 0
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddTicketMessage adds a reply or note and moves the ticket along. A
// customer-side reply reopens it; a team or admin reply waits on the
// requester; a note changes nothing a requester can see. status, when set,
// overrides that and is how staff close or reopen deliberately.
func (s *Store) AddTicketMessage(ticketID, authorID int64, authorKind, author, body string, internal bool, status string) (Ticket, TicketMessage, error) {
	body, err := ticketText(body, MaxTicketBody)
	if err != nil {
		return Ticket{}, TicketMessage{}, err
	}
	if authorKind != "user" && authorKind != "team" && authorKind != "admin" {
		return Ticket{}, TicketMessage{}, errors.New("a message is from the requester, a team manager or an administrator")
	}
	if internal && authorKind != "admin" {
		return Ticket{}, TicketMessage{}, errors.New("only an instance administrator writes internal notes")
	}
	t, err := s.GetTicket(ticketID)
	if err != nil {
		return Ticket{}, TicketMessage{}, err
	}
	next := t.Status
	switch {
	case internal:
	case status != "":
		if !ValidTicketStatus(status) {
			return Ticket{}, TicketMessage{}, errors.New("a ticket is open, waiting or closed")
		}
		next = status
	case authorKind == "user":
		next = TicketOpen
	default:
		next = TicketWaiting
	}
	timestamp := now()
	var msg TicketMessage
	err = s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO ticket_messages(ticket_id,author_id,author_kind,author,body,internal,created_at)
			VALUES(?,?,?,?,?,?,?)`, t.ID, authorID, authorKind, strings.TrimSpace(author), body, boolInt(internal), timestamp)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		msg = TicketMessage{ID: id, TicketID: t.ID, AuthorID: authorID, AuthorKind: authorKind, Author: strings.TrimSpace(author),
			Body: body, Internal: internal, CreatedAt: timestamp}
		if internal {
			return nil
		}
		_, err = tx.Exec(`UPDATE tickets SET status=?,updated_at=? WHERE id=?`, next, timestamp, t.ID)
		return err
	})
	if err != nil {
		return Ticket{}, TicketMessage{}, err
	}
	if !internal {
		t.Status, t.UpdatedAt = next, timestamp
		t.Messages++
	}
	return t, msg, nil
}

// SetTicketStatus moves a ticket without a message.
func (s *Store) SetTicketStatus(ticketID int64, status string) error {
	if !ValidTicketStatus(status) {
		return errors.New("a ticket is open, waiting or closed")
	}
	res, err := s.db.Exec(`UPDATE tickets SET status=?,updated_at=? WHERE id=?`, status, now(), ticketID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AssignTicket gives a ticket to an instance administrator, or to nobody with 0.
func (s *Store) AssignTicket(ticketID, assigneeID int64) error {
	if assigneeID != 0 {
		var role string
		var disabled int64
		err := s.db.QueryRow(`SELECT role,disabled_at FROM users WHERE id=?`, assigneeID).Scan(&role, &disabled)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (disabled != 0 || (role != "owner" && role != "admin"))) {
			return errors.New("assign a ticket to an active instance administrator")
		}
		if err != nil {
			return err
		}
	}
	var assignee any
	if assigneeID != 0 {
		assignee = assigneeID
	}
	res, err := s.db.Exec(`UPDATE tickets SET assignee_id=?,updated_at=? WHERE id=?`, assignee, now(), ticketID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AdminRecipients is the owner and every active instance administrator, for
// notices that must reach whoever is on support.
func (s *Store) AdminRecipients() ([]MailRecipient, error) {
	rows, err := s.db.Query(`SELECT id,name,email,role FROM users
		WHERE role IN ('owner','admin') AND disabled_at=0 ORDER BY lower(name),lower(email)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MailRecipient{}
	for rows.Next() {
		var r MailRecipient
		if err := rows.Scan(&r.ID, &r.Name, &r.Email, &r.Role); err != nil {
			return nil, err
		}
		r.Admin = true
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
