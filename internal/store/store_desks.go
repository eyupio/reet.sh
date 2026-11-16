package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
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
	// errStoredDesk is a desk row whose document cannot be read back.
	errStoredDesk = errors.New("stored desk")
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
			// desk_pane_dirs is the folder a machine itself last reported for a
			// pane's shell. A browser writes the document's dir and dirAt freely,
			// so a reopen outside home trusts only this record.
			`CREATE TABLE IF NOT EXISTS desk_pane_dirs (
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				desk_id TEXT NOT NULL, pane_id TEXT NOT NULL,
				agent_id TEXT NOT NULL, dir TEXT NOT NULL, dir_at INTEGER NOT NULL,
				PRIMARY KEY(user_id,desk_id,pane_id))`,
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
	if d.Name == "" || utf8.RuneCountInString(d.Name) > desk.MaxName || desk.HasUnsafe(d.Name) {
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
	parsed, err := desk.ParseStored([]byte(doc))
	if err != nil {
		return Desk{}, fmt.Errorf("%w %s: %v", errStoredDesk, d.ID, err)
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

// writeDeskSessions rewrites the session index of one desk from its document,
// and forgets the reported folder of every pane the document no longer has.
func writeDeskSessions(tx *sql.Tx, uid int64, id string, doc desk.Doc) error {
	if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE user_id=? AND desk_id=?`, uid, id); err != nil {
		return err
	}
	if err := pruneDeskPaneDirs(tx, uid, id, doc); err != nil {
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
		capDirTimes(&d.Doc, t)
		reports, err := adoptLiveDirsTx(tx, uid, &d.Doc)
		if err != nil {
			return err
		}
		// The cap and the adopted folders change the document after it was
		// normalised, so store what is now in d.Doc.
		b, err := json.Marshal(d.Doc)
		if err != nil {
			return err
		}
		if len(b) > desk.MaxDocBytes {
			return deskInvalid("document over %d bytes", desk.MaxDocBytes)
		}
		raw = string(b)
		if _, err := tx.Exec(`INSERT INTO desks(id,user_id,name,name_key,colour,template,position,doc,schema,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?,?,1)`,
			d.ID, uid, d.Name, nameKey(d.Name), d.Colour, d.Template, d.Position, raw, d.Schema, t, t); err != nil {
			return deskConflict(err)
		}
		if err := writeDeskSessions(tx, uid, d.ID, d.Doc); err != nil {
			return err
		}
		return reports.save(tx, uid, d.ID)
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
// newer schema and any whose stored document cannot be read.
func (s *Store) ListDesksFor(uid int64) ([]Desk, error) {
	rows, err := s.db.Query(`SELECT `+deskColumns+` FROM desks WHERE user_id=? AND schema<=? ORDER BY position,id`, uid, desk.Schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Desk{}
	for rows.Next() {
		d, err := scanDesk(rows)
		if errors.Is(err, errStoredDesk) {
			// One desk that cannot be read does not hide the others.
			log.Printf("desks: %v; left out of the list", err)
			continue
		}
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
	// Pane ids are unique across a desk, so a pane moved to another tab is
	// still matched.
	have := map[string]desk.Pane{}
	for _, t := range stored.Tabs {
		for id, p := range t.Panes {
			have[id] = p
		}
	}
	for _, t := range doc.Tabs {
		for id, p := range t.Panes {
			if old, ok := have[id]; ok && old.DirAt > p.DirAt {
				p.Dir, p.DirAt = old.Dir, old.DirAt
				t.Panes[id] = p
			}
		}
	}
}

// capDirTimes keeps every pane's dirAt no later than at (the controller's
// clock), so a browser cannot freeze a pane's folder against later machine
// reports with a far-future time.
func capDirTimes(doc *desk.Doc, at int64) {
	for i := range doc.Tabs {
		for id, p := range doc.Tabs[i].Panes {
			if p.DirAt > at {
				p.DirAt = at
				doc.Tabs[i].Panes[id] = p
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
		if expect != cur.Version {
			return ErrDeskStale
		}
		capDirTimes(&d.Doc, now())
		keepNewerDirs(&d.Doc, cur.Doc)
		reports, err := adoptLiveDirsTx(tx, uid, &d.Doc)
		if err != nil {
			return err
		}
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
		if err := writeDeskSessions(tx, uid, id, d.Doc); err != nil {
			return err
		}
		return reports.save(tx, uid, id)
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
		if expect != version {
			return ErrDeskStale
		}
		if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE user_id=? AND desk_id=?`, uid, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM desk_pane_dirs WHERE user_id=? AND desk_id=?`, uid, id); err != nil {
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

// ReopenDeskPaneFor puts a reopened shell's session into a pane, bumping the
// desk's version, only while the desk is still at version expect (ErrDeskStale
// otherwise), and makes the pane a shell: an agent pane reopens as a plain
// shell. A session the machine already reported a folder for gives the pane
// that folder when it is newer than the pane's.
func (s *Store) ReopenDeskPaneFor(uid int64, deskID, pane, session string, expect int64) (Desk, error) {
	if expect <= 0 || session == "" {
		return Desk{}, ErrDeskStale
	}
	return s.setDeskPaneSession(uid, deskID, pane, session, expect, true)
}

// UnbindDeskPaneSessionFor clears a pane's session if it is still session (a
// reopen whose create failed) and gives it back kind ("shell" or "agent"; any
// other value leaves the kind), bumping the version; otherwise it changes
// nothing and returns the desk as it is.
func (s *Store) UnbindDeskPaneSessionFor(uid int64, deskID, pane, session, kind string) (Desk, error) {
	var saved Desk
	err := s.withTx(func(tx *sql.Tx) error {
		d, err := deskForTx(tx, uid, deskID)
		if err != nil {
			return err
		}
		saved = d
		t, ok := findPane(&d.Doc, pane)
		if !ok || session == "" || t.Panes[pane].Session != session {
			return nil
		}
		p := t.Panes[pane]
		p.Session = ""
		if kind == "shell" || kind == "agent" {
			p.Kind = kind
		}
		t.Panes[pane] = p
		saved, err = saveDeskDoc(tx, uid, d, true)
		return err
	})
	if err != nil {
		return Desk{}, err
	}
	return saved, nil
}

func deskForTx(tx *sql.Tx, uid int64, deskID string) (Desk, error) {
	d, err := scanDesk(tx.QueryRow(`SELECT `+deskColumns+` FROM desks WHERE id=? AND user_id=? AND schema<=?`, deskID, uid, desk.Schema))
	if errors.Is(err, sql.ErrNoRows) {
		return Desk{}, ErrDeskMissing
	}
	return d, err
}

// setDeskPaneSession writes one pane's session. expect, when non-zero, must be
// the desk's version; shell makes the pane a shell.
func (s *Store) setDeskPaneSession(uid int64, deskID, pane, session string, expect int64, shell bool) (Desk, error) {
	var saved Desk
	err := s.withTx(func(tx *sql.Tx) error {
		d, err := deskForTx(tx, uid, deskID)
		if err != nil {
			return err
		}
		if expect != 0 && d.Version != expect {
			return ErrDeskStale
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
		if shell {
			p.Kind = "shell"
		}
		t.Panes[pane] = p
		if err := d.Doc.Validate(); err != nil {
			if session != "" {
				return ErrDeskSessionTaken
			}
			return fmt.Errorf("%w: %v", ErrDeskInvalid, err)
		}
		reports, err := adoptLiveDirsTx(tx, uid, &d.Doc)
		if err != nil {
			return err
		}
		if saved, err = saveDeskDoc(tx, uid, d, true); err != nil {
			return err
		}
		return reports.save(tx, uid, deskID)
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

// noteDeskPaneDirTx records the folder a machine reported for the pane that
// holds session, if at is newer than what is stored. It does not change the
// desk's version: a folder change never makes a browser's write stale. A
// session in no desk is ignored. The folder also becomes the pane's reported
// folder (DeskPaneReportedDirFor), tied to the machine the session runs on; a
// session reet has no record of updates the document only.
func noteDeskPaneDirTx(tx *sql.Tx, uid int64, session, dir string, at int64) error {
	if len(dir) > maxDeskDirBytes || strings.ContainsRune(dir, 0) {
		return deskInvalid("folder")
	}
	var deskID, paneID string
	err := tx.QueryRow(`SELECT desk_id,pane_id FROM desk_sessions WHERE user_id=? AND session_id=?`, uid, session).Scan(&deskID, &paneID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	d, err := deskForTx(tx, uid, deskID)
	if errors.Is(err, ErrDeskMissing) {
		return nil
	}
	if err != nil {
		return err
	}
	t, ok := findPane(&d.Doc, paneID)
	if !ok {
		return nil
	}
	var agent string
	err = tx.QueryRow(`SELECT agent_id FROM terminal_sessions WHERE id=? AND user_id=?`, session, uid).Scan(&agent)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if agent != "" {
		if err := (paneReports{{paneID, agent, dir, at}}).save(tx, uid, deskID); err != nil {
			return err
		}
	}
	p := t.Panes[paneID]
	if at <= p.DirAt {
		return nil
	}
	p.Dir, p.DirAt = dir, at
	t.Panes[paneID] = p
	_, err = saveDeskDoc(tx, uid, d, false)
	return err
}

// paneReport is one folder a machine reported for a pane's shell.
type paneReport struct {
	pane, agent, dir string
	at               int64
}

type paneReports []paneReport

// save keeps each report as its pane's reported folder unless a newer one is
// stored.
func (r paneReports) save(tx *sql.Tx, uid int64, deskID string) error {
	for _, p := range r {
		if _, err := tx.Exec(`INSERT INTO desk_pane_dirs(user_id,desk_id,pane_id,agent_id,dir,dir_at) VALUES(?,?,?,?,?,?)
			ON CONFLICT(user_id,desk_id,pane_id) DO UPDATE SET agent_id=excluded.agent_id,dir=excluded.dir,dir_at=excluded.dir_at WHERE excluded.dir_at>desk_pane_dirs.dir_at`,
			uid, deskID, p.pane, p.agent, p.dir, p.at); err != nil {
			return err
		}
	}
	return nil
}

// adoptLiveDirsTx gives every pane of doc that holds a live session (creating,
// running or end_requested) the folder its machine last reported for that
// session, when that is newer than the pane's own, so a session placed on a
// desk after its last folder change still has its folder. It returns the
// reports to save as the panes' reported folders once the desk row exists. An
// ended or lost session is not that pane's shell: a document write binding one
// copies nothing and moves no trust.
func adoptLiveDirsTx(tx *sql.Tx, uid int64, doc *desk.Doc) (paneReports, error) {
	var out paneReports
	for i := range doc.Tabs {
		for id, p := range doc.Tabs[i].Panes {
			if p.Session == "" {
				continue
			}
			var agent, dir string
			var at int64
			err := tx.QueryRow(`SELECT agent_id,live_dir,live_dir_at FROM terminal_sessions WHERE id=? AND user_id=? AND state IN ('creating','running','end_requested')`, p.Session, uid).Scan(&agent, &dir, &at)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if dir == "" || at <= 0 {
				continue
			}
			out = append(out, paneReport{id, agent, dir, at})
			if at > p.DirAt {
				p.Dir, p.DirAt = dir, at
				doc.Tabs[i].Panes[id] = p
			}
		}
	}
	return out, nil
}

// pruneDeskPaneDirs forgets the reported folder of every pane doc lacks.
func pruneDeskPaneDirs(tx *sql.Tx, uid int64, deskID string, doc desk.Doc) error {
	rows, err := tx.Query(`SELECT pane_id FROM desk_pane_dirs WHERE user_id=? AND desk_id=?`, uid, deskID)
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var pane string
		if err := rows.Scan(&pane); err != nil {
			rows.Close()
			return err
		}
		if _, ok := findPane(&doc, pane); !ok {
			gone = append(gone, pane)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, pane := range gone {
		if _, err := tx.Exec(`DELETE FROM desk_pane_dirs WHERE user_id=? AND desk_id=? AND pane_id=?`, uid, deskID, pane); err != nil {
			return err
		}
	}
	return nil
}

// DeskPaneReportedDirFor returns the folder a machine itself last reported for
// a pane's shell, and which machine (enrollment id) reported it. A folder a
// browser wrote into the document is never one: only this record may open a
// shell outside the machine's home.
func (s *Store) DeskPaneReportedDirFor(uid int64, deskID, pane string) (agent, dir string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT agent_id,dir FROM desk_pane_dirs WHERE user_id=? AND desk_id=? AND pane_id=?`, uid, deskID, pane).Scan(&agent, &dir)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return agent, dir, true, nil
}

// rewriteDesksTx applies edit to every listed desk of uid and saves those it
// changed, with a version bump. It is used when a machine's enrolment is
// revoked. A desk whose stored document cannot be read is left as it is.
func rewriteDesksTx(tx *sql.Tx, uid int64, edit func(*desk.Doc) bool) error {
	rows, err := tx.Query(`SELECT `+deskColumns+` FROM desks WHERE user_id=? AND schema<=?`, uid, desk.Schema)
	if err != nil {
		return err
	}
	var all []Desk
	for rows.Next() {
		d, err := scanDesk(rows)
		if errors.Is(err, errStoredDesk) {
			log.Printf("desks: %v; left as it is", err)
			continue
		}
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

// clearDeskSessionsForAgentTx is for an enrolment about to be deleted, whose
// terminal sessions go with it by cascade: every desk pane on that machine, or
// holding one of its sessions, loses its session, with a version bump; the
// session index loses those sessions; and what the machine reported for desk
// panes is forgotten.
func clearDeskSessionsForAgentTx(tx *sql.Tx, uid int64, agent string) error {
	if _, err := tx.Exec(`DELETE FROM desk_pane_dirs WHERE user_id=? AND agent_id=?`, uid, agent); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT id FROM terminal_sessions WHERE user_id=? AND agent_id=?`, uid, agent)
	if err != nil {
		return err
	}
	gone := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		gone[id] = true
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rewriteDesksTx(tx, uid, func(doc *desk.Doc) bool {
		changed := false
		for i := range doc.Tabs {
			for id, p := range doc.Tabs[i].Panes {
				if p.Session != "" && (p.Host == agent || gone[p.Session]) {
					p.Session = ""
					doc.Tabs[i].Panes[id] = p
					changed = true
				}
			}
		}
		return changed
	}); err != nil {
		return err
	}
	// A desk this reet cannot rewrite (a newer schema) keeps its document,
	// but the index never names a session that is gone.
	for session := range gone {
		if _, err := tx.Exec(`DELETE FROM desk_sessions WHERE user_id=? AND session_id=?`, uid, session); err != nil {
			return err
		}
	}
	return nil
}
