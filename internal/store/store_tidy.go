package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// EnvironmentChange describes one environment that a destructive operation
// skipped or would change.
type EnvironmentChange struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	SkillCount int    `json:"skillCount,omitempty"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
}

// DeleteEnvironmentsFor removes environment reports, refusing non-empty ones
// unless force is set. A report that still holds ownership receipts is refused
// the same way, since deleting it would silently forget them. Shelf skills are
// never deleted, only their install links.
func (s *Store) DeleteEnvironmentsFor(userID int64, ids []string, force bool) ([]Environment, []EnvironmentChange, error) {
	envs, err := s.ListEnvironmentsFor(userID)
	if err != nil {
		return nil, nil, err
	}
	receipts := map[string]int{}
	if !force {
		if receipts, err = s.ReceiptCountsFor(userID, ids); err != nil {
			return nil, nil, err
		}
	}
	index := make(map[string]Environment, len(envs))
	for _, env := range envs {
		index[env.ID] = env
	}
	deleted := []Environment{}
	blocked := []EnvironmentChange{}
	remove := []string{}
	for _, id := range ids {
		env, ok := index[id]
		if !ok {
			blocked = append(blocked, EnvironmentChange{ID: id, Code: "environment_not_found", Message: "environment not found"})
			continue
		}
		if env.SkillCount > 0 && !force {
			blocked = append(blocked, EnvironmentChange{ID: id, Name: env.Name, SkillCount: env.SkillCount, Code: "environment_has_skills", Message: "environment still has skills"})
			continue
		}
		if receipts[id] > 0 {
			blocked = append(blocked, EnvironmentChange{ID: id, Name: env.Name, Code: "environment_has_receipts", Message: "environment still holds ownership receipts; release them first"})
			continue
		}
		deleted = append(deleted, env)
		remove = append(remove, id)
	}
	if len(remove) == 0 {
		return deleted, blocked, nil
	}
	err = s.withTx(func(tx *sql.Tx) error {
		for _, id := range remove {
			if _, err := tx.Exec(`DELETE FROM environments WHERE user_id=? AND id=?`, userID, id); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return deleted, blocked, nil
}

// MergeEnvironmentsFor moves one report's skills into another and removes the
// source report. The destination keeps its own scan metadata.
func (s *Store) MergeEnvironmentsFor(userID int64, fromID, toID string) (int, error) {
	if fromID == "" || toID == "" || fromID == toID {
		return 0, errors.New("choose two different environment IDs")
	}
	moved := 0
	err := s.withTx(func(tx *sql.Tx) error {
		var fromName, toName string
		if err := tx.QueryRow(`SELECT name FROM environments WHERE user_id=? AND id=?`, userID, fromID).Scan(&fromName); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("source environment not found")
		} else if err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT name FROM environments WHERE user_id=? AND id=?`, userID, toID).Scan(&toName); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("destination environment not found")
		} else if err != nil {
			return err
		}
		var contentPins int
		if err := tx.QueryRow(`SELECT count(*) FROM skill_content_targets WHERE user_id=? AND environment_id=?`, userID, fromID).Scan(&contentPins); err != nil {
			return err
		}
		if contentPins > 0 {
			return ErrSkillContentTargetScopeMerge
		}
		var receipts int
		if err := tx.QueryRow(`SELECT count(*) FROM skill_content_receipts WHERE user_id=? AND environment_id=?`, userID, fromID).Scan(&receipts); err != nil {
			return err
		}
		if receipts > 0 {
			return ErrSkillContentReceiptScope
		}
		res, err := tx.Exec(`INSERT OR IGNORE INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path)
			SELECT skill_id,?,installed_at,last_seen,source_path FROM skill_envs WHERE env_id=?`, toID, fromID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		moved = int(n)
		if _, err := tx.Exec(`INSERT OR IGNORE INTO environment_scans(env_id,scanned_at,complete,os,arch,version)
			SELECT ?,scanned_at,complete,os,arch,version FROM environment_scans WHERE env_id=?`, toID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO scan_errors(env_id,error) SELECT ?,error FROM scan_errors WHERE env_id=?`, toID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO sync_targets(user_id,target_id,source_id,source_name,desired,revision,updated_at)
			SELECT user_id,?,source_id,source_name,desired,revision,updated_at FROM sync_targets WHERE user_id=? AND target_id=?`, toID, userID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO sync_options(target_id,mode,baseline,applied_revision)
			SELECT ?,mode,baseline,applied_revision FROM sync_options WHERE target_id=?`, toID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM environments WHERE user_id=? AND id=?`, userID, fromID); err != nil {
			return err
		}
		return nil
	})
	return moved, err
}

