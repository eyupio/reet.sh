package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/eyupio/reet.sh/internal/contentobs"
	"github.com/eyupio/reet.sh/internal/skillbundle"
	"github.com/eyupio/reet.sh/internal/skillplan"
)

const MaxSkillContentReceipts = 1024

const (
	skillContentReceiptAdopted  = "adopted"
	skillContentReceiptAttested = "controller_attested"
)

var (
	ErrSkillContentReceiptInvalid  = errors.New("invalid ownership receipt")
	ErrSkillContentReceiptMissing  = errors.New("ownership receipt unavailable")
	ErrSkillContentReceipt         = errors.New("ownership receipt changed, is blocked or must be released first")
	ErrSkillContentReceiptCapacity = errors.New("ownership receipt limit reached")

	// errReceiptIntegrity is deliberately none of the exported sentinels: stored
	// data that fails validation is a fault to surface, never a request outcome.
	errReceiptIntegrity = errors.New("stored ownership receipt failed integrity validation")
)

// SkillContentReceiptEvidence is the metadata an adoption was based on. It holds
// identifiers, hashes and times only, never file bytes or host paths.
type SkillContentReceiptEvidence struct {
	PreviewID      string `json:"previewId"`
	ReviewHash     string `json:"reviewHash"`
	CaptureHash    string `json:"captureHash"`
	ObservedDigest string `json:"observedDigest"`
	PinID          string `json:"pinId"`
	PinVersion     string `json:"pinVersion"`
	CapturedAt     int64  `json:"capturedAt"`
	AdoptedAt      int64  `json:"adoptedAt"`
	AdoptedBy      int64  `json:"adoptedBy"`
}

// SkillContentReceipt records which files of one skill folder this account's
// library provably owns. It is an attestation by the controller from a captured
// observation, never proof about the disk now, and it is independent of pins and
// previews: removing either never changes ownership. Managed is set on item
// reads only; summaries leave it out. Damaged marks a listed row whose stored
// columns could not be decoded: it carries only the identity and version needed
// to release it, and every read that proves ownership refuses it.
type SkillContentReceipt struct {
	ID            string                      `json:"id"`
	EnvironmentID string                      `json:"environmentId"`
	AgentID       string                      `json:"agentId"`
	Name          string                      `json:"name"`
	BundleID      string                      `json:"bundleId"`
	RevisionID    string                      `json:"revisionId"`
	ManagedDigest string                      `json:"managedDigest"`
	FileCount     int                         `json:"fileCount"`
	Managed       *skillbundle.Manifest       `json:"managed,omitempty"`
	Damaged       bool                        `json:"damaged,omitempty"`
	Provenance    string                      `json:"provenance"`
	Verification  string                      `json:"verification"`
	Evidence      SkillContentReceiptEvidence `json:"evidence"`
	Version       string                      `json:"version"`
	CreatedAt     int64                       `json:"createdAt"`
	UpdatedAt     int64                       `json:"updatedAt"`
}

