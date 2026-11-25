package store

import (
	"database/sql"
)

// pendingLibrarySkill names the shelf skill behind a library entry that has no
// saved content and that this folder reports under this name, or "".
func pendingLibrarySkill(tx *sql.Tx, uid int64, environment, name string) string {
	var id string
	_ = tx.QueryRow(`SELECT a.reference FROM library_items i
		JOIN library_aliases a ON a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf'
		JOIN skill_envs se ON se.skill_id=a.reference AND se.env_id=?
		JOIN environments e ON e.id=se.env_id AND e.user_id=i.user_id
		JOIN skills k ON k.id=a.reference
		WHERE i.user_id=? AND i.deleted_at=0 AND i.item_type='skill' AND i.revision_id='' AND i.bundle_id='' AND k.slug=? AND k.source_type='local'`, environment, uid, name).Scan(&id)
	return id
}

// The most attempts reet makes by itself to save one skill from one machine.
// After that the entry keeps saying it is waiting and a person can import it.
const libraryCaptureAttempts = 3
