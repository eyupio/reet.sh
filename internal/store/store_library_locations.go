package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"path"
	"strings"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillimport"
)

// A location belongs to one enrollment. Its absolute path is descriptive local
// metadata, never a path the controller can open or copy onto another machine.
type LibraryLocation struct {
	ID          string `json:"id"`
	Application string `json:"application"`
	Project     string `json:"project,omitempty"`
	Path        string `json:"path"`
	LegacyAgent string `json:"legacyAgent,omitempty"`
}

type LibraryLocationEntry struct {
	Name       string `json:"name"`
	ItemID     string `json:"itemId"`
	BundleID   string `json:"bundleId"`
	RevisionID string `json:"revisionId"`
	Digest     string `json:"digest"`
}

type LibraryLocationImport struct {
	LocationID   string                    `json:"locationId"`
	Name         string                    `json:"name"`
	RequestID    string                    `json:"requestId"`
	ExpectedBase *string                   `json:"expectedBase"`
	Files        []skillbundle.ContentFile `json:"files"`
}

func validLibraryLocation(location LibraryLocation) bool {
	app := location.Application
	validApp := app == "shared" || app == "claude" || app == "codex" || app == "opencode" || app == "agy"
	if location.LegacyAgent != "" {
		dirs := map[string]string{"agents": ".agents/skills", "claude": ".claude/skills", "codex": ".codex/skills", "opencode": ".config/opencode/skills"}
		dir, ok := dirs[location.LegacyAgent]
		if !ok || (app != location.LegacyAgent && !(app == "shared" && location.LegacyAgent == "agents")) {
			return false
		}
		if location.Project != "" {
			if location.LegacyAgent == "opencode" {
				return false
			}
			if location.Path != path.Join(location.Project, dir) {
				return false
			}
		} else if !strings.HasSuffix(location.Path, "/"+dir) {
			return false
		}
	}
	return privateSkillID.MatchString(location.ID) && validApp && contentobs.ValidText(location.Path, 4096) && path.IsAbs(location.Path) && path.Clean(location.Path) == location.Path && contentobs.ValidText(location.Project, 4096) && (location.Project == "" || path.IsAbs(location.Project))
}

