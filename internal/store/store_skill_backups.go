package store

import (
	"database/sql"
	"time"
)

const (
	// skillBackupKept is how long the controller remembers an edited copy's
	// backup: as long as the machine keeps the backup itself.
	skillBackupKept = 30 * 24 * time.Hour
	// skillBackupsKept bounds the backups remembered per machine, the oldest
	// going first. It is room for every one of the 1,024 skills reet can hold
	// edited in all four managed folders.
	skillBackupsKept = 4096
)

// SkillBackup is a copy of a skill that was edited on a machine, which the
// machine backed up before it put reet's copy back. Use this version reads it
// to capture that copy. Folder and Backup are the machine's own words (Backup
// is under ~/): they are shown and handed back to that machine, never used as
// paths on the controller.
type SkillBackup struct {
	AgentID string `json:"agentId"`
	Host    string `json:"host"`
	Folder  string `json:"folder"`
	Name    string `json:"name"`
	Backup  string `json:"backup"`
	Detail  string `json:"detail"`
	At      int64  `json:"at"`
}

// migrateSkillBackups keeps edited copies' backups apart from machine_skills,
// whose rows each report replaces. A row is one copy: the machine, its folder
// and the copy's name as it was on disk, in its own case.
func (s *Store) migrateSkillBackups() error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS skill_edit_backups(user_id INTEGER NOT NULL,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,folder TEXT NOT NULL,name TEXT NOT NULL,backup TEXT NOT NULL,detail TEXT NOT NULL DEFAULT '',at INTEGER NOT NULL,PRIMARY KEY(user_id,agent_id,folder,name))`,
		`CREATE INDEX IF NOT EXISTS skill_edit_backups_name ON skill_edit_backups(user_id,name COLLATE NOCASE)`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// recordSkillBackupsTx keeps the backup of every edited copy in rep, before
// entries are folded into one per folder and name, so no copy's backup is
// lost: case variants are separate copies. A newer backup of the same copy
// replaces the older one. A backup already kept is not kept again under
// another name: an agent names the first copy's backup again on the skill's
// own entry, under the skill's name, after each copy's entry. Backups older
// than skillBackupKept go, and each machine keeps its newest skillBackupsKept.
func recordSkillBackupsTx(tx *sql.Tx, uid int64, agentID string, entries []SkillReportEntry, at int64) error {
	if _, err := tx.Exec(`DELETE FROM skill_edit_backups WHERE user_id=? AND at<=?`, uid, at-skillBackupKept.Milliseconds()); err != nil {
		return err
	}
	known := map[string]bool{}
	rows, err := tx.Query(`SELECT folder,backup FROM skill_edit_backups WHERE user_id=? AND agent_id=?`, uid, agentID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var folder, backup string
		if err = rows.Scan(&folder, &backup); err != nil {
			break
		}
		known[folder+"\x00"+backup] = true
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	added := false
	for _, e := range entries {
		name, folder, backup := cleanReportText(e.Name, 256), cleanReportText(e.Folder, 512), cleanReportText(e.Backup, 512)
		if e.State != "edited" || name == "" || backup == "" || known[folder+"\x00"+backup] {
			continue
		}
		known[folder+"\x00"+backup] = true
		if _, err = tx.Exec(`INSERT INTO skill_edit_backups(user_id,agent_id,folder,name,backup,detail,at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(user_id,agent_id,folder,name) DO UPDATE SET backup=excluded.backup,detail=excluded.detail,at=excluded.at`,
			uid, agentID, folder, name, backup, cleanReportText(e.Detail, 500), at); err != nil {
			return err
		}
		added = true
	}
	if !added {
		return nil
	}
	_, err = tx.Exec(`DELETE FROM skill_edit_backups WHERE user_id=? AND agent_id=? AND rowid NOT IN (SELECT rowid FROM skill_edit_backups WHERE user_id=? AND agent_id=? ORDER BY at DESC,rowid DESC LIMIT ?)`, uid, agentID, uid, agentID, skillBackupsKept)
	return err
}

// SkillBackupsFor lists the backups of edited copies of the skill called
// name, compared without case, that are less than 30 days old at now, newest
// first. An empty name lists every one.
func (s *Store) SkillBackupsFor(uid int64, name string, now time.Time) ([]SkillBackup, error) {
	rows, err := s.db.Query(`SELECT b.agent_id,COALESCE(a.host,''),b.folder,b.name,b.backup,b.detail,b.at FROM skill_edit_backups b LEFT JOIN sync_agents a ON a.id=b.agent_id
 WHERE b.user_id=? AND (?='' OR b.name=? COLLATE NOCASE) AND b.at>?
 ORDER BY b.at DESC,a.host,b.folder,b.name`, uid, name, name, now.UnixMilli()-skillBackupKept.Milliseconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillBackup{}
	for rows.Next() {
		var b SkillBackup
		if err = rows.Scan(&b.AgentID, &b.Host, &b.Folder, &b.Name, &b.Backup, &b.Detail, &b.At); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
