package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// ErrSkillRevisionCurrent is a Put back of the version that is already the
// skill's current one.
var ErrSkillRevisionCurrent = errors.New("that version is already the current one")

// SkillHistoryEntry is one saved version of a library skill. Kind is how it
// was saved: user or fork (written in reet), edit, original (fetched from its
// link), capture or import (kept from a machine), use-version or restore (put
// back). Machine names the machine a kept version or Use this version came
// from; Reference is the link of a fetched one.
type SkillHistoryEntry struct {
	Revision    string `json:"revision"`
	Kind        string `json:"kind"`
	Reference   string `json:"reference,omitempty"`
	Machine     string `json:"machine,omitempty"`
	DerivedFrom string `json:"derivedFrom,omitempty"`
	Digest      string `json:"digest"`
	Files       int    `json:"files"`
	SavedAt     int64  `json:"savedAt"`
	Current     bool   `json:"current"`
}

// skillItemContent is the live library skill item of uid, or
// ErrPrivateSkillMissing.
func (s *Store) skillItemContent(uid int64, item string) (LibraryItem, error) {
	li, err := s.LibraryItemFor(uid, item)
	if err == nil && li.ItemType != LibrarySkill {
		err = ErrPrivateSkillMissing
	}
	return li, err
}

// SkillHistoryFor lists the saved versions of a library skill, newest first,
// with the current one marked. A skill reet has no copy of yet has none.
// Another account's skill is ErrPrivateSkillMissing.
func (s *Store) SkillHistoryFor(uid int64, item string) ([]SkillHistoryEntry, error) {
	li, err := s.skillItemContent(uid, item)
	if err != nil {
		return nil, err
	}
	out := []SkillHistoryEntry{}
	if li.BundleID == "" {
		return out, nil
	}
	revs, err := s.PrivateSkillRevisionsFor(uid, li.BundleID)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	rows, err := s.db.Query(`SELECT revision_id,count(*) FROM private_skill_revision_files WHERE user_id=? AND bundle_id=? GROUP BY revision_id`, uid, li.BundleID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id string
		var n int
		if err = rows.Scan(&id, &n); err != nil {
			rows.Close()
			return nil, err
		}
		counts[id] = n
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range newestFirst(revs, li.RevisionID) {
		e := SkillHistoryEntry{Revision: r.ID, Kind: r.Origin.Kind, DerivedFrom: r.Origin.DerivedFrom,
			Digest: r.ManifestDigest, Files: counts[r.ID], SavedAt: r.CreatedAt, Current: r.ID == li.RevisionID}
		// Only a link is shown as a reference; a machine's record is read
		// for its name instead.
		if r.Origin.Kind == "original" {
			e.Reference = r.Origin.Reference
		}
		if e.Machine, err = s.historyMachine(uid, r.Origin.Reference); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// RestoreSkillRevisionFor puts back an earlier version of a skill: its files
// become a new revision of kind restore, saved only while the skill's head is
// still head (else ErrPrivateSkillConflict), so the history only grows and
// every machine is given it like any edit. A version that is not in this
// skill's history is ErrPrivateSkillMissing; the current one is
// ErrSkillRevisionCurrent.
func (s *Store) RestoreSkillRevisionFor(uid int64, item, revision, head string) (PrivateSkillResult, error) {
	li, err := s.skillItemContent(uid, item)
	if err != nil {
		return PrivateSkillResult{}, err
	}
	if li.BundleID == "" || !PrivateSkillIDValid(revision) {
		return PrivateSkillResult{}, ErrPrivateSkillMissing
	}
	if revision == li.RevisionID {
		return PrivateSkillResult{}, ErrSkillRevisionCurrent
	}
	old, err := s.PrivateSkillRevisionFor(uid, li.BundleID, revision)
	if err != nil {
		return PrivateSkillResult{}, err
	}
	return s.SetSkillContentFromFor(uid, item, old.Files, PrivateSkillOrigin{Kind: "restore", DerivedFrom: revision}, head)
}

// newestFirst orders revisions from head back through their parents, so two
// saved in the same millisecond still read in the order they were made. Any
// not on that line follow, newest first.
func newestFirst(revs []PrivateSkillRevision, head string) []PrivateSkillRevision {
	byID := make(map[string]PrivateSkillRevision, len(revs))
	for _, r := range revs {
		byID[r.ID] = r
	}
	out := make([]PrivateSkillRevision, 0, len(revs))
	seen := map[string]bool{}
	for id := head; id != "" && !seen[id]; {
		r, ok := byID[id]
		if !ok {
			break
		}
		seen[id] = true
		out = append(out, r)
		id = r.Parent
	}
	for i := len(revs) - 1; i >= 0; i-- {
		if !seen[revs[i].ID] {
			out = append(out, revs[i])
		}
	}
	return out
}

// historyMachine names the machine a version came from, as its origin's
// reference records it. A capture (kind import) records the machine's
// enrollment, and Use this version its enrollment and host: the machine is
// named by its current name while reet knows it, else by the host the
// version saved, else not at all. A machine:<host>:<folder> reference names
// its host.
func (s *Store) historyMachine(uid int64, reference string) (string, error) {
	if parts := strings.SplitN(reference, ":", 3); len(parts) == 3 && parts[0] == "machine" {
		return parts[1], nil
	}
	if !strings.HasPrefix(reference, "{") {
		return "", nil
	}
	var ref struct{ Enrollment, Host string }
	if json.Unmarshal([]byte(reference), &ref) != nil {
		return "", nil
	}
	if ref.Enrollment != "" {
		var host string
		err := s.db.QueryRow(`SELECT host FROM sync_agents WHERE id=? AND user_id=?`, ref.Enrollment, uid).Scan(&host)
		if err == nil && host != "" {
			return host, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", err
		}
	}
	return ref.Host, nil
}
