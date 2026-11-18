package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/ref"
	"github.com/eyupio/reet.sh/internal/skillbundle"
)

var (
	// ErrSkillCapturePath is a capture asked for a path outside the managed
	// skill folders.
	ErrSkillCapturePath = errors.New("a skill can only be copied from a managed skills folder")
	// ErrSkillNoCaptureSource is a capture of a skill whose folder on that
	// machine reet has not seen.
	ErrSkillNoCaptureSource = errors.New("reet has not seen where that machine keeps this skill")
)

// SkillNoSourceProblem is why an Active skill with no content is not being
// copied from anywhere.
const SkillNoSourceProblem = "No machine has a copy and there is no link to get it from"

// SkillCopyNeed is a library skill with no content in reet and where its
// content can come from. Kind is "fetch" (the controller downloads the
// original) or "capture" (a machine that reports the skill sends its files);
// AgentID and Folder are set for a capture.
type SkillCopyNeed struct{ Item, Name, Kind, AgentID, Folder string }

// managedSkillFolders are the folders machines keep skills in, with the agent
// each belongs to.
var managedSkillFolders = []struct{ folder, agent string }{
	{"~/.agents/skills", "agents"},
	{"~/.claude/skills", "claude"},
	{"~/.cursor/skills", "cursor"},
	{"~/.config/opencode/skills", "opencode"},
}

// managedSkillPath cleans p and returns the agent of the managed folder it is
// in, or its folder's backups. It refuses a path with "..", an absolute path
// and anything outside the four folders.
func managedSkillPath(p string) (agent, folder string, ok bool) {
	if p == "" || len(p) > 1024 || !utf8.ValidString(p) || strings.ContainsAny(p, "\x00\r\n") {
		return "", "", false
	}
	for _, seg := range strings.Split(strings.ReplaceAll(p, "\\", "/"), "/") {
		if seg == ".." {
			return "", "", false
		}
	}
	if !strings.HasPrefix(p, "~/") {
		return "", "", false
	}
	clean := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	for _, m := range managedSkillFolders {
		backups := path.Dir(m.folder) + "/.reet-backups"
		for _, root := range []string{m.folder, m.folder + "/.reet-backups", backups} {
			if clean == root || strings.HasPrefix(clean, root+"/") {
				return m.agent, m.folder, true
			}
		}
	}
	return "", "", false
}

