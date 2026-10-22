package store

import (
	"database/sql"
	"errors"
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
	record.UpdatedAt = time.Now().UnixMilli()
	_, err := s.db.Exec(`INSERT INTO setup_profiles
		(user_id,name,revision,snapshot_id,snapshot_digest,updated_at,payload)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(user_id,name) DO UPDATE SET
		revision=excluded.revision, snapshot_id=excluded.snapshot_id,
		snapshot_digest=excluded.snapshot_digest, updated_at=excluded.updated_at, payload=excluded.payload`,
		record.UserID, record.Name, record.Revision, record.SnapshotID, record.SnapshotDigest, record.UpdatedAt, record.Payload)
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

func (s *Store) DeleteSetupProfileFor(userID int64, name string) error {
	_, err := s.db.Exec(`DELETE FROM setup_profiles WHERE user_id=? AND name=?`, userID, name)
	return err
}
