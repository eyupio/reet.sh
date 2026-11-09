package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// LibraryDeploymentCheck identifies a previously pinned local binding. A pin
// protects bytes; it is not proof that the connector owns a filesystem entry.
type LibraryDeploymentCheck struct {
	TargetID string `json:"targetId"`
	LibraryBase
	Generation string `json:"generation,omitempty"`
}

// LibraryDeploymentIntent is a snapshot, not an acknowledgement of filesystem
// work. Connectors must also prove local ownership and preserve unseen edits.
type LibraryDeploymentIntent struct {
	Generation string `json:"generation"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	Name       string `json:"name"`
	RevisionID string `json:"revisionId"`
	Digest     string `json:"digest"`
}

// LibraryDeploymentIntentFor checks authoritative status/selection in one
// transaction. Resubmitting Generation detects stale work, including an ABA
// change back to the same visible status or selected set.
func (s *Store) LibraryDeploymentIntentFor(uid int64, enrollment string, in LibraryDeploymentCheck) (out LibraryDeploymentIntent, err error) {
	if in.TargetID == "" || len(in.BindingID) != 64 || !PrivateSkillIDValid(in.ItemID) || !PrivateSkillIDValid(in.RevisionID) {
		return out, ErrPrivateSkillInvalid
	}
	if _, err := hex.DecodeString(in.BindingID); err != nil {
		return out, ErrPrivateSkillInvalid
	}
	err = s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		if err := contentAgent(tx, uid, enrollment); err != nil {
			return err
		}
		var selected, selectionVersion, host string
		err := tx.QueryRow(`SELECT c.item_ids,c.version,e.host FROM canonical_library_selections c JOIN environments e ON e.user_id=c.user_id AND e.id=c.target_id JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE c.user_id=? AND c.target_id=? AND a.id=?`, uid, in.TargetID, enrollment).Scan(&selected, &selectionVersion, &host)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		var pinned string
		err = tx.QueryRow(`SELECT revision_id FROM library_bases WHERE user_id=? AND enrollment_id=? AND binding_id=? AND item_id=?`, uid, enrollment, in.BindingID, in.ItemID).Scan(&pinned)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		if pinned != in.RevisionID {
			return ErrLibraryConflict
		}
		var status, itemType, problem string
		var itemVersion, deleted int64
		err = tx.QueryRow(`SELECT name,status,item_type,problem,version,deleted_at,revision_id,digest FROM library_items WHERE user_id=? AND id=?`, uid, in.ItemID).Scan(&out.Name, &status, &itemType, &problem, &itemVersion, &deleted, &out.RevisionID, &out.Digest)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRemoteNotFound
		}
		if err != nil {
			return err
		}
		var ids []string
		if err := json.Unmarshal([]byte(selected), &ids); err != nil {
			return err
		}
		isSelected := false
		for _, id := range ids {
			if id == in.ItemID {
				isSelected = true
				break
			}
		}
		out.Action, out.Reason = "maintain", "selected_in_use"
		switch {
		case deleted != 0:
			out.Action, out.Reason = "remove", "library_deleted"
		case !isSelected:
			out.Action, out.Reason = "remove", "deselected"
		case status != LibraryInUse:
			out.Action, out.Reason = "remove", "inactive"
		case itemType != LibrarySkill || problem != "" || out.RevisionID == "" || out.Digest == "":
			out.Action, out.Reason = "blocked", "content_unavailable"
		}
		// Include account/enrollment/binding and both durable control versions.
		// Never accept the same token on a different item, target or enrollment.
		raw, err := json.Marshal([]any{uid, enrollment, in.TargetID, in.BindingID, in.ItemID, pinned, host, selectionVersion, selected, itemVersion, deleted, status, itemType, problem, out.Name, out.RevisionID, out.Digest})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		out.Generation = hex.EncodeToString(digest[:])
		if in.Generation != "" && in.Generation != out.Generation {
			return ErrLibraryConflict
		}
		return nil
	})
	if err != nil {
		return LibraryDeploymentIntent{}, err
	}
	return out, nil
}
