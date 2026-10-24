package skillplan

import (
	"errors"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

// ErrNotClaimable reports that a library revision cannot prove ownership of the
// observed folder because its SKILL.md is not byte and mode identical.
var ErrNotClaimable = errors.New("revision does not match the observed SKILL.md")

// ClaimCounts describes how an observed folder overlaps a library revision.
// ObservedOnly counts observed files that were not claimed (local additions or
// files that differ from the revision); RevisionOnly counts revision files that
// the folder does not hold in identical form.
type ClaimCounts struct{ Matched, ObservedOnly, RevisionOnly int }

// ClaimSubset returns the files of observed that provably equal the same path in
// revision. A file is claimed only when its path, size, digest and executable bit
// all agree, so edited or mode-changed files are never claimed. SKILL.md must be
// claimed, otherwise the result is ErrNotClaimable. Both manifests must be
// complete and internally consistent; anything else is a validation error, never
// ErrNotClaimable, so forged or partial input cannot look like an ordinary
// mismatch. A refused claim returns an empty manifest and zero counts.
func ClaimSubset(observed, revision skillbundle.Manifest) (skillbundle.Manifest, ClaimCounts, error) {
	observed, err := canonical(observed)
	if err != nil {
		return skillbundle.Manifest{}, ClaimCounts{}, errors.New("invalid observed manifest")
	}
	revision, err = canonical(revision)
	if err != nil {
		return skillbundle.Manifest{}, ClaimCounts{}, errors.New("invalid revision manifest")
	}
	offered := fileMap(revision)
	matched := []skillbundle.File{}
	hasSkill := false
	for _, f := range observed.Files {
		if o, ok := offered[f.Path]; ok && o == f {
			matched = append(matched, f)
			hasSkill = hasSkill || f.Path == "SKILL.md"
		}
	}
	if !hasSkill {
		return skillbundle.Manifest{}, ClaimCounts{}, ErrNotClaimable
	}
	claimed, err := skillbundle.FromMetadata(matched)
	if err != nil {
		return skillbundle.Manifest{}, ClaimCounts{}, err
	}
	return claimed, ClaimCounts{
		Matched:      len(matched),
		ObservedOnly: len(observed.Files) - len(matched),
		RevisionOnly: len(revision.Files) - len(matched),
	}, nil
}

// IsSubset reports whether every file in managed equals the revision's file at
// the same path. It does not validate either manifest; callers verify stored
// receipts against the revision they record.
func IsSubset(managed, revision skillbundle.Manifest) bool {
	offered := fileMap(revision)
	for _, f := range managed.Files {
		if o, ok := offered[f.Path]; !ok || o != f {
			return false
		}
	}
	return true
}
