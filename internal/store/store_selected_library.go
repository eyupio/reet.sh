package store

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strings"
)

func (s *Store) selectedLibraryEnabled(uid int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM library_selection_accounts WHERE user_id=?`, uid).Scan(&n)
	return n > 0, err
}

// recordLibrarySelection retains the user's selection separately from currently
// eligible content. Existing policies are snapshotted once; future additions
// never become implicit selections.
func recordLibrarySelection(tx *sql.Tx, uid int64, target string, ids []string) error {
	if _, err := tx.Exec(`INSERT OR IGNORE INTO library_selection_accounts VALUES(?)`, uid); err != nil {
		return err
	}
	rows, err := tx.Query(`SELECT target_id,desired FROM sync_targets WHERE user_id=? AND source_id='library'`, uid)
	if err != nil {
		return err
	}
	type saved struct{ target, raw string }
	var prior []saved
	for rows.Next() {
		var r saved
		if err = rows.Scan(&r.target, &r.raw); err != nil {
			break
		}
		prior = append(prior, r)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range prior {
		var skills []SyncSkill
		if err = json.Unmarshal([]byte(r.raw), &skills); err != nil {
			return err
		}
		selected := []string{}
		for _, sk := range skills {
			selected = append(selected, sk.ID)
		}
		raw, _ := json.Marshal(selected)
		if _, err = tx.Exec(`INSERT OR IGNORE INTO library_selections VALUES(?,?,?)`, uid, r.target, string(raw)); err != nil {
			return err
		}
	}
	if target == "" {
		return nil
	}
	raw, _ := json.Marshal(ids)
	_, err = tx.Exec(`INSERT INTO library_selections VALUES(?,?,?) ON CONFLICT(user_id,target_id) DO UPDATE SET selected=excluded.selected`, uid, target, string(raw))
	return err
}

func (s *Store) refreshSelectedLibrary(uid int64, role string, agent SyncAgent) error {
	if err := s.refreshCanonicalSelections(uid, agent); err != nil {
		return err
	}
	candidates, err := s.SyncLibraryFor(uid, role)
	if err != nil {
		return err
	}
	eligible := map[string]SyncSkill{}
	for _, sk := range candidates {
		var status string
		e := s.db.QueryRow(`SELECT i.status FROM library_items i JOIN library_aliases a ON a.user_id=i.user_id AND a.item_id=i.id WHERE a.user_id=? AND a.kind='shelf' AND a.reference=? UNION ALL SELECT i.status FROM library_items i JOIN library_aliases a ON a.user_id=i.user_id AND a.item_id=i.id WHERE a.user_id=? AND a.kind='hosted' AND a.reference=? LIMIT 1`, uid, sk.ID, uid, sk.Slug).Scan(&status)
		if e != nil && e != sql.ErrNoRows {
			return e
		}
		if status == LibraryInUse && sk.Problem == "" && sk.Source != "" && sk.SourceType != "local" {
			eligible[sk.ID] = sk
		}
	}
	targets, err := s.SyncTargetsFor(uid)
	if err != nil {
		return err
	}
	for _, target := range targets {
		var canonical int
		if err = s.db.QueryRow(`SELECT count(*) FROM canonical_library_selections WHERE user_id=? AND target_id=?`, uid, target.Target.ID).Scan(&canonical); err != nil {
			return err
		}
		if canonical > 0 {
			continue
		}
		if !strings.EqualFold(target.Target.Host, agent.Host) {
			continue
		}
		var raw string
		err = s.db.QueryRow(`SELECT selected FROM library_selections WHERE user_id=? AND target_id=?`, uid, target.Target.ID).Scan(&raw)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		var selected []string
		if err = json.Unmarshal([]byte(raw), &selected); err != nil {
			return err
		}
		ids := []string{}
		desired := []SyncSkill{}
		for _, id := range selected {
			if sk, ok := eligible[id]; ok {
				ids = append(ids, id)
				desired = append(desired, sk)
			}
		}
		slices.SortFunc(desired, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
		current := slices.Clone(target.Desired)
		slices.SortFunc(current, func(a, b SyncSkill) int { return strings.Compare(a.ID, b.ID) })
		if slices.Equal(desired, current) && !target.FollowSource {
			continue
		}
		if err = s.SaveDesiredFor(uid, role, DesiredSelection{TargetID: target.Target.ID, SourceID: "library", SkillIDs: ids, Mode: "additive", Revision: target.Revision, selectedRefresh: true}); err != nil {
			return err
		}
	}
	return nil
}
