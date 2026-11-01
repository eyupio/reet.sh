package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// MaxProjectGroups is the most groups one account can have. It is generous:
// groups are a way to arrange the rail, not a second project limit.
const MaxProjectGroups = 50

var (
	ErrProjectGroupLimit     = fmt.Errorf("an account can have at most %d project groups", MaxProjectGroups)
	ErrProjectGroupName      = fmt.Errorf("a group name is 1 to %d characters of plain text, and no other group may have it", MaxProjectNameRunes)
	ErrProjectGroupNotFound  = errors.New("project group not found")
	ErrProjectGroupChanged   = errors.New("the project group changed since it was read")
	ErrProjectGroupInvalid   = errors.New("invalid project group")
	ErrProjectGroupForbidden = errors.New("that group is not on this account")
)

// ProjectGroup is a named, coloured collection of projects in the rail. Its
// position orders the groups; a project's own position orders it inside its
// group. Membership lives in project_group_members, so removing a group
// leaves its projects in the rail, ungrouped.
type ProjectGroup struct {
	ID, Name, Colour              string
	Position                      int
	CreatedAt, UpdatedAt, Version int64
}

func (s *Store) migrateProjectGroups() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS project_groups (
				id TEXT NOT NULL, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL, name_key TEXT NOT NULL,
				colour TEXT NOT NULL DEFAULT '',
				position INTEGER NOT NULL CHECK(position>=0),
				created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
				version INTEGER NOT NULL DEFAULT 1,
				PRIMARY KEY(user_id,id))`,
			`CREATE UNIQUE INDEX IF NOT EXISTS project_groups_name ON project_groups(user_id,name_key)`,
			// One group per project. Both foreign keys cascade: a removed
			// group or project cannot leave a membership behind.
			`CREATE TABLE IF NOT EXISTS project_group_members (
				user_id INTEGER NOT NULL,
				group_id TEXT NOT NULL,
				project_id TEXT NOT NULL,
				position INTEGER NOT NULL DEFAULT 0,
				PRIMARY KEY(user_id,project_id),
				FOREIGN KEY(user_id,group_id) REFERENCES project_groups(user_id,id) ON DELETE CASCADE,
				FOREIGN KEY(user_id,project_id) REFERENCES folder_projects(user_id,id) ON DELETE CASCADE)`,
			`CREATE INDEX IF NOT EXISTS project_group_members_group ON project_group_members(user_id,group_id)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

func projectGroupInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrProjectGroupInvalid, fmt.Sprintf(format, args...))
}

func normalizeProjectGroup(g ProjectGroup) (ProjectGroup, error) {
	g.Name = strings.TrimSpace(g.Name)
	if !validFolderProjectName(g.Name) {
		return g, ErrProjectGroupName
	}
	if g.Colour != "" && !validFolderProjectColour(g.Colour) {
		return g, projectGroupInvalid("group colour must be blank, red, amber, green, blue or violet")
	}
	return g, nil
}

const groupColumns = `id,name,colour,position,created_at,updated_at,version`

func scanProjectGroup(r rowScanner) (ProjectGroup, error) {
	var g ProjectGroup
	err := r.Scan(&g.ID, &g.Name, &g.Colour, &g.Position, &g.CreatedAt, &g.UpdatedAt, &g.Version)
	return g, err
}

// ListProjectGroupsFor returns uid's groups in rail order (never nil).
func (s *Store) ListProjectGroupsFor(uid int64) ([]ProjectGroup, error) {
	rows, err := s.db.Query(`SELECT `+groupColumns+` FROM project_groups WHERE user_id=? ORDER BY position,created_at,id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectGroup{}
	for rows.Next() {
		g, err := scanProjectGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ProjectGroupFor returns one of uid's groups.
func (s *Store) ProjectGroupFor(uid int64, id string) (ProjectGroup, error) {
	g, err := scanProjectGroup(s.db.QueryRow(`SELECT `+groupColumns+` FROM project_groups WHERE user_id=? AND id=?`, uid, id))
	if errors.Is(err, sql.ErrNoRows) {
		return ProjectGroup{}, ErrProjectGroupNotFound
	}
	return g, err
}

// CreateProjectGroupFor stores a new group at the end of uid's list.
func (s *Store) CreateProjectGroupFor(uid int64, g ProjectGroup) (ProjectGroup, error) {
	g, err := normalizeProjectGroup(g)
	if err != nil {
		return ProjectGroup{}, err
	}
	var saved ProjectGroup
	err = s.withTx(func(tx *sql.Tx) error {
		if err := s.userExists(tx, uid); err != nil {
			return ErrProjectGroupNotFound
		}
		var n, next int
		if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(MAX(position)+1,0) FROM project_groups WHERE user_id=?`, uid).Scan(&n, &next); err != nil {
			return err
		}
		if n >= MaxProjectGroups {
			return ErrProjectGroupLimit
		}
		if err := groupNameTaken(tx, uid, g.Name, ""); err != nil {
			return err
		}
		if g.ID, err = projectRandomID(); err != nil {
			return err
		}
		t := now()
		g.Position, g.CreatedAt, g.UpdatedAt, g.Version = next, t, t, 1
		_, err = tx.Exec(`INSERT INTO project_groups(id,user_id,name,name_key,colour,position,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,1)`,
			g.ID, uid, g.Name, nameKey(g.Name), g.Colour, g.Position, t, t)
		if err != nil {
			return groupConflict(err)
		}
		saved = g
		return nil
	})
	if err != nil {
		return ProjectGroup{}, err
	}
	return saved, nil
}

