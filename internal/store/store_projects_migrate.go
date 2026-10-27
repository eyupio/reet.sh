package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Saved multi-repository Projects become folder projects (spec "Migration",
// rulings R1 and R2). The migration copies: the projects, project_members,
// project_runs and project_run_members rows are never changed or deleted, so
// runs, their members and their worktree records keep loading, and a run's
// worktree can still be checked and removed.

// MigrationReport says what one run did. Users counts the accounts with a
// Project audited; Created the folder projects made; Skipped the repositories
// not made into one for good (a folder the person had already registered, or
// a row today's rules refuse); Waiting the repositories left for want of room
// (the account has the most projects reet keeps); Failed the accounts whose
// part failed and was rolled back, to be tried at the next start.
type MigrationReport struct {
	Users   int
	Created int
	Skipped int
	Waiting int
	Failed  int
}

func (r *MigrationReport) add(o MigrationReport) {
	r.Users += o.Users
	r.Created += o.Created
	r.Skipped += o.Skipped
	r.Waiting += o.Waiting
	r.Failed += o.Failed
}

// projectsMigratedSetting is set once a start left no account failed.
const projectsMigratedSetting = "projects_migrated"

// legacyKey names one member of a saved Project: "<projectID>:<position>".
func legacyKey(projectID string, position int) string {
	return projectID + ":" + strconv.Itoa(position)
}

// MigrateProjectsToFolders makes a folder project of every distinct machine
// and cleaned folder in every account's saved Projects, oldest Project first,
// in one transaction. It runs at every controller start:
//
//   - Re-running creates nothing. Each member's outcome is recorded in
//     folder_project_legacy (and the first member of a folder in the folder
//     project's legacy_project_id), so a member already handled is passed
//     over, and a folder project the person has since removed is not made
//     again. An account whose saved Projects have not changed since its last
//     run (folder_project_migration) is not even read.
//   - A folder already registered by the person is left exactly as it is.
//   - The first (oldest) Project's shared prompt becomes the starter prompt; a
//     later Project with another prompt for the same folder is named in the
//     folder project's Note.
//   - A row today's rules refuse is skipped for good and counted.
//   - At the account's project limit the remaining members wait. A start never
//     takes room the person makes later (it may be for a folder of their own):
//     MigrateProjectsToFoldersFor, the person's own action, brings them in.
//   - Each account runs inside its own savepoint: an unexpected error rolls
//     back that account alone, counts it as Failed and leaves the setting
//     unset, so the next start tries it again; the others migrate.
//
// The log gets one summary line per start (and one per failed account), with
// counts only: never names, folders, ids or prompts. Each Project gets one
// projects-migrated audit event naming its folder projects (another when a
// later run adds to them); never a prompt or folder.
func (s *Store) MigrateProjectsToFolders() (MigrationReport, error) {
	var rep MigrationReport
	err := s.withTx(func(tx *sql.Tx) error {
		rep = MigrationReport{}
		var done string
		err := tx.QueryRow(`SELECT value FROM settings WHERE key=?`, projectsMigratedSetting).Scan(&done)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if done != "" {
			return nil
		}
		users, prints, err := projectFingerprints(tx)
		if err != nil {
			return err
		}
		marked := map[int64]string{}
		rows, err := tx.Query(`SELECT user_id,fingerprint FROM folder_project_migration`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var uid int64
			var print string
			if err := rows.Scan(&uid, &print); err != nil {
				rows.Close()
				return err
			}
			marked[uid] = print
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, uid := range users {
			if marked[uid] == prints[uid] {
				continue
			}
			if _, err := tx.Exec(`SAVEPOINT migrate_account`); err != nil {
				return err
			}
			var acct MigrationReport
			err := migrateAccountProjects(tx, uid, &acct, "migration")
			if err == nil {
				err = markAccountMigrated(tx, uid, prints[uid])
			}
			if err != nil {
				if _, rerr := tx.Exec(`ROLLBACK TO migrate_account`); rerr != nil {
					return rerr
				}
				if _, rerr := tx.Exec(`RELEASE migrate_account`); rerr != nil {
					return rerr
				}
				rep.Failed++
				log.Printf("projects migration: one account's saved Projects could not be migrated; they will be tried again at the next start")
				continue
			}
			if _, err := tx.Exec(`RELEASE migrate_account`); err != nil {
				return err
			}
			rep.add(acct)
		}
		if rep.Skipped > 0 || rep.Waiting > 0 {
			log.Printf("projects migration: %d repositories skipped (already a project, or not valid as one), %d waiting for room", rep.Skipped, rep.Waiting)
		}
		if rep.Failed > 0 {
			return nil
		}
		_, err = tx.Exec(`INSERT INTO settings(key,value) VALUES(?,'1') ON CONFLICT(key) DO UPDATE SET value=excluded.value`, projectsMigratedSetting)
		return err
	})
	if err != nil {
		return MigrationReport{}, err
	}
	return rep, nil
}

// MigrateProjectsToFoldersFor runs the migration for uid alone, whatever
// was done before, with the same rules: it brings in what waited for room. It
// is idempotent; with nothing waiting it changes nothing.
func (s *Store) MigrateProjectsToFoldersFor(uid int64) (MigrationReport, error) {
	var rep MigrationReport
	err := s.withTx(func(tx *sql.Tx) error {
		rep = MigrationReport{}
		_, prints, err := projectFingerprints(tx)
		if err != nil {
			return err
		}
		if _, any := prints[uid]; !any {
			return nil
		}
		if err := migrateAccountProjects(tx, uid, &rep, "ui"); err != nil {
			return err
		}
		return markAccountMigrated(tx, uid, prints[uid])
	})
	if err != nil {
		return MigrationReport{}, err
	}
	return rep, nil
}

// SavedWaitingFor counts the folders (machine and cleaned folder) of uid's
// saved Project repositories that are not yet a folder project or settled
// (skipped for good): those waiting for room. Several members naming one
// folder count once; a folder the rules refuse, or one the account has
// registered since, is not waiting, because bringing it in makes nothing.
func (s *Store) SavedWaitingFor(uid int64) (int, error) {
	rows, err := s.db.Query(`SELECT m.host,m.dir FROM project_members m WHERE m.user_id=?
		AND NOT EXISTS(SELECT 1 FROM folder_project_legacy l WHERE l.user_id=m.user_id AND l.legacy_key=m.project_id||':'||m.position)
		AND NOT EXISTS(SELECT 1 FROM folder_projects f WHERE f.user_id=m.user_id AND f.legacy_project_id=m.project_id||':'||m.position)`, uid)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	waiting := map[string]bool{}
	for rows.Next() {
		var host, dir string
		if err := rows.Scan(&host, &dir); err != nil {
			return 0, err
		}
		if clean, err := CleanProjectDir(dir); err == nil {
			waiting[host+"\x00"+clean] = true
		}
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(waiting) == 0 {
		return 0, nil
	}
	registered, err := s.db.Query(`SELECT host,dir FROM folder_projects WHERE user_id=?`, uid)
	if err != nil {
		return 0, err
	}
	defer registered.Close()
	for registered.Next() {
		var host, dir string
		if err := registered.Scan(&host, &dir); err != nil {
			return 0, err
		}
		delete(waiting, host+"\x00"+dir)
	}
	return len(waiting), registered.Err()
}

// projectFingerprints returns the accounts with saved Projects, in order, and
// for each a fingerprint of them (counts and newest change): an account whose
// fingerprint matches its marker has nothing new to migrate.
func projectFingerprints(tx *sql.Tx) ([]int64, map[int64]string, error) {
	rows, err := tx.Query(`SELECT p.user_id,COUNT(DISTINCT p.id),COUNT(m.project_id),COALESCE(MAX(p.updated_at),0)
		FROM projects p LEFT JOIN project_members m ON m.project_id=p.id AND m.user_id=p.user_id
		GROUP BY p.user_id ORDER BY p.user_id`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var users []int64
	prints := map[int64]string{}
	for rows.Next() {
		var uid, projects, members, newest int64
		if err := rows.Scan(&uid, &projects, &members, &newest); err != nil {
			return nil, nil, err
		}
		users = append(users, uid)
		prints[uid] = fmt.Sprintf("%d:%d:%d", projects, members, newest)
	}
	return users, prints, rows.Err()
}

func markAccountMigrated(tx *sql.Tx, uid int64, print string) error {
	_, err := tx.Exec(`INSERT INTO folder_project_migration(user_id,fingerprint) VALUES(?,?) ON CONFLICT(user_id) DO UPDATE SET fingerprint=excluded.fingerprint`, uid, print)
	return err
}

// migrateAccountProjects migrates uid's Projects into rep. source is the
// audit source of its events.
func migrateAccountProjects(tx *sql.Tx, uid int64, rep *MigrationReport, source string) error {
	var projects []Project
	rows, err := tx.Query(`SELECT `+projectCols+` FROM projects WHERE user_id=? ORDER BY created_at, rowid`, uid)
	if err != nil {
		return err
	}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			rows.Close()
			return err
		}
		projects = append(projects, p)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for i := range projects {
		if projects[i].Members, err = loadProjectMembers(tx, uid, projects[i].ID); err != nil {
			return err
		}
	}

	// What is already there: the account's folder projects by machine and
	// folder with their notes, the members already handled, and the legacy
	// column (for a database that lost its mapping table).
	byFolder := map[string]*FolderProject{}
	byLegacy := map[string]string{}
	notes := map[string][]string{}
	frows, err := tx.Query(`SELECT `+folderColumns+`,note FROM folder_projects WHERE user_id=?`, uid)
	if err != nil {
		return err
	}
	for frows.Next() {
		var p FolderProject
		var raw string
		if err := frows.Scan(&p.ID, &p.Host, &p.Dir, &p.Name, &p.IsRepo, &p.BaseBranch, &p.WorktreeDefault, &p.DefaultTool, &p.DefaultModel,
			&p.StarterPrompt, &p.Colour, &p.Position, &p.LegacyProjectID, &p.Note, &p.CreatedAt, &p.UpdatedAt, &p.Version, &raw); err != nil {
			frows.Close()
			return err
		}
		byFolder[p.Host+"\x00"+p.Dir] = &p
		notes[p.ID] = noteNames(raw)
		if p.LegacyProjectID != "" {
			byLegacy[p.LegacyProjectID] = p.ID
		}
	}
	if err := frows.Close(); err != nil {
		return err
	}
	count := len(byFolder)
	mapped := map[string]string{}
	mrows, err := tx.Query(`SELECT legacy_key,folder_project_id FROM folder_project_legacy WHERE user_id=?`, uid)
	if err != nil {
		return err
	}
	for mrows.Next() {
		var k, id string
		if err := mrows.Scan(&k, &id); err != nil {
			mrows.Close()
			return err
		}
		mapped[k] = id
	}
	if err := mrows.Close(); err != nil {
		return err
	}
	// record remembers a member's outcome: its folder project, or "" for a
	// member skipped for good (it is not tried again).
	record := func(key, id string) error {
		mapped[key] = id
		_, err := tx.Exec(`INSERT INTO folder_project_legacy(user_id,legacy_key,folder_project_id) VALUES(?,?,?) ON CONFLICT(user_id,legacy_key) DO NOTHING`, uid, key, id)
		return err
	}
	skip := func(key string) error {
		rep.Skipped++
		return record(key, "")
	}
	// noted names the folder projects made from an older Project whose note
	// gained a later Project.
	noted := map[string]bool{}

	audited := false
	for _, p := range projects {
		var ids []string
		added, waiting := false, false
		for _, m := range p.Members {
			key := legacyKey(p.ID, m.Position)
			if id, ok := mapped[key]; ok {
				ids = appendOnce(ids, id)
				continue
			}
			if id, ok := byLegacy[key]; ok {
				if err := record(key, id); err != nil {
					return err
				}
				ids, added = appendOnce(ids, id), true
				continue
			}
			dir, derr := CleanProjectDir(m.Dir)
			if derr != nil {
				if err := skip(key); err != nil {
					return err
				}
				continue
			}
			if have, ok := byFolder[m.Host+"\x00"+dir]; ok {
				if err := record(key, have.ID); err != nil {
					return err
				}
				ids, added = appendOnce(ids, have.ID), true
				if have.LegacyProjectID == "" {
					// Registered by the person: left exactly as it is.
					rep.Skipped++
					continue
				}
				// Made from an older Project: its prompt stays.
				if p.SharedPrompt != "" && p.SharedPrompt != have.StarterPrompt {
					if names, changed := addName(notes[have.ID], p.Name); changed {
						notes[have.ID], noted[have.ID] = names, true
					}
				}
				continue
			}
			fp, note, ok := migratedFolderProject(p, m, dir, key)
			if !ok {
				if err := skip(key); err != nil {
					return err
				}
				continue
			}
			if count >= MaxFolderProjects {
				rep.Waiting++
				waiting = true
				continue
			}
			solo := ""
			if len(p.Members) == 1 {
				solo = p.Name
			}
			saved, err := insertMigrated(tx, uid, fp, solo, encodeNote(note))
			if errors.Is(err, ErrFolderProjectLimit) {
				rep.Waiting++
				waiting = true
				continue
			}
			if errors.Is(err, ErrFolderProjectInvalid) || errors.Is(err, ErrFolderProjectExists) || errors.Is(err, ErrFolderProjectName) {
				if err := skip(key); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			rep.Created++
			count++
			byFolder[saved.Host+"\x00"+saved.Dir] = &saved
			byLegacy[key] = saved.ID
			notes[saved.ID] = note
			if err := record(key, saved.ID); err != nil {
				return err
			}
			ids, added = appendOnce(ids, saved.ID), true
		}
		// One event per Project, once it has a folder project or nothing left
		// to wait for (another only when a later run adds to it).
		if _, seen := mapped[p.ID]; !added && (seen || waiting) {
			continue
		}
		if err := record(p.ID, ""); err != nil {
			return err
		}
		if ids == nil {
			ids = []string{}
		}
		detail, err := json.Marshal(map[string]any{"project": p.Name, "folderProjects": ids})
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO events(at,user_id,actor,source,action,skill_id,detail) VALUES(?,?,'',?,'projects-migrated','',?)`, now(), uid, source, string(detail)); err != nil {
			return err
		}
		audited = true
	}
	for id := range noted {
		if _, err := tx.Exec(`UPDATE folder_projects SET note=?,updated_at=MAX(updated_at,?),version=version+1 WHERE id=? AND user_id=?`, encodeNote(notes[id]), now(), id, uid); err != nil {
			return err
		}
	}
	if audited {
		rep.Users++
	}
	return nil
}

// appendOnce adds a folder project id once; "" (a member skipped for good)
// names none.
func appendOnce(ids []string, id string) []string {
	if id == "" {
		return ids
	}
	for _, have := range ids {
		if have == id {
			return ids
		}
	}
	return append(ids, id)
}

// addName adds a Project's whole name to a note's names, once, while the
// stored note fits. It reports whether the names changed.
func addName(names []string, name string) ([]string, bool) {
	for _, have := range names {
		if have == name {
			return names, false
		}
	}
	next := append(append([]string(nil), names...), name)
	if len(encodeNote(next)) > maxFolderProjectNoteBytes {
		return names, false
	}
	return next, true
}

// migratedFolderProject is the folder project a Project member becomes,
// normalized, with its note's names, or false when today's rules refuse it.
// The member's agent and model beat the Project's defaults (as a run resolves
// them). A shared prompt the starter-prompt rules refuse, or a single word
// the agent would read as a subcommand, is left out, and the Project is named
// in the note so the person can find it in the kept record.
func migratedFolderProject(p Project, m ProjectMember, dir, key string) (FolderProject, []string, bool) {
	tool, model := resolvedLaunch(p, m)
	worktrees := "no"
	if p.UseWorktrees {
		worktrees = "yes"
	}
	fp := FolderProject{Host: m.Host, Dir: dir, WorktreeDefault: worktrees, DefaultTool: tool, DefaultModel: model, StarterPrompt: p.SharedPrompt, LegacyProjectID: key}
	if !terminalproto.PromptNeedsWords(tool, fp.StarterPrompt) {
		if out, err := normalizeFolderProject(fp); err == nil {
			return out, nil, true
		}
	}
	if fp.StarterPrompt == "" {
		return FolderProject{}, nil, false
	}
	fp.StarterPrompt = ""
	out, err := normalizeFolderProject(fp)
	return out, []string{p.Name}, err == nil
}

// insertMigrated stores a migrated folder project with its stored note. A
// Project with one repository (solo is its name) lends it that name when no
// other project has it; otherwise the folder's name is used, with " (2)" and
// on after a clash.
func insertMigrated(tx *sql.Tx, uid int64, fp FolderProject, solo, note string) (FolderProject, error) {
	if solo != "" {
		named := fp
		named.Name = solo
		if n, err := normalizeFolderProject(named); err == nil {
			saved, err := insertFolderProject(tx, uid, n, note)
			if !errors.Is(err, ErrFolderProjectName) {
				return saved, err
			}
		}
	}
	return insertFolderProject(tx, uid, fp, note)
}

// FolderProjectForLegacyFor returns the folder project that the first
// migrated member of uid's saved Project projectID became and that still
// exists, for links to the old Project's page.
func (s *Store) FolderProjectForLegacyFor(uid int64, projectID string) (FolderProject, error) {
	if projectID == "" || strings.Contains(projectID, ":") {
		return FolderProject{}, ErrFolderProjectNotFound
	}
	prefix := projectID + ":"
	p, err := scanFolderProject(s.db.QueryRow(`SELECT `+folderColumns+` FROM folder_projects WHERE user_id=? AND id=(
		SELECT l.folder_project_id FROM folder_project_legacy l JOIN folder_projects f ON f.user_id=l.user_id AND f.id=l.folder_project_id
		WHERE l.user_id=? AND substr(l.legacy_key,1,length(?))=?
		ORDER BY CAST(substr(l.legacy_key,length(?)+1) AS INTEGER) LIMIT 1)`, uid, uid, prefix, prefix, prefix))
	if errors.Is(err, sql.ErrNoRows) {
		return FolderProject{}, ErrFolderProjectNotFound
	}
	return p, err
}
