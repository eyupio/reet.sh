package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

// Events are immutable. Base is the revision actually present when editing,
// never a head fetched after reconnect. SavedAt is calibrated source time.
type LibraryEdit struct {
	EventID     string                    `json:"eventId"`
	ItemID      string                    `json:"itemId"`
	Base        string                    `json:"base"`
	SavedAt     int64                     `json:"savedAt"`
	Uncertainty int64                     `json:"uncertainty"`
	Files       []skillbundle.ContentFile `json:"files"`
}
type LibraryEditReceipt struct {
	EventID       string `json:"eventId"`
	RevisionID    string `json:"revisionId"`
	Won           bool   `json:"won"`
	Base          string `json:"base"`
	SavedAt       int64  `json:"savedAt"`
	OrderAt       int64  `json:"orderAt"`
	Uncertainty   int64  `json:"uncertainty"`
	ClockFallback bool   `json:"clockFallback"`
}

func (s *Store) NegotiateLibraryFor(uid int64, enrollment string) error {
	return s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		if err := contentAgent(tx, uid, enrollment); err != nil {
			return err
		}
		var enabled int
		if err := tx.QueryRow(`SELECT count(*) FROM library_selection_accounts WHERE user_id=?`, uid).Scan(&enabled); err != nil {
			return err
		}
		if enabled == 0 {
			// Freeze existing library membership once. New folders receive no
			// implicit subscription, and custom policies retain their intent.
			if err := recordLibrarySelection(tx, uid, "", []string{}); err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM library_selections WHERE user_id=? AND target_id=''`, uid); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE sync_policies SET follow_source=0 WHERE target_id IN (SELECT target_id FROM sync_targets WHERE user_id=? AND source_id='library')`, uid); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO library_connectors VALUES(?,?,1) ON CONFLICT(user_id,enrollment_id) DO UPDATE SET protocol=1`, uid, enrollment)
		return err
	})
}
func libraryEditAllowed(tx *sql.Tx, uid int64, enrollment, itemID string) error {
	if err := contentAgent(tx, uid, enrollment); err != nil {
		return err
	}
	var n int
	err := tx.QueryRow(`SELECT count(*) FROM canonical_library_selections c JOIN environments e ON e.user_id=c.user_id AND e.id=c.target_id JOIN sync_agents a ON a.user_id=e.user_id AND a.host=e.host COLLATE NOCASE JOIN library_items i ON i.user_id=c.user_id AND i.id=? WHERE c.user_id=? AND a.id=? AND i.deleted_at=0 AND EXISTS(SELECT 1 FROM json_each(c.item_ids) WHERE value=i.id)`, itemID, uid, enrollment).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		// Explicitly imported roots are sources even without an install target.
		err = tx.QueryRow(`SELECT count(*) FROM library_aliases a JOIN library_locations l ON l.user_id=a.user_id AND a.reference LIKE l.enrollment_id||':'||l.id||':%' WHERE a.user_id=? AND a.item_id=? AND a.kind='machine' AND l.enrollment_id=?`, uid, itemID, enrollment).Scan(&n)
		if err != nil {
			return err
		}
		if n == 0 {
			if err := tx.QueryRow(`SELECT count(*) FROM library_bases WHERE user_id=? AND enrollment_id=? AND item_id=?`, uid, enrollment, itemID).Scan(&n); err != nil {
				return err
			}
			if n == 0 {
				return ErrRemoteNotFound
			}
		}
	}
	return nil
}
func (s *Store) AcceptLibraryEditFor(uid int64, enrollment string, in LibraryEdit) (LibraryEditReceipt, error) {
	out := LibraryEditReceipt{EventID: in.EventID, Base: in.Base, SavedAt: in.SavedAt, Uncertainty: in.Uncertainty}
	if !privateSkillRequestID.MatchString(in.EventID) || !PrivateSkillIDValid(in.ItemID) || !PrivateSkillIDValid(in.Base) || in.SavedAt <= 0 || in.Uncertainty < 0 || in.Uncertainty > int64((24*time.Hour)/time.Millisecond) {
		return out, ErrPrivateSkillInvalid
	}
	_, files, err := skillimport.Select(in.Files, nil)
	if err != nil {
		return out, ErrPrivateSkillInvalid
	}
	in.Files = files
	payload, _ := json.Marshal(in)
	sum := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(sum[:])
	key := secretHash(enrollment + "\x00" + in.EventID)
	for attempt := 0; attempt < 5; attempt++ {
		var oldFingerprint string
		err = s.withTx(func(tx *sql.Tx) error {
			if err := libraryEditAllowed(tx, uid, enrollment, in.ItemID); err != nil {
				return err
			}
			return tx.QueryRow(`SELECT fingerprint,revision_id,won,order_at,clock_fallback FROM library_edit_events WHERE user_id=? AND event_key=?`, uid, key).Scan(&oldFingerprint, &out.RevisionID, &out.Won, &out.OrderAt, &out.ClockFallback)
		})
		if err == nil {
			if oldFingerprint != fingerprint {
				return out, ErrPrivateSkillConflict
			}
			return out, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		item, err := s.LibraryItemFor(uid, in.ItemID)
		if err != nil {
			return out, err
		}
		bundle, err := s.PrivateSkillBundleFor(uid, item.BundleID)
		if err != nil {
			return out, err
		}
		// The transport append is serialized at the current bundle head. The event
		// retains its independent causal base; losing revisions never become heads.
		expected := bundle.Head
		write := PrivateSkillWrite{RequestID: "edit_" + key, ExpectedBase: &expected, Name: bundle.Name, Origin: PrivateSkillOrigin{Kind: "user", Reference: "machine-edit:" + enrollment, DerivedFrom: in.Base}, Files: files}
		result, err := s.savePrivateSkillRevisionWithReceipt(uid, bundle.ID, write, func(tx *sql.Tx) error {
			if err := libraryEditAllowed(tx, uid, enrollment, in.ItemID); err != nil {
				return err
			}
			var head, name string
			if err := tx.QueryRow(`SELECT revision_id,name FROM library_items WHERE user_id=? AND id=? AND deleted_at=0`, uid, item.ID).Scan(&head, &name); err != nil {
				return err
			}
			if head != item.RevisionID || name != item.Name {
				return ErrPrivateSkillConflict
			}
			// A base from another item cannot supply authority or causality.
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM private_skill_revisions r JOIN library_aliases a ON a.user_id=r.user_id AND a.kind='bundle' AND a.reference=r.bundle_id WHERE r.user_id=? AND r.id=? AND a.item_id=?`, uid, in.Base, item.ID).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return ErrPrivateSkillMissing
			}
			return nil
		}, nil, func(tx *sql.Tx, result PrivateSkillResult) error {
			var savedAt int64
			var priorKey string
			err := tx.QueryRow(`SELECT order_at,event_key FROM library_edit_events WHERE user_id=? AND revision_id=?`, uid, item.RevisionID).Scan(&savedAt, &priorKey)
			if errors.Is(err, sql.ErrNoRows) {
				err = tx.QueryRow(`SELECT created_at FROM private_skill_revisions WHERE user_id=? AND id=?`, uid, item.RevisionID).Scan(&savedAt)
				priorKey = item.RevisionID
			}
			if err != nil {
				return err
			}
			var baseTime int64
			err = tx.QueryRow(`SELECT order_at FROM library_edit_events WHERE user_id=? AND revision_id=?`, uid, in.Base).Scan(&baseTime)
			if errors.Is(err, sql.ErrNoRows) {
				err = tx.QueryRow(`SELECT created_at FROM private_skill_revisions WHERE user_id=? AND id=?`, uid, in.Base).Scan(&baseTime)
			}
			if err != nil {
				return err
			}
			out.OrderAt = in.SavedAt
			out.ClockFallback = in.Uncertainty > 30000 || in.SavedAt > time.Now().Add(5*time.Minute).UnixMilli() || in.SavedAt < baseTime-int64(5*time.Minute/time.Millisecond)
			if out.ClockFallback || out.OrderAt <= baseTime {
				out.OrderAt = baseTime + 1
			}
			// Logical save time respects causality, then calibrated wall time.
			// An unusable clock falls back to base+1, not receipt time. Stable
			// event keys settle ties and all alternatives stay recoverable.
			won := out.OrderAt > savedAt || (out.OrderAt == savedAt && key > priorKey)
			out.Won = won
			out.RevisionID = result.Revision.ID
			if _, err = tx.Exec(`INSERT INTO library_edit_events VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, uid, key, item.ID, enrollment, fingerprint, in.Base, result.Revision.ID, in.SavedAt, in.Uncertainty, won, out.OrderAt, out.ClockFallback); err != nil {
				return err
			}
			if !won {
				if _, err = tx.Exec(`UPDATE private_skill_bundles SET head=? WHERE user_id=? AND id=?`, expected, uid, bundle.ID); err != nil {
					return err
				}
				_, err = tx.Exec(`UPDATE library_items SET bundle_id=?,revision_id=?,digest=?,problem=? WHERE user_id=? AND id=?`, item.BundleID, item.RevisionID, item.Digest, item.Problem, uid, item.ID)
				return err
			}
			if err = attachLibraryBundle(tx, uid, item.ID, result); err != nil {
				return err
			}
			return updateHostedFromLibraryEdit(tx, uid, item, result, files)
		})
		if errors.Is(err, ErrPrivateSkillConflict) {
			continue
		}
		if err != nil {
			return out, err
		}
		out.RevisionID = result.Revision.ID
		return out, nil
	}
	return out, ErrLibraryConflict
}

// Keep the authoring editor and its binding on the accepted head. Otherwise a
// later library listing could mistakenly reimport the old editor as a new edit.
func updateHostedFromLibraryEdit(tx *sql.Tx, uid int64, item LibraryItem, result PrivateSkillResult, files []skillbundle.ContentFile) error {
	var slug string
	err := tx.QueryRow(`SELECT reference FROM library_aliases WHERE user_id=? AND item_id=? AND kind='hosted'`, uid, item.ID).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	// The editor projects bytes; its separate digest preserves private executable
	// modes without mistaking this projection for another source edit.
	content := ""
	supporting := map[string][]byte{}
	for _, f := range files {
		if f.Path == "SKILL.md" {
			content = string(f.Data)
		} else {
			supporting[f.Path] = f.Data
		}
	}
	editorFiles := make([]skillbundle.ContentFile, len(files))
	copy(editorFiles, files)
	for i := range editorFiles {
		editorFiles[i].Executable = false
	}
	editorManifest, e := skillbundle.FromContents(editorFiles)
	if e != nil {
		return e
	}
	if _, err = tx.Exec(`INSERT INTO library_hosted_editor_digests VALUES(?,?,?) ON CONFLICT(user_id,slug) DO UPDATE SET digest=excluded.digest`, uid, slug, editorManifest.Digest); err != nil {
		return err
	}
	check := HostedSkill{Slug: slug, Content: content, Files: supporting}
	if err := ValidateHostedSkill(&check); err != nil {
		return ErrPrivateSkillInvalid
	}
	raw, _ := json.Marshal(supporting)
	if _, err = tx.Exec(`UPDATE hosted_skills SET description=?,content=?,files=?,updated_at=? WHERE user_id=? AND slug=?`, check.Description, content, string(raw), now(), uid, slug); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE hosted_sync_sources SET bundle_id=?,revision_id=?,digest=? WHERE user_id=? AND slug=?`, result.Bundle.ID, result.Revision.ID, result.Revision.ManifestDigest, uid, slug)
	return err
}
func (s *Store) LibraryEditHistoryFor(uid int64, item string) ([]LibraryEditReceipt, error) {
	if _, err := s.LibraryItemFor(uid, item); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT event_key,revision_id,won,base,saved_at,order_at,uncertainty,clock_fallback FROM library_edit_events WHERE user_id=? AND item_id=? ORDER BY saved_at DESC,event_key DESC`, uid, item)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LibraryEditReceipt{}
	for rows.Next() {
		var r LibraryEditReceipt
		if err = rows.Scan(&r.EventID, &r.RevisionID, &r.Won, &r.Base, &r.SavedAt, &r.OrderAt, &r.Uncertainty, &r.ClockFallback); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type LibraryBase struct {
	BindingID  string `json:"bindingId"`
	ItemID     string `json:"itemId"`
	RevisionID string `json:"revisionId"`
}

func (s *Store) RecordLibraryBaseFor(uid int64, enrollment string, in LibraryBase) error {
	if len(in.BindingID) != 64 || !PrivateSkillIDValid(in.ItemID) || !PrivateSkillIDValid(in.RevisionID) {
		return ErrPrivateSkillInvalid
	}
	if _, err := hex.DecodeString(in.BindingID); err != nil {
		return ErrPrivateSkillInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if err := libraryEditAllowed(tx, uid, enrollment, in.ItemID); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(`SELECT count(*) FROM private_skill_revisions r JOIN library_aliases a ON a.user_id=r.user_id AND a.kind='bundle' AND a.reference=r.bundle_id WHERE r.user_id=? AND r.id=? AND a.item_id=?`, uid, in.RevisionID, in.ItemID).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return ErrPrivateSkillMissing
		}
		var entries int
		if err := tx.QueryRow(`SELECT count(*) FROM library_bases WHERE user_id=? AND enrollment_id=? AND binding_id<>?`, uid, enrollment, in.BindingID).Scan(&entries); err != nil {
			return err
		}
		if entries >= 1024 {
			return ErrPrivateSkillCapacity
		}
		_, err := tx.Exec(`INSERT INTO library_bases VALUES(?,?,?,?,?) ON CONFLICT(user_id,enrollment_id,binding_id) DO UPDATE SET revision_id=excluded.revision_id,item_id=excluded.item_id`, uid, enrollment, in.BindingID, in.ItemID, in.RevisionID)
		return err
	})
}
