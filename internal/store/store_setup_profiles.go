package store

import (
	"database/sql"
	"errors"
	"github.com/eyupio/reet.sh/internal/profilehistory"
	"time"
)

type SetupProfileRecord struct {
	UserID         int64  `json:"userId"`
	Name           string `json:"name"`
	Revision       int    `json:"revision"`
	SnapshotID     string `json:"snapshotId"`
	SnapshotDigest string `json:"snapshotDigest"`
	UpdatedAt      int64  `json:"updatedAt"`
	Payload        string `json:"payload"`
}

func (s *Store) SaveSetupProfile(record SetupProfileRecord) error {
	if record.UserID == 0 || record.Name == "" || record.Revision < 1 || record.SnapshotID == "" || record.Payload == "" {
		return errors.New("invalid setup profile")
	}
	current, exists, err := s.SetupProfileFor(record.UserID, record.Name)
	if err != nil {
		return err
	}
	if exists {
		if err := profilehistory.CanAdvance([]byte(current.Payload), []byte(record.Payload)); err != nil {
			return err
		}
	}
	record.UpdatedAt = time.Now().UnixMilli()
	result, err := s.db.Exec(`INSERT INTO setup_profiles
		(user_id,name,revision,snapshot_id,snapshot_digest,updated_at,payload)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(user_id,name) DO UPDATE SET
		revision=excluded.revision, snapshot_id=excluded.snapshot_id,
		snapshot_digest=excluded.snapshot_digest, updated_at=excluded.updated_at, payload=excluded.payload
        WHERE setup_profiles.payload=?`,
		record.UserID, record.Name, record.Revision, record.SnapshotID, record.SnapshotDigest, record.UpdatedAt, record.Payload, current.Payload)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed == 0 {
		return profilehistory.ErrConflict
	}
	return err
}

func (s *Store) SetupProfileFor(userID int64, name string) (SetupProfileRecord, bool, error) {
	var record SetupProfileRecord
	err := s.db.QueryRow(`SELECT user_id,name,revision,snapshot_id,snapshot_digest,updated_at,payload
		FROM setup_profiles WHERE user_id=? AND name=?`, userID, name).
		Scan(&record.UserID, &record.Name, &record.Revision, &record.SnapshotID, &record.SnapshotDigest, &record.UpdatedAt, &record.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return record, false, nil
	}
	return record, err == nil, err
}

func (s *Store) SetupProfilesFor(userID int64) ([]SetupProfileRecord, error) {
	rows, err := s.db.Query(`SELECT user_id,name,revision,snapshot_id,snapshot_digest,updated_at
		FROM setup_profiles WHERE user_id=? ORDER BY name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SetupProfileRecord
	for rows.Next() {
		var record SetupProfileRecord
		if err := rows.Scan(&record.UserID, &record.Name, &record.Revision, &record.SnapshotID, &record.SnapshotDigest, &record.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, record)
	}
	return out, rows.Err()
}

// RenameSetupProfileFor moves one server copy to a new name and replaces its payload.
// It returns false when the new name already exists.
func (s *Store) RenameSetupProfileFor(userID int64, oldName, newName, payload, expectedPayload string) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var exists int
	err = tx.QueryRow(`SELECT 1 FROM setup_profiles WHERE user_id=? AND name=?`, userID, newName).Scan(&exists)
	if err == nil {
		return false, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	result, err := tx.Exec(`UPDATE setup_profiles SET name=?, payload=? WHERE user_id=? AND name=? AND payload=?`, newName, payload, userID, oldName, expectedPayload)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		return false, profilehistory.ErrConflict
	}
	return true, tx.Commit()
}

func (s *Store) DeleteSetupProfileFor(userID int64, name string) error {
	_, err := s.db.Exec(`DELETE FROM setup_profiles WHERE user_id=? AND name=?`, userID, name)
	return err
}