func (s *Store) migrateSkillContentReceipts() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS skill_content_receipts(
 id TEXT PRIMARY KEY,user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 environment_id TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
 agent_id TEXT NOT NULL,name TEXT NOT NULL,bundle_id TEXT NOT NULL,revision_id TEXT NOT NULL,
 root_id TEXT NOT NULL,adapter TEXT NOT NULL,schema_version INTEGER NOT NULL,
 managed TEXT NOT NULL,provenance TEXT NOT NULL,evidence TEXT NOT NULL,request_key TEXT NOT NULL,
 version TEXT NOT NULL,created_at INTEGER NOT NULL,updated_at INTEGER NOT NULL,
 UNIQUE(user_id,environment_id,name),
 UNIQUE(user_id,request_key),
 FOREIGN KEY(agent_id,user_id) REFERENCES sync_agents(id,user_id) ON DELETE CASCADE,
 FOREIGN KEY(user_id,bundle_id,revision_id) REFERENCES private_skill_revisions(user_id,bundle_id,id) ON DELETE CASCADE)`)
	return err
}

func skillContentReceiptOwner(tx *sql.Tx, uid int64) error {
	err := privateSkillOwner(tx, uid)
	if errors.Is(err, ErrPrivateSkillMissing) {
		return ErrSkillContentReceiptMissing
	}
	return err
}

// storedReceipt is a row as stored: the public fields plus the raw JSON columns
// and the slot identity the planner needs to rebuild its target.
type storedReceipt struct {
	SkillContentReceipt
	rootID, adapter, managed, evidence string
	schema                             int
}

const receiptCols = `id,environment_id,agent_id,name,bundle_id,revision_id,root_id,adapter,schema_version,managed,provenance,evidence,version,created_at,updated_at`

func scanStoredReceipt(row interface{ Scan(...any) error }) (r storedReceipt, err error) {
	err = row.Scan(&r.ID, &r.EnvironmentID, &r.AgentID, &r.Name, &r.BundleID, &r.RevisionID, &r.rootID, &r.adapter, &r.schema, &r.managed, &r.Provenance, &r.evidence, &r.Version, &r.CreatedAt, &r.UpdatedAt)
	r.Verification = skillContentReceiptAttested
	return
}

func decodeStrict(raw string, v any) error {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}
	return nil
}

// decode parses the JSON columns. It reads the managed manifest only to report
// its digest and file count; proving it is a different step (verified).
func (r storedReceipt) decode() (SkillContentReceipt, skillbundle.Manifest, error) {
	out := r.SkillContentReceipt
	var managed skillbundle.Manifest
	if out.Provenance != skillContentReceiptAdopted || decodeStrict(r.managed, &managed) != nil || json.Unmarshal([]byte(r.evidence), &out.Evidence) != nil {
		return SkillContentReceipt{}, skillbundle.Manifest{}, errReceiptIntegrity
	}
	out.ManagedDigest, out.FileCount = managed.Digest, len(managed.Files)
	return out, managed, nil
}

// summary is the list view of a row. It reads the stored columns and nothing more:
// it never compares the manifest with its revision, so it does not imply the
// receipt has been verified. A row that does not decode is still returned, flagged
// Damaged, with only its identity, slot and version so it can be found and released.
func (r storedReceipt) summary() SkillContentReceipt {
	if out, _, err := r.decode(); err == nil {
		return out
	}
	return SkillContentReceipt{ID: r.ID, EnvironmentID: r.EnvironmentID, AgentID: r.AgentID, Name: r.Name, BundleID: r.BundleID, RevisionID: r.RevisionID, Version: r.Version, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Damaged: true}
}

// verifyManaged is the integrity check for ownership. IsSubset alone is not
// enough, since it accepts an empty or unvalidated manifest, so the stored
// manifest must first be exactly what the S1 identity format builds from its own
// files. That rebuild already refuses more than skillbundle.MaxFiles files and
// any manifest without SKILL.md, and the byte comparison refuses anything
// non-canonical (order, flags, issues, totals, digest). Only then must every
// managed file equal the same path of the revision the receipt records.
func verifyManaged(managed, revision skillbundle.Manifest) error {
	rebuilt, err := skillbundle.FromMetadata(managed.Files)
	if err != nil {
		return errReceiptIntegrity
	}
	want, _ := json.Marshal(rebuilt)
	got, _ := json.Marshal(managed)
	if !bytes.Equal(want, got) || !skillplan.IsSubset(managed, revision) {
		return errReceiptIntegrity
	}
	return nil
}

// verified decodes a receipt and proves its managed manifest against the revision
// it records. An unreadable revision comes back as privateSkillRevision reports
// it (ErrPrivateSkillMissing), for each caller to classify: an item read has
// already passed the account guard, a baseline has a receipt row in hand.
func (r storedReceipt) verified(q privateSkillQuery, uid int64) (SkillContentReceipt, error) {
	out, managed, err := r.decode()
	if err != nil {
		return SkillContentReceipt{}, err
	}
	revision, err := privateSkillRevision(q, uid, r.BundleID, r.RevisionID, false)
	if err != nil {
		return SkillContentReceipt{}, err
	}
	if err = verifyManaged(managed, *revision.Manifest); err != nil {
		return SkillContentReceipt{}, err
	}
	out.Managed = &managed
	return out, nil
}

// SkillContentReceiptsFor returns summaries: no managed files. Viewer, disabled
// and foreign accounts see nothing. The managed manifest is parsed only to report
// its digest and file count; comparing it with its revision happens on item reads
// and when a receipt is used as a baseline, so a summary is not a verification.
// A damaged row does not hide the others: it is listed with Damaged set, because
// its version is the only way to release it.
func (s *Store) SkillContentReceiptsFor(uid int64) ([]SkillContentReceipt, error) {
	rows, err := s.db.Query(`SELECT `+receiptCols+` FROM skill_content_receipts WHERE user_id=? AND `+privateSkillAccount+` ORDER BY environment_id,name,id`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SkillContentReceipt{}
	for rows.Next() {
		row, err := scanStoredReceipt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row.summary())
	}
	return out, rows.Err()
}

// SkillContentReceiptFor returns one receipt with its managed file metadata, after
// proving that manifest against the recorded revision.
func (s *Store) SkillContentReceiptFor(uid int64, id string) (out SkillContentReceipt, err error) {
	if !contentobs.ValidID(id) {
		return out, ErrSkillContentReceiptMissing
	}
	err = s.withTx(func(tx *sql.Tx) error {
		row, e := scanStoredReceipt(tx.QueryRow(`SELECT `+receiptCols+` FROM skill_content_receipts WHERE user_id=? AND id=? AND `+privateSkillAccount, uid, id, uid))
		if errors.Is(e, sql.ErrNoRows) {
			return ErrSkillContentReceiptMissing
		}
		if e != nil {
			return e
		}
		item, e := row.verified(tx, uid)
		if errors.Is(e, ErrPrivateSkillMissing) {
			return ErrSkillContentReceiptMissing
		}
		if e != nil {
			return e
		}
		out = item
		return nil
	})
	return
}

// ReleaseSkillContentReceiptFor ends ownership of one slot and nothing else: the
// files on the machine, the pin and the private revision are untouched.
func (s *Store) ReleaseSkillContentReceiptFor(uid int64, id, version string) error {
	if !contentobs.ValidID(id) || !contentobs.ValidID(version) {
		return ErrSkillContentReceiptInvalid
	}
	return s.withTx(func(tx *sql.Tx) error {
		if e := skillContentReceiptOwner(tx, uid); e != nil {
			return e
		}
		var current string
		e := tx.QueryRow(`SELECT version FROM skill_content_receipts WHERE user_id=? AND id=?`, uid, id).Scan(&current)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrSkillContentReceiptMissing
		}
		if e != nil {
			return e
		}
		if current != version {
			return ErrSkillContentReceipt
		}
		_, e = tx.Exec(`DELETE FROM skill_content_receipts WHERE user_id=? AND id=? AND version=?`, uid, id, version)
		return e
	})
}

// skillContentReceiptBaseline returns the planner baseline for one slot and the
// receipt version it was read at, or (nil, "", nil) when the slot has no receipt.
// The target is rebuilt from the row, so a renamed environment stays consistent.
// A receipt that fails validation is an error, never an absent baseline. Callers
// hold the account guard (privateSkillOwner) in the surrounding transaction.
func skillContentReceiptBaseline(q privateSkillQuery, uid int64, environmentID, name string) (*skillplan.Baseline, string, error) {
	row, err := scanStoredReceipt(q.QueryRow(`SELECT `+receiptCols+` FROM skill_content_receipts WHERE user_id=? AND environment_id=? AND name=?`, uid, environmentID, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	receipt, err := row.verified(q, uid)
	if errors.Is(err, ErrPrivateSkillMissing) {
		// The receipt exists, so an unreadable revision is damage, not an absent receipt.
		return nil, "", errReceiptIntegrity
	}
	if err != nil {
		return nil, "", err
	}
	return &skillplan.Baseline{
		Target:     skillplan.Target{OwnerID: uid, AgentID: row.AgentID, EnvironmentID: row.EnvironmentID, Generation: row.AgentID, RootID: row.rootID, SkillName: row.Name, Adapter: row.adapter, Schema: row.schema},
		BundleID:   row.BundleID,
		RevisionID: row.RevisionID,
		Manifest:   *receipt.Managed,
	}, row.Version, nil
}

// currentReceiptVersion is the version of the slot's receipt, or "" when it has
// none. Previews bind it so a release and re-adoption invalidates old reviews.
func currentReceiptVersion(q previewQuerier, uid int64, environmentID, name string) (string, error) {
	var version string
	err := q.QueryRow(`SELECT version FROM skill_content_receipts WHERE user_id=? AND environment_id=? AND name=?`, uid, environmentID, name).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return version, err
}
