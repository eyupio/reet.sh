package store

import (
	"database/sql"
	"errors"
	"slices"
	"time"
)

var (
	ErrSkillNameTaken = errors.New("a skill with that name is already in your library")
	ErrSkillNoCopy    = errors.New("reet has no copy of this skill")
)

// SkillUndoWindow is how long a deleted skill can be brought back. After it,
// PurgeDeletedSkillsFor removes its written skill, bundle and revisions.
const SkillUndoWindow = time.Minute

// skillUndoNow is the clock UndeleteSkillsFor measures the window with; tests
// replace it to stand exactly on the window's edge.
var skillUndoNow = now

// SkillRemoval is a name machines back up and remove. Reason is "archived" or
// "deleted".
type SkillRemoval struct {
	Name, Reason string
	CreatedAt    int64
}

// UpgradeNotice counts the skills the skill-states upgrade archived: the
// Parked ones and the ones on the old ignore list.
type UpgradeNotice struct {
	Parked  int `json:"parked"`
	Removed int `json:"removed"`
}

// library_items after skill-states-v1. origin is how a skill arrived:
// written, imported (by link), machine (a copy found on a machine), or empty
// when reet cannot tell. upgraded_from marks what the upgrade archived.
const skillStatesLibraryItems = `CREATE TABLE library_items_v2(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,id TEXT NOT NULL,item_type TEXT NOT NULL CHECK(item_type IN ('skill','agent_definition','portable_settings')),name TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('active','archived')),version INTEGER NOT NULL DEFAULT 1,bundle_id TEXT NOT NULL DEFAULT '',revision_id TEXT NOT NULL DEFAULT '',digest TEXT NOT NULL DEFAULT '',problem TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,deleted_at INTEGER NOT NULL DEFAULT 0,prev_status TEXT NOT NULL DEFAULT '',status_at INTEGER NOT NULL DEFAULT 0,origin TEXT NOT NULL DEFAULT '',upgraded_from TEXT NOT NULL DEFAULT '' CHECK(upgraded_from IN ('','parked','removed')),copy_problem TEXT NOT NULL DEFAULT '',PRIMARY KEY(user_id,id))`

// The upgrade maps In use and To test to Active and everything else to
// Archived, marking the Parked ones.
const skillStatesCopy = `INSERT INTO library_items_v2(user_id,id,item_type,name,status,version,bundle_id,revision_id,digest,problem,created_at,updated_at,deleted_at,origin,upgraded_from)
 SELECT i.user_id,i.id,i.item_type,i.name,CASE WHEN i.status IN ('in_use','to_test') THEN 'active' ELSE 'archived' END,i.version,i.bundle_id,i.revision_id,i.digest,i.problem,i.created_at,i.updated_at,i.deleted_at,
 CASE WHEN EXISTS(SELECT 1 FROM library_aliases a WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='hosted') THEN 'written'
  WHEN EXISTS(SELECT 1 FROM library_aliases a JOIN skills k ON k.id=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf' AND k.source<>'' AND k.source_type<>'local') THEN 'imported'
  ELSE 'machine' END,
 CASE WHEN i.status='on_shelf' THEN 'parked' ELSE '' END
 FROM library_items i`

// The triggers that refer to library_items. They are dropped before it is
// rebuilt and created again after; library_shelf_stage and
// library_status_projection mirrored the shelf flags and are not.
var skillStatesTriggers = []string{"library_shelf_insert", "library_shelf_stage", "library_status_projection", "library_hosted_insert", "library_hosted_update", "library_hosted_delete", "library_bundle_delete", "library_intent_fence"}