// UpdateProjectGroupFor renames or recolours a group. A non-zero expect is the
// version the caller read; zero is last write wins.
func (s *Store) UpdateProjectGroupFor(uid int64, g ProjectGroup, expect int64) (ProjectGroup, error) {
	g, err := normalizeProjectGroup(g)
	if err != nil {
		return ProjectGroup{}, err
	}
	id := g.ID
	var saved ProjectGroup
	err = s.withTx(func(tx *sql.Tx) error {
		cur, err := scanProjectGroup(tx.QueryRow(`SELECT `+groupColumns+` FROM project_groups WHERE user_id=? AND id=?`, uid, id))
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProjectGroupNotFound
		} else if err != nil {
			return err
		}
		if expect != 0 && expect != cur.Version {
			return ErrProjectGroupChanged
		}
		if err := groupNameTaken(tx, uid, g.Name, id); err != nil {
			return err
		}
		t := now()
		if t < cur.CreatedAt {
			t = cur.CreatedAt
		}
		saved = g
		saved.ID, saved.Position = id, cur.Position
		saved.CreatedAt, saved.UpdatedAt, saved.Version = cur.CreatedAt, t, cur.Version+1
		res, err := tx.Exec(`UPDATE project_groups SET name=?,name_key=?,colour=?,updated_at=?,version=? WHERE user_id=? AND id=? AND version=?`,
			saved.Name, nameKey(saved.Name), saved.Colour, t, saved.Version, uid, id, cur.Version)
		if err != nil {
			return groupConflict(err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrProjectGroupChanged
		}
		return nil
	})
	if err != nil {
		return ProjectGroup{}, err
	}
	return saved, nil
}

// DeleteProjectGroupFor removes the group; its projects stay in the rail,
// ungrouped (the membership rows cascade away).
func (s *Store) DeleteProjectGroupFor(uid int64, id string, expect int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		var version int64
		if err := tx.QueryRow(`SELECT version FROM project_groups WHERE user_id=? AND id=?`, uid, id).Scan(&version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProjectGroupNotFound
			}
			return err
		}
		if expect != 0 && expect != version {
			return ErrProjectGroupChanged
		}
		res, err := tx.Exec(`DELETE FROM project_groups WHERE user_id=? AND id=? AND version=?`, uid, id, version)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrProjectGroupChanged
		}
		return nil
	})
}

// ReorderProjectGroupsFor puts uid's groups in the order of ids, which must
// name every group once, each with the version read.
func (s *Store) ReorderProjectGroupsFor(uid int64, ids []string, versions []int64) error {
	if len(ids) != len(versions) {
		return projectGroupInvalid("every group needs the version that was read")
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[id] {
			return projectGroupInvalid("a group appears twice")
		}
		seen[id] = true
		if versions[i] < 1 {
			return projectGroupInvalid("every group needs the version that was read")
		}
	}
	return s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id,position,version FROM project_groups WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		type cur struct {
			pos int
			ver int64
		}
		have := map[string]cur{}
		for rows.Next() {
			var id string
			var c cur
			if err := rows.Scan(&id, &c.pos, &c.ver); err != nil {
				rows.Close()
				return err
			}
			have[id] = c
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				if len(have) > 0 {
					return ErrProjectGroupChanged
				}
				return ErrProjectGroupNotFound
			}
		}
		if len(have) != len(ids) {
			return ErrProjectGroupChanged
		}
		for i, id := range ids {
			if have[id].ver != versions[i] {
				return ErrProjectGroupChanged
			}
		}
		t := now()
		for i, id := range ids {
			if have[id].pos == i {
				continue
			}
			res, err := tx.Exec(`UPDATE project_groups SET position=?,updated_at=MAX(updated_at,?),version=version+1 WHERE user_id=? AND id=? AND version=?`,
				i, t, uid, id, versions[i])
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n != 1 {
				return ErrProjectGroupChanged
			}
		}
		return nil
	})
}

