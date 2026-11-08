package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strings"
)

type LibrarySelection struct {
	TargetID string   `json:"targetId"`
	ItemIDs  []string `json:"itemIds"`
	Version  string   `json:"version"`
}

func (s *Store) LibrarySelectionFor(uid int64, target string) (LibrarySelection, error) {
	out := LibrarySelection{TargetID: target, ItemIDs: []string{}}
	var raw string
	err := s.db.QueryRow(`SELECT item_ids,version FROM canonical_library_selections WHERE user_id=? AND target_id=?`, uid, target).Scan(&raw, &out.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrPrivateSkillMissing
	}
	if err != nil {
		return out, err
	}
	err = json.Unmarshal([]byte(raw), &out.ItemIDs)
	return out, err
}

// SaveLibrarySelectionFor persists intent without requiring a live connector or
// complete recent inventory. Local execution retains its normal preconditions.
func (s *Store) SaveLibrarySelectionFor(uid int64, in LibrarySelection) (LibrarySelection, error) {
	if in.ItemIDs == nil || len(in.ItemIDs) > 256 {
		return LibrarySelection{}, ErrPrivateSkillInvalid
	}
	ids := slices.Clone(in.ItemIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if !PrivateSkillIDValid(id) || (i > 0 && ids[i-1] == id) {
			return LibrarySelection{}, ErrPrivateSkillInvalid
		}
	}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		var agent string
		if err := tx.QueryRow(`SELECT e.agent FROM environments e JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE WHERE e.user_id=? AND e.id=?`, uid, in.TargetID).Scan(&agent); err != nil {
			return ErrRemoteNotFound
		}
		if !Syncable(agent) {
			return ErrSync
		}
		for _, id := range ids {
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, id).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return ErrPrivateSkillMissing
			}
		}
		var prior string
		err := tx.QueryRow(`SELECT version FROM canonical_library_selections WHERE user_id=? AND target_id=?`, uid, in.TargetID).Scan(&prior)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if prior != in.Version {
			return ErrLibraryConflict
		}
		if err = recordLibrarySelection(tx, uid, in.TargetID, []string{}); err != nil {
			return err
		}
		raw, _ := json.Marshal(ids)
		_, err = tx.Exec(`INSERT INTO canonical_library_selections VALUES(?,?,?,?) ON CONFLICT(user_id,target_id) DO UPDATE SET item_ids=excluded.item_ids,version=excluded.version`, uid, in.TargetID, string(raw), syncID())
		if err != nil {
			return err
		}
		// Invalidate queued previews immediately, even while the machine is offline.
		if _, err = tx.Exec(`UPDATE sync_jobs SET status='cancelled',message='Library selection changed',updated_at=? WHERE user_id=? AND target_id=? AND status IN ('queued','awaiting-approval')`, now(), uid, in.TargetID); err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE sync_targets SET revision=? WHERE user_id=? AND target_id=?`, syncID(), uid, in.TargetID)
		return err
	})
	if err != nil {
		return LibrarySelection{}, err
	}
	return s.LibrarySelectionFor(uid, in.TargetID)
}

func (s *Store) refreshCanonicalSelections(uid int64, agent SyncAgent) error {
	rows, err := s.db.Query(`SELECT c.target_id,c.item_ids,c.version FROM canonical_library_selections c JOIN environments e ON e.user_id=c.user_id AND e.id=c.target_id WHERE c.user_id=? AND e.host=? COLLATE NOCASE`, uid, agent.Host)
	if err != nil {
		return err
	}
	type selection struct{ target, raw, version string }
	var selections []selection
	for rows.Next() {
		var entry selection
		if err = rows.Scan(&entry.target, &entry.raw, &entry.version); err != nil {
			break
		}
		selections = append(selections, entry)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(selections) == 0 {
		return err
	}
	items, err := s.LibraryFor(uid)
	if err != nil {
		return err
	}
	byID := map[string]LibraryItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	var origin string
	_ = s.db.QueryRow(`SELECT origin FROM sync_library WHERE user_id=?`, uid).Scan(&origin)
	parsed, _ := url.Parse(origin)
	username, _ := s.PublishingUsername(uid)
	for _, selection := range selections {
		var ids []string
		if err = json.Unmarshal([]byte(selection.raw), &ids); err != nil {
			return err
		}
		desired := []SyncSkill{}
		names := map[string]int{}
		for _, id := range ids {
			item, ok := byID[id]
			if ok && item.Status == LibraryInUse && item.ItemType == LibrarySkill && item.RevisionID != "" {
				names[item.Name]++
			}
		}
		for _, id := range ids {
			item, ok := byID[id]
			if !ok || item.Status != LibraryInUse || item.ItemType != LibrarySkill || item.RevisionID == "" || names[item.Name] > 1 {
				continue
			}
			legacyID := "library/" + item.ID
			for _, alias := range item.Aliases {
				if alias.Kind == "shelf" {
					legacyID = alias.Reference
					break
				}
				if alias.Kind == "hosted" && parsed != nil && parsed.Host != "" {
					legacyID = "hosted/" + strings.ToLower(parsed.Host) + "/" + username + "/" + item.Name
				}
			}
			desired = append(desired, SyncSkill{ItemID: item.ID, ID: legacyID, Name: item.Name, Slug: item.Name, SourceType: "private", Source: "private:" + item.BundleID + ":" + item.RevisionID + ":" + item.Digest})
		}
		slices.SortFunc(desired, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
		raw, _ := json.Marshal(desired)
		err = s.withTx(func(tx *sql.Tx) error {
			// A concurrent selection save supersedes this computed projection.
			var current, version string
			if err := tx.QueryRow(`SELECT item_ids,version FROM canonical_library_selections WHERE user_id=? AND target_id=?`, uid, selection.target).Scan(&current, &version); err != nil {
				return err
			}
			if current != selection.raw || version != selection.version {
				return nil
			}
			for _, id := range ids {
				item, ok := byID[id]
				if !ok {
					continue
				}
				var head, status, name string
				if err := tx.QueryRow(`SELECT revision_id,status,name FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, id).Scan(&head, &status, &name); err != nil {
					return nil
				}
				if head != item.RevisionID || status != item.Status || name != item.Name {
					return nil
				}
			}
			var previous string
			err := tx.QueryRow(`SELECT desired FROM sync_targets WHERE user_id=? AND target_id=?`, uid, selection.target).Scan(&previous)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if previous != string(raw) {
				if _, err = tx.Exec(`INSERT INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at) VALUES(?,?,'library','Selected library items',?,?,?) ON CONFLICT(user_id,target_id) DO UPDATE SET desired=excluded.desired,revision=excluded.revision,updated_at=excluded.updated_at,source_id='library'`, uid, selection.target, string(raw), syncID(), now()); err != nil {
					return err
				}
			}
			if _, err = tx.Exec(`INSERT INTO sync_policies(target_id,follow_source) VALUES(?,0) ON CONFLICT(target_id) DO UPDATE SET follow_source=0`, selection.target); err != nil {
				return err
			}
			_, err = tx.Exec(`INSERT INTO sync_options(target_id,mode,baseline) VALUES(?,'additive','[]') ON CONFLICT(target_id) DO UPDATE SET mode='additive'`, selection.target)
			return err
		})
		if err != nil {
			return err
		}
	}
	return nil
}
