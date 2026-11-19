package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// SkillPublishProblem is why a library skill cannot be published, in plain
// words for the person.
type SkillPublishProblem struct{ Reason string }

func (p SkillPublishProblem) Error() string { return p.Reason }

// SkillPublished is a library skill's public page after Publish or Unpublish:
// its name in the owner's hosted skills ("" when it has none) and whether it
// is public.
type SkillPublished struct {
	Slug      string `json:"slug"`
	Published bool   `json:"published"`
	// Written says the person wrote the skill in Write a skill, whose
	// switch also puts its public entry on the shelf or takes it off.
	Written bool `json:"-"`
}

// PublishSkillFor makes a library skill public, or private again. A skill the
// person wrote keeps the content Write a skill holds and only changes its
// switch. Any other skill (imported, or kept from a machine) is published from
// its head revision, which becomes its hosted skill: its public page then
// serves what every machine has. Unpublishing keeps the hosted skill as a
// draft, so publishing again keeps the same address.
func (s *Store) PublishSkillFor(uid int64, item string, public bool) (SkillPublished, error) {
	var out SkillPublished
	err := s.withTx(func(tx *sql.Tx) error {
		var name, origin string
		err := tx.QueryRow(`SELECT name,origin FROM library_items WHERE user_id=? AND id=? AND item_type=? AND deleted_at=0`, uid, item, LibrarySkill).Scan(&name, &origin)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPrivateSkillMissing
		}
		if err != nil {
			return err
		}
		slug, hosted, err := itemHostedSlugTx(tx, uid, item)
		if err != nil {
			return err
		}
		if !public {
			out = SkillPublished{Slug: slug, Written: hosted && origin == "written"}
			if !hosted {
				return nil
			}
			_, err = tx.Exec(`UPDATE hosted_skills SET published=0,updated_at=? WHERE user_id=? AND slug=?`, now(), uid, slug)
			return err
		}
		if hosted && origin == "written" {
			out = SkillPublished{Slug: slug, Published: true, Written: true}
			_, err = tx.Exec(`UPDATE hosted_skills SET published=1,updated_at=? WHERE user_id=? AND slug=?`, now(), uid, slug)
			return err
		}
		if !hosted {
			slug = name
		}
		if err = hostedFromHeadTx(tx, uid, item, slug, true); err != nil {
			return err
		}
		out = SkillPublished{Slug: slug, Published: true}
		return nil
	})
	return out, err
}

// RefreshHostedSkillFor brings a library skill's hosted skill up to its head
// revision after its content was edited, keeping it public or private as it
// was. It does nothing for a skill with no hosted skill. A problem means the
// public page still shows what it did before.
func (s *Store) RefreshHostedSkillFor(uid int64, item string) error {
	return s.withTx(func(tx *sql.Tx) error {
		slug, hosted, err := itemHostedSlugTx(tx, uid, item)
		if err != nil || !hosted {
			return err
		}
		var published bool
		if err = tx.QueryRow(`SELECT published FROM hosted_skills WHERE user_id=? AND slug=?`, uid, slug).Scan(&published); err != nil {
			return err
		}
		return hostedFromHeadTx(tx, uid, item, slug, published)
	})
}

// refreshHostedTx brings the item's hosted skill, when it has one, up to the
// item's head revision, keeping it public or private as it was. Content a
// public page can't serve (SkillPublishProblem) leaves the page as it was
// rather than refusing the content; RefreshHostedSkillFor says why.
func refreshHostedTx(tx *sql.Tx, uid int64, item string) error {
	slug, hosted, err := itemHostedSlugTx(tx, uid, item)
	if err != nil || !hosted {
		return err
	}
	var published bool
	if err = tx.QueryRow(`SELECT published FROM hosted_skills WHERE user_id=? AND slug=?`, uid, slug).Scan(&published); err != nil {
		return err
	}
	err = hostedFromHeadTx(tx, uid, item, slug, published)
	if errors.As(err, new(SkillPublishProblem)) {
		return nil
	}
	return err
}

// itemHostedSlugTx is the item's name in the owner's hosted skills, when it
// has a hosted skill.
func itemHostedSlugTx(tx *sql.Tx, uid int64, item string) (string, bool, error) {
	var slug string
	err := tx.QueryRow(`SELECT h.slug FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=? AND a.item_id=? AND a.kind='hosted' ORDER BY h.slug LIMIT 1`, uid, item).Scan(&slug)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return slug, err == nil, err
}