func (s *Store) migrateSkillContent() error {
	ok, err := hasColumn(s.db, "library_items", "origin_ref")
	if err != nil {
		return err
	}
	if !ok {
		if _, err = s.db.Exec(`ALTER TABLE library_items ADD COLUMN origin_ref TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// An earlier build stored the skills.sh page here; that is not the original.
	if _, err = s.db.Exec(`UPDATE library_items SET origin_ref='' WHERE origin_ref LIKE 'https://www.skills.sh/%'`); err != nil {
		return err
	}
	// An imported skill's link is known from the shelf row it came from.
	_, err = s.db.Exec(`UPDATE library_items SET origin_ref=COALESCE((SELECT 'https://github.com/'||k.source FROM library_aliases a JOIN skills k ON k.id=a.reference WHERE a.user_id=library_items.user_id AND a.item_id=library_items.id AND a.kind='shelf' AND k.source_type='github' AND k.source<>'' ORDER BY a.reference LIMIT 1),'') WHERE origin='imported' AND origin_ref=''`)
	return err
}

// SkillSourceFor is the public source an imported skill came from, read from
// the shelf row it was added by. ok is false when it has none to fetch from.
func (s *Store) SkillSourceFor(uid int64, item string) (r ref.Ref, ok bool, err error) {
	err = s.db.QueryRow(`SELECT k.id,k.source,k.slug,k.source_type,k.url FROM library_items i JOIN library_aliases a ON a.user_id=i.user_id AND a.item_id=i.id AND a.kind='shelf' JOIN skills k ON k.id=a.reference
 WHERE i.user_id=? AND i.id=? AND i.item_type=? AND i.deleted_at=0 AND k.source_type IN ('github','well-known') ORDER BY a.reference LIMIT 1`, uid, item, LibrarySkill).Scan(&r.ID, &r.Source, &r.Slug, &r.SourceType, &r.URL)
	if errors.Is(err, sql.ErrNoRows) {
		return r, false, nil
	}
	return r, err == nil, err
}

// skillCopyPlan sorts the live skills that have no content and no problem
// recorded: those reet can fetch, those a machine can send, and the Active
// ones with neither.
func (s *Store) skillCopyPlan(uid int64) (needs []SkillCopyNeed, unsourced []string, err error) {
	rows, err := s.db.Query(`SELECT id,name,status FROM library_items WHERE user_id=? AND item_type=? AND deleted_at=0 AND revision_id='' AND copy_problem='' ORDER BY lower(name),id`, uid, LibrarySkill)
	if err != nil {
		return nil, nil, err
	}
	type item struct{ id, name, status string }
	var items []item
	for rows.Next() {
		var it item
		if err = rows.Scan(&it.id, &it.name, &it.status); err != nil {
			break
		}
		items = append(items, it)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	for _, it := range items {
		_, fetch, err := s.SkillSourceFor(uid, it.id)
		if err != nil {
			return nil, nil, err
		}
		if fetch {
			needs = append(needs, SkillCopyNeed{Item: it.id, Name: it.name, Kind: "fetch"})
			continue
		}
		agent, folder, found, err := s.captureCandidate(uid, it.id, it.name)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case found:
			needs = append(needs, SkillCopyNeed{Item: it.id, Name: it.name, Kind: "capture", AgentID: agent, Folder: folder})
		case it.status == SkillActive:
			unsourced = append(unsourced, it.id)
		}
	}
	return needs, unsourced, nil
}

// captureCandidate is the first machine that reports the skill in a managed
// folder and that reet can ask for it: it must have an environment for that
// folder tied to the skill. Machines that cannot be asked are skipped.
func (s *Store) captureCandidate(uid int64, item, name string) (agent, folder string, found bool, err error) {
	rows, err := s.db.Query(`SELECT a.id,m.folder FROM machine_skills m JOIN sync_agents a ON a.id=m.agent_id AND a.user_id=m.user_id
 WHERE m.user_id=? AND lower(m.name)=lower(?) AND m.state IN ('installed','edited','unknown') AND a.paused=0 AND a.skills_protocol=2
 ORDER BY m.state='installed' DESC,a.host,m.folder,a.id`, uid, name)
	if err != nil {
		return "", "", false, err
	}
	type cand struct{ agent, folder string }
	var cands []cand
	for rows.Next() {
		var c cand
		if err = rows.Scan(&c.agent, &c.folder); err != nil {
			break
		}
		cands = append(cands, c)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return "", "", false, err
	}
	for _, c := range cands {
		kind, _, ok := managedSkillPath(c.folder)
		if !ok {
			continue
		}
		if _, _, _, ok, err = s.captureSource(uid, item, c.agent, kind); err != nil {
			return "", "", false, err
		} else if ok {
			return c.agent, c.folder, true, nil
		}
	}
	return "", "", false, nil
}

// captureSource finds the shelf skill, environment and slug the machine
// capture flow needs to ask agentID for the skill, if reet knows them.
func (s *Store) captureSource(uid int64, item, agentID, kind string) (skill, environment, slug string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT a.reference,e.id,k.slug FROM library_aliases a
 JOIN skills k ON k.id=a.reference JOIN skill_envs se ON se.skill_id=a.reference
 JOIN environments e ON e.id=se.env_id AND e.user_id=a.user_id AND e.agent=? AND e.project=''
 JOIN sync_agents g ON g.id=? AND g.user_id=a.user_id AND g.host=e.host COLLATE NOCASE
 WHERE a.user_id=? AND a.item_id=? AND a.kind='shelf' ORDER BY a.reference,e.id LIMIT 1`, kind, agentID, uid, item).Scan(&skill, &environment, &slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, nil
	}
	return skill, environment, slug, err == nil, err
}

// SkillsNeedingCopyFor lists the skills reet holds no copy of and could get
// one of now. Archived ones are included: their name joins the removal list
// only once reet has the copy.
func (s *Store) SkillsNeedingCopyFor(uid int64) ([]SkillCopyNeed, error) {
	needs, _, err := s.skillCopyPlan(uid)
	return needs, err
}

// MarkUnsourcedSkillsFor records, on each Active skill with no content that
// nothing can supply, why it will not get any. It returns how many it marked.
func (s *Store) MarkUnsourcedSkillsFor(uid int64) (int, error) {
	_, unsourced, err := s.skillCopyPlan(uid)
	if err != nil {
		return 0, err
	}
	for _, id := range unsourced {
		if err = s.SetSkillCopyProblemFor(uid, id, SkillNoSourceProblem); err != nil {
			return 0, err
		}
	}
	return len(unsourced), nil
}

// SetSkillCopyProblemFor says why a skill's content could not be fetched or
// copied. The skill stays where it is: held if Active, and not taken off any
// machine if Archived.
func (s *Store) SetSkillCopyProblemFor(uid int64, item, problem string) error {
	problem = strings.TrimSpace(problem)
	if len(problem) > 500 {
		problem = strings.ToValidUTF8(problem[:500], "")
	}
	if problem == "" {
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		res, err := tx.Exec(`UPDATE library_items SET copy_problem=? WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0 AND revision_id=''`, problem, uid, item, LibrarySkill)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrPrivateSkillMissing
		}
		return nil
	})
}

