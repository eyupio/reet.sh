package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/desk"
)

// MaxDesks is the most desks one account can have.
const MaxDesks = 50

// maxDeskDirBytes bounds a folder a machine reported for a pane.
const maxDeskDirBytes = 4096

var (
	ErrDeskMissing      = errors.New("desk not found")
	ErrDeskStale        = errors.New("the desk changed since it was read")
	ErrDeskInvalid      = errors.New("invalid desk")
	ErrDeskFull         = fmt.Errorf("an account can have at most %d desks", MaxDesks)
	ErrDeskNameTaken    = errors.New("another desk has that name")
	ErrDeskSessionTaken = errors.New("a session can be in only one pane")
)

// Desk is one saved workspace: a name and a document of tabs, split trees and
// panes. A template holds no sessions. Schema is the document schema; a desk
// written by a newer schema is neither listed nor overwritten.
type Desk struct {
	ID, Name, Colour              string
	Template                      bool
	Position                      int
	Doc                           desk.Doc
	Schema                        int
	CreatedAt, UpdatedAt, Version int64
}

func (s *Store) migrateDesks() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS desks (
				id TEXT NOT NULL, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL, name_key TEXT NOT NULL,
				colour TEXT NOT NULL DEFAULT '',
				template INTEGER NOT NULL DEFAULT 0,
				position INTEGER NOT NULL DEFAULT 0,
				doc TEXT NOT NULL DEFAULT '{}',
				schema INTEGER NOT NULL DEFAULT 1,
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
				version INTEGER NOT NULL DEFAULT 1,
				PRIMARY KEY(user_id,id))`,
			`CREATE UNIQUE INDEX IF NOT EXISTS desks_name ON desks(user_id,name_key)`,
			`CREATE TABLE IF NOT EXISTS desk_sessions (
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				session_id TEXT PRIMARY KEY, desk_id TEXT NOT NULL, pane_id TEXT NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS desk_sessions_desk ON desk_sessions(user_id,desk_id)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

func deskInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrDeskInvalid, fmt.Sprintf(format, args...))
}

// normalizeDesk checks the editable fields and returns the document in the
// form it is stored (parsed, so ratios are clamped and empties are not nil).
func normalizeDesk(d Desk) (Desk, string, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" || utf8.RuneCountInString(d.Name) > desk.MaxName || strings.ContainsFunc(d.Name, unicode.IsControl) {
		return d, "", deskInvalid("a desk name is 1 to %d characters of plain text", desk.MaxName)
	}
	if d.Colour != "" && !validFolderProjectColour(d.Colour) {
		return d, "", deskInvalid("colour")
	}
	raw, err := json.Marshal(d.Doc)
	if err != nil {
		return d, "", err
	}
	doc, err := desk.Parse(raw)
	if err != nil {
		return d, "", fmt.Errorf("%w: %v", ErrDeskInvalid, err)
	}
	if d.Template && len(doc.Sessions()) > 0 {
		return d, "", deskInvalid("a template holds no sessions")
	}
	if raw, err = json.Marshal(doc); err != nil {
		return d, "", err
	}
	d.Doc, d.Schema = doc, desk.Schema
	return d, string(raw), nil
}

const deskColumns = `id,name,colour,template,position,doc,schema,created_at,updated_at,version`

func scanDesk(r rowScanner) (Desk, error) {
	var d Desk
	var doc string
	if err := r.Scan(&d.ID, &d.Name, &d.Colour, &d.Template, &d.Position, &doc, &d.Schema, &d.CreatedAt, &d.UpdatedAt, &d.Version); err != nil {
		return Desk{}, err
	}
	parsed, err := desk.Parse([]byte(doc))
	if err != nil {
		return Desk{}, fmt.Errorf("stored desk %s: %w", d.ID, err)
	}
	d.Doc = parsed
	return d, nil
}

func deskConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		if strings.Contains(err.Error(), "name_key") {
			return ErrDeskNameTaken
		}
		if strings.Contains(err.Error(), "session_id") {
			return ErrDeskSessionTaken
		}
	}
	return err
}

