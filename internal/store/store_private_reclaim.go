package store

import (
	"database/sql"
	"regexp"
)

// privateSkillHistoryKept is how many superseded revisions of each skill survive
// the first reclaim pass, so the last few edits stay recoverable while there is
// room. The second pass keeps only what something still uses.
const privateSkillHistoryKept = 3

// A revision ID is 32 lowercase hex characters. Saved plans and job plans carry
// them inside JSON, in "private:<bundle>:<revision>:<digest>" sources.
var revisionToken = regexp.MustCompile(`[a-f0-9]{32}`)

// livePrivateRevisions names every revision of one account that something still
// depends on, and so must never be freed:
//
//   - a bundle's head, which is what the skill currently is;
//   - a revision bound as a written or captured skill's sync source;
//   - a revision pinned to a machine, or recorded in an install receipt. Both are
//     foreign keys that cascade, so deleting the revision would silently delete
//     the pin or receipt too;
//   - a revision named by a saved sync plan, or by the plan of a job that has not
//     finished. The agent downloads exactly that revision by ID.
//
// TestPrivateStorageKeepsEveryRevisionSomethingUses checks this list against the
// schema, so a table added later cannot be forgotten.
func livePrivateRevisions(tx *sql.Tx, uid int64, heads bool) (map[string]bool, error) {
	live := map[string]bool{}
	ids := func(query string) error {
		rows, err := tx.Query(query, uid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				return err
			}
			live[id] = true
		}
		return rows.Err()
	}
	mentions := func(query string) error {
		rows, err := tx.Query(query, uid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var text string
			if err = rows.Scan(&text); err != nil {
				return err
			}
			for _, id := range revisionToken.FindAllString(text, -1) {
				live[id] = true
			}
		}
		return rows.Err()
	}
	// A bundle that is itself being freed does not keep its own head alive, so
	// deciding that is the one caller that leaves heads out.
	if heads {
		if err := ids(`SELECT head FROM private_skill_bundles WHERE user_id=? AND head<>''`); err != nil {
			return nil, err
		}
	}
	for _, query := range []string{
		`SELECT i.revision_id FROM library_items i WHERE i.user_id=? AND i.deleted_at=0 AND i.revision_id<>'' AND ` + claimedLibraryContent,
		`SELECT revision_id FROM hosted_sync_sources WHERE user_id=?`,
		`SELECT revision_id FROM library_edit_events WHERE user_id=?`,
		`SELECT base FROM library_edit_events WHERE user_id=?`,
		`SELECT revision_id FROM library_bases WHERE user_id=?`,
		`SELECT revision_id FROM sync_private_sources WHERE user_id=?`,
		`SELECT revision_id FROM skill_content_targets WHERE user_id=?`,
		`SELECT revision_id FROM skill_content_receipts WHERE user_id=?`,
	} {
		if err := ids(query); err != nil {
			return nil, err
		}
	}
	for _, query := range []string{
		`SELECT desired FROM sync_targets WHERE user_id=?`,
		`SELECT o.baseline FROM sync_options o JOIN sync_targets t ON t.target_id=o.target_id WHERE t.user_id=?`,
		`SELECT plan FROM sync_jobs WHERE user_id=? AND status IN ('queued','running','awaiting-approval')`,
	} {
		if err := mentions(query); err != nil {
			return nil, err
		}
	}
	return live, nil
}