func (s *Store) migrateSkillStates() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS skill_removals(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,name TEXT NOT NULL COLLATE NOCASE,reason TEXT NOT NULL CHECK(reason IN ('archived','deleted')),created_at INTEGER NOT NULL,PRIMARY KEY(user_id,name))`,
			`CREATE TABLE IF NOT EXISTS skill_removal_confirms(user_id INTEGER NOT NULL,name TEXT NOT NULL COLLATE NOCASE,agent_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,at INTEGER NOT NULL,PRIMARY KEY(user_id,name,agent_id),FOREIGN KEY(user_id,name) REFERENCES skill_removals(user_id,name) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS skill_notices(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,parked INTEGER NOT NULL,removed INTEGER NOT NULL,dismissed INTEGER NOT NULL DEFAULT 0)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		var migrated int
		if err := tx.QueryRow(`SELECT count(*) FROM library_migrations WHERE name='skill-states-v1'`).Scan(&migrated); err != nil {
			return err
		}
		if migrated == 0 {
			if err := s.upgradeSkillStates(tx); err != nil {
				return err
			}
		}
		for _, stmt := range append([]string{libraryItemProjection, libraryIntentFence}, canonicalLibraryTriggers...) {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) upgradeSkillStates(tx *sql.Tx) error {
	for _, name := range skillStatesTriggers {
		if _, err := tx.Exec(`DROP TRIGGER IF EXISTS ` + name); err != nil {
			return err
		}
	}
	// Dropping library_items cascades to its aliases, so they are kept aside
	// and put back. The old stages decide which same-name copy wins.
	for _, stmt := range []string{
		`CREATE TEMP TABLE skill_states_aliases AS SELECT * FROM library_aliases`,
		`CREATE TEMP TABLE skill_states_in_use AS SELECT user_id,id FROM library_items WHERE status='in_use'`,
		skillStatesLibraryItems,
		skillStatesCopy,
		`DROP TABLE library_items`,
		`ALTER TABLE library_items_v2 RENAME TO library_items`,
		`INSERT INTO library_aliases SELECT * FROM temp.skill_states_aliases`,
		`DROP TABLE temp.skill_states_aliases`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	if err := ignoredIntoArchived(tx); err != nil {
		return err
	}
	if err := s.foldSkillNames(tx); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE temp.skill_states_in_use`); err != nil {
		return err
	}
	// Only a skill reet holds a copy of is taken off machines. The others stay
	// where they are until reet has one.
	at := now()
	if _, err := tx.Exec(`INSERT INTO skill_removals(user_id,name,reason,created_at) SELECT user_id,name,'archived',? FROM library_items WHERE item_type='skill' AND status='archived' AND deleted_at=0 AND revision_id<>'' ON CONFLICT DO NOTHING`, at); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO skill_notices(user_id,parked,removed) SELECT user_id,sum(upgraded_from='parked'),sum(upgraded_from='removed') FROM library_items WHERE item_type='skill' AND status='archived' AND deleted_at=0 GROUP BY user_id HAVING sum(upgraded_from<>'')>0 ON CONFLICT(user_id) DO UPDATE SET parked=excluded.parked,removed=excluded.removed,dismissed=0`); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO library_migrations VALUES('skill-states-v1',?)`, at)
	return err
}

// ignoredIntoArchived turns the old ignore list into Archived items. A skill
// that still has its library item keeps it; one without gets a new item with
// its name and no content, which the ignored ID points to so a later sighting
// of it finds the archived item.
func ignoredIntoArchived(tx *sql.Tx) error {
	for _, stmt := range []string{
		`UPDATE library_items SET status='archived',upgraded_from='removed' WHERE deleted_at=0 AND EXISTS(SELECT 1 FROM ignored g JOIN library_aliases a ON a.user_id=g.user_id AND a.kind='shelf' AND a.reference=g.id WHERE a.user_id=library_items.user_id AND a.item_id=library_items.id)`,
		`INSERT INTO library_aliases(user_id,kind,reference,item_id) SELECT g.user_id,'shelf',g.id,lower(hex(randomblob(16))) FROM ignored g WHERE EXISTS(SELECT 1 FROM users u WHERE u.id=g.user_id) AND NOT EXISTS(SELECT 1 FROM library_aliases a WHERE a.user_id=g.user_id AND a.kind='shelf' AND a.reference=g.id)`,
		// An ignored row without a name is named after its ID's last segment.
		`INSERT INTO library_items(user_id,id,item_type,name,status,origin,upgraded_from,created_at,updated_at) SELECT g.user_id,a.item_id,'skill',CASE WHEN g.name<>'' THEN g.name ELSE replace(g.id,rtrim(g.id,replace(g.id,'/','')),'') END,'archived',CASE WHEN g.id LIKE 'local/%' THEN 'machine' ELSE 'imported' END,'removed',g.at,g.at FROM ignored g JOIN library_aliases a ON a.user_id=g.user_id AND a.kind='shelf' AND a.reference=g.id WHERE NOT EXISTS(SELECT 1 FROM library_items i WHERE i.user_id=a.user_id AND i.id=a.item_id)`,
		`DELETE FROM ignored`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// foldSkillNames leaves one live item per name. A written skill wins, then an
// imported one, then a copy found on a machine; among equals the In use one,
// then the most recently updated. The winner keeps its own status.
func (s *Store) foldSkillNames(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT i.user_id,i.item_type||char(0)||lower(i.name),i.id FROM library_items i
 WHERE i.deleted_at=0 AND EXISTS(SELECT 1 FROM library_items o WHERE o.user_id=i.user_id AND o.item_type=i.item_type AND o.deleted_at=0 AND lower(o.name)=lower(i.name) AND o.id<>i.id)
 ORDER BY i.user_id,i.item_type,lower(i.name),CASE i.origin WHEN 'written' THEN 0 WHEN 'imported' THEN 1 ELSE 2 END,EXISTS(SELECT 1 FROM temp.skill_states_in_use u WHERE u.user_id=i.user_id AND u.id=i.id) DESC,i.updated_at DESC,i.id`)
	if err != nil {
		return err
	}
	type copyOf struct {
		uid      int64
		name, id string
	}
	var copies []copyOf
	for rows.Next() {
		var c copyOf
		if err = rows.Scan(&c.uid, &c.name, &c.id); err != nil {
			break
		}
		copies = append(copies, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	winner := copyOf{}
	folded := map[int64]bool{}
	for _, c := range copies {
		if c.uid != winner.uid || c.name != winner.name {
			winner = c
			continue
		}
		if err = s.foldLibraryItem(tx, c.uid, winner.id, c.id); err != nil {
			return err
		}
		folded[c.uid] = true
	}
	for uid := range folded {
		if err = dropUnusedBlobs(tx, uid); err != nil {
			return err
		}
	}
	return nil
}

// foldLibraryItem moves the loser's tags, notes, like and list membership onto
// the winner's shelf row and deletes the loser from reet, its content included.
// Machines are not touched. When the winner has no shelf row the loser's
// becomes its own.
func (s *Store) foldLibraryItem(tx *sql.Tx, uid int64, winner, loser string) error {
	shelfOf := func(item string) ([]string, error) {
		rows, err := tx.Query(`SELECT a.reference FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference WHERE a.user_id=? AND a.item_id=? AND a.kind='shelf' ORDER BY a.reference`, uid, item)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, rows.Err()
	}
	kept, err := shelfOf(winner)
	if err != nil {
		return err
	}
	folded, err := shelfOf(loser)
	if err != nil {
		return err
	}
	for _, from := range folded {
		if len(kept) == 0 {
			if _, err = tx.Exec(`UPDATE library_aliases SET item_id=? WHERE user_id=? AND kind='shelf' AND reference=?`, winner, uid, from); err != nil {
				return err
			}
			kept = []string{from}
			continue
		}
		if err = s.moveShelfOrganisation(tx, uid, from, kept[0]); err != nil {
			return err
		}
	}
	return s.purgeSkillTx(tx, uid, loser)
}

func (s *Store) moveShelfOrganisation(tx *sql.Tx, uid int64, from, to string) error {
	var liked bool
	var notes, keptNotes string
	if err := tx.QueryRow(`SELECT liked,notes FROM shelf WHERE user_id=? AND skill_id=?`, uid, from).Scan(&liked, &notes); err != nil {
		return err
	}
	if err := tx.QueryRow(`SELECT notes FROM shelf WHERE user_id=? AND skill_id=?`, uid, to).Scan(&keptNotes); err != nil {
		return err
	}
	if keptNotes != "" && notes != "" && keptNotes != notes {
		notes = keptNotes + "\n\n" + notes
	} else if keptNotes != "" {
		notes = keptNotes
	}
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`UPDATE shelf SET liked=liked OR ?,notes=? WHERE user_id=? AND skill_id=?`, []any{liked, notes, uid, to}},
		{`INSERT OR IGNORE INTO skill_tags(user_id,skill_id,tag_id,source,hidden) SELECT user_id,?,tag_id,source,hidden FROM skill_tags WHERE user_id=? AND skill_id=?`, []any{to, uid, from}},
		{`INSERT OR IGNORE INTO list_skills(list_id,skill_id,note,added_by,position,added_at) SELECT list_id,?,note,added_by,position,added_at FROM list_skills WHERE skill_id=? AND list_id IN (SELECT id FROM lists WHERE owner_id=?)`, []any{to, from, uid}},
		{`DELETE FROM list_skills WHERE skill_id=? AND list_id IN (SELECT id FROM lists WHERE owner_id=?)`, []any{from, uid}},
		{`INSERT OR IGNORE INTO skill_envs(skill_id,env_id,installed_at,last_seen,source_path) SELECT ?,env_id,installed_at,last_seen,source_path FROM skill_envs WHERE skill_id=? AND env_id IN (SELECT id FROM environments WHERE user_id=?)`, []any{to, from, uid}},
	} {
		if _, err := tx.Exec(stmt.sql, stmt.args...); err != nil {
			return err
		}
	}
	return s.removeShelfTx(tx, uid, from, false)
}