// writeDeskSessions rewrites the session index of one desk from its document.
func writeDeskSessions(tx *sql.Tx, uid int64, id string, doc desk.Doc) error {
	if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE user_id=? AND desk_id=?`, uid, id); err != nil {
		return err
	}
	for _, t := range doc.Tabs {
		for pid, p := range t.Panes {
			if p.Session == "" {
				continue
			}
			if _, err := tx.Exec(`INSERT INTO desk_sessions(user_id,session_id,desk_id,pane_id) VALUES(?,?,?,?)`, uid, p.Session, id, pid); err != nil {
				return deskConflict(err)
			}
		}
	}
	return nil
}

// CreateDeskFor stores a new desk at the end of uid's list. ID, Position,
// times, Schema and Version are assigned here.
func (s *Store) CreateDeskFor(uid int64, d Desk) (Desk, error) {
	d, raw, err := normalizeDesk(d)
	if err != nil {
		return Desk{}, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if err := s.userExists(tx, uid); err != nil {
			return ErrDeskMissing
		}
		var n, next int
		if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(MAX(position)+1,0) FROM desks WHERE user_id=?`, uid).Scan(&n, &next); err != nil {
			return err
		}
		if n >= MaxDesks {
			return ErrDeskFull
		}
		if d.ID, err = projectRandomID(); err != nil {
			return err
		}
		t := now()
		d.Position, d.CreatedAt, d.UpdatedAt, d.Version = next, t, t, 1
		if _, err := tx.Exec(`INSERT INTO desks(id,user_id,name,name_key,colour,template,position,doc,schema,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?,?,1)`,
			d.ID, uid, d.Name, nameKey(d.Name), d.Colour, d.Template, d.Position, raw, d.Schema, t, t); err != nil {
			return deskConflict(err)
		}
		return writeDeskSessions(tx, uid, d.ID, d.Doc)
	})
	if err != nil {
		return Desk{}, err
	}
	return d, nil
}

// DeskFor returns one of uid's desks.
func (s *Store) DeskFor(uid int64, id string) (Desk, error) {
	d, err := scanDesk(s.db.QueryRow(`SELECT `+deskColumns+` FROM desks WHERE id=? AND user_id=? AND schema<=?`, id, uid, desk.Schema))
	if errors.Is(err, sql.ErrNoRows) {
		return Desk{}, ErrDeskMissing
	}
	return d, err
}