// PruneEmptyServerEnvironmentsFor deletes empty reet-server reports older than
// days. It is called after a heartbeat so stale container names do not pile up.
func (s *Store) PruneEmptyServerEnvironmentsFor(userID int64, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	cutoff := now() - int64(days)*24*60*60*1000
	res, err := s.db.Exec(`DELETE FROM environments WHERE user_id=? AND agent='reet-server' AND last_seen<?
		AND NOT EXISTS (SELECT 1 FROM skill_envs se WHERE se.env_id=environments.id)`, userID, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) autoPruneServerEnvironments(userID int64) {
	raw, err := s.GetSetting("prune_server_environments_days")
	if err != nil {
		return
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || days <= 0 {
		return
	}
	_, _ = s.PruneEmptyServerEnvironmentsFor(userID, days)
}

// MatchShelfSkillFor resolves an exact ID or a display name/slug. It returns
// every match so callers can report an ambiguity instead of guessing.
func (s *Store) MatchShelfSkillFor(userID int64, value string) (Skill, []Skill, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Skill{}, nil, ErrNotFound
	}
	if sk, ok, err := s.GetFor(userID, value); err != nil {
		return Skill{}, nil, err
	} else if ok {
		return sk, nil, nil
	}
	skills, _, err := s.ListFor(userID, Filter{})
	if err != nil {
		return Skill{}, nil, err
	}
	want := normalName(value)
	wantSlug := normalName(slugOf(value))
	matches := []Skill{}
	for _, sk := range skills {
		if (want != "" && normalName(sk.Name) == want) ||
			(wantSlug != "" && normalName(slugOf(sk.ID)) == wantSlug) ||
			strings.EqualFold(sk.ID, value) {
			matches = append(matches, sk)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil, nil
	}
	if len(matches) > 1 {
		return Skill{}, matches, nil
	}
	return Skill{}, nil, ErrNotFound
}

// CreateTag makes an empty account-visible tag. It does not attach it to a skill.
func (s *Store) CreateTag(name string) (Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return Tag{}, errors.New("tag name must be 1 to 100 characters")
	}
	var tag Tag
	err := s.withTx(func(tx *sql.Tx) error {
		id, err := upsertTagTx(tx, name)
		if err != nil {
			return err
		}
		return tx.QueryRow(`SELECT id,name FROM tags WHERE id=?`, id).Scan(&tag.ID, &tag.Name)
	})
	return tag, err
}

// RenameTagFor renames a tag for one account. The global tag row is only
// renamed when no other account or curated topic still uses it.
func (s *Store) RenameTagFor(userID int64, id int64, name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 {
		return 0, errors.New("tag name must be 1 to 100 characters")
	}
	affected := 0
	err := s.withTx(func(tx *sql.Tx) error {
		var oldName string
		if err := tx.QueryRow(`SELECT name FROM tags WHERE id=?`, id).Scan(&oldName); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if strings.EqualFold(oldName, name) {
			return nil
		}
		newID, err := upsertTagTx(tx, name)
		if err != nil {
			return err
		}
		res, err := tx.Exec(`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden)
			SELECT user_id,skill_id,?,source,hidden FROM skill_tags WHERE user_id=? AND tag_id=?`, newID, userID, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			affected = int(n)
		}
		if _, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=? AND tag_id=?`, userID, id); err != nil {
			return err
		}
		var other int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM skill_tags WHERE tag_id=?`, id).Scan(&other); err != nil {
			return err
		}
		if other == 0 {
			var topics int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM topic_skills WHERE tag_id=?`, id).Scan(&topics); err != nil {
				return err
			}
			if topics == 0 {
				if _, err := tx.Exec(`DELETE FROM tags WHERE id=?`, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return affected, err
}

// TagUsageFor returns how many of this account's skills carry the tag.
func (s *Store) TagUsageFor(userID int64, id int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM skill_tags WHERE user_id=? AND tag_id=?`, userID, id).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM tags WHERE id=?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	} else if err != nil {
		return 0, err
	}
	return count, nil
}

// DeleteTagFor removes this account's use of a tag and removes the global row
// when nobody else references it. Other accounts and curated topics are kept.
func (s *Store) DeleteTagFor(userID int64, id int64) (int, error) {
	affected := 0
	err := s.withTx(func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRow(`SELECT 1 FROM tags WHERE id=?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		res, err := tx.Exec(`DELETE FROM skill_tags WHERE user_id=? AND tag_id=?`, userID, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			affected = int(n)
		}
		var other int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM skill_tags WHERE tag_id=?`, id).Scan(&other); err != nil {
			return err
		}
		if other == 0 {
			var topics int
			if err := tx.QueryRow(`SELECT COUNT(*) FROM topic_skills WHERE tag_id=?`, id).Scan(&topics); err != nil {
				return err
			}
			if topics == 0 {
				if _, err := tx.Exec(`DELETE FROM tags WHERE id=?`, id); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return affected, err
}

// BulkChange is the combined change reet_bulk_update_skills applies.
type BulkChange struct {
	Liked        *bool
	Bookmarked   *bool
	Used         *bool
	AddTags      []string
	RemoveTagIDs []int64
	AddToListID  int64
}

// BulkItemResult is one skill's outcome in a bulk operation.
type BulkItemResult struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// BulkUpdateFor applies flags, tags and one list membership in a single
// transaction. dryRun runs the same work and rolls it back.
func (s *Store) BulkUpdateFor(userID int64, ids []string, change BulkChange, dryRun bool) (int, []BulkItemResult, error) {
	results := make([]BulkItemResult, 0, len(ids))
	updated := 0
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	var addIDs []int64
	for _, name := range change.AddTags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		tagID, err := upsertTagTx(tx, name)
		if err != nil {
			return 0, nil, err
		}
		addIDs = append(addIDs, tagID)
	}
	for _, id := range ids {
		var one int
		if err := tx.QueryRow(`SELECT 1 FROM shelf WHERE user_id=? AND skill_id=?`, userID, id).Scan(&one); errors.Is(err, sql.ErrNoRows) {
			results = append(results, BulkItemResult{ID: id, Code: "skill_not_found", Message: "skill is not on this account's shelf"})
			continue
		} else if err != nil {
			return 0, nil, err
		}
		if change.AddToListID > 0 {
			var visibility, sourceType string
			if err := tx.QueryRow(`SELECT visibility FROM lists WHERE id=?`, change.AddToListID).Scan(&visibility); err != nil {
				return 0, nil, err
			}
			if err := tx.QueryRow(`SELECT source_type FROM skills WHERE id=?`, id).Scan(&sourceType); err != nil {
				return 0, nil, err
			}
			if sourceType == "local" && visibility != "personal" {
				results = append(results, BulkItemResult{ID: id, Code: "local_skill_not_allowed_in_list", Message: "local skills can only be added to personal lists"})
				continue
			}
		}
		if change.Liked != nil || change.Bookmarked != nil || change.Used != nil {
			if _, err := tx.Exec(`UPDATE shelf SET liked=COALESCE(?,liked), bookmarked=COALESCE(?,bookmarked), used=COALESCE(?,used), updated_at=?
				WHERE user_id=? AND skill_id=?`, nullBool(change.Liked), nullBool(change.Bookmarked), nullBool(change.Used), now(), userID, id); err != nil {
				return 0, nil, err
			}
		}
		for _, tagID := range addIDs {
			if _, err := tx.Exec(`INSERT INTO skill_tags(user_id,skill_id,tag_id,source,hidden) VALUES(?,?,?,'user',0)
				ON CONFLICT(user_id,skill_id,tag_id) DO UPDATE SET source='user', hidden=0`, userID, id, tagID); err != nil {
				return 0, nil, err
			}
		}
		for _, tagID := range change.RemoveTagIDs {
			if _, err := tx.Exec(`UPDATE skill_tags SET hidden=1 WHERE user_id=? AND skill_id=? AND tag_id=?`, userID, id, tagID); err != nil {
				return 0, nil, err
			}
		}
		if change.AddToListID > 0 {
			if _, err := tx.Exec(`INSERT INTO list_skills(list_id,skill_id,note,added_by,position,added_at)
				VALUES(?,?,'',?,COALESCE((SELECT MAX(position)+1 FROM list_skills WHERE list_id=?),0),?)
				ON CONFLICT(list_id,skill_id) DO NOTHING`, change.AddToListID, id, userID, change.AddToListID, now()); err != nil {
				return 0, nil, err
			}
		}
		updated++
		results = append(results, BulkItemResult{ID: id, OK: true})
	}
	if change.AddToListID > 0 {
		if _, err := tx.Exec(`UPDATE lists SET updated_at=? WHERE id=?`, now(), change.AddToListID); err != nil {
			return 0, nil, err
		}
	}
	if dryRun {
		return updated, results, nil
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return updated, results, nil
}

// DuplicateSkill is one member of a suspected duplicate group.
type DuplicateSkill struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Source     string `json:"source"`
	SourceType string `json:"sourceType"`
}

// DuplicateGroup is a set of shelf skills that look like the same skill.
type DuplicateGroup struct {
	Kind                 string           `json:"kind"`
	Key                  string           `json:"key"`
	Reason               string           `json:"reason"`
	SuggestedCanonicalID string           `json:"suggestedCanonicalId"`
	Skills               []DuplicateSkill `json:"skills"`
}

func normalName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func slugOf(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

func canonicalSkill(skills []Skill) string {
	best := ""
	for _, sk := range skills {
		if sk.SourceType == "local" || sk.Source == "local" {
			continue
		}
		if best == "" || len(sk.ID) < len(best) || (len(sk.ID) == len(best) && sk.ID < best) {
			best = sk.ID
		}
	}
	if best == "" {
		for _, sk := range skills {
			if best == "" || len(sk.ID) < len(best) || (len(sk.ID) == len(best) && sk.ID < best) {
				best = sk.ID
			}
		}
	}
	return best
}

// FindDuplicatesFor groups shelf skills by normalised name and by upstream
// source. It only reports groups with more than one distinct skill ID.
func (s *Store) FindDuplicatesFor(userID int64) ([]DuplicateGroup, error) {
	skills, _, err := s.ListFor(userID, Filter{})
	if err != nil {
		return nil, err
	}
	byName := map[string][]Skill{}
	bySource := map[string][]Skill{}
	for _, sk := range skills {
		name := normalName(sk.Name)
		if name == "" {
			name = normalName(slugOf(sk.ID))
		}
		if name != "" {
			byName[name] = append(byName[name], sk)
		}
		source := strings.ToLower(strings.TrimSpace(sk.Source))
		if source != "" && source != "local" {
			key := source + "|" + normalName(slugOf(sk.ID))
			bySource[key] = append(bySource[key], sk)
		}
	}
	seen := map[string]bool{}
	groups := []DuplicateGroup{}
	add := func(kind, key, reason string, members []Skill) {
		if len(members) < 2 {
			return
		}
		ids := make([]string, 0, len(members))
		distinct := map[string]bool{}
		for _, sk := range members {
			if !distinct[sk.ID] {
				distinct[sk.ID] = true
				ids = append(ids, sk.ID)
			}
		}
		if len(ids) < 2 {
			return
		}
		sort.Strings(ids)
		signature := strings.Join(ids, "\x00")
		if seen[signature] {
			return
		}
		seen[signature] = true
		out := make([]DuplicateSkill, 0, len(ids))
		hasLocal, hasUpstream := false, false
		byID := map[string]Skill{}
		for _, sk := range members {
			if _, ok := byID[sk.ID]; !ok {
				byID[sk.ID] = sk
			}
		}
		for _, id := range ids {
			sk := byID[id]
			if sk.SourceType == "local" || sk.Source == "local" {
				hasLocal = true
			} else {
				hasUpstream = true
			}
			out = append(out, DuplicateSkill{ID: sk.ID, Name: sk.Name, Source: sk.Source, SourceType: sk.SourceType})
		}
		if hasLocal && hasUpstream {
			kind = "local_upstream"
			reason = "a local install shares a name with an upstream skill"
		}
		groups = append(groups, DuplicateGroup{Kind: kind, Key: key, Reason: reason, SuggestedCanonicalID: canonicalSkill(members), Skills: out})
	}
	for name, members := range byName {
		add("name", name, "skills share a normalised display name", members)
	}
	for key, members := range bySource {
		add("same_source", key, "skills from the same upstream source share a normalised slug", members)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Kind != groups[j].Kind {
			return groups[i].Kind < groups[j].Kind
		}
		return groups[i].Key < groups[j].Key
	})
	return groups, nil
}

// EventQuery selects one page of audit events.
type EventQuery struct {
	Limit         int
	SkillID       string
	Action        string
	Source        string
	Before, After int64
}

// ListEventsPageFor reads audit events with ID cursors. Before pages older,
// After pages newer; otherwise the newest events come first.
func (s *Store) ListEventsPageFor(userID int64, q EventQuery) ([]Event, bool, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	clauses := []string{"ev.user_id=?"}
	args := []any{userID}
	if q.SkillID != "" {
		clauses = append(clauses, "ev.skill_id=?")
		args = append(args, q.SkillID)
	}
	if q.Action != "" {
		clauses = append(clauses, "ev.action=?")
		args = append(args, q.Action)
	}
	if q.Source != "" {
		clauses = append(clauses, "ev.source=?")
		args = append(args, q.Source)
	}
	if q.Before > 0 {
		clauses = append(clauses, "ev.id<?")
		args = append(args, q.Before)
	}
	if q.After > 0 {
		clauses = append(clauses, "ev.id>?")
		args = append(args, q.After)
	}
	order := "DESC"
	if q.After > 0 && q.Before == 0 {
		order = "ASC"
	}
	query := `SELECT ev.id,ev.at,ev.actor,ev.source,ev.action,ev.skill_id,COALESCE(s.name,''),ev.detail
		FROM events ev LEFT JOIN skills s ON s.id=ev.skill_id WHERE ` + strings.Join(clauses, " AND ") +
		` ORDER BY ev.id ` + order + ` LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var ev Event
		if err := rows.Scan(&ev.ID, &ev.At, &ev.Actor, &ev.Source, &ev.Action, &ev.SkillID, &ev.SkillName, &ev.Detail); err != nil {
			return nil, false, err
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}
