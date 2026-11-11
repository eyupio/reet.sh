package store

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// LibraryInstallEvidence says whether the controller has its own record of
// installing a revision of an item into a target on this machine.
type LibraryInstallEvidence struct {
	Installed bool   `json:"installed"`
	JobID     string `json:"jobId,omitempty"`
	At        int64  `json:"at,omitempty"`
}

// LibraryInstallEvidenceFor looks for a succeeded apply on this machine and
// target whose plan added exactly this revision of this item. It is the
// controller-side half of migrating a legacy local binding to a verified
// deployment. The evidence is deliberately narrow:
//
//   - the revision must be the machine's pinned base for the binding, so a
//     stale or invented revision proves nothing;
//   - only an apply that succeeded counts, and only an "add" action (a fresh
//     install or a replacement of an unowned copy): an item that was already
//     in sync, adopted or imported has no add, so it has no evidence;
//   - another target's or another item's apply is never evidence.
//
// Absence of evidence is not an error. It means "not proven", and the machine
// keeps the copy unowned.
func (s *Store) LibraryInstallEvidenceFor(uid int64, enrollment string, in LibraryDeploymentCheck) (out LibraryInstallEvidence, err error) {
	if in.TargetID == "" || len(in.BindingID) != 64 || !PrivateSkillIDValid(in.ItemID) || !PrivateSkillIDValid(in.RevisionID) {
		return out, ErrPrivateSkillInvalid
	}
	if _, err = hex.DecodeString(in.BindingID); err != nil {
		return out, ErrPrivateSkillInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if e := privateSkillOwner(tx, uid); e != nil {
			return e
		}
		if e := contentAgent(tx, uid, enrollment); e != nil {
			return e
		}
		var pinned string
		e := tx.QueryRow(`SELECT revision_id FROM library_bases WHERE user_id=? AND enrollment_id=? AND binding_id=? AND item_id=?`, uid, enrollment, in.BindingID, in.ItemID).Scan(&pinned)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if e != nil {
			return e
		}
		if pinned != in.RevisionID {
			return ErrLibraryConflict
		}
		rows, e := tx.Query(`SELECT id,plan,updated_at FROM sync_jobs WHERE user_id=? AND agent_id=? AND target_id=? AND kind='apply' AND status='succeeded' ORDER BY updated_at DESC LIMIT 500`, uid, enrollment, in.TargetID)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var plan []byte
			var at int64
			if e = rows.Scan(&id, &plan, &at); e != nil {
				return e
			}
			if planAddsRevision(plan, in.ItemID, in.RevisionID) {
				out = LibraryInstallEvidence{Installed: true, JobID: id, At: at}
				return nil
			}
		}
		return rows.Err()
	})
	return out, err
}

// planAddsRevision reports whether a plan installs this exact private revision
// of this item.
func planAddsRevision(plan json.RawMessage, itemID, revisionID string) bool {
	var parsed struct {
		Actions []struct {
			Add   bool      `json:"add"`
			Skill SyncSkill `json:"skill"`
		} `json:"actions"`
	}
	if json.Unmarshal(plan, &parsed) != nil {
		return false
	}
	for _, action := range parsed.Actions {
		parts := strings.Split(action.Skill.Source, ":")
		if action.Add && action.Skill.ItemID == itemID && len(parts) == 4 && parts[0] == "private" && parts[2] == revisionID {
			return true
		}
	}
	return false
}
