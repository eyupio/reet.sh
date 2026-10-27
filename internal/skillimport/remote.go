package skillimport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

const RemoteVersion = 1

type PreviewFile struct {
	skillbundle.File
	Blocked string `json:"blocked,omitempty"`
}
type Preview struct {
	SchemaVersion int           `json:"schemaVersion"`
	SourceDigest  string        `json:"sourceDigest"`
	SkillValid    bool          `json:"skillValid"`
	Files         []PreviewFile `json:"files"`
}

// Manifest validates the preview's full source and any exact excluded paths.
// Hazard labels are metadata; raw received files must pass Select again.
func (p Preview) Manifest(excluded []string) (skillbundle.Manifest, error) {
	var empty skillbundle.Manifest
	if p.SchemaVersion != RemoteVersion {
		return empty, errors.New("unsupported import preview")
	}
	all := make([]skillbundle.File, len(p.Files))
	names := map[string]bool{}
	for i, f := range p.Files {
		all[i] = f.File
		names[f.Path] = true
		switch f.Blocked {
		case "", "credential_path", "key_material", "credential_content":
		default:
			return empty, errors.New("invalid hazard code")
		}
	}
	source, err := skillbundle.FromMetadata(all)
	if err != nil || source.Digest != p.SourceDigest {
		return empty, errors.New("invalid source fingerprint")
	}
	omit := map[string]bool{}
	for _, name := range excluded {
		if name == "SKILL.md" || !names[name] || omit[name] {
			return empty, ErrSelection
		}
		omit[name] = true
	}
	selected := []skillbundle.File{}
	for _, f := range p.Files {
		if !omit[f.Path] {
			selected = append(selected, f.File)
		}
	}
	return skillbundle.FromMetadata(selected)
}
func (p Preview) Allowed(excluded []string) bool {
	if !p.SkillValid {
		return false
	}
	if _, err := p.Manifest(excluded); err != nil {
		return false
	}
	omit := map[string]bool{}
	for _, v := range excluded {
		omit[v] = true
	}
	for _, f := range p.Files {
		if !omit[f.Path] && f.Blocked != "" {
			return false
		}
	}
	return true
}
func (p Preview) Hash() (string, error) {
	if _, err := p.Manifest(nil); err != nil {
		return "", err
	}
	p.Files = append([]PreviewFile{}, p.Files...)
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
