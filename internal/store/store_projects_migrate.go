package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
)

// Saved multi-repository Projects become folder projects (spec "Migration",
// rulings R1 and R2). The migration copies: the projects, project_members,
// project_runs and project_run_members rows are never changed or deleted, so
// runs, their members and their worktree records keep loading, and a run's
// worktree can still be checked and removed.

// MigrationReport says what one run of MigrateProjectsToFolders did. Users
// counts the accounts with a Project audited in this run; Created the folder
// projects made; Skipped the repositories not made into a folder project in
// this run: a folder the person had already registered, a row today's rules
// refuse, or one past the account's project limit.
type MigrationReport struct {
	Users   int
	Created int
	Skipped int
}

// projectsMigratedSetting is set once a run left nothing to do later.
const projectsMigratedSetting = "projects_migrated"

// legacyKey names one member of a saved Project: "<projectID>:<position>".
func legacyKey(projectID string, position int) string {
	return projectID + ":" + strconv.Itoa(position)
}

// MigrateProjectsToFolders makes a folder project of every distinct machine
// and cleaned folder in every account's saved Projects, oldest Project first,
// in one transaction. It is meant to run at every controller start:
//
//   - Re-running creates nothing. Each member's folder project is recorded in
//     folder_project_legacy (and the first member of a folder in the folder
//     project's legacy_project_id), so a member already handled is passed
//     over even without the projects_migrated setting, and a folder project
//     the person has since removed is not made again.
//   - A folder already registered by the person is left exactly as it is.
//   - The first (oldest) Project's shared prompt becomes the starter prompt; a
//     later Project with another prompt for the same folder is named in the
//     folder project's Note.
//   - A row today's rules refuse is skipped, counted and logged (without
//     names, folders or prompts); the rest still migrate.
//   - At the account's project limit the remaining members are skipped and
//     the setting is left unset, so the next start carries on.
//
// Each Project gets one projects-migrated audit event naming its folder
// projects (another when a later run adds to them); never a prompt or folder.
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
		var users []int64
		rows, err := tx.Query(`SELECT DISTINCT user_id FROM projects ORDER BY user_id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var uid int64
			if err := rows.Scan(&uid); err != nil {
				rows.Close()
				return err
			}
			users = append(users, uid)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		pending := false
		for _, uid := range users {
			audited, more, err := migrateAccountProjects(tx, uid, &rep)
			if err != nil {
				return err
			}
			if audited {
				rep.Users++
			}
			pending = pending || more
		}
		if pending {
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

func logMigrationSkip(reason string) {
	log.Printf("projects migration: skipped one repository of a saved Project: %s", reason)
}

// migrateAccountProjects migrates uid's Projects. audited reports whether an
// event was written; pending whether a member was left for a later run.
func migrateAccountProjects(tx *sql.Tx, uid int64, rep *MigrationReport) (audited, pending bool, err error) {
	var projects []Project
	rows, err := tx.Query(`SELECT `+projectCols+` FROM projects WHERE user_id=? ORDER BY created_at, rowid`, uid)
	if err != nil {
		return false, false, err
	}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			rows.Close()
			return false, false, err
		}
		projects = append(projects, p)
	}
	if err := rows.Close(); err != nil {
		return false, false, err
	}
	for i := range projects {
		if projects[i].Members, err = loadProjectMembers(tx, uid, projects[i].ID); err != nil {
			return false, false, err
		}
	}

	// What is already there: the account's folder projects by machine and
	// folder, the members already mapped, and the legacy column (for a
	// database that lost its mapping table).
	byFolder := map[string]*FolderProject{}
	byLegacy := map[string]string{}
	frows, err := tx.Query(`SELECT `+folderColumns+` FROM folder_projects WHERE user_id=?`, uid)
	if err != nil {
		return false, false, err
	}
	for frows.Next() {
		p, err := scanFolderProject(frows)
		if err != nil {
			frows.Close()
			return false, false, err
		}
		byFolder[p.Host+"\x00"+p.Dir] = &p
		if p.LegacyProjectID != "" {
			byLegacy[p.LegacyProjectID] = p.ID
		}
	}
	if err := frows.Close(); err != nil {
		return false, false, err
	}
	mapped := map[string]string{}
	mrows, err := tx.Query(`SELECT legacy_key,folder_project_id FROM folder_project_legacy WHERE user_id=?`, uid)
	if err != nil {
		return false, false, err
	}
	for mrows.Next() {
		var k, id string
		if err := mrows.Scan(&k, &id); err != nil {
			mrows.Close()
			return false, false, err
		}
		mapped[k] = id
	}
	if err := mrows.Close(); err != nil {
		return false, false, err
	}
	record := func(key, id string) error {
		mapped[key] = id
		_, err := tx.Exec(`INSERT INTO folder_project_legacy(user_id,legacy_key,folder_project_id) VALUES(?,?,?) ON CONFLICT(user_id,legacy_key) DO NOTHING`, uid, key, id)
		return err
	}
	// noted collects, per folder project made from an older Project, the
	// later Projects whose prompt for it was not kept.
	noted := map[string]*FolderProject{}

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
					return false, false, err
				}
				ids, added = appendOnce(ids, id), true
				continue
			}
			dir, derr := CleanProjectDir(m.Dir)
			if derr != nil {
				rep.Skipped++
				logMigrationSkip("its folder is not a valid absolute path")
				continue
			}
			if have, ok := byFolder[m.Host+"\x00"+dir]; ok {
				if err := record(key, have.ID); err != nil {
					return false, false, err
				}
				ids, added = appendOnce(ids, have.ID), true
				if have.LegacyProjectID == "" {
					// Registered by the person: left exactly as it is.
					rep.Skipped++
					continue
				}
				// Made from an older Project: its prompt stays.
				if p.SharedPrompt != "" && p.SharedPrompt != have.StarterPrompt && addNote(have, p.Name) {
					noted[have.ID] = have
				}
				continue
			}
			fp, ok := migratedFolderProject(p, m, dir, key)
			if !ok {
				rep.Skipped++
				logMigrationSkip("its machine, agent, model or prompt is not one a project can keep")
				continue
			}
			solo := ""
			if len(p.Members) == 1 {
				solo = p.Name
			}
			saved, err := insertMigrated(tx, uid, fp, solo)
			if errors.Is(err, ErrFolderProjectLimit) {
				rep.Skipped++
				pending, waiting = true, true
				logMigrationSkip("the account has the most projects reet keeps")
				continue
			}
			if errors.Is(err, ErrFolderProjectInvalid) || errors.Is(err, ErrFolderProjectExists) || errors.Is(err, ErrFolderProjectName) {
				rep.Skipped++
				logMigrationSkip("it could not be saved as a project")
				continue
			}
			if err != nil {
				return false, false, err
			}
			rep.Created++
			byFolder[saved.Host+"\x00"+saved.Dir] = &saved
			byLegacy[key] = saved.ID
			if err := record(key, saved.ID); err != nil {
				return false, false, err
			}
			ids, added = appendOnce(ids, saved.ID), true
		}
		// One event per Project, once it has a folder project or nothing left
		// to wait for (another only when a later run adds to it).
		if _, seen := mapped[p.ID]; !added && (seen || waiting) {
			continue
		}
		if err := record(p.ID, ""); err != nil {
			return false, false, err
		}
		if ids == nil {
			ids = []string{}
		}
		detail, err := json.Marshal(map[string]any{"project": p.Name, "folderProjects": ids})
		if err != nil {
			return false, false, err
		}
		if _, err := tx.Exec(`INSERT INTO events(at,user_id,actor,source,action,skill_id,detail) VALUES(?,?,'','migration','projects-migrated','',?)`, now(), uid, string(detail)); err != nil {
			return false, false, err
		}
		audited = true
	}
	for _, fp := range noted {
		if _, err := tx.Exec(`UPDATE folder_projects SET note=?,updated_at=MAX(updated_at,?),version=version+1 WHERE id=? AND user_id=?`, fp.Note, now(), fp.ID, uid); err != nil {
			return false, false, err
		}
	}
	return audited, pending, nil
}

func appendOnce(ids []string, id string) []string {
	for _, have := range ids {
		if have == id {
			return ids
		}
	}
	return append(ids, id)
}

// addNote adds a Project's name to the folder project's note, once, while it
// fits. It reports whether the note changed.
func addNote(p *FolderProject, name string) bool {
	for _, have := range strings.Split(p.Note, ", ") {
		if have == name {
			return false
		}
	}
	next := name
	if p.Note != "" {
		next = p.Note + ", " + name
	}
	if len(next) > maxFolderProjectNoteBytes {
		return false
	}
	p.Note = next
	return true
}

// migratedFolderProject is the folder project a Project member becomes,
// normalized, or false when today's rules refuse it. The member's agent and
// model beat the Project's defaults (as a run resolves them); a shared prompt
// the starter-prompt rules refuse is left out, and the Project is named in the
// note so the person can find it in the kept record.
func migratedFolderProject(p Project, m ProjectMember, dir, key string) (FolderProject, bool) {
	tool, model := resolvedLaunch(p, m)
	worktrees := "no"
	if p.UseWorktrees {
		worktrees = "yes"
	}
	fp := FolderProject{Host: m.Host, Dir: dir, WorktreeDefault: worktrees, DefaultTool: tool, DefaultModel: model, StarterPrompt: p.SharedPrompt, LegacyProjectID: key}
	if out, err := normalizeFolderProject(fp); err == nil {
		return out, true
	}
	if fp.StarterPrompt == "" {
		return FolderProject{}, false
	}
	fp.StarterPrompt = ""
	addNote(&fp, p.Name)
	out, err := normalizeFolderProject(fp)
	return out, err == nil
}

// insertMigrated stores a migrated folder project. A Project with one
// repository (solo is its name) lends it that name when no other project has it; otherwise
// the folder's name is used, with " (2)" and on after a clash.
func insertMigrated(tx *sql.Tx, uid int64, fp FolderProject, solo string) (FolderProject, error) {
	if solo != "" {
		named := fp
		named.Name = solo
		if n, err := normalizeFolderProject(named); err == nil {
			saved, err := insertFolderProject(tx, uid, n)
			if !errors.Is(err, ErrFolderProjectName) {
				return saved, err
			}
		}
	}
	return insertFolderProject(tx, uid, fp)
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
