// Package skillimport applies explicit file selection and conservative credential
// blocking to skill uploads. It never executes instructions or changes files.
package skillimport

import (
	"errors"
	"path"
	"regexp"
	"strings"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

var ErrSelection = errors.New("invalid skill file selection")

type BlockedError struct{ Path, Reason string }

func (e *BlockedError) Error() string { return "blocked skill file " + e.Path + ": " + e.Reason }

var credential = regexp.MustCompile(`-----BEGIN (?:[A-Z0-9]+ )*PRIVATE KEY-----|\b(?:sk-(?:proj-)?[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|(?:AKIA|ASIA)[A-Z0-9]{16}|AIza[A-Za-z0-9_-]{30,})\b`)

// Select requires exact existing portable relative file paths for exclusions.
// Unsupported filesystem entries cannot be bypassed by exclusions: capture must
// already have completed. These checks detect common hazards, not every secret.
func Select(files []skillbundle.ContentFile, exclude []string) (skillbundle.Manifest, []skillbundle.ContentFile, error) {
	var empty skillbundle.Manifest
	if _, err := skillbundle.FromContents(files); err != nil {
		return empty, nil, ErrSelection
	}
	names := map[string]bool{}
	for _, f := range files {
		names[f.Path] = true
	}
	omitted := map[string]bool{}
	for _, v := range exclude {
		if v == "SKILL.md" || !names[v] || omitted[v] {
			return empty, nil, ErrSelection
		}
		omitted[v] = true
	}
	selected := []skillbundle.ContentFile{}
	for _, f := range files {
		if omitted[f.Path] {
			continue
		}
		if code := BlockedReason(f); code != "" {
			return empty, nil, &BlockedError{f.Path, BlockDescription(code)}
		}
		selected = append(selected, f)
	}
	m, err := skillbundle.FromContents(selected)
	if err != nil {
		return empty, nil, ErrSelection
	}
	return m, selected, nil
}

// BlockedReason exposes bounded hazard codes for previews, never matched values.
func BlockedReason(f skillbundle.ContentFile) string {
	for _, part := range strings.Split(strings.ToLower(f.Path), "/") {
		switch part {
		case ".git", ".ssh", ".aws", ".env", ".npmrc", ".netrc", "credentials", "credentials.json", "secrets.json", "auth.json", "settings.json", "config.toml", "opencode.json", "claude_desktop_config.json", ".claude.json", ".kube", "id_rsa", "id_ed25519", "id_ecdsa":
			return "credential_path"
		}
		if strings.HasPrefix(part, ".env.") {
			return "credential_path"
		}
	}
	ext := strings.ToLower(path.Ext(f.Path))
	if ext == ".pem" || ext == ".key" || ext == ".p12" || ext == ".pfx" {
		return "key_material"
	}
	if credential.Match(f.Data) {
		return "credential_content"
	}
	return ""
}
func BlockDescription(code string) string {
	switch code {
	case "credential_path":
		return "credential or configuration path"
	case "key_material":
		return "key material"
	case "credential_content":
		return "credential-like content"
	}
	return "unsupported content"
}
