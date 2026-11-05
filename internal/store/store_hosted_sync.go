package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// A skill someone wrote on the Hosting page can reach their machines without
// being published: the agent already installs private bundle revisions from
// the controller with its own token, so a draft is served that way. The
// binding below remembers which immutable revision holds the draft's current
// bytes, keyed by hosted slug rather than a shelf skill, because a draft has
// no shelf entry. Nothing here publishes, and the revisions are only readable
// by the owning account.
const hostedSyncSourcesTable = `CREATE TABLE IF NOT EXISTS hosted_sync_sources (user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, slug TEXT NOT NULL, bundle_id TEXT NOT NULL, revision_id TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(user_id,slug))`

func hostedContentFiles(sk HostedSkill) []skillbundle.ContentFile {
	files := []skillbundle.ContentFile{{Path: "SKILL.md", Data: []byte(sk.Content)}}
	paths := make([]string, 0, len(sk.Files))
	for path := range sk.Files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		files = append(files, skillbundle.ContentFile{Path: path, Data: sk.Files[path]})
	}
	return files
}

// hostedPrivateSource returns the private sync source for one hosted skill,
// appending a revision only when the skill's bytes differ from the bound one.
// Listing candidates for an unchanged draft writes nothing.
func (s *Store) hostedPrivateSource(uid int64, sk HostedSkill) (string, error) {
	files := hostedContentFiles(sk)
	m, err := skillbundle.FromContents(files)
	if err != nil {
		return "", fmt.Errorf("%s cannot be synced privately: %w", sk.Slug, err)
	}
	var bundle, revision, digest string
	err = s.db.QueryRow(`SELECT bundle_id,revision_id,digest FROM hosted_sync_sources WHERE user_id=? AND slug=?`, uid, sk.Slug).Scan(&bundle, &revision, &digest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if err == nil && digest == m.Digest {
		return "private:" + bundle + ":" + revision + ":" + digest, nil
	}
	base := ""
	if bundle != "" {
		head, err := s.PrivateSkillBundleFor(uid, bundle)
		if err != nil {
			return "", err
		}
		base = head.Head
	}
	// The request ID makes a retried write idempotent: the same draft bytes on
	// the same base never produce two revisions.
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%s\x00%s\x00%s", uid, sk.Slug, base, m.Digest)))
	in := PrivateSkillWrite{RequestID: "hosted_" + hex.EncodeToString(sum[:])[:40], ExpectedBase: &base, Name: sk.Slug, Origin: PrivateSkillOrigin{Kind: "user", Reference: "hosted:" + sk.Slug}, Files: files}
	result, err := s.savePrivateSkillRevisionFor(uid, bundle, in, nil)
	if err != nil {
		if errors.Is(err, ErrPrivateSkillCapacity) {
			// Old revisions and the bundles of deleted written skills are freed on
			// demand, so this means every slot holds a skill that is still in use.
			// Deleting a written skill is the one thing a person can do about that, and
			// it does free its slot.
			return "", fmt.Errorf("%s cannot be synced privately: private skill storage is full of skills still in use (at most %d skills or %d MiB per account). Delete a written skill you no longer need, then sync again.", sk.Slug, MaxPrivateSkillBundles, MaxPrivateSkillBytes>>20)
		}
		return "", fmt.Errorf("%s cannot be synced privately: %w", sk.Slug, err)
	}
	if _, err := s.db.Exec(`INSERT INTO hosted_sync_sources(user_id,slug,bundle_id,revision_id,digest) VALUES(?,?,?,?,?) ON CONFLICT(user_id,slug) DO UPDATE SET bundle_id=excluded.bundle_id,revision_id=excluded.revision_id,digest=excluded.digest`, uid, sk.Slug, result.Bundle.ID, result.Revision.ID, result.Revision.ManifestDigest); err != nil {
		return "", err
	}
	return "private:" + result.Bundle.ID + ":" + result.Revision.ID + ":" + result.Revision.ManifestDigest, nil
}

func (s *Store) forgetHostedSyncSource(uid int64, slug string) error {
	_, err := s.db.Exec(`DELETE FROM hosted_sync_sources WHERE user_id=? AND slug=?`, uid, slug)
	return err
}

// privateDraftCandidate is the sync candidate for one unpublished hosted skill.
// A failure to prepare its revision is carried on the candidate as a problem,
// so one full store or broken draft never hides the account's other skills.
func (s *Store) privateDraftCandidate(uid int64, id string, sk HostedSkill) SyncSkill {
	source, err := s.hostedPrivateSource(uid, sk)
	if err != nil {
		return SyncSkill{ID: id, Name: sk.Slug, Slug: sk.Slug, SourceType: "local", Problem: err.Error()}
	}
	return SyncSkill{ID: id, Name: sk.Slug, Slug: sk.Slug, Source: source, SourceType: "private"}
}

// applyDraftSources replaces a shelf entry that points at this account's own
// hosted skill, when that skill is currently unpublished, with its private
// draft candidate. Publishing adds a shelf entry and unpublishing keeps it, so
// without this the shelf would still offer a public URL that no longer serves.
func (s *Store) applyDraftSources(uid int64, candidates []SyncSkill) ([]SyncSkill, error) {
	username, err := s.PublishingUsername(uid)
	if err != nil || username == "" {
		return candidates, err
	}
	hosted, err := s.HostedSkillsFor(uid)
	if err != nil {
		return nil, err
	}
	drafts := map[string]HostedSkill{}
	for _, sk := range hosted {
		if !sk.Published {
			drafts[sk.Slug] = sk
		}
	}
	if len(drafts) == 0 {
		return candidates, nil
	}
	for i, sk := range candidates {
		draft, ok := drafts[sk.Slug]
		hostedDraft := ok && sk.SourceType == "hosted" && strings.HasSuffix(sk.ID, "/"+username+"/"+sk.Slug)
		if !hostedDraft {
			continue
		}
		candidates[i] = s.privateDraftCandidate(uid, sk.ID, draft)
	}
	return candidates, nil
}
