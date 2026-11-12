package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/eyupio/reet.sh/internal/contentobs"
)

const (
	LibrarySkill    = "skill"
	LibraryAgent    = "agent_definition"
	LibrarySettings = "portable_settings"
	LibraryInUse    = "in_use"
	LibraryToTest   = "to_test"
	LibraryOnShelf  = "on_shelf"
	LibraryDisabled = "disabled"
)

var ErrLibraryConflict = errors.New("library item changed")

// LibraryItem is account-owned identity and lifecycle, independent of a slug,
// publication, source URL, current revision or any destination selection.
// Metadata is the existing shelf projection; its likes/lists/tags are retained.
type LibraryItem struct {
	ID           string         `json:"id"`
	ItemType     string         `json:"itemType"`
	Name         string         `json:"name"`
	Status       string         `json:"status"`
	Version      int64          `json:"version"`
	BundleID     string         `json:"bundleId,omitempty"`
	RevisionID   string         `json:"revisionId,omitempty"`
	Digest       string         `json:"digest,omitempty"`
	Problem      string         `json:"problem,omitempty"`
	ContentState string         `json:"contentState"`
	Published    bool           `json:"published"`
	CreatedAt    int64          `json:"createdAt"`
	UpdatedAt    int64          `json:"updatedAt"`
	Metadata     *Skill         `json:"metadata,omitempty"`
	Aliases      []LibraryAlias `json:"aliases"`
}

type LibraryAlias struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

func validLibraryStatus(status string) bool {
	return status == LibraryInUse || status == LibraryToTest || status == LibraryOnShelf || status == LibraryDisabled
}