type skillStateItem struct {
	id, itemType, name, status, revision string
	version, deletedAt                   int64
}

func skillStateItemTx(tx *sql.Tx, uid int64, id string) (skillStateItem, error) {
	it := skillStateItem{id: id}
	err := tx.QueryRow(`SELECT item_type,name,status,revision_id,version,deleted_at FROM library_items WHERE user_id=? AND id=?`, uid, id).Scan(&it.itemType, &it.name, &it.status, &it.revision, &it.version, &it.deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrPrivateSkillMissing
	}
	return it, err
}

func skillNameTakenTx(tx *sql.Tx, uid int64, itemType, name, except string) (bool, error) {
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0 AND lower(name)=lower(?) AND id<>?`, uid, itemType, name, except).Scan(&n)
	return n > 0, err
}

// linkedSkillNameFree refuses a pasted or linked source whose name the library
// already has. A shelf row or alias that exists keeps its item, and a link to
// the account's own written skill is that skill. Machine sightings (local
// rows) are not checked: they join the skill of their name.
func linkedSkillNameFree(tx *sql.Tx, uid int64, id, slug string) error {
	var known int
	if err := tx.QueryRow(`SELECT (SELECT count(*) FROM shelf WHERE user_id=? AND skill_id=?)+(SELECT count(*) FROM library_aliases WHERE user_id=? AND kind='shelf' AND reference=?)`, uid, id, uid, id).Scan(&known); err != nil {
		return err
	}
	if known > 0 {
		return nil
	}
	own := ""
	err := tx.QueryRow(`SELECT a.item_id FROM publishers p JOIN library_aliases a ON a.user_id=p.user_id AND a.kind='hosted' AND a.reference=? WHERE p.user_id=? AND ? GLOB 'hosted/*/'||p.username||'/'||?`, slug, uid, id, slug).Scan(&own)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	taken, err := skillNameTakenTx(tx, uid, LibrarySkill, slug, own)
	if err == nil && taken {
		err = ErrSkillNameTaken
	}
	return err
}

// refuseSecondSkillName ends a content write that can make a library item.
// Content is saved before it is bound to an item, so the check runs last: it
// fails when the item the bundle ended up on shares its name with another live
// item, which only happens when the write made a new one.
func refuseSecondSkillName(tx *sql.Tx, uid int64, result PrivateSkillResult) error {
	id, err := libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var itemType, name string
	if err = tx.QueryRow(`SELECT item_type,name FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, id).Scan(&itemType, &name); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	taken, err := skillNameTakenTx(tx, uid, itemType, name, id)
	if err == nil && taken {
		err = ErrSkillNameTaken
	}
	return err
}