// RetrySkillCopiesFor clears the copy problem of live skills so the next run
// tries again, and forgets their failed captures so a machine is asked afresh.
// It returns how many skills it cleared.
func (s *Store) RetrySkillCopiesFor(uid int64, items []string) (int, error) {
	cleared := 0
	err := s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		for _, id := range items {
			res, err := tx.Exec(`UPDATE library_items SET copy_problem='' WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0 AND revision_id='' AND copy_problem<>''`, uid, id, LibrarySkill)
			if err != nil {
				return err
			}
			n, _ := res.RowsAffected()
			cleared += int(n)
			if _, err = tx.Exec(`DELETE FROM machine_import_requests WHERE user_id=? AND state IN ('failed','expired','cancelled') AND json_extract(data,'$.syncSkillId') IN (SELECT reference FROM library_aliases WHERE user_id=? AND item_id=? AND kind='shelf')`, uid, uid, id); err != nil {
				return err
			}
		}
		return nil
	})
	return cleared, err
}

// RequestSkillCaptureFor asks a machine for the files of a skill it reports,
// through the existing machine capture. path is where the machine keeps it and
// must sit inside one of the four managed folders or their .reet-backups. It
// does nothing while a capture of the skill is running, and records a problem
// once the machine has failed to send it libraryCaptureAttempts times.
func (s *Store) RequestSkillCaptureFor(uid int64, item, agentID, p string) error {
	kind, _, ok := managedSkillPath(p)
	if !ok {
		return ErrSkillCapturePath
	}
	var name, host string
	var revision string
	err := s.db.QueryRow(`SELECT i.name,i.revision_id,a.host FROM library_items i JOIN sync_agents a ON a.user_id=i.user_id AND a.id=? WHERE i.user_id=? AND i.id=? AND i.item_type=? AND i.deleted_at=0`, agentID, uid, item, LibrarySkill).Scan(&name, &revision, &host)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPrivateSkillMissing
	}
	if err != nil {
		return err
	}
	if revision != "" {
		return nil
	}
	skill, environment, slug, found, err := s.captureSource(uid, item, agentID, kind)
	if err != nil {
		return err
	}
	if !found {
		return ErrSkillNoCaptureSource
	}
	var active, ended int
	if err = s.db.QueryRow(`SELECT COALESCE(SUM(state IN ('queued','previewing','reviewing','approved','uploading','completed')),0),COALESCE(SUM(state IN ('failed','expired','cancelled')),0) FROM machine_import_requests WHERE user_id=? AND json_extract(data,'$.syncSkillId')=?`, uid, skill).Scan(&active, &ended); err != nil {
		return err
	}
	if active > 0 {
		return nil
	}
	if ended >= libraryCaptureAttempts {
		return s.SetSkillCopyProblemFor(uid, item, fmt.Sprintf("%s did not send its files after %d tries", host, ended))
	}
	// A skill copied from a machine is a machine skill, whatever it was
	// before reet knew where it came from.
	if _, err = s.db.Exec(`UPDATE library_items SET origin='machine' WHERE user_id=? AND id=? AND origin=''`, uid, item); err != nil {
		return err
	}
	key := "capture_" + secretHash(skill+"\x00"+string(rune('0'+ended)))
	_, err = s.CreateRemoteImportFor(uid, RemoteImportCreate{RequestKey: key, AgentID: agentID, EnvironmentID: environment, Name: slug, SyncSkillID: skill})
	return err
}