func (s *Store) migrateCanonicalLibrary() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS library_hosted_editor_digests(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,slug TEXT NOT NULL,digest TEXT NOT NULL,PRIMARY KEY(user_id,slug))`,
			`CREATE TABLE IF NOT EXISTS library_bases(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,enrollment_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,binding_id TEXT NOT NULL,item_id TEXT NOT NULL,revision_id TEXT NOT NULL,PRIMARY KEY(user_id,enrollment_id,binding_id),FOREIGN KEY(user_id,revision_id) REFERENCES private_skill_revisions(user_id,id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS library_connectors(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,enrollment_id TEXT NOT NULL REFERENCES sync_agents(id) ON DELETE CASCADE,protocol INTEGER NOT NULL,PRIMARY KEY(user_id,enrollment_id))`,
			`CREATE TABLE IF NOT EXISTS library_edit_events(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,event_key TEXT NOT NULL,item_id TEXT NOT NULL,enrollment_id TEXT NOT NULL,fingerprint TEXT NOT NULL,base TEXT NOT NULL,revision_id TEXT NOT NULL,saved_at INTEGER NOT NULL,uncertainty INTEGER NOT NULL,won INTEGER NOT NULL,order_at INTEGER NOT NULL,clock_fallback INTEGER NOT NULL,PRIMARY KEY(user_id,event_key),FOREIGN KEY(user_id,revision_id) REFERENCES private_skill_revisions(user_id,id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS canonical_library_selections(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,item_ids TEXT NOT NULL,version TEXT NOT NULL,PRIMARY KEY(user_id,target_id))`,
			`CREATE TABLE IF NOT EXISTS library_selection_accounts(user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS library_selections(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,target_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,selected TEXT NOT NULL,PRIMARY KEY(user_id,target_id))`,
			`CREATE TABLE IF NOT EXISTS library_items(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,id TEXT NOT NULL,item_type TEXT NOT NULL CHECK(item_type IN ('skill','agent_definition','portable_settings')),name TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('in_use','to_test','on_shelf','disabled')),version INTEGER NOT NULL DEFAULT 1,bundle_id TEXT NOT NULL DEFAULT '',revision_id TEXT NOT NULL DEFAULT '',digest TEXT NOT NULL DEFAULT '',problem TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,deleted_at INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(user_id,id))`,
			`CREATE TABLE IF NOT EXISTS library_aliases(user_id INTEGER NOT NULL,kind TEXT NOT NULL,reference TEXT NOT NULL,item_id TEXT NOT NULL,PRIMARY KEY(user_id,kind,reference),FOREIGN KEY(user_id,item_id) REFERENCES library_items(user_id,id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED)`,
			`CREATE TRIGGER IF NOT EXISTS library_intent_fence AFTER UPDATE OF status,revision_id,name,deleted_at ON library_items WHEN OLD.status<>NEW.status OR OLD.revision_id<>NEW.revision_id OR OLD.name<>NEW.name OR OLD.deleted_at<>NEW.deleted_at BEGIN
            UPDATE sync_targets SET revision=lower(hex(randomblob(16))) WHERE user_id=NEW.user_id AND target_id IN (SELECT c.target_id FROM canonical_library_selections c WHERE c.user_id=NEW.user_id AND EXISTS(SELECT 1 FROM json_each(c.item_ids) WHERE value=NEW.id));
            UPDATE sync_jobs SET status='cancelled',message='Library content or status changed',updated_at=CAST(unixepoch('subsec')*1000 AS INTEGER) WHERE user_id=NEW.user_id AND status IN ('queued','awaiting-approval') AND target_id IN (SELECT c.target_id FROM canonical_library_selections c WHERE c.user_id=NEW.user_id AND EXISTS(SELECT 1 FROM json_each(c.item_ids) WHERE value=NEW.id));
            END`,
			`CREATE INDEX IF NOT EXISTS library_alias_item ON library_aliases(user_id,item_id)`,
			`CREATE INDEX IF NOT EXISTS library_shelf_projection ON library_aliases(user_id,kind,reference,item_id)`,
			`CREATE INDEX IF NOT EXISTS library_item_projection ON library_items(user_id,id,status)`,
			`CREATE TABLE IF NOT EXISTS library_projection_guard(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,item_id TEXT NOT NULL,PRIMARY KEY(user_id,item_id))`,
			`CREATE TABLE IF NOT EXISTS library_migrations(name TEXT PRIMARY KEY,completed_at INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS library_legacy_stages(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,skill_id TEXT NOT NULL,used INTEGER NOT NULL,bookmarked INTEGER NOT NULL,liked INTEGER NOT NULL,notes TEXT NOT NULL,PRIMARY KEY(user_id,skill_id))`,
			`CREATE TABLE IF NOT EXISTS library_locations(user_id INTEGER NOT NULL,enrollment_id TEXT NOT NULL,id TEXT NOT NULL,application TEXT NOT NULL,project TEXT NOT NULL,path TEXT NOT NULL,seen_at INTEGER NOT NULL,legacy_agent TEXT NOT NULL DEFAULT '',PRIMARY KEY(user_id,enrollment_id,id),FOREIGN KEY(enrollment_id) REFERENCES sync_agents(id) ON DELETE CASCADE)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		// Additive backfill: no catalog, metadata, selection, receipt or content
		// row is deleted. Retain the original stage evidence before projections.
		var migrated int
		if err := tx.QueryRow(`SELECT count(*) FROM library_migrations WHERE name='identity-v1'`).Scan(&migrated); err != nil {
			return err
		}
		if migrated == 0 {
			if _, err := tx.Exec(`INSERT INTO library_legacy_stages SELECT user_id,skill_id,used,bookmarked,liked,notes FROM shelf`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO library_aliases SELECT user_id,'hosted',slug,lower(hex(randomblob(16))) FROM hosted_skills`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,created_at,updated_at)
			 SELECT h.user_id,a.item_id,'skill',h.slug,'to_test',h.created_at,h.updated_at FROM hosted_skills h JOIN library_aliases a ON a.user_id=h.user_id AND a.kind='hosted' AND a.reference=h.slug`); err != nil {
				return err
			}
			if _, err := tx.Exec(shelfAliasBackfill); err != nil {
				return err
			}
			if _, err := tx.Exec(shelfItemBackfill); err != nil {
				return err
			}
			// A pre-existing explicit shelf stage wins over the authored default.
			if _, err := tx.Exec(`UPDATE library_items SET status=COALESCE((SELECT CASE WHEN sh.used THEN 'in_use' WHEN sh.bookmarked THEN 'to_test' ELSE 'on_shelf' END FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference WHERE a.user_id=library_items.user_id AND a.item_id=library_items.id AND a.kind='shelf' ORDER BY sh.used DESC,sh.bookmarked DESC LIMIT 1),status)`); err != nil {
				return err
			}
			if err := backfillLibraryBundles(tx); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE library_items SET status=COALESCE((SELECT CASE WHEN sh.used THEN 'in_use' WHEN sh.bookmarked THEN 'to_test' ELSE 'on_shelf' END FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference WHERE a.user_id=library_items.user_id AND a.item_id=library_items.id AND a.kind='shelf' ORDER BY sh.used DESC,sh.bookmarked DESC LIMIT 1),status)`); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO library_migrations VALUES('identity-v1',?)`, now()); err != nil {
				return err
			}
		}
		for _, stmt := range canonicalLibraryTriggers {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}

// source_type=hosted is the persisted controller-hosted provenance. Match the
// author's namespace as a complete component; a coincident name is insufficient.
const ownHostedLibraryID = `(SELECT a.item_id FROM skills k JOIN publishers p ON p.user_id=NEW.user_id JOIN library_aliases a ON a.user_id=p.user_id AND a.kind='hosted' AND a.reference=k.slug WHERE k.id=NEW.skill_id AND k.source_type='hosted' AND k.id GLOB 'hosted/*/'||p.username||'/'||k.slug)`

const shelfAliasBackfill = `INSERT INTO library_aliases(user_id,kind,reference,item_id)
 SELECT sh.user_id,'shelf',sh.skill_id,COALESCE((SELECT a.item_id FROM skills k JOIN publishers p ON p.user_id=sh.user_id JOIN library_aliases a ON a.user_id=p.user_id AND a.kind='hosted' AND a.reference=k.slug WHERE k.id=sh.skill_id AND k.source_type='hosted' AND k.id GLOB 'hosted/*/'||p.username||'/'||k.slug),lower(hex(randomblob(16)))) FROM shelf sh`
const shelfItemBackfill = `INSERT OR IGNORE INTO library_items(user_id,id,item_type,name,status,created_at,updated_at)
 SELECT sh.user_id,a.item_id,'skill',k.slug,CASE WHEN sh.used THEN 'in_use' WHEN sh.bookmarked THEN 'to_test' ELSE 'on_shelf' END,sh.created_at,sh.updated_at FROM shelf sh JOIN skills k ON k.id=sh.skill_id JOIN library_aliases a ON a.user_id=sh.user_id AND a.kind='shelf' AND a.reference=sh.skill_id`

const canonicalShelfStatus = `COALESCE((SELECT i.status FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=sh.user_id AND a.kind='shelf' AND a.reference=sh.skill_id),CASE WHEN sh.used THEN 'in_use' WHEN sh.bookmarked THEN 'to_test' ELSE 'on_shelf' END)`

// Bare legacy import copies have not been adopted: retaining them forever would
// turn old transfer attempts into quota-consuming library entries. Only a
// trusted binding/explicit upload, or user/fork origin, claims their content.
const claimedLibraryContent = `(EXISTS(SELECT 1 FROM library_aliases a WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind IN ('hosted','shelf','machine','machine_capture','user_upload')) OR EXISTS(SELECT 1 FROM private_skill_revisions r WHERE r.user_id=i.user_id AND r.id=i.revision_id AND json_extract(r.origin,'$.kind')<>'import'))`

var canonicalLibraryTriggers = []string{
	`CREATE TRIGGER IF NOT EXISTS library_shelf_insert AFTER INSERT ON shelf BEGIN
	 INSERT OR IGNORE INTO library_aliases VALUES(NEW.user_id,'shelf',NEW.skill_id,COALESCE(` + ownHostedLibraryID + `,lower(hex(randomblob(16)))));
	 INSERT OR IGNORE INTO library_items(user_id,id,item_type,name,status,created_at,updated_at) SELECT NEW.user_id,a.item_id,'skill',k.slug,CASE WHEN NEW.used THEN 'in_use' WHEN NEW.bookmarked THEN 'to_test' ELSE 'on_shelf' END,NEW.created_at,NEW.updated_at FROM library_aliases a JOIN skills k ON k.id=NEW.skill_id WHERE a.user_id=NEW.user_id AND a.kind='shelf' AND a.reference=NEW.skill_id;
	 UPDATE library_items SET status='in_use' WHERE NEW.used=1 AND user_id=NEW.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id);
	 INSERT OR IGNORE INTO library_projection_guard SELECT NEW.user_id,item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id;
	 UPDATE shelf SET used=(SELECT status='in_use' FROM library_items WHERE user_id=NEW.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id)),bookmarked=CASE (SELECT status FROM library_items WHERE user_id=NEW.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id)) WHEN 'to_test' THEN 1 WHEN 'on_shelf' THEN 0 ELSE bookmarked END WHERE user_id=NEW.user_id AND skill_id=NEW.skill_id;
	 DELETE FROM library_projection_guard WHERE user_id=NEW.user_id AND item_id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id);
	 END`,
	`CREATE TRIGGER IF NOT EXISTS library_shelf_stage AFTER UPDATE OF used,bookmarked ON shelf
	 WHEN (OLD.used<>NEW.used OR OLD.bookmarked<>NEW.bookmarked) AND NOT EXISTS(SELECT 1 FROM library_projection_guard g JOIN library_aliases a ON a.user_id=g.user_id AND a.item_id=g.item_id WHERE a.user_id=NEW.user_id AND a.kind='shelf' AND a.reference=NEW.skill_id)
	 BEGIN UPDATE library_items SET status=CASE WHEN NEW.used THEN 'in_use' WHEN NEW.bookmarked THEN 'to_test' ELSE 'on_shelf' END WHERE user_id=NEW.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='shelf' AND reference=NEW.skill_id); END`,
	`CREATE TRIGGER IF NOT EXISTS library_status_projection AFTER UPDATE OF status ON library_items WHEN OLD.status<>NEW.status BEGIN
	 INSERT INTO library_projection_guard VALUES(NEW.user_id,NEW.id);
	 UPDATE shelf SET used=(NEW.status='in_use'),bookmarked=CASE WHEN NEW.status='to_test' THEN 1 WHEN NEW.status='on_shelf' THEN 0 ELSE bookmarked END WHERE user_id=NEW.user_id AND skill_id IN (SELECT reference FROM library_aliases WHERE user_id=NEW.user_id AND item_id=NEW.id AND kind='shelf');
	 UPDATE library_items SET version=version+1,updated_at=CAST(unixepoch('subsec')*1000 AS INTEGER) WHERE user_id=NEW.user_id AND id=NEW.id;
	 DELETE FROM library_projection_guard WHERE user_id=NEW.user_id AND item_id=NEW.id;
	 END`,
	`CREATE TRIGGER IF NOT EXISTS library_hosted_insert AFTER INSERT ON hosted_skills BEGIN
	 INSERT OR IGNORE INTO library_aliases VALUES(NEW.user_id,'hosted',NEW.slug,lower(hex(randomblob(16))));
	 INSERT OR IGNORE INTO library_items(user_id,id,item_type,name,status,created_at,updated_at) SELECT NEW.user_id,item_id,'skill',NEW.slug,'to_test',NEW.created_at,NEW.updated_at FROM library_aliases WHERE user_id=NEW.user_id AND kind='hosted' AND reference=NEW.slug;
	 END`,
	`CREATE TRIGGER IF NOT EXISTS library_hosted_update AFTER UPDATE ON hosted_skills BEGIN UPDATE library_items SET name=NEW.slug,updated_at=NEW.updated_at WHERE user_id=NEW.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=NEW.user_id AND kind='hosted' AND reference=NEW.slug); END`,
	`CREATE TRIGGER IF NOT EXISTS library_hosted_delete AFTER DELETE ON hosted_skills BEGIN
	 UPDATE library_items SET deleted_at=CAST(unixepoch('subsec')*1000 AS INTEGER) WHERE user_id=OLD.user_id AND id=(SELECT item_id FROM library_aliases WHERE user_id=OLD.user_id AND kind='hosted' AND reference=OLD.slug);
	 UPDATE library_aliases SET kind='deleted_hosted',reference=reference||':'||lower(hex(randomblob(16))) WHERE user_id=OLD.user_id AND kind='hosted' AND reference=OLD.slug;
	 END`,
	`CREATE TRIGGER IF NOT EXISTS library_bundle_delete AFTER DELETE ON private_skill_bundles BEGIN UPDATE library_items SET bundle_id='',revision_id='',digest='',problem='Retained content was reclaimed under the private storage policy.' WHERE user_id=OLD.user_id AND bundle_id=OLD.id; END`,
}

func libraryAliasID(q privateSkillQuery, uid int64, kind, reference string) (string, error) {
	var id string
	err := q.QueryRow(`SELECT item_id FROM library_aliases WHERE user_id=? AND kind=? AND reference=?`, uid, kind, reference).Scan(&id)
	return id, err
}

func ensureLibraryAlias(tx *sql.Tx, uid int64, kind, reference, itemType, name, status string) (string, error) {
	id, err := libraryAliasID(tx, uid, kind, reference)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id, err = privateSkillRandomID()
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(`INSERT INTO library_items(user_id,id,item_type,name,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, uid, id, itemType, name, status, now(), now()); err != nil {
		return "", err
	}
	_, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,?,?,?)`, uid, kind, reference, id)
	return id, err
}

// Merge only an explicitly proven binding, never a name/digest heuristic. The
// destination identity/status survives and all immutable bundles stay readable.
func attachLibraryBundle(tx *sql.Tx, uid int64, id string, result PrivateSkillResult) error {
	if result.Revision.ID != "" {
		var head string
		if err := tx.QueryRow(`SELECT head FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, result.Bundle.ID).Scan(&head); err != nil {
			return err
		}
		if head != result.Revision.ID {
			return nil
		}
	}
	other, err := libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if other != "" && other != id {
		if _, err = tx.Exec(`UPDATE library_aliases SET item_id=? WHERE user_id=? AND item_id=?`, id, uid, other); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM library_items WHERE user_id=? AND id=?`, uid, other); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,'bundle',?,?) ON CONFLICT(user_id,kind,reference) DO UPDATE SET item_id=excluded.item_id`, uid, result.Bundle.ID, id); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE library_items SET bundle_id=?,revision_id=?,digest=?,problem='',updated_at=? WHERE user_id=? AND id=?`, result.Bundle.ID, result.Revision.ID, result.Revision.ManifestDigest, now(), uid, id)
	return err
}

func recordLibraryPrivateRevision(tx *sql.Tx, uid int64, result PrivateSkillResult) error {
	id, err := ensureLibraryAlias(tx, uid, "bundle", result.Bundle.ID, result.Bundle.ItemType, result.Bundle.Name, LibraryToTest)
	if err != nil {
		return err
	}
	return attachLibraryBundle(tx, uid, id, result)
}

func recordLibraryMachineCapture(tx *sql.Tx, uid int64, enrollment, agent, project, name string, result PrivateSkillResult) error {
	reference := enrollment + ":" + agent + ":" + project + ":" + name
	id, err := libraryAliasID(tx, uid, "machine_capture", reference)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
		if err == nil {
			_, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,'machine_capture',?,?)`, uid, reference, id)
		}
	}
	if err != nil {
		return err
	}
	return attachLibraryBundle(tx, uid, id, result)
}

// Explicit user API uploads claim their bundles even when their optional origin
// says "import". Internal legacy transfer copies retain their old retention.
func (s *Store) SaveLibraryContentFor(uid int64, id string, in PrivateSkillWrite) (PrivateSkillResult, error) {
	return s.savePrivateSkillRevisionWithReceipt(uid, id, in, nil, func(tx *sql.Tx, result PrivateSkillResult) error {
		item, err := libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,'user_upload',?,?) ON CONFLICT(user_id,kind,reference) DO NOTHING`, uid, result.Bundle.ID, item)
		return err
	})
}

func backfillLibraryBundles(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT b.user_id,b.id,b.name,b.head,b.item_type,COALESCE(r.manifest_digest,''),COALESCE(h.slug,''),COALESCE(s.skill_id,''),COALESCE(r.origin,'{}') FROM private_skill_bundles b LEFT JOIN private_skill_revisions r ON r.user_id=b.user_id AND r.bundle_id=b.id AND r.id=b.head LEFT JOIN hosted_sync_sources h ON h.user_id=b.user_id AND h.bundle_id=b.id LEFT JOIN sync_private_sources s ON s.user_id=b.user_id AND s.bundle_id=b.id ORDER BY b.created_at,b.id`)
	if err != nil {
		return err
	}
	type entry struct {
		uid                                                     int64
		bundle, name, head, kind, digest, hosted, shelf, origin string
	}
	var entries []entry
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.uid, &e.bundle, &e.name, &e.head, &e.kind, &e.digest, &e.hosted, &e.shelf, &e.origin); err != nil {
			break
		}
		entries = append(entries, e)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		id := ""
		if e.hosted != "" {
			id, err = libraryAliasID(tx, e.uid, "hosted", e.hosted)
		} else if e.shelf != "" {
			id, err = libraryAliasID(tx, e.uid, "shelf", e.shelf)
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if id == "" {
			id, err = ensureLibraryAlias(tx, e.uid, "bundle", e.bundle, e.kind, e.name, LibraryToTest)
			if err != nil {
				return err
			}
		}
		result := PrivateSkillResult{Bundle: PrivateSkillBundle{ID: e.bundle}, Revision: PrivateSkillRevision{ID: e.head, ManifestDigest: e.digest}}
		if err = attachLibraryBundle(tx, e.uid, id, result); err != nil {
			return err
		}
		var origin PrivateSkillOrigin
		var source struct{ Enrollment, Agent, Project, Name string }
		if json.Unmarshal([]byte(e.origin), &origin) != nil || origin.Kind != "import" || json.Unmarshal([]byte(origin.Reference), &source) != nil || source.Name != e.name || source.Enrollment == "" || !contentobs.ValidScope(source.Agent, source.Project) {
			continue
		}
		var owned, receipts int
		if err = tx.QueryRow(`SELECT count(*) FROM sync_agents WHERE user_id=? AND id=?`, e.uid, source.Enrollment).Scan(&owned); err != nil {
			return err
		}
		if owned != 1 {
			continue
		}
		if err = tx.QueryRow(`SELECT count(*) FROM machine_import_requests WHERE user_id=? AND agent_id=? AND json_extract(data,'$.receipt.bundle.id')=? AND json_extract(data,'$.receipt.revision.id')=?`, e.uid, source.Enrollment, e.bundle, e.head).Scan(&receipts); err != nil {
			return err
		}
		if e.shelf != "" || receipts > 0 {
			if err = recordLibraryMachineCapture(tx, e.uid, source.Enrollment, source.Agent, source.Project, source.Name, result); err != nil {
				return err
			}
		}
	}
	return nil
}

func syncLibraryShelfStatus(tx *sql.Tx, uid int64, skill string) error {
	_, err := tx.Exec(`UPDATE library_items SET status=(SELECT CASE WHEN used THEN 'in_use' WHEN bookmarked THEN 'to_test' ELSE 'on_shelf' END FROM shelf WHERE user_id=? AND skill_id=?) WHERE user_id=? AND id=(SELECT item_id FROM library_aliases WHERE user_id=? AND kind='shelf' AND reference=?) AND EXISTS(SELECT 1 FROM shelf WHERE user_id=? AND skill_id=?)`, uid, skill, uid, uid, skill, uid, skill)
	return err
}

func (s *Store) SetLibraryStatusFor(uid int64, id, status string, version int64) error {
	if !validLibraryStatus(status) || version < 1 {
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE library_items SET status=? WHERE user_id=? AND id=? AND version=? AND deleted_at=0`, status, uid, id, version)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err == nil && n == 0 {
			err = ErrLibraryConflict
		}
		return err
	})
}