// setSkillStatusTx archives or restores one live item. Archiving puts a skill
// on the removal list only when reet holds a copy of it, so the only copy is
// never taken off a machine; restoring takes its name off the list.
func setSkillStatusTx(tx *sql.Tx, uid int64, it skillStateItem, status string, at time.Time) error {
	if it.status == status {
		return nil
	}
	if _, err := tx.Exec(`UPDATE library_items SET status=?,prev_status=CASE WHEN status_at>?-? AND prev_status<>'' THEN prev_status ELSE status END,status_at=? WHERE user_id=? AND id=?`, status, at.UnixMilli(), SkillGrace.Milliseconds(), at.UnixMilli(), uid, it.id); err != nil {
		return err
	}
	if status == SkillArchived {
		return addSkillRemovalTx(tx, uid, it, "archived", at.UnixMilli())
	}
	return dropSkillRemovalTx(tx, uid, it)
}

// dropSkillRemovalTx takes an item's name off the removal list. The list holds
// skill names only, so an item of another type with the same name leaves it.
func dropSkillRemovalTx(tx *sql.Tx, uid int64, it skillStateItem) error {
	if it.itemType != LibrarySkill {
		return nil
	}
	_, err := tx.Exec(`DELETE FROM skill_removals WHERE user_id=? AND name=?`, uid, it.name)
	return err
}

