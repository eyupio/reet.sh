package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/contentobs"
)

const MaxSkillContentTargets = 1024

var (
	ErrSkillContentTargetInvalid  = errors.New("invalid skill content target")
	ErrSkillContentTargetMissing  = errors.New("skill content target unavailable")
	ErrSkillContentTargetConflict = errors.New("skill content target changed")
	ErrSkillContentTargetScope    = errors.New("skill content target scope unavailable")
	ErrSkillContentTargetCapacity = errors.New("skill content target limit reached")

	// The refusals a machine merge and a scope merge answer with when pins are in
	// the way. Callers match them with errors.Is.
	ErrSkillContentTargetMerge      = errors.New("these reports have different private skill pins; remove one pin before merging")
	ErrSkillContentTargetScopeMerge = errors.New("remove private skill pins before merging this report into a different scope")
)

// SkillContentTarget is desired content, never an installed/ownership receipt.
// Its independent version must not alter legacy SyncSkill/desired-set hashes.
type SkillContentTarget struct {
	ID             string `json:"id"`
	EnvironmentID  string `json:"environmentId"`
	AgentID        string `json:"agentId"`
	Name           string `json:"name"`
	BundleID       string `json:"bundleId"`
	RevisionID     string `json:"revisionId"`
	ManifestDigest string `json:"manifestDigest"`
	Mode           string `json:"mode"`
	Version        string `json:"version"`
	UpdatedAt      int64  `json:"updatedAt"`
	Verification   string `json:"verification"`
}
type SkillContentTargetWrite struct {
	EnvironmentID   string  `json:"environmentId"`
	AgentID         string  `json:"agentId"`
	BundleID        string  `json:"bundleId"`
	RevisionID      string  `json:"revisionId"`
	Mode            string  `json:"mode"`
	ExpectedVersion *string `json:"expectedVersion"`
}

