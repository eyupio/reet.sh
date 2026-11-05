package store

import (
	"database/sql"
	"errors"
	"strings"
)

const MaxGafferProviders = 12

var (
	ErrGafferConflict = errors.New("this Gaffer provider changed; reload it before trying again")
	ErrGafferName     = errors.New("you already have a Gaffer provider with that name; choose another name")
	ErrGafferLimit    = errors.New("you can keep up to twelve Gaffer providers; remove one before adding another")
)

type GafferCheck struct {
	At            int64  `json:"at"`
	OK            bool   `json:"ok"`
	Model         string `json:"model"`
	LatencyMS     int64  `json:"latencyMs"`
	UsageReported bool   `json:"usageReported"`
	Error         string `json:"error,omitempty"`
}

// KeySealed contains an encrypted, owner/address-bound envelope. Public
// responses say only whether a key exists, never its value or ciphertext.
type GafferProvider struct {
	ID              string       `json:"id"`
	OwnerID         int64        `json:"-"`
	Revision        int64        `json:"revision"`
	Name            string       `json:"name"`
	Kind            string       `json:"kind"`
	BaseURL         string       `json:"baseUrl"`
	Model           string       `json:"model"`
	ReasoningEffort string       `json:"reasoningEffort"`
	Enabled         bool         `json:"enabled"`
	Default         bool         `json:"isDefault"`
	LocalOnly       bool         `json:"localOnly"`
	AllowStatus     bool         `json:"allowStatus"`
	AllowSessions   bool         `json:"allowSessions"`
	AllowCode       bool         `json:"allowCode"`
	KeySealed       string       `json:"-"`
	HasKey          bool         `json:"hasKey"`
	LastCheck       *GafferCheck `json:"lastCheck,omitempty"`
	CreatedAt       int64        `json:"createdAt"`
	UpdatedAt       int64        `json:"updatedAt"`
}

func (s *Store) migrateGaffer() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS gaffer_providers (
	 id TEXT PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	 revision INTEGER NOT NULL DEFAULT 1, name TEXT NOT NULL, kind TEXT NOT NULL,
	 base_url TEXT NOT NULL, model TEXT NOT NULL, reasoning_effort TEXT NOT NULL DEFAULT '',
	 enabled INTEGER NOT NULL DEFAULT 1, is_default INTEGER NOT NULL DEFAULT 0,
	 local_only INTEGER NOT NULL DEFAULT 0, allow_status INTEGER NOT NULL DEFAULT 0,
	 allow_sessions INTEGER NOT NULL DEFAULT 0, allow_code INTEGER NOT NULL DEFAULT 0,
	 key_sealed TEXT NOT NULL DEFAULT '', check_at INTEGER NOT NULL DEFAULT 0,
	 check_ok INTEGER NOT NULL DEFAULT 0, check_model TEXT NOT NULL DEFAULT '',
	 check_latency INTEGER NOT NULL DEFAULT 0, check_usage INTEGER NOT NULL DEFAULT 0,
	 check_error TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
	 UNIQUE(user_id,name));
	 CREATE UNIQUE INDEX IF NOT EXISTS gaffer_default_per_user ON gaffer_providers(user_id) WHERE is_default=1;`)
	return err
}

const gafferColumns = `id,user_id,revision,name,kind,base_url,model,reasoning_effort,enabled,is_default,local_only,allow_status,allow_sessions,allow_code,key_sealed,check_at,check_ok,check_model,check_latency,check_usage,check_error,created_at,updated_at`

func scanGaffer(row interface{ Scan(...any) error }) (GafferProvider, error) {
	var p GafferProvider
	var c GafferCheck
	err := row.Scan(&p.ID, &p.OwnerID, &p.Revision, &p.Name, &p.Kind, &p.BaseURL, &p.Model, &p.ReasoningEffort, &p.Enabled, &p.Default, &p.LocalOnly, &p.AllowStatus, &p.AllowSessions, &p.AllowCode, &p.KeySealed, &c.At, &c.OK, &c.Model, &c.LatencyMS, &c.UsageReported, &c.Error, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	p.HasKey = p.KeySealed != ""
	if c.At != 0 {
		p.LastCheck = &c
	}
	return p, err
}

func (s *Store) GafferProvidersFor(uid int64) ([]GafferProvider, error) {
	rows, err := s.db.Query(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? ORDER BY is_default DESC,name,id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GafferProvider{}
	for rows.Next() {
		p, err := scanGaffer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GafferProviderFor(uid int64, id string) (GafferProvider, error) {
	return scanGaffer(s.db.QueryRow(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? AND id=?`, uid, id))
}