// hostedHeadTx is the hosted skill slug the item's head revision would be,
// with the bundle, revision and manifest digest it comes from, or a
// SkillPublishProblem saying why a public page can't serve it.
func hostedHeadTx(tx *sql.Tx, uid int64, item, slug string) (sk HostedSkill, bundle, revision, digest string, err error) {
	var name string
	if err = tx.QueryRow(`SELECT name,bundle_id,revision_id FROM library_items WHERE user_id=? AND id=?`, uid, item).Scan(&name, &bundle, &revision); err != nil {
		return
	}
	if !ValidHostedName(slug) {
		err = SkillPublishProblem{fmt.Sprintf("reet.sh can only publish a skill whose name uses lowercase letters, numbers and single hyphens, so %s stays private.", name)}
		return
	}
	if bundle == "" || revision == "" {
		err = SkillPublishProblem{fmt.Sprintf("reet has no copy of %s yet, so there is nothing to publish.", name)}
		return
	}
	var username string
	err = tx.QueryRow(`SELECT username FROM publishers WHERE user_id=?`, uid).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && username == "") {
		err = SkillPublishProblem{"Choose a publishing username in Write a skill first; it is the first part of every public address."}
		return
	}
	if err != nil {
		return
	}
	rev, err := privateSkillRevision(tx, uid, bundle, revision, true)
	if err != nil {
		return
	}
	sk = HostedSkill{Slug: slug, Files: map[string][]byte{}}
	for _, f := range rev.Files {
		if f.Path == "SKILL.md" {
			sk.Content = string(f.Data)
		} else {
			sk.Files[f.Path] = f.Data
		}
	}
	if e := validateSkillContent(&sk, func(md string) bool { return md == slug }); e != nil {
		err = SkillPublishProblem{fmt.Sprintf("%s can't be published yet: %s.", name, e.Error())}
		return
	}
	owner, e := libraryAliasID(tx, uid, "hosted", slug)
	if e == nil && owner != item {
		err = SkillPublishProblem{fmt.Sprintf("You already have a written skill named %s, so this one can't be published under that name.", slug)}
		return
	}
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		err = e
		return
	}
	return sk, bundle, revision, rev.ManifestDigest, nil
}

// hostedFromHeadTx writes the item's head revision as its hosted skill slug,
// public or not, and binds the hosted skill to that revision, so Write a skill
// edits it and the next save of it becomes the item's next revision rather
// than a skill of its own.
func hostedFromHeadTx(tx *sql.Tx, uid int64, item, slug string, published bool) error {
	sk, bundle, revision, digest, err := hostedHeadTx(tx, uid, item, slug)
	if err != nil {
		return err
	}
	sk.Published = published
	// The alias first: the hosted insert then joins this item instead of
	// making a written skill of its own.
	if _, err = tx.Exec(`INSERT INTO library_aliases VALUES(?,'hosted',?,?) ON CONFLICT(user_id,kind,reference) DO NOTHING`, uid, slug, item); err != nil {
		return err
	}
	files, err := json.Marshal(sk.Files)
	if err != nil {
		return err
	}
	at := now()
	if _, err = tx.Exec(`INSERT INTO hosted_skills(user_id,slug,description,content,files,published,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)
 ON CONFLICT(user_id,slug) DO UPDATE SET description=excluded.description,content=excluded.content,files=excluded.files,published=excluded.published,updated_at=excluded.updated_at`, uid, slug, sk.Description, sk.Content, string(files), sk.Published, at, at); err != nil {
		return err
	}
	editor, err := skillbundle.FromContents(hostedContentFiles(sk))
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO hosted_sync_sources(user_id,slug,bundle_id,revision_id,digest) VALUES(?,?,?,?,?) ON CONFLICT(user_id,slug) DO UPDATE SET bundle_id=excluded.bundle_id,revision_id=excluded.revision_id,digest=excluded.digest`, uid, slug, bundle, revision, digest); err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO library_hosted_editor_digests VALUES(?,?,?) ON CONFLICT(user_id,slug) DO UPDATE SET digest=excluded.digest`, uid, slug, editor.Digest)
	return err
}

// HostedSkillImportedFrom is the original link of a published skill that was
// imported, for its public page to credit; "" for any other skill.
func (s *Store) HostedSkillImportedFrom(username, slug string) (string, error) {
	var link string
	err := s.db.QueryRow(`SELECT i.origin_ref FROM publishers p JOIN library_aliases a ON a.user_id=p.user_id AND a.kind='hosted' AND a.reference=? JOIN library_items i ON i.user_id=a.user_id AND i.id=a.item_id WHERE p.username=? AND i.origin='imported' AND i.deleted_at=0`, slug, username).Scan(&link)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return link, err
}

// SkillPublicProblemFor says, in plain words, why a published skill's public
// page no longer follows it: its head is content a public page can't serve,
// so the page keeps the version before. "" when it follows, or is not public.
func (s *Store) SkillPublicProblemFor(uid int64, item string) (string, error) {
	out := ""
	err := s.withTx(func(tx *sql.Tx) error {
		slug, hosted, err := itemHostedSlugTx(tx, uid, item)
		if err != nil || !hosted {
			return err
		}
		var published bool
		if err = tx.QueryRow(`SELECT published FROM hosted_skills WHERE user_id=? AND slug=?`, uid, slug).Scan(&published); err != nil || !published {
			return err
		}
		_, _, _, _, err = hostedHeadTx(tx, uid, item, slug)
		var problem SkillPublishProblem
		if errors.As(err, &problem) {
			out = "The public page still shows the version before: " + problem.Reason
			return nil
		}
		return err
	})
	return out, err
}

// SkillPublishedFor says whether a library skill has a public page.
func (s *Store) SkillPublishedFor(uid int64, item string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM library_aliases a JOIN hosted_skills h ON h.user_id=a.user_id AND h.slug=a.reference WHERE a.user_id=? AND a.item_id=? AND a.kind='hosted' AND h.published=1`, uid, item).Scan(&n)
	return n > 0, err
}