// ListDesksFor lists uid's desks by position, leaving out any written by a
// newer schema.
func (s *Store) ListDesksFor(uid int64) ([]Desk, error) {
	rows, err := s.db.Query(`SELECT `+deskColumns+` FROM desks WHERE user_id=? AND schema<=? ORDER BY position,id`, uid, desk.Schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Desk{}
	for rows.Next() {
		d, err := scanDesk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// keepNewerDirs copies the stored Dir and DirAt onto any pane of doc whose
// stored DirAt is newer, so a whole-document write cannot undo a folder the
// machine reported more recently.
func keepNewerDirs(doc *desk.Doc, stored desk.Doc) {
	have := map[string]desk.Pane{}
	for _, t := range stored.Tabs {
		for id, p := range t.Panes {
			have[t.ID+"\x00"+id] = p
		}
	}
	for _, t := range doc.Tabs {
		for id, p := range t.Panes {
			if old, ok := have[t.ID+"\x00"+id]; ok && old.DirAt > p.DirAt {
				p.Dir, p.DirAt = old.Dir, old.DirAt
				t.Panes[id] = p
			}
		}
	}
}

// UpdateDeskFor replaces the name, colour, template flag and document of
// desk d.ID. expect must be the stored version (ErrDeskStale
// otherwise, and the caller re-reads). The
// version goes up by one. Position and CreatedAt are kept.
func (s *Store) UpdateDeskFor(uid int64, d Desk, expect int64) (Desk, error) {
	if expect <= 0 {
		return Desk{}, ErrDeskStale
	}
	id := d.ID
	d, raw, err := normalizeDesk(d)
	if err != nil {
		return Desk{}, err
	}
	d.ID = id
	err = s.withTx(func(tx *sql.Tx) error {
		cur, err := scanDesk(tx.QueryRow(`SELECT `+deskColumns+` FROM desks WHERE id=? AND user_id=? AND schema<=?`, id, uid, desk.Schema))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeskMissing
		}
		if err != nil {
			return err
		}
		if expect != 0 && expect != cur.Version {
			return ErrDeskStale
		}
		keepNewerDirs(&d.Doc, cur.Doc)
		b, err := json.Marshal(d.Doc)
		if err != nil {
			return err
		}
		if len(b) > desk.MaxDocBytes {
			return deskInvalid("document over %d bytes", desk.MaxDocBytes)
		}
		raw = string(b)
		t := now()
		if t < cur.CreatedAt {
			t = cur.CreatedAt
		}
		d.Position, d.CreatedAt, d.UpdatedAt, d.Version = cur.Position, cur.CreatedAt, t, cur.Version+1
		res, err := tx.Exec(`UPDATE desks SET name=?,name_key=?,colour=?,template=?,doc=?,schema=?,updated_at=?,version=? WHERE id=? AND user_id=? AND version=?`,
			d.Name, nameKey(d.Name), d.Colour, d.Template, raw, d.Schema, t, d.Version, id, uid, cur.Version)
		if err != nil {
			return deskConflict(err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrDeskStale
		}
		return writeDeskSessions(tx, uid, id, d.Doc)
	})
	if err != nil {
		return Desk{}, err
	}
	return d, nil
}

// DeleteDeskFor removes a desk. expect must match its version.
func (s *Store) DeleteDeskFor(uid int64, id string, expect int64) error {
	if expect <= 0 {
		return ErrDeskStale
	}
	return s.withTx(func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRow(`SELECT version FROM desks WHERE id=? AND user_id=? AND schema<=?`, id, uid, desk.Schema).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrDeskMissing
			}
			return err
		}
		if expect != 0 && expect != version {
			return ErrDeskStale
		}
		if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE user_id=? AND desk_id=?`, uid, id); err != nil {
			return err
		}
		res, err := tx.Exec(`DELETE FROM desks WHERE id=? AND user_id=? AND version=?`, id, uid, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrDeskStale
		}
		return nil
	})
}

// ReorderDesksFor sets the order of uid's listed desks; ids must name each of
// them once. Order is not part of the document, so versions do not change.
func (s *Store) ReorderDesksFor(uid int64, ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return deskInvalid("a desk appears twice")
		}
		seen[id] = true
	}
	return s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id FROM desks WHERE user_id=? AND schema<=?`, uid, desk.Schema)
		if err != nil {
			return err
		}
		have := 0
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			have++
			if !seen[id] {
				rows.Close()
				return deskInvalid("every desk must be named once")
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if have != len(ids) {
			return deskInvalid("every desk must be named once")
		}
		for i, id := range ids {
			if _, err := tx.Exec(`UPDATE desks SET position=? WHERE id=? AND user_id=?`, i, id, uid); err != nil {
				return err
			}
		}
		return nil
	})
}

// findPane returns the tab holding the pane. Pane ids are unique across a desk
// (desk.Doc.Validate enforces it), so at most one tab matches.
func findPane(doc *desk.Doc, pane string) (*desk.Tab, bool) {
	for i := range doc.Tabs {
		if _, ok := doc.Tabs[i].Panes[pane]; ok {
			return &doc.Tabs[i], true
		}
	}
	return nil, false
}

// SetDeskPaneSessionFor sets (or, when empty, clears) the session of one pane
// and bumps the desk's version.
func (s *Store) SetDeskPaneSessionFor(uid int64, deskID, pane, session string) (Desk, error) {
	var saved Desk
	err := s.withTx(func(tx *sql.Tx) error {
		d, err := scanDesk(tx.QueryRow(`SELECT `+deskColumns+` FROM desks WHERE id=? AND user_id=? AND schema<=?`, deskID, uid, desk.Schema))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeskMissing
		}
		if err != nil {
			return err
		}
		t, ok := findPane(&d.Doc, pane)
		if !ok {
			return deskInvalid("pane not in the desk")
		}
		if session != "" && d.Template {
			return deskInvalid("a template holds no sessions")
		}
		p := t.Panes[pane]
		p.Session = session
		t.Panes[pane] = p
		if err := d.Doc.Validate(); err != nil {
			if session != "" {
				return ErrDeskSessionTaken
			}
			return fmt.Errorf("%w: %v", ErrDeskInvalid, err)
		}
		saved, err = saveDeskDoc(tx, uid, d, true)
		return err
	})
	if err != nil {
		return Desk{}, err
	}
	return saved, nil
}

