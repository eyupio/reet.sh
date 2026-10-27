package store

import (
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/eyupio/reet.sh/internal/contentobs"
)

var ErrContentObservationStale = errors.New("content observation grant or inventory changed")

func (s *Store) migrateContentObservations() error {
	return s.withTx(func(tx *sql.Tx) error {
		var columns int
		rows, err := tx.Query(`PRAGMA table_info(environment_scans)`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var n, notnull, pk int
			var name, kind string
			var def any
			if err = rows.Scan(&n, &name, &kind, &notnull, &def, &pk); err != nil {
				rows.Close()
				return err
			}
			if name == "inventory_revision" {
				columns++
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if columns == 0 {
			if _, err = tx.Exec(`ALTER TABLE environment_scans ADD COLUMN inventory_revision INTEGER NOT NULL DEFAULT 0`); err != nil {
				return err
			}
		}
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS content_observation_grants(agent_id TEXT PRIMARY KEY,user_id INTEGER NOT NULL,scan_id TEXT NOT NULL,expires_at INTEGER NOT NULL,inventories TEXT NOT NULL,receipt TEXT NOT NULL DEFAULT '',FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS content_observation_scopes(agent_id TEXT NOT NULL,user_id INTEGER NOT NULL,agent TEXT NOT NULL,project TEXT NOT NULL,inventory_revision INTEGER NOT NULL,complete INTEGER NOT NULL,observed_at INTEGER NOT NULL,received_at INTEGER NOT NULL,skills TEXT NOT NULL,PRIMARY KEY(agent_id,agent,project),FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE)`,
		} {
			if _, err = tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}
func contentInventories(tx *sql.Tx, uid int64, agent string) (map[string]int64, error) {
	rows, err := tx.Query(`SELECT e.agent,e.project,sc.inventory_revision FROM environments e JOIN environment_scans sc ON sc.env_id=e.id JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE a.id=? AND a.user_id=?`, agent, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var a, p string
		var revision int64
		if err = rows.Scan(&a, &p, &revision); err != nil {
			return nil, err
		}
		out[a+"\x00"+p] = revision
	}
	return out, rows.Err()
}
func contentAgent(tx *sql.Tx, uid int64, agent string) error {
	if err := lockRemoteAgent(tx, uid, agent, true); err != nil {
		return err
	}
	var paused bool
	if err := tx.QueryRow(`SELECT paused FROM sync_agents WHERE id=? AND user_id=?`, agent, uid).Scan(&paused); err != nil {
		return err
	}
	if paused {
		return ErrContentObservationStale
	}
	return nil
}
func (s *Store) PrepareContentObservationFor(uid int64, agent string) (out contentobs.Grant, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, agent); err != nil {
			return err
		}
		inventories, err := contentInventories(tx, uid, agent)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(inventories)
		out = contentobs.Grant{SchemaVersion: contentobs.Version, ScanID: syncID(), ExpiresAt: now() + 10*60*1000}
		_, err = tx.Exec(`INSERT INTO content_observation_grants(agent_id,user_id,scan_id,expires_at,inventories) VALUES(?,?,?,?,?) ON CONFLICT(agent_id) DO UPDATE SET scan_id=excluded.scan_id,expires_at=excluded.expires_at,inventories=excluded.inventories,receipt=''`, agent, uid, out.ScanID, out.ExpiresAt, string(raw))
		return err
	})
	return
}
func (s *Store) RecordContentObservationFor(uid int64, agent string, in contentobs.Report) error {
	if err := in.Validate(); err != nil {
		return err
	}
	raw, _ := json.Marshal(in)
	return s.withTx(func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, agent); err != nil {
			return err
		}
		var token, inventories, receipt string
		var expires int64
		err := tx.QueryRow(`SELECT scan_id,expires_at,inventories,receipt FROM content_observation_grants WHERE agent_id=? AND user_id=?`, agent, uid).Scan(&token, &expires, &inventories, &receipt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrContentObservationStale
		}
		if err != nil {
			return err
		}
		if token != in.ScanID {
			return ErrContentObservationStale
		}
		if receipt != "" {
			if receipt == string(raw) {
				return nil
			}
			return ErrContentObservationStale
		}
		if expires < now() {
			return ErrContentObservationStale
		}
		var expected map[string]int64
		if err = json.Unmarshal([]byte(inventories), &expected); err != nil {
			return err
		}
		current, err := contentInventories(tx, uid, agent)
		if err != nil {
			return err
		}
		for _, scope := range in.Scopes {
			key := scope.Agent + "\x00" + scope.Project
			version, ok := expected[key]
			if !ok || current[key] != version {
				return ErrContentObservationStale
			}
			if _, ok = current[key]; !ok {
				return ErrContentObservationStale
			}
		}
		if _, err = tx.Exec(`DELETE FROM content_observation_scopes WHERE agent_id=? AND user_id=?`, agent, uid); err != nil {
			return err
		}
		for _, scope := range in.Scopes {
			skills, _ := json.Marshal(scope.Skills)
			if _, err = tx.Exec(`INSERT INTO content_observation_scopes(agent_id,user_id,agent,project,inventory_revision,complete,observed_at,received_at,skills) VALUES(?,?,?,?,?,?,?,?,?)`, agent, uid, scope.Agent, scope.Project, current[scope.Agent+"\x00"+scope.Project], scope.Complete, in.ObservedAt, now(), string(skills)); err != nil {
				return err
			}
		}
		_, err = tx.Exec(`UPDATE content_observation_grants SET receipt=? WHERE agent_id=? AND user_id=? AND scan_id=?`, string(raw), agent, uid, in.ScanID)
		return err
	})
}
func (s *Store) ContentObservationFor(uid int64, env string) (out *contentobs.Inventory, err error) {
	out = &contentobs.Inventory{State: "not_reported", Skills: []contentobs.Skill{}}
	var raw string
	var revision, current int64
	var complete, inventoryComplete, paused bool
	err = s.db.QueryRow(`SELECT o.inventory_revision,sc.inventory_revision,o.complete,sc.complete,a.paused,o.observed_at,o.received_at,o.skills FROM content_observation_scopes o JOIN sync_agents a ON a.id=o.agent_id AND a.user_id=o.user_id JOIN environments e ON e.user_id=a.user_id AND e.host=a.host COLLATE NOCASE AND e.agent=o.agent AND e.project=o.project JOIN environment_scans sc ON sc.env_id=e.id WHERE e.id=? AND e.user_id=?`, env, uid).Scan(&revision, &current, &complete, &inventoryComplete, &paused, &out.ObservedAt, &out.ReceivedAt, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if revision != current {
		return &contentobs.Inventory{State: "not_reported", Skills: []contentobs.Skill{}}, nil
	}
	if err = json.Unmarshal([]byte(raw), &out.Skills); err != nil {
		return nil, err
	}
	out.SchemaVersion = contentobs.Version
	out.State = "observed"
	if !complete || !inventoryComplete {
		out.State = "incomplete"
	}
	if now()-out.ReceivedAt > contentobs.FreshMillis {
		out.State = "stale"
	}
	if paused {
		out.State = "paused"
	}
	return out, nil
}
