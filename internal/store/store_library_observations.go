package store

import (
	"database/sql"
	"errors"
)

// An explicit root or managed destination proves identity. A scan's same-name
// catalog row must not manufacture a second canonical item or change its status.
func (s *Store) libraryRootIdentityFor(uid int64, host, agent, project, name string) (SyncSkill, bool, error) {
	var out SyncSkill
	err := s.db.QueryRow(`SELECT i.id,i.name,i.bundle_id,i.revision_id,i.digest FROM library_locations l JOIN sync_agents a ON a.id=l.enrollment_id AND a.user_id=l.user_id JOIN library_aliases b ON b.user_id=l.user_id AND b.kind='machine' AND b.reference=l.enrollment_id||':'||l.id||':'||? JOIN library_items i ON i.user_id=b.user_id AND i.id=b.item_id JOIN environments e ON e.user_id=l.user_id AND e.host=a.host COLLATE NOCASE AND e.agent=l.legacy_agent AND e.project=l.project AND (e.path=l.path OR e.path=CASE l.legacy_agent WHEN 'agents' THEN '.agents/skills' WHEN 'claude' THEN '.claude/skills' WHEN 'codex' THEN '.codex/skills' WHEN 'opencode' THEN '.config/opencode/skills' END) WHERE l.user_id=? AND a.host=? COLLATE NOCASE AND l.legacy_agent=? AND l.project=? AND i.deleted_at=0`, name, uid, host, agent, project).Scan(&out.ItemID, &out.Name, &out.Source, &out.ID, &out.URL)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncSkill{}, false, nil
	}
	if err != nil {
		return out, false, err
	}
	out.Source = "private:" + out.Source + ":" + out.ID + ":" + out.URL
	out.URL = ""
	out.ID = "library/" + out.ItemID
	out.Slug = out.Name
	out.SourceType = "private"
	return out, true, nil
}
func (s *Store) RecordLibraryObservationFor(uid int64, itemID, host, agent, project, path string) (string, error) {
	id := "library/" + itemID
	err := s.withTx(func(tx *sql.Tx) error {
		var name, status string
		if err := tx.QueryRow(`SELECT name,status FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, itemID).Scan(&name, &status); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO skills(`+cols+`) VALUES(?,?,?,?,?,?,'','',0,0,'',0,0,0,?,?) ON CONFLICT(id) DO NOTHING`, id, "local", name, "local", "", name, now(), now()); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO library_aliases VALUES(?,'shelf',?,?) ON CONFLICT(user_id,kind,reference) DO NOTHING`, uid, id, itemID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO shelf(user_id,skill_id,liked,bookmarked,used,notes,created_at,updated_at) VALUES(?,?,0,?,0,'',?,?) ON CONFLICT(user_id,skill_id) DO NOTHING`, uid, id, status == LibraryToTest, now(), now())
		return err
	})
	if err != nil {
		return "", err
	}
	sourcePath := path
	if sourcePath != "" {
		sourcePath += "/"
		var name string
		if err = s.db.QueryRow(`SELECT name FROM library_items WHERE user_id=? AND id=?`, uid, itemID).Scan(&name); err != nil {
			return "", err
		}
		sourcePath += name
	}
	err = s.RecordEnvironmentFor(uid, id, host, agent, project, path, sourcePath)
	return id, err
}

// Reuse a metadata-only shelf identity only when one observed source proves
// exactly this registered standard root. Same-name multi-source reports remain
// distinct; an already captured/bound item can never be claimed by a new root.
func libraryUncapturedRootIdentity(tx *sql.Tx, uid int64, enrollment, location, name string) (string, error) {
	var id string
	err := tx.QueryRow(`SELECT i.id FROM library_locations l
 JOIN sync_agents a ON a.id=l.enrollment_id AND a.user_id=l.user_id
 JOIN environments e ON e.user_id=l.user_id AND e.host=a.host COLLATE NOCASE AND e.agent=l.legacy_agent AND e.project=l.project
 JOIN skill_envs se ON se.env_id=e.id JOIN skills k ON k.id=se.skill_id
 JOIN library_aliases b ON b.user_id=l.user_id AND b.kind='shelf' AND b.reference=k.id
 JOIN library_items i ON i.user_id=b.user_id AND i.id=b.item_id
 WHERE l.user_id=? AND l.enrollment_id=? AND l.id=? AND l.legacy_agent<>'' AND k.slug=? AND k.source_type='local' AND i.revision_id='' AND i.deleted_at=0
 AND (e.path=l.path OR e.path=CASE l.legacy_agent WHEN 'agents' THEN '.agents/skills' WHEN 'claude' THEN '.claude/skills' WHEN 'codex' THEN '.codex/skills' WHEN 'opencode' THEN '.config/opencode/skills' END)
 AND NOT EXISTS(SELECT 1 FROM library_aliases other WHERE other.user_id=i.user_id AND other.item_id=i.id AND other.kind<>'shelf')
 AND (SELECT count(*) FROM skill_envs seen JOIN environments env ON env.id=seen.env_id WHERE env.user_id=l.user_id AND seen.skill_id=k.id)=1`, uid, enrollment, location, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}