// SetSkillContentFor saves files as a new revision of a library skill and
// binds it to that item, whether the item has content or not. origin.Kind is
// original (fetched), capture, edit or use-version; for original and capture
// origin.Reference is the link or machine the skill is a copy of and becomes
// the item's origin_ref. A copy landing on an Archived skill puts its name on
// the removal list in the same transaction, so a name is only removed from
// machines once reet holds it.
func (s *Store) SetSkillContentFor(uid int64, item string, files []skillbundle.ContentFile, origin PrivateSkillOrigin) (PrivateSkillResult, error) {
	return s.setSkillContent(uid, item, files, origin, nil)
}

// SetSkillContentFromFor is SetSkillContentFor for a write that was decided
// against a revision the caller read earlier: it saves only while the item's
// head is still expectedRevision, and otherwise returns ErrPrivateSkillConflict
// and changes nothing.
func (s *Store) SetSkillContentFromFor(uid int64, item string, files []skillbundle.ContentFile, origin PrivateSkillOrigin, expectedRevision string) (PrivateSkillResult, error) {
	return s.setSkillContent(uid, item, files, origin, &expectedRevision)
}

func (s *Store) setSkillContent(uid int64, item string, files []skillbundle.ContentFile, origin PrivateSkillOrigin, expected *string) (PrivateSkillResult, error) {
	var name, bundle, head string
	err := s.db.QueryRow(`SELECT name,bundle_id FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0`, uid, item, LibrarySkill).Scan(&name, &bundle)
	if errors.Is(err, sql.ErrNoRows) {
		return PrivateSkillResult{}, ErrPrivateSkillMissing
	}
	if err != nil {
		return PrivateSkillResult{}, err
	}
	if bundle != "" {
		b, e := s.PrivateSkillBundleFor(uid, bundle)
		if e != nil && !errors.Is(e, ErrPrivateSkillMissing) {
			return PrivateSkillResult{}, e
		}
		if e == nil {
			name, head = b.Name, b.Head
		} else {
			bundle = ""
		}
	}
	if expected != nil && *expected != head {
		return PrivateSkillResult{}, ErrPrivateSkillConflict
	}
	id, err := privateSkillRandomID()
	if err != nil {
		return PrivateSkillResult{}, err
	}
	write := PrivateSkillWrite{RequestID: "content_" + id, ExpectedBase: &head, Name: name, Origin: origin, Files: files}
	return s.savePrivateSkillRevisionWithReceipt(uid, bundle, write, func(tx *sql.Tx) error {
		var live int
		if err := tx.QueryRow(`SELECT count(*) FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, item).Scan(&live); err != nil {
			return err
		}
		if live != 1 {
			return ErrPrivateSkillMissing
		}
		return nil
	}, nil, func(tx *sql.Tx, result PrivateSkillResult) error {
		// A new bundle made an item of its own; fold it into this one.
		if err := attachLibraryBundle(tx, uid, item, result); err != nil {
			return err
		}
		ref := ""
		if origin.Kind == "original" || origin.Kind == "capture" {
			ref = origin.Reference
		}
		return skillCopyLandedTx(tx, uid, item, ref)
	})
}

// skillCopyLandedTx follows a copy landing on an item: it clears the copy
// problem, records where the copy came from, and, for an Archived skill,
// adds its name to the removal list.
func skillCopyLandedTx(tx *sql.Tx, uid int64, item, originRef string) error {
	if _, err := tx.Exec(`UPDATE library_items SET copy_problem='' WHERE user_id=? AND id=?`, uid, item); err != nil {
		return err
	}
	if originRef != "" {
		if _, err := tx.Exec(`UPDATE library_items SET origin_ref=? WHERE user_id=? AND id=?`, originRef, uid, item); err != nil {
			return err
		}
	}
	_, err := tx.Exec(`INSERT INTO skill_removals(user_id,name,reason,created_at) SELECT user_id,name,'archived',? FROM library_items WHERE user_id=? AND id=? AND item_type=? AND status=? AND deleted_at=0 AND revision_id<>'' ON CONFLICT DO NOTHING`, now(), uid, item, LibrarySkill, SkillArchived)
	return err
}

// machineCopyLandedTx is skillCopyLandedTx for a skill a machine sent through
// the machine capture: its origin_ref is machine:{host}:{folder}.
func machineCopyLandedTx(tx *sql.Tx, uid int64, enrollment, agent string, result PrivateSkillResult) error {
	item, err := libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var host string
	if err = tx.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, enrollment, uid).Scan(&host); err != nil {
		return err
	}
	folder := agent
	for _, m := range managedSkillFolders {
		if m.agent == agent {
			folder = m.folder
		}
	}
	var current string
	if err = tx.QueryRow(`SELECT origin_ref FROM library_items WHERE user_id=? AND id=?`, uid, item).Scan(&current); err != nil {
		return err
	}
	if current != "" {
		return skillCopyLandedTx(tx, uid, item, "")
	}
	return skillCopyLandedTx(tx, uid, item, "machine:"+host+":"+folder)
}

// SkillEditedSinceImportFor says whether a skill's content was changed after
// the last revision fetched from its original: any later revision that is not
// itself fetched counts. A skill with no fetched revision is not edited.
func (s *Store) SkillEditedSinceImportFor(uid int64, item string) (bool, error) {
	var bundle string
	err := s.db.QueryRow(`SELECT bundle_id FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0`, uid, item, LibrarySkill).Scan(&bundle)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrPrivateSkillMissing
	}
	if err != nil || bundle == "" {
		return false, err
	}
	rows, err := s.db.Query(`SELECT COALESCE(json_extract(origin,'$.kind'),'') FROM private_skill_revisions WHERE user_id=? AND bundle_id=? ORDER BY created_at,rowid`, uid, bundle)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	edited, imported := false, false
	for rows.Next() {
		var kind string
		if err = rows.Scan(&kind); err != nil {
			return false, err
		}
		if kind == "original" {
			imported, edited = true, false
		} else if imported {
			edited = true
		}
	}
	return edited, rows.Err()
}

// SkillOriginFor is how a skill arrived (written, imported, machine or empty)
// and the reference it is a copy of: the original link, or machine:{host}:{folder}.
func (s *Store) SkillOriginFor(uid int64, item string) (origin, reference string, err error) {
	err = s.db.QueryRow(`SELECT origin,origin_ref FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0`, uid, item, LibrarySkill).Scan(&origin, &reference)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrPrivateSkillMissing
	}
	return
}