// shelfStageStatusTx moves a skill's live library item to the status its
// shelf stage now names, for the Skills page that still sets stages: In use or
// To test is Active, Parked is Archived, as the upgrade maps them. It goes
// through setSkillStatusTx so the removal list follows.
func shelfStageStatusTx(tx *sql.Tx, uid int64, skill string, at time.Time) error {
	var used, bookmarked bool
	var item string
	err := tx.QueryRow(`SELECT sh.used,sh.bookmarked,a.item_id FROM shelf sh JOIN library_aliases a ON a.user_id=sh.user_id AND a.kind='shelf' AND a.reference=sh.skill_id WHERE sh.user_id=? AND sh.skill_id=?`, uid, skill).Scan(&used, &bookmarked, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	it, err := skillStateItemTx(tx, uid, item)
	if err != nil || it.deletedAt != 0 {
		return err
	}
	status := SkillArchived
	if used || bookmarked {
		status = SkillActive
	}
	return setSkillStatusTx(tx, uid, it, status, at)
}

func addSkillRemovalTx(tx *sql.Tx, uid int64, it skillStateItem, reason string, at int64) error {
	if it.itemType != LibrarySkill || (reason == "archived" && it.revision == "") {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO skill_removals(user_id,name,reason,created_at) VALUES(?,?,?,?) ON CONFLICT(user_id,name) DO UPDATE SET name=excluded.name,reason=excluded.reason,created_at=excluded.created_at`, uid, it.name, reason, at)
	return err
}

// SetSkillStatusFor archives (status "archived") or restores (status
// "active") live library items.
func (s *Store) SetSkillStatusFor(uid int64, ids []string, status string, at time.Time) error {
	if !validLibraryStatus(status) {
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		for _, id := range ids {
			it, err := skillStateItemTx(tx, uid, id)
			if err == nil && it.deletedAt != 0 {
				err = ErrPrivateSkillMissing
			}
			if err != nil {
				return err
			}
			if err = setSkillStatusTx(tx, uid, it, status, at); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteSkillsFor marks items deleted and puts their names on the removal
// list. An item reet holds no copy of is deleted only when noCopyConfirmed,
// since its machines then keep only their backup.
func (s *Store) DeleteSkillsFor(uid int64, ids []string, noCopyConfirmed bool, at time.Time) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		for _, id := range ids {
			it, err := skillStateItemTx(tx, uid, id)
			if err == nil && it.deletedAt != 0 {
				err = ErrPrivateSkillMissing
			}
			if err != nil {
				return err
			}
			if it.revision == "" && !noCopyConfirmed {
				return ErrSkillNoCopy
			}
			if err = deleteSkillItemTx(tx, uid, it, at); err != nil {
				return err
			}
		}
		return nil
	})
}

// deleteSkillItemTx marks a live item deleted and puts its name on the
// removal list.
func deleteSkillItemTx(tx *sql.Tx, uid int64, it skillStateItem, at time.Time) error {
	if _, err := tx.Exec(`UPDATE library_items SET deleted_at=? WHERE user_id=? AND id=?`, at.UnixMilli(), uid, it.id); err != nil {
		return err
	}
	return addSkillRemovalTx(tx, uid, it, "deleted", at.UnixMilli())
}

// RemoveShelfSkillsFor is the old shelf Remove of shelf rows. Each row's live
// library skill is deleted as DeleteSkillsFor deletes it, so it leaves every
// machine and frees its name, and the row goes from the shelf; with ignore
// its ID also goes on the ignore list, which older machines' scans still
// read. A row whose skill reet holds no copy of is removed only when
// noCopyConfirmed: otherwise nothing changes, and noCopy names those skills
// with ErrSkillNoCopy. Rows that are not on the shelf are skipped; removed
// counts the rows that went.
//
// A skill the person wrote is not deleted: its shelf row is only the
// published page they saved, and their writing stays where it is written,
// as the old Remove always left it.
func (s *Store) RemoveShelfSkillsFor(uid int64, shelfIDs []string, ignore, noCopyConfirmed bool, at time.Time) (removed int, noCopy []string, err error) {
	err = s.withTx(func(tx *sql.Tx) error {
		type row struct {
			id, name string
			item     skillStateItem
			live     bool
		}
		var found []row
		seen := map[string]bool{}
		for _, id := range shelfIDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			r := row{id: id}
			e := tx.QueryRow(`SELECT k.name FROM shelf sh JOIN skills k ON k.id=sh.skill_id WHERE sh.user_id=? AND sh.skill_id=?`, uid, id).Scan(&r.name)
			if errors.Is(e, sql.ErrNoRows) {
				continue
			}
			if e != nil {
				return e
			}
			var item string
			e = tx.QueryRow(`SELECT i.id FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=? AND a.kind='shelf' AND a.reference=? AND i.item_type=? AND i.deleted_at=0
 AND i.origin<>'written' AND NOT EXISTS(SELECT 1 FROM library_aliases h WHERE h.user_id=i.user_id AND h.item_id=i.id AND h.kind='hosted')`, uid, id, LibrarySkill).Scan(&item)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if item != "" {
				if r.item, e = skillStateItemTx(tx, uid, item); e != nil {
					return e
				}
				r.live = true
				if r.item.revision == "" && !noCopyConfirmed && !slices.Contains(noCopy, r.item.name) {
					noCopy = append(noCopy, r.item.name)
				}
			}
			found = append(found, r)
		}
		if len(noCopy) > 0 && !noCopyConfirmed {
			return ErrSkillNoCopy
		}
		deleted := map[string]bool{}
		for _, r := range found {
			if r.live && len(deleted) == 0 {
				if e := privateSkillOwner(tx, uid); e != nil {
					return e
				}
			}
			if r.live && !deleted[r.item.id] {
				if e := deleteSkillItemTx(tx, uid, r.item, at); e != nil {
					return e
				}
				deleted[r.item.id] = true
			}
			if ignore {
				if _, e := tx.Exec(`INSERT INTO ignored(user_id,id,name,reason,at) VALUES(?,?,?,'removed',?) ON CONFLICT(user_id,id) DO UPDATE SET name=excluded.name,reason=excluded.reason,at=excluded.at`, uid, r.id, r.name, now()); e != nil {
					return e
				}
			}
			if e := s.removeShelfTx(tx, uid, r.id, false); e != nil {
				return e
			}
			removed++
		}
		return nil
	})
	if err != nil {
		removed = 0
	}
	return removed, noCopy, err
}

// undeleteShelfItemTx brings back the deleted skill a shelf row put back on
// the shelf stands for, when it is still within SkillUndoWindow and its name
// is free: the old pages' Undo of Remove puts the row back. Outside the
// window, or with the name taken, it does nothing.
func undeleteShelfItemTx(tx *sql.Tx, uid int64, shelfID string) error {
	var item string
	err := tx.QueryRow(`SELECT i.id FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=? AND a.kind='shelf' AND a.reference=? AND i.deleted_at>0`, uid, shelfID).Scan(&item)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	it, err := skillStateItemTx(tx, uid, item)
	if err != nil || it.deletedAt <= skillUndoNow()-SkillUndoWindow.Milliseconds() {
		return err
	}
	if err = undeleteSkillItemTx(tx, uid, it); errors.Is(err, ErrSkillNameTaken) {
		return nil
	}
	return err
}

// UndeleteSkillsFor brings deleted items back within SkillUndoWindow of their
// deletion; an item deleted longer ago is gone, whether or not purge has run
// yet. An Archived one goes back on the removal list as archived.
func (s *Store) UndeleteSkillsFor(uid int64, ids []string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		for _, id := range ids {
			it, err := skillStateItemTx(tx, uid, id)
			if err != nil {
				return err
			}
			if it.deletedAt == 0 {
				continue
			}
			if it.deletedAt <= skillUndoNow()-SkillUndoWindow.Milliseconds() {
				return ErrPrivateSkillMissing
			}
			if err = undeleteSkillItemTx(tx, uid, it); err != nil {
				return err
			}
		}
		return nil
	})
}

// undeleteSkillItemTx clears an item's deleted mark unless its name was taken
// in the meantime, and takes its removal row off the list (an Archived item
// goes back on it as archived).
func undeleteSkillItemTx(tx *sql.Tx, uid int64, it skillStateItem) error {
	taken, err := skillNameTakenTx(tx, uid, it.itemType, it.name, it.id)
	if err != nil {
		return err
	}
	if taken {
		return ErrSkillNameTaken
	}
	if _, err = tx.Exec(`UPDATE library_items SET deleted_at=0 WHERE user_id=? AND id=?`, uid, it.id); err != nil {
		return err
	}
	if err = dropSkillRemovalTx(tx, uid, it); err != nil {
		return err
	}
	if it.status == SkillArchived {
		return addSkillRemovalTx(tx, uid, it, "archived", now())
	}
	return nil
}

// SkillRemovalsFor lists the names machines are to back up and remove.
func (s *Store) SkillRemovalsFor(uid int64) ([]SkillRemoval, error) {
	return skillRemovalsOf(s.db, uid)
}

func skillRemovalsOf(q privateSkillQuery, uid int64) ([]SkillRemoval, error) {
	rows, err := q.Query(`SELECT name,reason,created_at FROM skill_removals WHERE user_id=? ORDER BY name`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillRemoval{}
	for rows.Next() {
		var r SkillRemoval
		if err = rows.Scan(&r.Name, &r.Reason, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PurgeDeletedSkillsFor removes what is left of items deleted more than
// SkillUndoWindow before now: their written skill, shelf rows, bundles and
// revisions, and the item itself. Their names stay on the removal list.
func (s *Store) PurgeDeletedSkillsFor(uid int64, now time.Time) error {
	return s.withTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id FROM library_items WHERE user_id=? AND deleted_at>0 AND deleted_at<=?`, uid, now.Add(-SkillUndoWindow).UnixMilli())
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil || len(ids) == 0 {
			return err
		}
		for _, id := range ids {
			if err = s.purgeSkillTx(tx, uid, id); err != nil {
				return err
			}
		}
		return dropUnusedBlobs(tx, uid)
	})
}