func (s *Store) SaveGafferProviderFor(uid int64, p GafferProvider) (GafferProvider, error) {
	if uid <= 0 || len(p.ID) != 32 || len(p.Name) == 0 || len(p.Name) > 320 || len(p.BaseURL) > 2048 || len(p.Model) > 256 || len(p.KeySealed) > 65536 || p.Revision < 0 {
		return GafferProvider{}, errors.New("invalid Gaffer provider")
	}
	err := s.withTx(func(tx *sql.Tx) error {
		at := now()
		if p.Revision == 0 {
			var count int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM gaffer_providers WHERE user_id=?`, uid).Scan(&count); err != nil {
				return err
			}
			if count >= MaxGafferProviders {
				return ErrGafferLimit
			}
			if p.Default {
				if !p.Enabled {
					return errors.New("enable this provider before making it the default")
				}
				if _, err := tx.Exec(`UPDATE gaffer_providers SET is_default=0 WHERE user_id=?`, uid); err != nil {
					return err
				}
			}
			_, err := tx.Exec(`INSERT INTO gaffer_providers(id,user_id,name,kind,base_url,model,reasoning_effort,enabled,is_default,local_only,allow_status,allow_sessions,allow_code,key_sealed,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, p.ID, uid, p.Name, p.Kind, p.BaseURL, p.Model, p.ReasoningEffort, p.Enabled, p.Default, p.LocalOnly, p.AllowStatus, p.AllowSessions, p.AllowCode, p.KeySealed, at, at)
			return err
		}
		old, err := scanGaffer(tx.QueryRow(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? AND id=?`, uid, p.ID))
		if err != nil {
			return err
		}
		if old.Revision != p.Revision {
			return ErrGafferConflict
		}
		_, err = tx.Exec(`UPDATE gaffer_providers SET revision=revision+1,name=?,kind=?,base_url=?,model=?,reasoning_effort=?,enabled=?,local_only=?,allow_status=?,allow_sessions=?,allow_code=?,key_sealed=?,check_at=0,check_error='',updated_at=? WHERE user_id=? AND id=? AND revision=?`, p.Name, p.Kind, p.BaseURL, p.Model, p.ReasoningEffort, p.Enabled, p.LocalOnly, p.AllowStatus, p.AllowSessions, p.AllowCode, p.KeySealed, at, uid, p.ID, p.Revision)
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: gaffer_providers.user_id, gaffer_providers.name") {
			err = ErrGafferName
		} else if strings.Contains(err.Error(), "UNIQUE constraint") {
			err = ErrGafferConflict
		}
		return GafferProvider{}, err
	}
	return s.GafferProviderFor(uid, p.ID)
}

func (s *Store) DefaultGafferProviderFor(uid int64, id string, revision int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		p, err := scanGaffer(tx.QueryRow(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? AND id=?`, uid, id))
		if err != nil {
			return err
		}
		if p.Revision != revision {
			return ErrGafferConflict
		}
		if !p.Enabled {
			return errors.New("enable this provider before making it the default")
		}
		if _, err = tx.Exec(`UPDATE gaffer_providers SET is_default=0 WHERE user_id=?`, uid); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE gaffer_providers SET is_default=1 WHERE user_id=? AND id=?`, uid, id)
		return err
	})
}

func (s *Store) DeleteGafferProviderFor(uid int64, id string, revision int64) error {
	return s.withTx(func(tx *sql.Tx) error {
		p, err := scanGaffer(tx.QueryRow(`SELECT `+gafferColumns+` FROM gaffer_providers WHERE user_id=? AND id=?`, uid, id))
		if err != nil {
			return err
		}
		if p.Revision != revision {
			return ErrGafferConflict
		}
		_, err = tx.Exec(`DELETE FROM gaffer_providers WHERE user_id=? AND id=?`, uid, id)
		return err
	})
}

func (s *Store) RecordGafferCheckFor(uid int64, id string, revision int64, c GafferCheck) error {
	res, err := s.db.Exec(`UPDATE gaffer_providers SET check_at=?,check_ok=?,check_model=?,check_latency=?,check_usage=?,check_error=? WHERE user_id=? AND id=? AND revision=?`, c.At, c.OK, c.Model, c.LatencyMS, c.UsageReported, c.Error, uid, id, revision)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrGafferConflict
	}
	return nil
}
