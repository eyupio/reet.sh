// Package skillplan computes bounded content plans without reading or writing a
// filesystem. Inputs must come from authenticated adapters and verified receipts;
// matching metadata alone does not establish ownership or authorise an apply.
package skillplan

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

const Version = 1
const MaxAgeMillis int64 = 5 * 60 * 1000

// Target identifies one skill slot. RootID is an adapter-derived identity for the
// physical root, never a controller-provided filesystem path. Generation changes
// when a connector is re-enrolled; EnvironmentID is opaque (and may contain '/').
type Target struct {
	OwnerID       int64  `json:"ownerId"`
	AgentID       string `json:"agentId"`
	EnvironmentID string `json:"environmentId"`
	Generation    string `json:"generation"`
	RootID        string `json:"rootId"`
	SkillName     string `json:"skillName"`
	Adapter       string `json:"adapter"`
	Schema        int    `json:"schema"`
}

type Revision struct {
	OwnerID    int64                `json:"ownerId"`
	BundleID   string               `json:"bundleId"`
	RevisionID string               `json:"revisionId"`
	Name       string               `json:"name"`
	Manifest   skillbundle.Manifest `json:"manifest"`
}

// Baseline is the last verified managed subset, not necessarily the complete
// installed tree or the original private revision (additive mode retains files).
// Future persistence must create this only from a verified durable apply receipt.
type Baseline struct {
	Target     Target               `json:"target"`
	BundleID   string               `json:"bundleId"`
	RevisionID string               `json:"revisionId"`
	Manifest   skillbundle.Manifest `json:"manifest"`
}

type Observation struct {
	InventoryRevision int64  `json:"inventoryRevision"`
	Generation        string `json:"generation"`
	// Presence is present, absent or unknown. Absent requires a complete adapter
	// observation; a failed/missing scan must use unknown, never absent.
	Presence   string                `json:"presence"`
	Complete   bool                  `json:"complete"`
	ReceivedAt int64                 `json:"receivedAt"`
	Manifest   *skillbundle.Manifest `json:"manifest,omitempty"`
}

type Snapshot struct {
	// Current controller inventory identity, independent of enrollment generation.
	// It changes on every inventory report and must not invalidate stored baseline
	// ownership merely because a new scan was received.
	InventoryRevision int64       `json:"inventoryRevision"`
	Target            Target      `json:"target"`
	Desired           Revision    `json:"desired"`
	Observation       Observation `json:"observation"`
	Baseline          *Baseline   `json:"baseline,omitempty"`
	// RootOwnership is exclusive, shared or unknown, established by the adapter
	// across all scopes resolving to this physical root, not merely this account.
	RootOwnership string `json:"rootOwnership"`
	// additive retains obsolete managed files; managed removes only unchanged
	// baseline-owned obsolete files. Neither mode removes unmanaged files.
	Mode string `json:"mode"`
}

type Action struct {
	Path   string            `json:"path"`
	Kind   string            `json:"kind"` // create, update or remove
	Before *skillbundle.File `json:"before,omitempty"`
	After  *skillbundle.File `json:"after,omitempty"`
}
type Conflict struct {
	Path string `json:"path"`
	Code string `json:"code"`
}
type Preserved struct {
	Path   string `json:"path"`
	Reason string `json:"reason"` // local_only or additive_managed
}

