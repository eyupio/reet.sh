// Package contentobs defines bounded, metadata-only skill-content observations.
// A reported digest is an observation, never permission to import or apply bytes.
package contentobs

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

const (
	Version            = 1
	MaxScopes          = 64
	MaxSkills          = 512
	MaxBody            = 1 << 20
	MaxScanBytes int64 = 64 << 20
	FreshMillis  int64 = 5 * 60 * 1000
)

var ErrInvalid = errors.New("invalid content observation")
var hexDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var scanID = regexp.MustCompile(`^[a-f0-9]{48}$`)
var issueCode = regexp.MustCompile(`^[a-z_]{1,64}$`)

type Grant struct {
	SchemaVersion int    `json:"schemaVersion"`
	ScanID        string `json:"scanId"`
	ExpiresAt     int64  `json:"expiresAt"`
}
type Skill struct {
	Name          string   `json:"name"`
	SchemaVersion int      `json:"schemaVersion"`
	Algorithm     string   `json:"algorithm"`
	ModeSemantics string   `json:"modeSemantics"`
	Complete      bool     `json:"complete"`
	Digest        string   `json:"digest,omitempty"`
	Files         int      `json:"files"`
	Bytes         int64    `json:"bytes"`
	Issues        []string `json:"issues,omitempty"`
}
type Scope struct {
	Agent    string  `json:"agent"`
	Project  string  `json:"project"`
	Complete bool    `json:"complete"`
	Skills   []Skill `json:"skills"`
}
type Report struct {
	SchemaVersion int     `json:"schemaVersion"`
	ScanID        string  `json:"scanId"`
	ObservedAt    int64   `json:"observedAt"`
	Scopes        []Scope `json:"scopes"`
}
type Inventory struct {
	State         string  `json:"state"`
	SchemaVersion int     `json:"schemaVersion,omitempty"`
	ObservedAt    int64   `json:"observedAt,omitempty"`
	ReceivedAt    int64   `json:"receivedAt,omitempty"`
	Skills        []Skill `json:"skills"`
}

func ValidID(id string) bool { return scanID.MatchString(id) }
func ValidText(v string, limit int) bool {
	return len(v) <= limit && utf8.ValidString(v) && !strings.ContainsFunc(v, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
	})
}
func ValidScope(agent, project string) bool {
	switch agent {
	case "claude", "agents", "cursor", "codex", "opencode":
	default:
		return false
	}
	return ValidText(project, 1024)
}
func (r Report) Validate() error {
	if r.SchemaVersion != Version || !ValidID(r.ScanID) || r.ObservedAt <= 0 || r.ObservedAt > 253402300799999 || r.Scopes == nil || len(r.Scopes) > MaxScopes {
		return ErrInvalid
	}
	scopes := map[string]bool{}
	count := 0
	var bytes int64
	for _, s := range r.Scopes {
		key := s.Agent + "\x00" + s.Project
		if !ValidScope(s.Agent, s.Project) || scopes[key] || s.Skills == nil {
			return ErrInvalid
		}
		scopes[key] = true
		names := map[string]bool{}
		for _, v := range s.Skills {
			count++
			bytes += v.Bytes
			if bytes > MaxScanBytes || (s.Complete && !v.Complete) {
				return ErrInvalid
			}
			if count > MaxSkills || v.Name == "" || !ValidText(v.Name, 255) || strings.ContainsAny(v.Name, "/\\") || v.Name == "." || v.Name == ".." || names[v.Name] || v.SchemaVersion != skillbundle.SchemaVersion || v.Algorithm != skillbundle.Algorithm || v.ModeSemantics != skillbundle.ModeSemantics || v.Files < 0 || v.Files > skillbundle.MaxFiles || v.Bytes < 0 || v.Bytes > skillbundle.MaxBytes || len(v.Issues) > 8 {
				return ErrInvalid
			}
			names[v.Name] = true
			if v.Complete {
				if !hexDigest.MatchString(v.Digest) || v.Files == 0 || len(v.Issues) != 0 {
					return ErrInvalid
				}
			} else if v.Digest != "" {
				return ErrInvalid
			}
			for _, code := range v.Issues {
				if !issueCode.MatchString(code) {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}