func (s *Store) migrateSkillContentTargets() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS skill_content_targets(
 id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
 agent_id TEXT NOT NULL,name TEXT NOT NULL,bundle_id TEXT NOT NULL,revision_id TEXT NOT NULL,
 mode TEXT NOT NULL,version TEXT NOT NULL,updated_at INTEGER NOT NULL,
 UNIQUE(user_id,environment_id,name),
 FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,bundle_id,revision_id) REFERENCES private_skill_revisions(user_id,bundle_id,id) ON DELETE CASCADE)`)
	return err
}

func skillContentTargetOwner(tx *sql.Tx, uid int64) error {
	err := privateSkillOwner(tx, uid)
	if errors.Is(err, ErrPrivateSkillMissing) {
		return ErrSkillContentTargetMissing
	}
	return err
}

const contentTargetCols = `t.id,t.environment_id,t.agent_id,t.name,t.bundle_id,t.revision_id,r.manifest_digest,t.mode,t.version,t.updated_at`

func scanContentTarget(row interface{ Scan(...any) error }) (t SkillContentTarget, err error) {
	err = row.Scan(&t.ID, &t.EnvironmentID, &t.AgentID, &t.Name, &t.BundleID, &t.RevisionID, &t.ManifestDigest, &t.Mode, &t.Version, &t.UpdatedAt)
	t.Verification = "not_verified"
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrSkillContentTargetMissing
	}
	return
}

const contentTargetJoin = ` FROM skill_content_targets t JOIN private_skill_revisions r ON r.user_id=t.user_id AND r.bundle_id=t.bundle_id AND r.id=t.revision_id`

// Reads stay available while a connector is paused/offline so users can inspect
// and remove intent. Disabled/read-only accounts cannot access private pins.
func (s *Store) SkillContentTargetsFor(uid int64) (out []SkillContentTarget, err error) {
	out = []SkillContentTarget{}
	err = s.withTx(func(tx *sql.Tx) error {
		if err := skillContentTargetOwner(tx, uid); err != nil {
			return err
		}
		rows, e := tx.Query(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? ORDER BY t.environment_id,t.name,t.id`, uid)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			item, e := scanContentTarget(rows)
			if e != nil {
				return e
			}
			out = append(out, item)
		}
		return rows.Err()
	})
	return
}
func (s *Store) SkillContentTargetFor(uid int64, id string) (out SkillContentTarget, err error) {
	if !contentobs.ValidID(id) {
		return out, ErrSkillContentTargetMissing
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if e := skillContentTargetOwner(tx, uid); e != nil {
			return e
		}
		var e error
		out, e = scanContentTarget(tx.QueryRow(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? AND t.id=?`, uid, id))
		return e
	})
	return
}
func (s *Store) SaveSkillContentTargetFor(uid int64, in SkillContentTargetWrite) (out SkillContentTarget, err error) {
	if in.ExpectedVersion == nil || (*in.ExpectedVersion != "" && !contentobs.ValidID(*in.ExpectedVersion)) || !contentobs.ValidID(in.AgentID) || in.EnvironmentID == "" || !contentobs.ValidText(in.EnvironmentID, 2048) || !PrivateSkillIDValid(in.BundleID) || !PrivateSkillIDValid(in.RevisionID) || (in.Mode != "additive" && in.Mode != "managed") {
		return out, ErrSkillContentTargetInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if e := skillContentTargetOwner(tx, uid); e != nil {
			return e
		}
		if e := contentAgent(tx, uid, in.AgentID); e != nil {
			if errors.Is(e, ErrRemoteNotFound) || errors.Is(e, ErrContentObservationStale) {
				return ErrSkillContentTargetScope
			}
			return e
		}
		if _, _, _, e := reportedPinScope(tx, uid, in.EnvironmentID, in.AgentID); e != nil {
			return e
		}
		bundle, e := privateSkillBundle(tx, uid, in.BundleID)
		if e != nil {
			return e
		}
		revision, e := privateSkillRevision(tx, uid, in.BundleID, in.RevisionID, false)
		if e != nil {
			return e
		}
		current, e := scanContentTarget(tx.QueryRow(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? AND t.environment_id=? AND t.name=?`, uid, in.EnvironmentID, bundle.Name))
		if e != nil && !errors.Is(e, ErrSkillContentTargetMissing) {
			return e
		}
		exists := e == nil
		// Repeating a lost-response write can read back the identical current intent,
		// without rewinding a newer selection. Creation has no historical receipt;
		// clients reconcile uncertain replies with GET rather than blindly replaying.
		if exists && current.AgentID == in.AgentID && current.BundleID == in.BundleID && current.RevisionID == in.RevisionID && current.Mode == in.Mode {
			out = current
			return nil
		}
		if (exists && current.Version != *in.ExpectedVersion) || (!exists && *in.ExpectedVersion != "") {
			return ErrSkillContentTargetConflict
		}
		id := current.ID
		if !exists {
			var count int
			if e = tx.QueryRow(`SELECT count(*) FROM skill_content_targets WHERE user_id=?`, uid).Scan(&count); e != nil {
				return e
			}
			if count >= MaxSkillContentTargets {
				return ErrSkillContentTargetCapacity
			}
			id = syncID()
		}
		out = SkillContentTarget{ID: id, EnvironmentID: in.EnvironmentID, AgentID: in.AgentID, Name: bundle.Name, BundleID: in.BundleID, RevisionID: in.RevisionID, ManifestDigest: revision.ManifestDigest, Mode: in.Mode, Version: syncID(), UpdatedAt: now(), Verification: "not_verified"}
		_, e = tx.Exec(`INSERT INTO skill_content_targets(id,user_id,environment_id,agent_id,name,bundle_id,revision_id,mode,version,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id,environment_id,name) DO UPDATE SET agent_id=excluded.agent_id,bundle_id=excluded.bundle_id,revision_id=excluded.revision_id,mode=excluded.mode,version=excluded.version,updated_at=excluded.updated_at`, out.ID, uid, out.EnvironmentID, out.AgentID, out.Name, out.BundleID, out.RevisionID, out.Mode, out.Version, out.UpdatedAt)
		return e
	})
	return
}

