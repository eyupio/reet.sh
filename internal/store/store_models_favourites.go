package store

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MaxModelFavourites is the most starred models one account can keep.
const MaxModelFavourites = 50

var (
	ErrModelFavouritesFull    = fmt.Errorf("an account can keep at most %d favourite models", MaxModelFavourites)
	ErrModelFavouriteNotFound = errors.New("that model is not a favourite")
	ErrModelFavouritesStale   = errors.New("the favourites changed since they were read")
)

// ModelFavourite is one starred model. AgentDefault marks the model to use
// when the person switches to this agent (decision D3).
type ModelFavourite struct {
	Tool         string `json:"tool"`
	Model        string `json:"model"`
	AgentDefault bool   `json:"agentDefault"`
}

// ModelFavourites is the account's list and the version a picker sends back
// with an order change. Version only moves when something really changed.
type ModelFavourites struct {
	Version    int64            `json:"version"`
	Favourites []ModelFavourite `json:"favourites"`
}

// ModelFavouriteRef names one favourite in an order change.
type ModelFavouriteRef struct {
	Tool  string `json:"tool"`
	Model string `json:"model"`
}

func (s *Store) migrateModelFavourites() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS model_favourites (
				user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				tool TEXT NOT NULL,
				model TEXT NOT NULL,
				position INTEGER NOT NULL DEFAULT 0,
				agent_default INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				PRIMARY KEY(user_id,tool,model))`,
			`CREATE UNIQUE INDEX IF NOT EXISTS model_favourites_agent_default ON model_favourites(user_id,tool) WHERE agent_default=1`,
			`CREATE INDEX IF NOT EXISTS model_favourites_order ON model_favourites(user_id,position,created_at,model)`,
			`CREATE TABLE IF NOT EXISTS model_favourite_meta (
				user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
				version INTEGER NOT NULL DEFAULT 0)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

func validModelFavourite(tool, model string) error {
	if !terminalproto.ValidToolV2(tool) || !terminalproto.ValidModel(model) {
		return terminalproto.ErrInvalid
	}
	return nil
}

func modelFavouritesTx(q dbQuerier, uid int64) (ModelFavourites, error) {
	out := ModelFavourites{Favourites: []ModelFavourite{}}
	if err := q.QueryRow(`SELECT COALESCE((SELECT version FROM model_favourite_meta WHERE user_id=?),0)`, uid).Scan(&out.Version); err != nil {
		return out, err
	}
	rows, err := q.Query(`SELECT tool,model,agent_default FROM model_favourites WHERE user_id=? ORDER BY position,created_at,model`, uid)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var f ModelFavourite
		var isDefault int
		if err := rows.Scan(&f.Tool, &f.Model, &isDefault); err != nil {
			return out, err
		}
		f.AgentDefault = isDefault != 0
		out.Favourites = append(out.Favourites, f)
	}
	return out, rows.Err()
}

func bumpModelFavouritesTx(tx *sql.Tx, uid int64) error {
	_, err := tx.Exec(`INSERT INTO model_favourite_meta(user_id,version) VALUES(?,1)
		ON CONFLICT(user_id) DO UPDATE SET version=version+1`, uid)
	return err
}

// ModelFavouritesFor returns uid's starred models in display order.
func (s *Store) ModelFavouritesFor(uid int64) (ModelFavourites, error) {
	var out ModelFavourites
	err := s.withTx(func(tx *sql.Tx) error {
		var err error
		out, err = modelFavouritesTx(tx, uid)
		return err
	})
	return out, err
}