// reclaimPrivateSkillStorage frees what nothing needs: superseded revisions
// beyond the newest `keep` of each skill, then every blob no revision still
// refers to. extra names revisions of a write that is still in progress.
//
// Storage used to be append-only, so ordinary editing filled an account for good
// and no skill could be saved after that.
func reclaimPrivateSkillStorage(tx *sql.Tx, uid int64, keep int, extra ...string) error {
	live, err := livePrivateRevisions(tx, uid, true)
	if err != nil {
		return err
	}
	for _, id := range extra {
		live[id] = true
	}
	rows, err := tx.Query(`SELECT bundle_id,id FROM private_skill_revisions WHERE user_id=? ORDER BY bundle_id,created_at DESC,rowid DESC`, uid)
	if err != nil {
		return err
	}
	type revision struct{ bundle, id string }
	var superseded []revision
	kept := map[string]int{}
	for rows.Next() {
		var r revision
		if err = rows.Scan(&r.bundle, &r.id); err != nil {
			rows.Close()
			return err
		}
		if live[r.id] {
			continue
		}
		if kept[r.bundle] < keep {
			kept[r.bundle]++
			continue
		}
		superseded = append(superseded, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, r := range superseded {
		if _, err = tx.Exec(`DELETE FROM private_skill_revisions WHERE user_id=? AND bundle_id=? AND id=?`, uid, r.bundle, r.id); err != nil {
			return err
		}
	}
	return dropUnusedBlobs(tx, uid)
}

// dropUnusedBlobs deletes every blob no revision refers to. Before this existed a
// blob outlived its revisions for good, so deleted history still counted.
func dropUnusedBlobs(tx *sql.Tx, uid int64) error {
	_, err := tx.Exec(`DELETE FROM private_skill_blobs WHERE user_id=? AND NOT EXISTS (SELECT 1 FROM private_skill_revision_files f WHERE f.user_id=private_skill_blobs.user_id AND f.digest=private_skill_blobs.digest)`, uid)
	return err
}

// deadHostedGraceMillis keeps a bundle that was only just made out of reach of
// freeDeadHostedBundles. hostedPrivateSource binds a new bundle to its skill in a
// statement of its own after the write commits, so for a moment the bundle looks
// unused.
const deadHostedGraceMillis = 10 * 60 * 1000

// freeDeadHostedBundles frees whole bundles that only ever held bytes copied from
// a skill and that nothing uses any more. Deleting or renaming a written skill
// drops its binding but leaves its bundle behind (a bundle's name is immutable, so
// a rename starts a fresh one), and a machine capture that started a new bundle
// instead of reusing the skill's own leaves the older one behind. Nothing can
// delete a bundle, so each kept a slot against the bound for good, until an
// account that renamed, deleted or re-captured enough skills could not sync a new
// one.
//
// These bundles are copies of a skill's bytes, not something a person saved, so
// they are the one kind of bundle that is freed, and only when a slot is needed:
// oldest first, and no more than `need`, so as much of a deleted skill's last
// bytes as possible survives. A bundle is left alone when it is bound to a
// written skill or a captured machine skill, has any revision that is pinned,
// received, planned or in a job, was not made by copying a skill, or is recent.
func freeDeadHostedBundles(tx *sql.Tx, uid int64, need int) (int, error) {
	if need <= 0 {
		return 0, nil
	}
	live, err := livePrivateRevisions(tx, uid, false)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(`SELECT b.id FROM private_skill_bundles b
 WHERE b.user_id=?
 AND NOT EXISTS (SELECT 1 FROM hosted_sync_sources h WHERE h.user_id=b.user_id AND h.bundle_id=b.id)
 AND NOT EXISTS (SELECT 1 FROM sync_private_sources s WHERE s.user_id=b.user_id AND s.bundle_id=b.id)
 AND EXISTS (SELECT 1 FROM private_skill_revisions r WHERE r.user_id=b.user_id AND r.bundle_id=b.id)
 AND NOT EXISTS (SELECT 1 FROM private_skill_revisions r WHERE r.user_id=b.user_id AND r.bundle_id=b.id AND NOT (coalesce(json_extract(r.origin,'$.reference'),'')='hosted:'||b.name OR json_extract(r.origin,'$.kind')='import'))
 AND (SELECT max(r.created_at) FROM private_skill_revisions r WHERE r.user_id=b.user_id AND r.bundle_id=b.id)<?
 ORDER BY (SELECT max(r.created_at) FROM private_skill_revisions r WHERE r.user_id=b.user_id AND r.bundle_id=b.id),b.created_at,b.id`, uid, now()-deadHostedGraceMillis)
	if err != nil {
		return 0, err
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, id)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	freed := 0
	for _, bundle := range candidates {
		if freed >= need {
			break
		}
		revisions, err := tx.Query(`SELECT id FROM private_skill_revisions WHERE user_id=? AND bundle_id=?`, uid, bundle)
		if err != nil {
			return freed, err
		}
		inUse := false
		for revisions.Next() {
			var id string
			if err = revisions.Scan(&id); err != nil {
				revisions.Close()
				return freed, err
			}
			inUse = inUse || live[id]
		}
		if err = revisions.Err(); err != nil {
			revisions.Close()
			return freed, err
		}
		revisions.Close()
		if inUse {
			continue
		}
		if _, err = tx.Exec(`DELETE FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, bundle); err != nil {
			return freed, err
		}
		freed++
	}
	if freed == 0 {
		return 0, nil
	}
	return freed, dropUnusedBlobs(tx, uid)
}

// makeRoomInPrivateStorage reclaims, gently and then thoroughly, until full
// reports that there is room. It returns true when the account is still full, so
// the bound only applies to what is genuinely in use.
func makeRoomInPrivateStorage(tx *sql.Tx, uid int64, full func() (bool, error), extra ...string) (bool, error) {
	for _, keep := range []int{privateSkillHistoryKept, 0} {
		if err := reclaimPrivateSkillStorage(tx, uid, keep, extra...); err != nil {
			return false, err
		}
		stillFull, err := full()
		if err != nil || !stillFull {
			return stillFull, err
		}
	}
	return true, nil
}
