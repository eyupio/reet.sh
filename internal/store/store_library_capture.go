package store

import (
	"database/sql"
	"errors"
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

// captureMachineLibraryContent makes reet the holder of every skill a machine
// shares. A scan only tells the library a skill exists; this asks the machine
// that reported it for the files, once, and the copy fills that same entry, so
// a name appears in the library once however many machines have it.
//
// It runs on the machine's own check-in, needs no browser, and installs
// nothing: choosing where a skill goes stays the person's decision.
func (s *Store) captureMachineLibraryContent(uid int64, role string, agent SyncAgent) error {
	if role == "viewer" || agent.Paused {
		return nil
	}
	rows, err := s.db.Query(`SELECT a.reference,e.id,k.slug FROM library_items i
		JOIN library_aliases a ON a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf'
		JOIN skill_envs se ON se.skill_id=a.reference
		JOIN environments e ON e.id=se.env_id AND e.user_id=i.user_id
		JOIN skills k ON k.id=a.reference
		WHERE i.user_id=? AND i.deleted_at=0 AND i.item_type='skill' AND i.revision_id='' AND i.bundle_id=''
		AND e.host=? COLLATE NOCASE AND e.project='' AND e.agent IN ('agents','claude','cursor','opencode') AND k.source_type='local'
		ORDER BY a.reference,CASE e.agent WHEN 'agents' THEN 0 ELSE 1 END,e.id`, uid, agent.Host)
	if err != nil {
		return err
	}
	type capture struct{ skill, environment, name string }
	var wanted []capture
	seen := map[string]bool{}
	for rows.Next() {
		var c capture
		if err = rows.Scan(&c.skill, &c.environment, &c.name); err != nil {
			break
		}
		if !seen[c.skill] {
			seen[c.skill] = true
			wanted = append(wanted, c)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range wanted {
		// One capture per skill at a time, whichever machine it is running on.
		var active, ended int
		if err = s.db.QueryRow(`SELECT COALESCE(SUM(state IN ('queued','previewing','reviewing','approved','uploading','completed')),0),COALESCE(SUM(state IN ('failed','expired','cancelled')),0) FROM machine_import_requests WHERE user_id=? AND json_extract(data,'$.syncSkillId')=?`, uid, c.skill).Scan(&active, &ended); err != nil {
			return err
		}
		if active > 0 || ended >= libraryCaptureAttempts {
			continue
		}
		key := "capture_" + secretHash(c.skill+"\x00"+string(rune('0'+ended)))
		_, err = s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: key, AgentID: agent.ID, EnvironmentID: c.environment, Name: c.name, SyncSkillID: c.skill})
		// A machine that cannot take the request yet (it has not said it can
		// import, or storage is full) is asked again on a later check-in.
		if err != nil && !errors.Is(err, ErrRemoteImport) && !errors.Is(err, ErrRemoteImportCapacity) && !errors.Is(err, ErrPrivateSkillMissing) && !errors.Is(err, ErrPrivateSkillInvalid) {
			return err
		}
	}
	return nil
}