func (s *Store) LibraryItemFor(uid int64, id string) (LibraryItem, error) {
	var item LibraryItem
	err := s.db.QueryRow(`SELECT id,item_type,name,status,version,bundle_id,revision_id,digest,problem,created_at,updated_at FROM library_items WHERE user_id=? AND id=? AND deleted_at=0 AND `+privateSkillAccount, uid, id, uid).Scan(&item.ID, &item.ItemType, &item.Name, &item.Status, &item.Version, &item.BundleID, &item.RevisionID, &item.Digest, &item.Problem, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return item, ErrPrivateSkillMissing
	}
	if err != nil {
		return item, err
	}
	item.ContentState = "pending_capture"
	if item.RevisionID != "" {
		item.ContentState = "complete"
	}
	if item.Problem != "" {
		item.ContentState = "unavailable"
		if item.RevisionID != "" {
			item.ContentState = "previous_revision"
		}
	}
	item.Aliases = []LibraryAlias{}
	rows, err := s.db.Query(`SELECT kind,reference FROM library_aliases WHERE user_id=? AND item_id=? ORDER BY kind,reference`, uid, id)
	if err != nil {
		return item, err
	}
	for rows.Next() {
		var alias LibraryAlias
		if err = rows.Scan(&alias.Kind, &alias.Reference); err != nil {
			break
		}
		item.Aliases = append(item.Aliases, alias)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return item, err
	}
	for _, alias := range item.Aliases {
		if alias.Kind == "shelf" && item.Metadata == nil {
			sk, ok, e := s.GetFor(uid, alias.Reference)
			if e != nil {
				return item, e
			}
			if ok {
				item.Metadata = &sk
			}
		}
		if alias.Kind == "hosted" {
			if e := s.db.QueryRow(`SELECT published FROM hosted_skills WHERE user_id=? AND slug=?`, uid, alias.Reference).Scan(&item.Published); e != nil && !errors.Is(e, sql.ErrNoRows) {
				return item, e
			}
		}
	}
	return item, nil
}