// AddModelFavouriteFor stars tool/model. Starring an already starred model is
// a no-op that does not move the version. created says whether a row was
// added.
func (s *Store) AddModelFavouriteFor(uid int64, tool, model string) (out ModelFavourites, created bool, err error) {
	if err := validModelFavourite(tool, model); err != nil {
		return ModelFavourites{}, false, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM model_favourites WHERE user_id=? AND tool=? AND model=?`, uid, tool, model).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			var n, next int
			if err := tx.QueryRow(`SELECT COUNT(*),COALESCE(MAX(position)+1,0) FROM model_favourites WHERE user_id=?`, uid).Scan(&n, &next); err != nil {
				return err
			}
			if n >= MaxModelFavourites {
				return ErrModelFavouritesFull
			}
			t := now()
			if _, err := tx.Exec(`INSERT INTO model_favourites(user_id,tool,model,position,agent_default,created_at,updated_at)
				VALUES(?,?,?,?,0,?,?)`, uid, tool, model, next, t, t); err != nil {
				return err
			}
			if err := bumpModelFavouritesTx(tx, uid); err != nil {
				return err
			}
			created = true
		}
		out, err = modelFavouritesTx(tx, uid)
		return err
	})
	return out, created, err
}

// RemoveModelFavouriteFor unstars tool/model. Removing one that is not starred
// is a no-op that does not move the version. removed says whether a row went.
func (s *Store) RemoveModelFavouriteFor(uid int64, tool, model string) (out ModelFavourites, removed bool, err error) {
	if err := validModelFavourite(tool, model); err != nil {
		return ModelFavourites{}, false, err
	}
	err = s.withTx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`DELETE FROM model_favourites WHERE user_id=? AND tool=? AND model=?`, uid, tool, model)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n > 0 {
			removed = true
			if err := bumpModelFavouritesTx(tx, uid); err != nil {
				return err
			}
		}
		out, err = modelFavouritesTx(tx, uid)
		return err
	})
	return out, removed, err
}

// ReorderModelFavouritesFor puts the starred models in the order of items,
// which must name the current set exactly once. version must be the one read;
// a mismatch, a missing item or an extra one is ErrModelFavouritesStale and
// nothing moves. An unchanged order does not move the version.
func (s *Store) ReorderModelFavouritesFor(uid, version int64, items []ModelFavouriteRef) (ModelFavourites, error) {
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if err := validModelFavourite(item.Tool, item.Model); err != nil {
			return ModelFavourites{}, err
		}
		key := item.Tool + "\x00" + item.Model
		if seen[key] {
			return ModelFavourites{}, ErrModelFavouritesStale
		}
		seen[key] = true
	}
	var out ModelFavourites
	err := s.withTx(func(tx *sql.Tx) error {
		var current int64
		if err := tx.QueryRow(`SELECT COALESCE((SELECT version FROM model_favourite_meta WHERE user_id=?),0)`, uid).Scan(&current); err != nil {
			return err
		}
		if current != version {
			return ErrModelFavouritesStale
		}
		rows, err := tx.Query(`SELECT tool,model FROM model_favourites WHERE user_id=? ORDER BY position,created_at,model`, uid)
		if err != nil {
			return err
		}
		var currentOrder []string
		currentSet := map[string]bool{}
		for rows.Next() {
			var tool, model string
			if err := rows.Scan(&tool, &model); err != nil {
				rows.Close()
				return err
			}
			key := tool + "\x00" + model
			currentOrder = append(currentOrder, key)
			currentSet[key] = true
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(currentOrder) != len(items) {
			return ErrModelFavouritesStale
		}
		same := true
		for i, item := range items {
			key := item.Tool + "\x00" + item.Model
			if !currentSet[key] {
				return ErrModelFavouritesStale
			}
			if currentOrder[i] != key {
				same = false
			}
		}
		if !same {
			t := now()
			for i, item := range items {
				if _, err := tx.Exec(`UPDATE model_favourites SET position=?,updated_at=MAX(updated_at,?) WHERE user_id=? AND tool=? AND model=?`, i, t, uid, item.Tool, item.Model); err != nil {
					return err
				}
			}
			if err := bumpModelFavouritesTx(tx, uid); err != nil {
				return err
			}
		}
		out, err = modelFavouritesTx(tx, uid)
		return err
	})
	return out, err
}

// SetModelFavouriteDefaultFor makes model the one default for its agent, or
// clears the default when model is empty. A non-empty model must already be
// starred for that tool. An unchanged default does not move the version.
func (s *Store) SetModelFavouriteDefaultFor(uid int64, tool, model string) (out ModelFavourites, changed bool, err error) {
	if !terminalproto.ValidToolV2(tool) || model != "" && !terminalproto.ValidModel(model) {
		return ModelFavourites{}, false, terminalproto.ErrInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if model != "" {
			var exists int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM model_favourites WHERE user_id=? AND tool=? AND model=?`, uid, tool, model).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return ErrModelFavouriteNotFound
			}
		}
		var current string
		err := tx.QueryRow(`SELECT model FROM model_favourites WHERE user_id=? AND tool=? AND agent_default=1`, uid, tool).Scan(&current)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if current == model {
			out, err = modelFavouritesTx(tx, uid)
			return err
		}
		if _, err := tx.Exec(`UPDATE model_favourites SET agent_default=0,updated_at=MAX(updated_at,?) WHERE user_id=? AND tool=? AND agent_default=1`, now(), uid, tool); err != nil {
			return err
		}
		if model != "" {
			if _, err := tx.Exec(`UPDATE model_favourites SET agent_default=1,updated_at=MAX(updated_at,?) WHERE user_id=? AND tool=? AND model=?`, now(), uid, tool, model); err != nil {
				return err
			}
		}
		if err := bumpModelFavouritesTx(tx, uid); err != nil {
			return err
		}
		changed = true
		out, err = modelFavouritesTx(tx, uid)
		return err
	})
	return out, changed, err
}
