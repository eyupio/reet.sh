package store

import (
	"database/sql"
	"errors"
)

// A capture from a machine may name the shelf skill it is a copy of: the
// capture then becomes that skill's content, kept in an account-owned
// immutable revision.

func bindPrivateSync(tx *sql.Tx, uid int64, r RemoteImport, result PrivateSkillResult) error {
	if r.SyncSkillID == "" {
		return nil
	}
	if id, e := libraryAliasID(tx, uid, "shelf", r.SyncSkillID); e == nil {
		var bound string
		if e = tx.QueryRow(`SELECT bundle_id FROM library_items WHERE user_id=? AND id=?`, uid, id).Scan(&bound); e != nil {
			return e
		}
		// A local/name alias is ambiguous after a different complete capture.
		// Preserve both imported items instead of silently merging that name.
		if bound == "" || bound == result.Bundle.ID {
			if e = attachLibraryBundle(tx, uid, id, result); e != nil {
				return e
			}
		}
	} else if !errors.Is(e, sql.ErrNoRows) {
		return e
	}
	_, err := tx.Exec(`INSERT INTO sync_private_sources(user_id,skill_id,bundle_id,revision_id,digest) VALUES(?,?,?,?,?) ON CONFLICT(user_id,skill_id) DO UPDATE SET bundle_id=excluded.bundle_id,revision_id=excluded.revision_id,digest=excluded.digest`, uid, r.SyncSkillID, result.Bundle.ID, result.Revision.ID, result.Revision.ManifestDigest)
	return err
}

func (s *Store) validateSyncImport(tx *sql.Tx, uid int64, in RemoteImportCreate) error {
	if in.DestinationID == "" && in.SyncSkillID == "" {
		if in.FollowShelf {
			return ErrRemoteImport
		}
		return nil
	}
	var count int
	err := tx.QueryRow(`SELECT count(*) FROM skill_envs se JOIN environments e ON e.id=se.env_id JOIN skills sk ON sk.id=se.skill_id WHERE se.skill_id=? AND e.id=? AND e.user_id=? AND sk.slug=?`, in.SyncSkillID, in.EnvironmentID, uid, in.Name).Scan(&count)
	if err != nil || count != 1 {
		return ErrRemoteImport
	}
	// A capture with no destination only saves the machine's copy into the
	// library; nothing is installed anywhere, so there is no folder to check.
	if in.DestinationID == "" {
		if in.FollowShelf {
			return ErrRemoteImport
		}
		return nil
	}
	err = tx.QueryRow(`SELECT count(*) FROM environments e JOIN sync_agents a ON a.host=e.host COLLATE NOCASE AND a.user_id=e.user_id WHERE e.id=? AND e.user_id=? AND a.paused=0 AND e.agent IN ('agents','claude','cursor','opencode')`, in.DestinationID, uid).Scan(&count)
	if err != nil || count != 1 {
		return ErrRemoteImport
	}
	return nil
}