// LibraryFor deliberately preserves distinct same-name items. Legacy sync
// candidate selection remains separate until the Stage 3 scheduler cutover.
func (s *Store) LibraryFor(uid int64) ([]LibraryItem, error) {
	var account int
	if err := s.db.QueryRow(`SELECT count(*) FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer'`, uid).Scan(&account); err != nil {
		return nil, err
	}
	if account != 1 {
		return nil, ErrPrivateSkillMissing
	}
	if err := s.materializeLibraryHosted(uid); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT i.id FROM library_items i WHERE i.user_id=? AND i.deleted_at=0 AND `+privateSkillAccount+` AND (EXISTS(SELECT 1 FROM library_aliases a JOIN shelf sh ON sh.user_id=a.user_id AND sh.skill_id=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf') OR EXISTS(SELECT 1 FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=i.user_id AND a.item_id=i.id AND a.kind='hosted') OR (i.revision_id<>'' AND `+claimedLibraryContent+`)) ORDER BY i.id`, uid, uid)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	items := []LibraryItem{}
	for _, id := range ids {
		item, e := s.LibraryItemFor(uid, id)
		if e != nil {
			return nil, e
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

// LibraryViewFor is LibraryFor as a person reads it: one canonical copy per
// name. Selection and scheduler code must keep using LibraryFor, which returns
// every item, so a saved selection is never dropped merely because a linked
// copy of the same name exists elsewhere in the library.
func (s *Store) LibraryViewFor(uid int64) ([]LibraryItem, error) {
	items, err := s.LibraryFor(uid)
	if err != nil {
		return nil, err
	}
	return oneCanonicalCopyPerName(items), nil
}

// linkedLibraryItem reports whether an item has a known source: an authored or
// hosted alias, or a shelf skill linked to an install source. An unlinked
// machine capture is only an observation of a skill, not a source of its own.
// Source alone also counts, matching the sync library, so rows saved before
// source types were recorded stay linked.
func linkedLibraryItem(item LibraryItem) bool {
	for _, alias := range item.Aliases {
		if alias.Kind == "hosted" {
			return true
		}
	}
	return item.Metadata != nil && item.Metadata.Source != "" && item.Metadata.SourceType != "local"
}

// oneCanonicalCopyPerName drops unlinked machine captures when the account
// already holds a linked library item of the same name. The linked item is the
// single source: automatic sync maintains it, and manually importing or linking
// an unlinked copy is how that copy becomes a source of its own. Distinct
// same-name items that each have a known source are left alone, because the
// sync library settles which copy one machine receives.
func oneCanonicalCopyPerName(items []LibraryItem) []LibraryItem {
	linked := map[string]bool{}
	for _, item := range items {
		if linkedLibraryItem(item) {
			linked[item.Name] = true
		}
	}
	if len(linked) == 0 {
		return items
	}
	out := items[:0]
	for _, item := range items {
		if linked[item.Name] && !linkedLibraryItem(item) {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (s *Store) materializeLibraryHosted(uid int64) error {
	hosted, err := s.HostedSkillsFor(uid)
	if err != nil {
		return err
	}
	for _, skill := range hosted {
		if _, e := s.hostedPrivateSource(uid, skill); e != nil {
			if _, err = s.db.Exec(`UPDATE library_items SET problem=? WHERE user_id=? AND id=(SELECT item_id FROM library_aliases WHERE user_id=? AND kind='hosted' AND reference=?)`, e.Error(), uid, uid, skill.Slug); err != nil {
				return err
			}
		}
	}
	return nil
}