// reportedPinScope returns the agent, project and current inventory revision of a
// scope that may carry private intent: complete, recently reported, owned, on the
// connector's host and supported by a writable root adapter. Only adapter-supported
// scopes qualify. This is not a content observation or proof that a skill is absent.
func reportedPinScope(tx *sql.Tx, uid int64, environmentID, agentID string) (agent, project string, inventory int64, err error) {
	var complete bool
	var scanned int64
	err = tx.QueryRow(`SELECT e.agent,e.project,sc.complete,sc.scanned_at,sc.inventory_revision FROM environments e JOIN environment_scans sc ON sc.env_id=e.id JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.user_id=? AND e.id=? AND a.id=?`, uid, environmentID, agentID).Scan(&agent, &project, &complete, &scanned, &inventory)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", 0, ErrSkillContentTargetScope
	}
	if err != nil {
		return "", "", 0, err
	}
	if !complete || scanned < time.Now().Add(-14*24*time.Hour).UnixMilli() || !Syncable(agent) || !contentobs.ValidScope(agent, project) || (project != "" && (project[0] != '/' || (agent != "claude" && agent != "agents"))) {
		return "", "", 0, ErrSkillContentTargetScope
	}
	return agent, project, inventory, nil
}

// Removing intent never queues an uninstall or changes machine files. Keeping the
// version check separate from execution receipts avoids replaying host actions.
func (s *Store) DeleteSkillContentTargetFor(uid int64, id, version string) error {
	if !contentobs.ValidID(id) || !contentobs.ValidID(version) {
		return ErrSkillContentTargetInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if e := skillContentTargetOwner(tx, uid); e != nil {
			return e
		}
		var current string
		e := tx.QueryRow(`SELECT version FROM skill_content_targets WHERE user_id=? AND id=?`, uid, id).Scan(&current)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrSkillContentTargetMissing
		}
		if e != nil {
			return e
		}
		if current != version {
			return ErrSkillContentTargetConflict
		}
		_, e = tx.Exec(`DELETE FROM skill_content_targets WHERE user_id=? AND id=? AND version=?`, uid, id, version)
		return e
	})
}

// PinCountsFor counts this account's private skill pins per report, the way
// ReceiptCountsFor counts receipts: one query, absent when none, no query for no
// IDs. Callers keep the list to 500 IDs.
func (s *Store) PinCountsFor(userID int64, environmentIDs []string) (map[string]int, error) {
	return s.countByEnvironment("skill_content_targets", userID, environmentIDs)
}

// moveSkillContentTargets follows a machine rename within the same enrollment,
// never a cross-scope adoption. Bumping versions fences every old review, including
// the destination of an identical duplicate. Conflicts roll back the whole rename.
func moveSkillContentTargets(tx *sql.Tx, uid int64, from, to string) error {
	rows, err := tx.Query(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? AND t.environment_id=? ORDER BY t.id`, uid, from)
	if err != nil {
		return err
	}
	pins := []SkillContentTarget{}
	for rows.Next() {
		pin, e := scanContentTarget(rows)
		if e != nil {
			rows.Close()
			return e
		}
		pins = append(pins, pin)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, pin := range pins {
		other, e := scanContentTarget(tx.QueryRow(`SELECT `+contentTargetCols+contentTargetJoin+` WHERE t.user_id=? AND t.environment_id=? AND t.name=?`, uid, to, pin.Name))
		if e != nil && !errors.Is(e, ErrSkillContentTargetMissing) {
			return e
		}
		if e == nil {
			if other.AgentID != pin.AgentID || other.BundleID != pin.BundleID || other.RevisionID != pin.RevisionID || other.Mode != pin.Mode {
				return ErrSkillContentTargetMerge
			}
			if _, err = tx.Exec(`UPDATE skill_content_targets SET version=?,updated_at=? WHERE user_id=? AND id=?`, syncID(), now(), uid, other.ID); err != nil {
				return err
			}
			if _, err = tx.Exec(`DELETE FROM skill_content_targets WHERE user_id=? AND id=?`, uid, pin.ID); err != nil {
				return err
			}
		} else if _, err = tx.Exec(`UPDATE skill_content_targets SET environment_id=?,version=?,updated_at=? WHERE user_id=? AND id=?`, to, syncID(), now(), uid, pin.ID); err != nil {
			return err
		}
	}
	return nil
}