func (s *Store) RegisterLibraryLocationFor(uid int64, enrollment string, location LibraryLocation) ([]LibraryLocationEntry, error) {
	if !validLibraryLocation(location) {
		return nil, ErrPrivateSkillInvalid
	}
	var entries = []LibraryLocationEntry{}
	err := s.withTx(func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, enrollment); err != nil {
			return err
		}
		var previous LibraryLocation
		err := tx.QueryRow(`SELECT id,application,project,path,legacy_agent FROM library_locations WHERE user_id=? AND enrollment_id=? AND id=?`, uid, enrollment, location.ID).Scan(&previous.ID, &previous.Application, &previous.Project, &previous.Path, &previous.LegacyAgent)
		if err == nil && previous != location {
			return ErrLibraryConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO library_locations VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(user_id,enrollment_id,id) DO UPDATE SET seen_at=excluded.seen_at`, uid, enrollment, location.ID, location.Application, location.Project, location.Path, now(), location.LegacyAgent); err != nil {
			return err
		}
		prefix := enrollment + ":" + location.ID + ":"
		if location.LegacyAgent != "" {
			// The local connector identifies its unchanged standard scope. Custom
			// and overridden roots never claim a legacy scope merely by name.
			legacy := enrollment + ":" + location.LegacyAgent + ":" + location.Project + ":"
			if _, err = tx.Exec(`INSERT OR IGNORE INTO library_aliases(user_id,kind,reference,item_id) SELECT user_id,'machine',?||substr(reference,?),item_id FROM library_aliases WHERE user_id=? AND kind='machine_capture' AND substr(reference,1,?)=?`, prefix, len(legacy)+1, uid, len(legacy), legacy); err != nil {
				return err
			}
		}
		rows, err := tx.Query(`SELECT a.reference,i.id,i.bundle_id,i.revision_id,i.digest FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=? AND a.kind='machine' AND substr(a.reference,1,?)=? ORDER BY a.reference`, uid, len(prefix), prefix)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref string
			var e LibraryLocationEntry
			if err = rows.Scan(&ref, &e.ItemID, &e.BundleID, &e.RevisionID, &e.Digest); err != nil {
				return err
			}
			e.Name = strings.TrimPrefix(ref, prefix)
			entries = append(entries, e)
		}
		return rows.Err()
	})
	return entries, err
}

func libraryLocationBinding(q privateSkillQuery, uid int64, reference string) (id, bundle, head string, err error) {
	err = q.QueryRow(`SELECT i.id,i.bundle_id,i.revision_id FROM library_aliases a JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE a.user_id=? AND a.kind='machine' AND a.reference=? AND i.deleted_at=0`, uid, reference).Scan(&id, &bundle, &head)
	if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	return
}

// ImportLibraryLocationFor is a content-ingestion path, not deployment intent.
// The connector explicitly selected the local roots. Complete bytes use the
// same immutable store, expected-base fence and receipts as other captures.
func (s *Store) ImportLibraryLocationFor(uid int64, enrollment string, in LibraryLocationImport) (PrivateSkillResult, error) {
	var empty PrivateSkillResult
	if !privateSkillID.MatchString(in.LocationID) || !ValidHostedName(in.Name) || in.ExpectedBase == nil {
		return empty, ErrPrivateSkillInvalid
	}
	if _, _, err := skillimport.Select(in.Files, nil); err != nil {
		return empty, ErrPrivateSkillInvalid
	}
	reference := enrollment + ":" + in.LocationID + ":" + in.Name
	_, bundle, _, err := libraryLocationBinding(s.db, uid, reference)
	if err != nil {
		return empty, err
	}
	// A create retry keeps its original empty target in the write fingerprint.
	if *in.ExpectedBase == "" {
		bundle = ""
	}
	origin, _ := json.Marshal(struct{ EnrollmentID, LocationID, Name string }{enrollment, in.LocationID, in.Name})
	write := PrivateSkillWrite{RequestID: in.RequestID, ExpectedBase: in.ExpectedBase, Name: in.Name, Origin: PrivateSkillOrigin{Kind: "import", Reference: string(origin)}, Files: in.Files}
	result, err := s.savePrivateSkillRevisionWithReceipt(uid, bundle, write, func(tx *sql.Tx) error {
		if err := contentAgent(tx, uid, enrollment); err != nil {
			return err
		}
		var one int
		if err := tx.QueryRow(`SELECT count(*) FROM library_locations WHERE user_id=? AND enrollment_id=? AND id=?`, uid, enrollment, in.LocationID).Scan(&one); err != nil {
			return err
		}
		if one != 1 {
			return ErrRemoteNotFound
		}
		var replay int
		if err := tx.QueryRow(`SELECT count(*) FROM private_skill_revisions WHERE user_id=? AND request_id=?`, uid, in.RequestID).Scan(&replay); err != nil {
			return err
		}
		if replay > 0 {
			return nil
		} // fingerprint and current authorization still checked
		_, _, head, err := libraryLocationBinding(tx, uid, reference)
		if err != nil {
			return err
		}
		if head != *in.ExpectedBase {
			return ErrPrivateSkillConflict
		}
		return nil
	}, nil, func(tx *sql.Tx, result PrivateSkillResult) error {
		var currentHead string
		if err := tx.QueryRow(`SELECT head FROM private_skill_bundles WHERE user_id=? AND id=?`, uid, result.Bundle.ID).Scan(&currentHead); err != nil {
			return err
		}
		if currentHead != result.Revision.ID {
			return nil
		} // a late receipt cannot rewind a head
		id, _, _, err := libraryLocationBinding(tx, uid, reference)
		if err != nil {
			return err
		}
		if id == "" {
			id, err = libraryAliasID(tx, uid, "bundle", result.Bundle.ID)
			if err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,'machine',?,?)`, uid, reference, id); err != nil {
				return err
			}
		}
		if err := attachLibraryBundle(tx, uid, id, result); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO library_bases VALUES(?,?,?,?,?) ON CONFLICT(user_id,enrollment_id,binding_id) DO UPDATE SET revision_id=excluded.revision_id,item_id=excluded.item_id`, uid, enrollment, secretHash("root:"+in.LocationID+":"+in.Name), id, result.Revision.ID)
		return err
	})
	if err == nil {
		result.ItemID, _, _, err = libraryLocationBinding(s.db, uid, reference)
	}
	return result, err
}