// groupNameTaken is ErrProjectGroupName when another group of uid has the
// name. self is the group being updated ("" on create).
func groupNameTaken(tx *sql.Tx, uid int64, name, self string) error {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM project_groups WHERE user_id=? AND name_key=? AND id<>?`, uid, nameKey(name), self).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrProjectGroupName
	}
	return nil
}

// groupConflict maps a unique-index refusal to the name error.
func groupConflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), "name_key") {
		return ErrProjectGroupName
	}
	return err
}

// FolderProjectGroupsFor returns, for uid, each grouped project's group id.
func (s *Store) FolderProjectGroupsFor(uid int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT m.project_id,m.group_id FROM project_group_members m JOIN project_groups g ON g.user_id=m.user_id AND g.id=m.group_id WHERE m.user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var project, group string
		if err := rows.Scan(&project, &group); err != nil {
			return nil, err
		}
		out[project] = group
	}
	return out, rows.Err()
}

// SetFolderProjectGroupFor puts one project in groupID ("" takes it out). The
// project's own position in the rail is unchanged.
func (s *Store) SetFolderProjectGroupFor(uid int64, projectID, groupID string) error {
	return s.withTx(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM folder_projects WHERE user_id=? AND id=?`, uid, projectID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrFolderProjectNotFound
		}
		if groupID == "" {
			_, err := tx.Exec(`DELETE FROM project_group_members WHERE user_id=? AND project_id=?`, uid, projectID)
			return err
		}
		if err := tx.QueryRow(`SELECT COUNT(*) FROM project_groups WHERE user_id=? AND id=?`, uid, groupID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return ErrProjectGroupForbidden
		}
		_, err := tx.Exec(`INSERT INTO project_group_members(user_id,group_id,project_id) VALUES(?,?,?)
			ON CONFLICT(user_id,project_id) DO UPDATE SET group_id=excluded.group_id`, uid, groupID, projectID)
		return err
	})
}

// ReorderFolderProjectsInGroupsFor arranges the rail: ids names every project
// once in the order shown (ungrouped first, then each group's projects).
// groupIDs[i] is "" for ungrouped or the group id that holds ids[i]. Versions
// guard the whole change exactly as ReorderFolderProjectsFor does; a project
// whose place or group changes gets a new version.
func (s *Store) ReorderFolderProjectsInGroupsFor(uid int64, ids []string, versions []int64, groupIDs []string) error {
	if len(ids) != len(versions) || len(ids) != len(groupIDs) {
		return folderInvalid("every project needs the version and group that were read")
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[id] {
			return folderInvalid("a project appears twice")
		}
		seen[id] = true
		if versions[i] < 1 {
			return folderInvalid("every project needs the version that was read")
		}
	}
	return s.withTx(func(tx *sql.Tx) error {
		groups := map[string]bool{}
		grows, err := tx.Query(`SELECT id FROM project_groups WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		for grows.Next() {
			var id string
			if err := grows.Scan(&id); err != nil {
				grows.Close()
				return err
			}
			groups[id] = true
		}
		if err := grows.Close(); err != nil {
			return err
		}
		for _, gid := range groupIDs {
			if gid != "" && !groups[gid] {
				return ErrProjectGroupForbidden
			}
		}
		type cur struct {
			pos   int
			ver   int64
			group string
		}
		have := map[string]cur{}
		rows, err := tx.Query(`SELECT id,position,version FROM folder_projects WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			var c cur
			if err := rows.Scan(&id, &c.pos, &c.ver); err != nil {
				rows.Close()
				return err
			}
			have[id] = c
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		members := map[string]string{}
		mrows, err := tx.Query(`SELECT project_id,group_id FROM project_group_members WHERE user_id=?`, uid)
		if err != nil {
			return err
		}
		for mrows.Next() {
			var project, group string
			if err := mrows.Scan(&project, &group); err != nil {
				mrows.Close()
				return err
			}
			members[project] = group
		}
		if err := mrows.Close(); err != nil {
			return err
		}
		for _, id := range ids {
			if _, ok := have[id]; !ok {
				if len(have) > 0 {
					return ErrFolderProjectChanged
				}
				return ErrFolderProjectNotFound
			}
		}
		if len(have) != len(ids) {
			return ErrFolderProjectChanged
		}
		for i, id := range ids {
			if have[id].ver != versions[i] {
				return ErrFolderProjectChanged
			}
		}
		t := now()
		for i, id := range ids {
			groupChanged := members[id] != groupIDs[i]
			if have[id].pos == i && !groupChanged {
				continue
			}
			res, err := tx.Exec(`UPDATE folder_projects SET position=?,updated_at=MAX(updated_at,?),version=version+1 WHERE user_id=? AND id=? AND version=?`,
				i, t, uid, id, versions[i])
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil {
				return err
			} else if n != 1 {
				return ErrFolderProjectChanged
			}
			if groupChanged {
				if groupIDs[i] == "" {
					if _, err := tx.Exec(`DELETE FROM project_group_members WHERE user_id=? AND project_id=?`, uid, id); err != nil {
						return err
					}
				} else {
					if _, err := tx.Exec(`INSERT INTO project_group_members(user_id,group_id,project_id) VALUES(?,?,?)
						ON CONFLICT(user_id,project_id) DO UPDATE SET group_id=excluded.group_id`, uid, groupIDs[i], id); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
}
