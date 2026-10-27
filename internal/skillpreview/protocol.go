// Package skillpreview defines the metadata a connector reports about one skill
// folder on its own machine, and captures it without following links or reading
// anything outside the connector's configured scope roots. A capture is an
// observation of a changing filesystem: it carries no file bytes, grants no
// ownership and is not permission to write.
package skillpreview

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/skillbundle"
)

const (
	Version   = 1
	Adapter   = "posix-executable-v1"
	MaxIssues = 32
	// Unavailable stands in for a physical-root identity the connector could not
	// establish. It is only valid together with unknown ownership and presence.
	Unavailable = "unavailable"
)

var ErrInvalid = errors.New("invalid skill content capture")

var (
	rootID    = regexp.MustCompile(`^[a-f0-9]{64}$`)
	issueCode = regexp.MustCompile(`^[a-z_]{1,64}$`)
	skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

// Issue explains why a capture is unknown. Paths are relative to the skill folder
// ("." for the folder or scope root itself); no absolute host path ever appears.
type Issue struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

// Capture is what a connector could establish about one skill slot.
//
//   - present: a complete traversal produced Manifest, listing every file,
//     including ones reet does not manage.
//   - absent: the scope root is a verified real directory and holds no entry with
//     this name. Only this state may be read as "not installed".
//   - unknown: anything else. A failed, partial or refused scan is never absence.
//
// RootOwnership says whether the connector could show that no other configured
// scope resolves to the same physical directory. It cannot see other users or
// tools on the machine, so "exclusive" means exclusive among configured scopes.
type Capture struct {
	SchemaVersion int                   `json:"schemaVersion"`
	Adapter       string                `json:"adapter"`
	RootID        string                `json:"rootId"`
	RootOwnership string                `json:"rootOwnership"`
	Presence      string                `json:"presence"`
	Complete      bool                  `json:"complete"`
	Manifest      *skillbundle.Manifest `json:"manifest,omitempty"`
	Issues        []Issue               `json:"issues"`
}

// Roots are connector configuration, never controller input. Target is the scope
// root holding skill folders; Others are every other configured scope root, used
// only to detect aliases. OthersComplete is false when that list was truncated.
type Roots struct {
	Target         string
	Others         []string
	OthersComplete bool
}

func validIssuePath(p string) bool {
	return p == "." || (p != "" && len(p) <= skillbundle.MaxPathBytes && utf8.ValidString(p) && !strings.ContainsFunc(p, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r)
	}))
}

// ValidName reports whether name can be a skill folder this adapter will look up.
func ValidName(name string) bool { return len(name) <= 64 && skillName.MatchString(name) }

// canonical returns the validated manifest in S1 canonical form.
func (c Capture) canonical() (*skillbundle.Manifest, error) {
	m := c.Manifest
	if !m.Complete || len(m.Issues) != 0 || m.SchemaVersion != skillbundle.SchemaVersion || m.Algorithm != skillbundle.Algorithm || m.ModeSemantics != skillbundle.ModeSemantics {
		return nil, ErrInvalid
	}
	canonical, err := skillbundle.FromMetadata(m.Files)
	if err != nil || canonical.Digest != m.Digest || canonical.TotalBytes != m.TotalBytes {
		return nil, ErrInvalid
	}
	return &canonical, nil
}

// Validate rejects any combination that would let a partial or refused scan read
// as a clean one. It does not attest that the host really holds these bytes.
func (c Capture) Validate() error {
	if c.SchemaVersion != Version || c.Adapter != Adapter || c.Issues == nil || len(c.Issues) > MaxIssues {
		return ErrInvalid
	}
	switch c.RootOwnership {
	case "exclusive", "shared", "unknown":
	default:
		return ErrInvalid
	}
	switch {
	case c.RootID == Unavailable:
		if c.RootOwnership != "unknown" || c.Presence != "unknown" {
			return ErrInvalid
		}
	case !rootID.MatchString(c.RootID):
		return ErrInvalid
	}
	for _, issue := range c.Issues {
		if !issueCode.MatchString(issue.Code) || !validIssuePath(issue.Path) {
			return ErrInvalid
		}
	}
	switch c.Presence {
	case "present":
		if !c.Complete || c.Manifest == nil || len(c.Issues) != 0 {
			return ErrInvalid
		}
		if _, err := c.canonical(); err != nil {
			return err
		}
	case "absent":
		if !c.Complete || c.Manifest != nil || len(c.Issues) != 0 {
			return ErrInvalid
		}
	case "unknown":
		if c.Complete || c.Manifest != nil || len(c.Issues) == 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// Hash is SHA-256 of the canonical compact JSON, independent of file and issue
// order. It identifies one observation; it is not a signature or an approval.
func (c Capture) Hash() (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	if c.Manifest != nil {
		m, err := c.canonical()
		if err != nil {
			return "", err
		}
		c.Manifest = m
	}
	c.Issues = append([]Issue{}, c.Issues...)
	sort.Slice(c.Issues, func(i, j int) bool {
		if c.Issues[i].Path != c.Issues[j].Path {
			return c.Issues[i].Path < c.Issues[j].Path
		}
		return c.Issues[i].Code < c.Issues[j].Code
	})
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