func (s *Store) purgeSkillTx(tx *sql.Tx, uid int64, id string) error {
	references := func(query string, args ...any) ([]string, error) {
		rows, err := tx.Query(query, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var ref string
			if err = rows.Scan(&ref); err != nil {
				return nil, err
			}
			out = append(out, ref)
		}
		return out, rows.Err()
	}
	slugs, err := references(`SELECT reference FROM library_aliases WHERE user_id=? AND item_id=? AND kind='hosted'`, uid, id)
	if err != nil {
		return err
	}
	shelf, err := references(`SELECT a.reference FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference WHERE a.user_id=? AND a.item_id=? AND a.kind='shelf'`, uid, id)
	if err != nil {
		return err
	}
	bundles, err := references(`SELECT b FROM (SELECT reference AS b FROM library_aliases WHERE user_id=? AND item_id=? AND kind='bundle' UNION SELECT bundle_id FROM library_items WHERE user_id=? AND id=? AND bundle_id<>'') WHERE NOT EXISTS(SELECT 1 FROM library_items o WHERE o.user_id=? AND o.bundle_id=b AND o.id<>?)`, uid, id, uid, id, uid, id)
	if err != nil {
		return err
	}
	for _, slug := range slugs {
		for _, table := range []string{"hosted_skills", "hosted_sync_sources", "library_hosted_editor_digests"} {
			if _, err = tx.Exec(`DELETE FROM `+table+` WHERE user_id=? AND slug=?`, uid, slug); err != nil {
				return err
			}
		}
	}
	for _, skill := range shelf {
		if err = s.removeShelfTx(tx, uid, skill, false); err != nil {
			return err
		}
	}
	for _, bundle := range bundles {
		if _, err = tx.Exec(`DELETE FROM sync_private_sources WHERE user_id=? AND bundle_id=?`, uid, bundle); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, bundle); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM library_items WHERE user_id=? AND id=?`, uid, id)
	return err
}

// SkillUpgradeNoticeFor returns what the upgrade archived, or nil when it
// archived nothing or the notice was dismissed.
func (s *Store) SkillUpgradeNoticeFor(uid int64) (*UpgradeNotice, error) {
	var n UpgradeNotice
	var dismissed int64
	err := s.db.QueryRow(`SELECT parked,removed,dismissed FROM skill_notices WHERE user_id=?`, uid).Scan(&n.Parked, &n.Removed, &dismissed)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && dismissed != 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Store) DismissSkillUpgradeNoticeFor(uid int64) error {
	return s.withTx(func(tx *sql.Tx) error { return dismissSkillUpgradeNoticeTx(tx, uid) })
}

func dismissSkillUpgradeNoticeTx(tx *sql.Tx, uid int64) error {
	_, err := tx.Exec(`UPDATE skill_notices SET dismissed=? WHERE user_id=? AND dismissed=0`, now(), uid)
	return err
}

// RestoreParkedSkillsFor makes the skills the upgrade archived from Parked
// Active again, and dismisses the notice about them.
func (s *Store) RestoreParkedSkillsFor(uid int64, at time.Time) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT id FROM library_items WHERE user_id=? AND upgraded_from='parked' AND status='archived' AND deleted_at=0`, uid)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			it, err := skillStateItemTx(tx, uid, id)
			if err != nil {
				return err
			}
			if err = setSkillStatusTx(tx, uid, it, SkillActive, at); err != nil {
				return err
			}
		}
		return dismissSkillUpgradeNoticeTx(tx, uid)
	})
}