// saveDeskDoc writes d's document and its session index, bumping the version
// when bump is set.
func saveDeskDoc(tx *sql.Tx, uid int64, d Desk, bump bool) (Desk, error) {
	b, err := json.Marshal(d.Doc)
	if err != nil {
		return d, err
	}
	if len(b) > desk.MaxDocBytes {
		return d, deskInvalid("document over %d bytes", desk.MaxDocBytes)
	}
	if bump {
		t := now()
		if t < d.CreatedAt {
			t = d.CreatedAt
		}
		d.UpdatedAt, d.Version = t, d.Version+1
		_, err = tx.Exec(`UPDATE desks SET doc=?,updated_at=?,version=? WHERE id=? AND user_id=?`, string(b), d.UpdatedAt, d.Version, d.ID, uid)
	} else {
		_, err = tx.Exec(`UPDATE desks SET doc=? WHERE id=? AND user_id=?`, string(b), d.ID, uid)
	}
	if err != nil {
		return d, deskConflict(err)
	}
	return d, writeDeskSessions(tx, uid, d.ID, d.Doc)
}

// DeskPaneForSession says which desk and pane hold a session.
func (s *Store) DeskPaneForSession(uid int64, session string) (deskID, pane string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT ds.desk_id,ds.pane_id FROM desk_sessions ds JOIN desks d ON d.id=ds.desk_id AND d.user_id=ds.user_id WHERE ds.user_id=? AND ds.session_id=? AND d.schema<=?`, uid, session, desk.Schema).Scan(&deskID, &pane)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return deskID, pane, true, nil
}

// NoteDeskPaneDirFor records the folder a machine reported for the pane that
// holds session, if at is newer than what is stored. It does not change the
// desk's version: a folder change never makes a browser's write stale. A
// session in no desk is ignored.
func (s *Store) NoteDeskPaneDirFor(uid int64, session, dir string, at int64) error {
	if len(dir) > maxDeskDirBytes || strings.ContainsRune(dir, 0) {
		return deskInvalid("folder")
	}
	return s.withTx(func(tx *sql.Tx) error {
		var deskID, paneID string
		err := tx.QueryRow(`SELECT desk_id,pane_id FROM desk_sessions WHERE user_id=? AND session_id=?`, uid, session).Scan(&deskID, &paneID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		d, err := scanDesk(tx.QueryRow(`SELECT `+deskColumns+` FROM desks WHERE id=? AND user_id=? AND schema<=?`, deskID, uid, desk.Schema))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		t, ok := findPane(&d.Doc, paneID)
		if !ok {
			return nil
		}
		p := t.Panes[paneID]
		if at <= p.DirAt {
			return nil
		}
		p.Dir, p.DirAt = dir, at
		t.Panes[paneID] = p
		_, err = saveDeskDoc(tx, uid, d, false)
		return err
	})
}

// rewriteDesksTx applies edit to every listed desk of uid and saves those it
// changed, with a version bump. It is used inside machine forget and merge.
func rewriteDesksTx(tx *sql.Tx, uid int64, edit func(*desk.Doc) bool) error {
	rows, err := tx.Query(`SELECT `+deskColumns+` FROM desks WHERE user_id=? AND schema<=?`, uid, desk.Schema)
	if err != nil {
		return err
	}
	var all []Desk
	for rows.Next() {
		d, err := scanDesk(rows)
		if err != nil {
			rows.Close()
			return err
		}
		all = append(all, d)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, d := range all {
		if edit(&d.Doc) {
			if _, err := saveDeskDoc(tx, uid, d, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// agentIDsForHost lists the enrollment ids of a machine name.
func agentIDsForHost(tx *sql.Tx, uid int64, host string) ([]string, error) {
	rows, err := tx.Query(`SELECT id FROM sync_agents WHERE user_id=? AND host=? COLLATE NOCASE`, uid, host)
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

// clearDeskSessionsForHostsTx empties the session of every desk pane on any
// of the machines (enrollment ids).
func clearDeskSessionsForHostsTx(tx *sql.Tx, uid int64, hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	on := map[string]bool{}
	for _, h := range hosts {
		on[h] = true
	}
	return rewriteDesksTx(tx, uid, func(doc *desk.Doc) bool {
		changed := false
		for i := range doc.Tabs {
			for id, p := range doc.Tabs[i].Panes {
				if on[p.Host] && p.Session != "" {
					p.Session = ""
					doc.Tabs[i].Panes[id] = p
					changed = true
				}
			}
		}
		return changed
	})
}