type Plan struct {
	Version       int         `json:"version"`
	Preconditions Snapshot    `json:"preconditions"`
	Ready         bool        `json:"ready"`
	Actions       []Action    `json:"actions"`
	Conflicts     []Conflict  `json:"conflicts"`
	Preserved     []Preserved `json:"preserved"`
	// ExpectedInstalled includes local-only and additive retained files. Comparing
	// it to Desired alone would incorrectly report those files as synchronised.
	ExpectedInstalled *skillbundle.Manifest `json:"expectedInstalled,omitempty"`
	ExpectedManaged   *skillbundle.Manifest `json:"expectedManaged,omitempty"`
	Hash              string                `json:"hash"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
var revisionPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

// Build is deterministic for the same snapshot while it remains fresh. now is
// authoritative server time, not a connector clock. Hash binds the complete
// canonical snapshot and result, including the observation receipt timestamp;
// it is a review identity, not a signed approval or permission to write.
func Build(input Snapshot, now int64) (Plan, error) {
	var empty Plan
	if !bounded(input) {
		return empty, errors.New("planner input exceeds bounds")
	}
	// JSON cloning prevents callers' slice/pointer mutations from altering a plan.
	raw, err := json.Marshal(input)
	if err != nil {
		return empty, err
	}
	var s Snapshot
	if err = json.Unmarshal(raw, &s); err != nil {
		return empty, err
	}
	if now <= 0 || s.InventoryRevision <= 0 || !validTarget(s.Target) || s.Desired.OwnerID != s.Target.OwnerID ||
		!revisionPattern.MatchString(s.Desired.BundleID) || !revisionPattern.MatchString(s.Desired.RevisionID) ||
		s.Desired.Name != s.Target.SkillName || (s.Mode != "additive" && s.Mode != "managed") {
		return empty, errors.New("invalid planner identity or mode")
	}
	if s.RootOwnership != "exclusive" && s.RootOwnership != "shared" && s.RootOwnership != "unknown" {
		return empty, errors.New("invalid root ownership")
	}
	s.Desired.Manifest, err = canonical(s.Desired.Manifest)
	if err != nil {
		return empty, errors.New("invalid desired manifest")
	}
	if b := s.Baseline; b != nil {
		// A baseline for another owner is a caller bug and stays an error. A baseline
		// for another slot or bundle is ordinary state and is reported as a blocked
		// plan once the context fences below have passed.
		if b.Target.OwnerID != s.Target.OwnerID {
			return empty, errors.New("baseline owner mismatch")
		}
		if !revisionPattern.MatchString(b.RevisionID) {
			return empty, errors.New("invalid baseline revision")
		}
		b.Manifest, err = canonical(b.Manifest)
		if err != nil {
			return empty, errors.New("invalid baseline manifest")
		}
	}
	p := Plan{Version: Version, Preconditions: s, Actions: []Action{}, Conflicts: []Conflict{}, Preserved: []Preserved{}}
	// Canonicalise valid observed metadata even when a context fence blocks the
	// plan, so reordered files do not change that diagnostic plan's identity.
	var observationErr error
	if observed := p.Preconditions.Observation.Manifest; observed != nil {
		m, e := canonical(*observed)
		observationErr = e
		if e == nil {
			p.Preconditions.Observation.Manifest = &m
		}
	}
	block := func(code string) (Plan, error) {
		p.Conflicts = append(p.Conflicts, Conflict{Path: ".", Code: code})
		return seal(p)
	}
	if s.Target.Adapter != "posix-executable-v1" || s.Target.Schema != Version {
		return block("adapter_unsupported")
	}
	if s.RootOwnership != "exclusive" {
		return block("root_" + s.RootOwnership)
	}
	o := &p.Preconditions.Observation
	if o.Generation != s.Target.Generation {
		return block("generation_changed")
	}
	if o.InventoryRevision != s.InventoryRevision {
		return block("inventory_changed")
	}
	if !o.Complete || (o.Presence != "present" && o.Presence != "absent") {
		return block("observation_unknown")
	}
	if o.ReceivedAt <= 0 || o.ReceivedAt > now || now-o.ReceivedAt > MaxAgeMillis {
		return block("observation_stale")
	}
	if b := s.Baseline; b != nil {
		if b.Target != s.Target {
			return block("baseline_target_mismatch")
		}
		if b.BundleID != s.Desired.BundleID {
			return block("baseline_bundle_mismatch")
		}
	}
	if o.Presence == "absent" && o.Manifest != nil {
		return empty, errors.New("absent observation has a manifest")
	}
	if o.Presence == "present" {
		if o.Manifest == nil {
			return block("observation_unknown")
		}
		if observationErr != nil {
			return block("observation_invalid")
		}
		if s.Baseline == nil {
			return block("adoption_required")
		}
	} else if s.Baseline != nil {
		return block("managed_target_missing")
	}
	current := map[string]skillbundle.File{}
	base := map[string]skillbundle.File{}
	desired := fileMap(s.Desired.Manifest)
	if o.Manifest != nil {
		current = fileMap(*o.Manifest)
	}
	if s.Baseline != nil {
		base = fileMap(s.Baseline.Manifest)
	}
	paths := map[string]bool{}
	for path := range current {
		paths[path] = true
	}
	for path := range base {
		paths[path] = true
	}
	for path := range desired {
		paths[path] = true
	}
	sorted := make([]string, 0, len(paths))
	for path := range paths {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	installed := map[string]skillbundle.File{}
	managed := map[string]skillbundle.File{}
	for path, f := range current {
		installed[path] = f
	}
	for _, path := range sorted {
		c, hasC := current[path]
		b, hasB := base[path]
		d, hasD := desired[path]
		conflict := func(code string) { p.Conflicts = append(p.Conflicts, Conflict{Path: path, Code: code}) }
		switch {
		case hasD && hasC:
			if !hasB {
				conflict("unmanaged_path")
				continue
			}
			if c == d {
				managed[path] = d
				continue
			}
			if c != b {
				if d == b {
					conflict("local_modified")
				} else {
					conflict("both_modified")
				}
				continue
			}
			p.Actions = append(p.Actions, Action{Path: path, Kind: "update", Before: &c, After: &d})
			installed[path] = d
			managed[path] = d
		case hasD && !hasC:
			if hasB {
				conflict("local_deleted")
				continue
			}
			p.Actions = append(p.Actions, Action{Path: path, Kind: "create", After: &d})
			installed[path] = d
			managed[path] = d
		case !hasD && hasC && hasB:
			if c != b {
				conflict("local_modified")
				continue
			}
			if s.Mode == "managed" {
				p.Actions = append(p.Actions, Action{Path: path, Kind: "remove", Before: &c})
				delete(installed, path)
			} else {
				managed[path] = c
				p.Preserved = append(p.Preserved, Preserved{Path: path, Reason: "additive_managed"})
			}
		case !hasD && hasC:
			p.Preserved = append(p.Preserved, Preserved{Path: path, Reason: "local_only"})
		}
	}
	if len(p.Conflicts) == 0 {
		after, e := skillbundle.FromMetadata(fileValues(installed))
		if e != nil {
			p.Conflicts = append(p.Conflicts, Conflict{Path: ".", Code: "result_invalid"})
		} else {
			owned, e := skillbundle.FromMetadata(fileValues(managed))
			if e != nil {
				return empty, errors.New("invalid managed result")
			}
			p.ExpectedInstalled = &after
			p.ExpectedManaged = &owned
			p.Ready = true
		}
	}
	// A blocked plan is diagnostic only; do not expose a partially applicable set.
	if !p.Ready {
		p.Actions = []Action{}
		p.ExpectedInstalled = nil
		p.ExpectedManaged = nil
	}
	return seal(p)
}

// Bound allocations before cloning. This is not an HTTP ingress limit; future
// endpoints must also enforce request sizes before decoding JSON.
func bounded(s Snapshot) bool {
	manifests := []skillbundle.Manifest{s.Desired.Manifest}
	if s.Observation.Manifest != nil {
		manifests = append(manifests, *s.Observation.Manifest)
	}
	if s.Baseline != nil {
		manifests = append(manifests, s.Baseline.Manifest)
	}
	for _, m := range manifests {
		if len(m.Files) > skillbundle.MaxFiles || len(m.Issues) > skillbundle.MaxEntries || len(m.Digest) > 64 || len(m.Algorithm) > 64 || len(m.ModeSemantics) > 64 {
			return false
		}
		for _, f := range m.Files {
			if len(f.Path) > skillbundle.MaxPathBytes || len(f.Digest) > 64 || !utf8.ValidString(f.Path) || !utf8.ValidString(f.Digest) {
				return false
			}
		}
		for _, issue := range m.Issues {
			if len(issue.Path) > skillbundle.MaxPathBytes || len(issue.Code) > 128 || !utf8.ValidString(issue.Path) || !utf8.ValidString(issue.Code) {
				return false
			}
		}
	}
	targets := []Target{s.Target}
	stringsToCheck := []string{s.Desired.Name, s.Desired.BundleID, s.Desired.RevisionID, s.Observation.Generation, s.Observation.Presence, s.RootOwnership, s.Mode}
	if s.Baseline != nil {
		targets = append(targets, s.Baseline.Target)
		stringsToCheck = append(stringsToCheck, s.Baseline.BundleID, s.Baseline.RevisionID)
	}
	for _, t := range targets {
		stringsToCheck = append(stringsToCheck, t.AgentID, t.EnvironmentID, t.Generation, t.RootID, t.SkillName, t.Adapter)
	}
	for _, v := range stringsToCheck {
		if len(v) > 1024 || !utf8.ValidString(v) {
			return false
		}
	}
	return true
}

func validTarget(t Target) bool {
	return t.OwnerID > 0 && opaque(t.AgentID) && opaque(t.EnvironmentID) && opaque(t.Generation) && opaque(t.RootID) && opaque(t.Adapter) && len(t.SkillName) <= 64 && namePattern.MatchString(t.SkillName)
}
func opaque(s string) bool {
	return len(s) > 0 && len(s) <= 1024 && utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
}
func canonical(m skillbundle.Manifest) (skillbundle.Manifest, error) {
	if !m.Complete || len(m.Issues) != 0 || m.SchemaVersion != skillbundle.SchemaVersion || m.Algorithm != skillbundle.Algorithm || m.ModeSemantics != skillbundle.ModeSemantics {
		return skillbundle.Manifest{}, errors.New("unsupported or incomplete manifest")
	}
	c, err := skillbundle.FromMetadata(m.Files)
	if err != nil || c.Digest != m.Digest || c.TotalBytes != m.TotalBytes {
		return skillbundle.Manifest{}, errors.New("manifest identity mismatch")
	}
	return c, nil
}
func fileMap(m skillbundle.Manifest) map[string]skillbundle.File {
	out := map[string]skillbundle.File{}
	for _, f := range m.Files {
		out[f.Path] = f
	}
	return out
}
func fileValues(m map[string]skillbundle.File) []skillbundle.File {
	out := make([]skillbundle.File, 0, len(m))
	for _, f := range m {
		out = append(out, f)
	}
	return out
}
func seal(p Plan) (Plan, error) {
	raw, err := json.Marshal(p)
	if err != nil {
		return Plan{}, err
	}
	digest := sha256.Sum256(raw)
	p.Hash = hex.EncodeToString(digest[:])
	return p, nil
}
