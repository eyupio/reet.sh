package store

import (
	"encoding/json"
	"errors"
	"sort"
)

// SyncHeld is a library item selected for a folder that sync is not installing.
// ID is the shelf skill it belongs to, where there is one, so the shelf can say
// the same thing about it as the library does.
type SyncHeld struct {
	ItemID string `json:"itemId"`
	ID     string `json:"id,omitempty"`
	Name   string `json:"name"`
	// Reason is "content" (no saved copy to install yet), "status" (not In
	// use) or "duplicate" (another selected item has the same name).
	Reason string `json:"reason"`
}

// heldLibrarySelections reads, per folder, the selected library items that
// refreshCanonicalSelections leaves out of the folder's desired set. It uses the
// same rules, in the same order, so the two can never disagree.
func (s *Store) heldLibrarySelections(uid int64) (map[string][]SyncHeld, error) {
	rows, err := s.db.Query(`SELECT target_id,item_ids FROM canonical_library_selections WHERE user_id=?`, uid)
	if err != nil {
		return nil, err
	}
	selections := map[string][]string{}
	for rows.Next() {
		var target, raw string
		if err = rows.Scan(&target, &raw); err != nil {
			break
		}
		var ids []string
		if json.Unmarshal([]byte(raw), &ids) == nil && len(ids) > 0 {
			selections[target] = ids
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil || len(selections) == 0 {
		return nil, err
	}
	items, err := s.LibraryFor(uid)
	if errors.Is(err, ErrPrivateSkillMissing) {
		// A read-only or disabled account has no library to hold anything back.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	byID := map[string]LibraryItem{}
	for _, item := range items {
		byID[item.ID] = item
	}
	out := map[string][]SyncHeld{}
	for target, ids := range selections {
		names := map[string]int{}
		for _, id := range ids {
			if item, ok := byID[id]; ok && item.Status == LibraryInUse && item.ItemType == LibrarySkill && item.RevisionID != "" {
				names[item.Name]++
			}
		}
		for _, id := range ids {
			item, ok := byID[id]
			if !ok || item.ItemType != LibrarySkill {
				continue
			}
			reason := ""
			switch {
			case item.Status != LibraryInUse:
				reason = "status"
			case item.RevisionID == "":
				reason = "content"
			case names[item.Name] > 1:
				reason = "duplicate"
			default:
				continue
			}
			entry := SyncHeld{ItemID: item.ID, Name: item.Name, Reason: reason}
			for _, alias := range item.Aliases {
				if alias.Kind == "shelf" {
					entry.ID = alias.Reference
					break
				}
			}
			out[target] = append(out[target], entry)
		}
		sort.Slice(out[target], func(i, j int) bool { return out[target][i].Name < out[target][j].Name })
	}
	return out, nil
}
