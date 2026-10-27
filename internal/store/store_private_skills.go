package store

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

const (
	MaxPrivateSkillBundles   = 128
	MaxPrivateSkillRevisions = 256
	MaxPrivateSkillBytes     = 64 << 20
)

var (
	ErrPrivateSkillInvalid  = errors.New("invalid private skill revision")
	ErrPrivateSkillMissing  = errors.New("private skill unavailable")
	ErrPrivateSkillConflict = errors.New("private skill revision changed or request ID was reused")
	ErrPrivateSkillCapacity = errors.New("private skill storage limit reached")
	privateSkillID          = regexp.MustCompile(`^[a-f0-9]{32}$`)
	privateSkillRequestID   = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)
)

type PrivateSkillOrigin struct {
	Kind        string `json:"kind"`
	Reference   string `json:"reference,omitempty"`
	DerivedFrom string `json:"derivedFrom,omitempty"`
}
type PrivateSkillBundle struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Head      string `json:"head"`
	CreatedAt int64  `json:"createdAt"`
}
type PrivateSkillRevision struct {
	ID             string                    `json:"id"`
	BundleID       string                    `json:"bundleId"`
	Parent         string                    `json:"parent,omitempty"`
	ManifestDigest string                    `json:"manifestDigest"`
	Origin         PrivateSkillOrigin        `json:"origin"`
	CreatedAt      int64                     `json:"createdAt"`
	Manifest       *skillbundle.Manifest     `json:"manifest,omitempty"`
	Files          []skillbundle.ContentFile `json:"files,omitempty"`
}
type PrivateSkillWrite struct {
	RequestID    string                    `json:"requestId"`
	ExpectedBase *string                   `json:"expectedBase"`
	Name         string                    `json:"name"`
	Origin       PrivateSkillOrigin        `json:"origin"`
	Files        []skillbundle.ContentFile `json:"files"`
}
type PrivateSkillResult struct {
	Bundle   PrivateSkillBundle   `json:"bundle"`
	Revision PrivateSkillRevision `json:"revision"`
}

func (s *Store) migratePrivateSkills() error {
	return s.withTx(func(tx *sql.Tx) error {
		for _, stmt := range []string{
			`CREATE TABLE IF NOT EXISTS private_skill_bundles(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,id TEXT NOT NULL,name TEXT NOT NULL,head TEXT NOT NULL DEFAULT '',created_at INTEGER NOT NULL,PRIMARY KEY(user_id,id))`,
			`CREATE TABLE IF NOT EXISTS private_skill_blobs(user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,digest TEXT NOT NULL,data BLOB NOT NULL,PRIMARY KEY(user_id,digest))`,
			`CREATE TABLE IF NOT EXISTS private_skill_revisions(user_id INTEGER NOT NULL,bundle_id TEXT NOT NULL,id TEXT NOT NULL,parent TEXT NOT NULL,request_id TEXT NOT NULL,fingerprint TEXT NOT NULL,manifest TEXT NOT NULL,manifest_digest TEXT NOT NULL,origin TEXT NOT NULL,created_at INTEGER NOT NULL,PRIMARY KEY(user_id,bundle_id,id),UNIQUE(user_id,id),UNIQUE(user_id,request_id),FOREIGN KEY(user_id,bundle_id) REFERENCES private_skill_bundles(user_id,id) ON DELETE CASCADE)`,
			`CREATE TABLE IF NOT EXISTS private_skill_revision_files(user_id INTEGER NOT NULL,bundle_id TEXT NOT NULL,revision_id TEXT NOT NULL,path TEXT NOT NULL,digest TEXT NOT NULL,executable INTEGER NOT NULL,PRIMARY KEY(user_id,bundle_id,revision_id,path),FOREIGN KEY(user_id,bundle_id,revision_id) REFERENCES private_skill_revisions(user_id,bundle_id,id) ON DELETE CASCADE,FOREIGN KEY(user_id,digest) REFERENCES private_skill_blobs(user_id,digest))`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	})
}
func privateSkillRandomID() (string, error) {
	var raw [16]byte
	_, err := rand.Read(raw[:])
	return hex.EncodeToString(raw[:]), err
}
func privateSkillOwner(tx *sql.Tx, uid int64) error {
	result, err := tx.Exec(`UPDATE users SET id=id WHERE id=? AND disabled_at=0 AND role<>'viewer'`, uid)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrPrivateSkillMissing
	}
	return nil
}
func validatePrivateSkill(in PrivateSkillWrite) (skillbundle.Manifest, error) {
	if !privateSkillRequestID.MatchString(in.RequestID) || in.ExpectedBase == nil || (*in.ExpectedBase != "" && !privateSkillID.MatchString(*in.ExpectedBase)) || !ValidHostedName(in.Name) {
		return skillbundle.Manifest{}, ErrPrivateSkillInvalid
	}
	if in.Origin.Kind != "user" && in.Origin.Kind != "import" && in.Origin.Kind != "fork" {
		return skillbundle.Manifest{}, ErrPrivateSkillInvalid
	}
	if len(in.Origin.Reference) > 2048 || !utf8.ValidString(in.Origin.Reference) || (in.Origin.DerivedFrom != "" && !privateSkillID.MatchString(in.Origin.DerivedFrom)) {
		return skillbundle.Manifest{}, ErrPrivateSkillInvalid
	}
	for _, r := range in.Origin.Reference {
		if unicode.IsControl(r) {
			return skillbundle.Manifest{}, ErrPrivateSkillInvalid
		}
	}
	m, err := skillbundle.FromContents(in.Files)
	if err != nil {
		return m, ErrPrivateSkillInvalid
	}
	for _, f := range in.Files {
		if f.Path == "SKILL.md" {
			// Validate an isolated copy: hosted validation normalizes line endings, while
			// private identity and storage must retain the exact submitted bytes/modes.
			check := HostedSkill{Slug: in.Name, Content: string(f.Data)}
			if ValidateHostedSkill(&check) != nil {
				return m, ErrPrivateSkillInvalid
			}
		}
	}
	return m, nil
}

// SavePrivateSkillRevisionFor atomically creates an immutable revision and moves
// only its owned bundle's head. A digest never identifies an account or revision.
// Reusing an identical request returns its original receipt, even after later edits.
func (s *Store) SavePrivateSkillRevisionFor(uid int64, id string, in PrivateSkillWrite) (PrivateSkillResult, error) {
	return s.savePrivateSkillRevisionFor(uid, id, in, nil)
}
func (s *Store) savePrivateSkillRevisionFor(uid int64, id string, in PrivateSkillWrite, guard func(*sql.Tx) error) (PrivateSkillResult, error) {
	return s.savePrivateSkillRevisionWithReceipt(uid, id, in, guard, nil)
}
func (s *Store) savePrivateSkillRevisionWithReceipt(uid int64, id string, in PrivateSkillWrite, guard func(*sql.Tx) error, finish func(*sql.Tx, PrivateSkillResult) error) (out PrivateSkillResult, err error) {
	if id != "" && !privateSkillID.MatchString(id) {
		return out, ErrPrivateSkillMissing
	}
	if in.Origin.Kind == "" {
		in.Origin.Kind = "user"
	}
	m, err := validatePrivateSkill(in)
	if err != nil {
		return out, err
	}
	manifest, _ := json.Marshal(m)
	origin, _ := json.Marshal(in.Origin)
	fingerprintData, _ := json.Marshal(struct {
		Target, Name, Base, Digest string
		Origin                     PrivateSkillOrigin
	}{id, in.Name, *in.ExpectedBase, m.Digest, in.Origin})
	fingerprintSum := sha256.Sum256(fingerprintData)
	fingerprint := hex.EncodeToString(fingerprintSum[:])
	err = s.withTx(func(tx *sql.Tx) error {
		if err := privateSkillOwner(tx, uid); err != nil {
			return err
		}
		if guard != nil {
			if err := guard(tx); err != nil {
				return err
			}
		}
		var priorBundle, priorRevision, priorFingerprint string
		e := tx.QueryRow(`SELECT bundle_id,id,fingerprint FROM private_skill_revisions WHERE user_id=? AND request_id=?`, uid, in.RequestID).Scan(&priorBundle, &priorRevision, &priorFingerprint)
		if e == nil {
			if priorFingerprint != fingerprint {
				return ErrPrivateSkillConflict
			}
			var err error
			out.Bundle, err = privateSkillBundle(tx, uid, priorBundle)
			if err != nil {
				return err
			}
			// The receipt describes the original write, not a later head movement.
			out.Bundle.Head = priorRevision
			out.Revision, err = privateSkillRevision(tx, uid, priorBundle, priorRevision, false)
			if err == nil && finish != nil {
				return finish(tx, out)
			}
			return err
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		if id == "" {
			if *in.ExpectedBase != "" {
				return ErrPrivateSkillConflict
			}
			var n int
			if err := tx.QueryRow(`SELECT count(*) FROM private_skill_bundles WHERE user_id=?`, uid).Scan(&n); err != nil {
				return err
			}
			if n >= MaxPrivateSkillBundles {
				return ErrPrivateSkillCapacity
			}
			id, e = privateSkillRandomID()
			if e != nil {
				return e
			}
			if _, e = tx.Exec(`INSERT INTO private_skill_bundles(user_id,id,name,created_at) VALUES(?,?,?,?)`, uid, id, in.Name, now()); e != nil {
				return e
			}
		}
		b, e := privateSkillBundle(tx, uid, id)
		if e != nil {
			return e
		}
		if b.Name != in.Name || b.Head != *in.ExpectedBase {
			return ErrPrivateSkillConflict
		}
		if in.Origin.DerivedFrom != "" {
			var n int
			if e = tx.QueryRow(`SELECT count(*) FROM private_skill_revisions WHERE user_id=? AND id=?`, uid, in.Origin.DerivedFrom).Scan(&n); e != nil {
				return e
			}
			if n != 1 {
				return ErrPrivateSkillMissing
			}
		}
		var revisions int
		if e = tx.QueryRow(`SELECT count(*) FROM private_skill_revisions WHERE user_id=?`, uid).Scan(&revisions); e != nil {
			return e
		}
		if revisions >= MaxPrivateSkillRevisions {
			return ErrPrivateSkillCapacity
		}
		rid, e := privateSkillRandomID()
		if e != nil {
			return e
		}
		created := now()
		if _, e = tx.Exec(`INSERT INTO private_skill_revisions(user_id,bundle_id,id,parent,request_id,fingerprint,manifest,manifest_digest,origin,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uid, id, rid, b.Head, in.RequestID, fingerprint, string(manifest), m.Digest, string(origin), created); e != nil {
			return e
		}
		for _, f := range in.Files {
			sum := sha256.Sum256(f.Data)
			digest := hex.EncodeToString(sum[:])
			data := f.Data
			if data == nil {
				data = []byte{}
			}
			if _, e = tx.Exec(`INSERT INTO private_skill_blobs(user_id,digest,data) VALUES(?,?,?) ON CONFLICT(user_id,digest) DO NOTHING`, uid, digest, data); e != nil {
				return e
			}
			if _, e = tx.Exec(`INSERT INTO private_skill_revision_files(user_id,bundle_id,revision_id,path,digest,executable) VALUES(?,?,?,?,?,?)`, uid, id, rid, f.Path, digest, f.Executable); e != nil {
				return e
			}
		}
		var total int64
		if e = tx.QueryRow(`SELECT coalesce(sum(length(data)),0) FROM private_skill_blobs WHERE user_id=?`, uid).Scan(&total); e != nil {
			return e
		}
		if total > MaxPrivateSkillBytes {
			return ErrPrivateSkillCapacity
		}
		result, e := tx.Exec(`UPDATE private_skill_bundles SET head=? WHERE user_id=? AND id=? AND head=?`, rid, uid, id, b.Head)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return ErrPrivateSkillConflict
		}
		b.Head = rid
		out = PrivateSkillResult{Bundle: b, Revision: PrivateSkillRevision{ID: rid, BundleID: id, Parent: *in.ExpectedBase, ManifestDigest: m.Digest, Origin: in.Origin, CreatedAt: created, Manifest: &m}}
		if finish != nil {
			return finish(tx, out)
		}
		return nil
	})
	return out, err
}

type privateSkillQuery interface {
	QueryRow(string, ...any) *sql.Row
	Query(string, ...any) (*sql.Rows, error)
}

const privateSkillAccount = `EXISTS(SELECT 1 FROM users WHERE id=? AND disabled_at=0 AND role<>'viewer')`

func privateSkillBundle(q privateSkillQuery, uid int64, id string) (b PrivateSkillBundle, err error) {
	err = q.QueryRow(`SELECT id,name,head,created_at FROM private_skill_bundles WHERE user_id=? AND id=? AND `+privateSkillAccount, uid, id, uid).Scan(&b.ID, &b.Name, &b.Head, &b.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrPrivateSkillMissing
	}
	return
}
func (s *Store) PrivateSkillBundleFor(uid int64, id string) (PrivateSkillBundle, error) {
	return privateSkillBundle(s.db, uid, id)
}
func (s *Store) PrivateSkillBundlesFor(uid int64) ([]PrivateSkillBundle, error) {
	rows, err := s.db.Query(`SELECT id,name,head,created_at FROM private_skill_bundles WHERE user_id=? AND `+privateSkillAccount+` ORDER BY name,id LIMIT ?`, uid, uid, MaxPrivateSkillBundles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PrivateSkillBundle{}
	for rows.Next() {
		var b PrivateSkillBundle
		if err = rows.Scan(&b.ID, &b.Name, &b.Head, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
func privateSkillRevision(q privateSkillQuery, uid int64, bundle, id string, contents bool) (r PrivateSkillRevision, err error) {
	var origin, manifest string
	err = q.QueryRow(`SELECT id,bundle_id,parent,manifest_digest,origin,created_at,manifest FROM private_skill_revisions WHERE user_id=? AND bundle_id=? AND id=? AND `+privateSkillAccount, uid, bundle, id, uid).Scan(&r.ID, &r.BundleID, &r.Parent, &r.ManifestDigest, &origin, &r.CreatedAt, &manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrPrivateSkillMissing
	}
	if err != nil {
		return r, err
	}
	if err = json.Unmarshal([]byte(origin), &r.Origin); err != nil {
		return r, err
	}
	var m skillbundle.Manifest
	if err = json.Unmarshal([]byte(manifest), &m); err != nil {
		return r, err
	}
	r.Manifest = &m
	if !contents {
		return r, nil
	}
	rows, err := q.Query(`SELECT f.path,b.data,f.executable FROM private_skill_revision_files f JOIN private_skill_blobs b ON b.user_id=f.user_id AND b.digest=f.digest WHERE f.user_id=? AND f.bundle_id=? AND f.revision_id=? ORDER BY f.path`, uid, bundle, id)
	if err != nil {
		return r, err
	}
	r.Files = []skillbundle.ContentFile{}
	for rows.Next() {
		var f skillbundle.ContentFile
		if err = rows.Scan(&f.Path, &f.Data, &f.Executable); err != nil {
			rows.Close()
			return r, err
		}
		if f.Data == nil {
			f.Data = []byte{}
		}
		r.Files = append(r.Files, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return r, err
	}
	recomputed, err := skillbundle.FromContents(r.Files)
	expected, _ := json.Marshal(m)
	actual, _ := json.Marshal(recomputed)
	if err != nil || recomputed.Digest != r.ManifestDigest || !bytes.Equal(expected, actual) {
		return r, errors.New("private skill stored contents failed integrity validation")
	}
	return r, nil
}
func (s *Store) PrivateSkillRevisionFor(uid int64, bundle, id string) (out PrivateSkillRevision, err error) {
	// One read transaction binds the account/metadata/content snapshot together.
	err = s.withTx(func(tx *sql.Tx) error {
		var e error
		out, e = privateSkillRevision(tx, uid, bundle, id, true)
		return e
	})
	return
}
func (s *Store) PrivateSkillRevisionsFor(uid int64, bundle string) ([]PrivateSkillRevision, error) {
	if _, err := s.PrivateSkillBundleFor(uid, bundle); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id,bundle_id,parent,manifest_digest,origin,created_at FROM private_skill_revisions WHERE user_id=? AND bundle_id=? AND `+privateSkillAccount+` ORDER BY created_at,id LIMIT ?`, uid, bundle, uid, MaxPrivateSkillRevisions)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PrivateSkillRevision{}
	for rows.Next() {
		var r PrivateSkillRevision
		var origin string
		if err = rows.Scan(&r.ID, &r.BundleID, &r.Parent, &r.ManifestDigest, &origin, &r.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(origin), &r.Origin); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PrivateSkillIDValid is shared with transport validation; it conveys no access.
func PrivateSkillIDValid(id string) bool {
	return privateSkillID.MatchString(strings.TrimSpace(id)) && strings.TrimSpace(id) == id
}
