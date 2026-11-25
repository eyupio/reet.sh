package terminalproto

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Fleet setup copy: the controller reads one machine's global instruction
// files and portable settings, asks other machines for a plan, applies it one
// machine at a time and can undo it from the machine's own journal. Each step
// is a separate permission a connector negotiates on its own.
const (
	// SetupReadFeature lets a connector send its global instruction files and
	// portable settings. Credentials never leave the machine.
	SetupReadFeature = "setup-read-v1"
	// SetupApplyFeature lets a connector plan (dry run) and apply a copy.
	SetupApplyFeature = "setup-apply-v1"
	// SetupRollbackFeature lets a connector undo an applied copy from its journal.
	SetupRollbackFeature = "setup-rollback-v1"
)

// Setup operations, each served only where its feature was negotiated.
const (
	SetupOpRead     = "read"
	SetupOpPlan     = "plan" // a dry run of apply: nothing is written
	SetupOpApply    = "apply"
	SetupOpRollback = "rollback"
)

// SetupFeatureFor names the feature an operation needs.
func SetupFeatureFor(op string) string {
	switch op {
	case SetupOpRead:
		return SetupReadFeature
	case SetupOpPlan, SetupOpApply:
		return SetupApplyFeature
	case SetupOpRollback:
		return SetupRollbackFeature
	}
	return ""
}

const (
	// MaxSetupItems bounds the files one copy carries.
	MaxSetupItems = 32
	// MaxSetupBytes keeps a read reply, and the apply that carries the same
	// items back, inside one control frame with its envelope.
	MaxSetupBytes = MaxControlBytes - 4<<10
	// MaxSetupDiffLines bounds each side of an instruction file's diff.
	MaxSetupDiffLines = 40
)

// Setup result states and the closed reasons a machine may give.
const (
	SetupOK      = "ok"
	SetupRefused = "refused"
	SetupFailed  = "failed"

	SetupReasonTooLarge   = "too_large"
	SetupReasonCredential = "credential"
	SetupReasonOff        = "off"         // terminal access is off on the machine
	SetupReasonNotAllowed = "not_allowed" // allow-apply is off on the machine
	SetupReasonLocked     = "locked"
	SetupReasonChanged    = "changed"
	SetupReasonNothing    = "nothing"
	SetupReasonBusy       = "busy"
	SetupReasonInvalid    = "invalid"
	SetupReasonUnreadable = "unreadable"
	SetupReasonTimedOut   = "timed_out"
	SetupReasonWrite      = "write_failed"
	SetupReasonNoJournal  = "no_journal"
	SetupReasonDamaged    = "damaged"
)

func validSetupReason(r string) bool {
	switch r {
	case SetupReasonTooLarge, SetupReasonCredential, SetupReasonOff, SetupReasonNotAllowed, SetupReasonLocked,
		SetupReasonChanged, SetupReasonNothing, SetupReasonBusy, SetupReasonInvalid, SetupReasonUnreadable,
		SetupReasonTimedOut, SetupReasonWrite, SetupReasonNoJournal, SetupReasonDamaged:
		return true
	}
	return false
}

// SetupItem is one file the source machine offers. Label is the source's own
// path, shown to a person only: a target never writes to it, it maps the item
// by agent, scope, kind and file name to its own location.
type SetupItem struct {
	Agent   string `json:"agent"`
	Scope   string `json:"scope"`
	Kind    string `json:"kind"`   // instructions or settings
	Name    string `json:"name"`   // the file name, never a path
	Format  string `json:"format"` // text, json, jsonc or toml
	Label   string `json:"label,omitempty"`
	Content string `json:"content"`
	SHA256  string `json:"sha256"`
}

// SetupItemSHA256 is the digest a SetupItem carries.
func SetupItemSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// ValidSetupAgent is an agent whose global setup can be copied.
func ValidSetupAgent(agent string) bool {
	switch agent {
	case "claude", "codex", "opencode", "pi":
		return true
	}
	return false
}

func validSetupName(name string) bool {
	return plainText(name, 128) && name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, `/\`)
}

func (i SetupItem) valid() bool {
	validKind := i.Kind == "instructions" && i.Format == "text" ||
		i.Kind == "settings" && (i.Format == "json" || i.Format == "jsonc" || i.Format == "toml")
	return ValidSetupAgent(i.Agent) && i.Scope == "global" && validKind && validSetupName(i.Name) &&
		plainText(i.Label, 4096) && utf8.ValidString(i.Content) && i.SHA256 == SetupItemSHA256(i.Content)
}

// SetupFilePlan is what a target would do with one item. It names fields and
// lines, never a whole settings value.
type SetupFilePlan struct {
	Agent string `json:"agent"`
	Kind  string `json:"kind"`
	Name  string `json:"name"`
	// Destination is the target's own path for the file.
	Destination string `json:"destination,omitempty"`
	// State is create, merge (settings: listed fields overwritten, others
	// kept; instructions: the file's text replaced), unchanged or blocked.
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	// Fields are the settings fields whose value would change.
	Fields []string `json:"fields,omitempty"`
	// Widens names the fields that would let agents do more without asking.
	Widens []string `json:"widens,omitempty"`
	// Added and Removed are an instruction file's changed lines, bounded.
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	// HoldsCredential says the target's own file looks like it holds a
	// credential, so none of its text (no removed lines) is in the plan.
	HoldsCredential bool `json:"holdsCredential,omitempty"`
}

func (f SetupFilePlan) valid() bool {
	switch f.State {
	case "create", "merge", "unchanged", "blocked":
	default:
		return false
	}
	ok := ValidSetupAgent(f.Agent) && (f.Kind == "instructions" || f.Kind == "settings") && validSetupName(f.Name) &&
		plainText(f.Destination, 4096) && plainText(f.Reason, 64) &&
		len(f.Fields) <= 64 && len(f.Widens) <= 64 && len(f.Added) <= MaxSetupDiffLines && len(f.Removed) <= MaxSetupDiffLines
	for _, list := range [][]string{f.Fields, f.Widens} {
		for _, v := range list {
			ok = ok && plainText(v, 128)
		}
	}
	for _, list := range [][]string{f.Added, f.Removed} {
		for _, v := range list {
			ok = ok && plainText(v, 256)
		}
	}
	return ok
}

// SetupFleet asks a machine for one setup operation. Items travel with plan
// and apply; Fingerprint is the reviewed plan an apply must still match;
// Label names the apply's journal so a repeated apply finds its own;
// Journal names the journal a rollback reverses.
type SetupFleet struct {
	Op          string      `json:"op"`
	Items       []SetupItem `json:"items,omitempty"`
	Fingerprint string      `json:"fingerprint,omitempty"`
	Label       string      `json:"label,omitempty"`
	Journal     string      `json:"journal,omitempty"`
}

func (SetupFleet) kind() string { return "setup_fleet" }

func validSetupLabel(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// ValidSetupJournal is a journal id as an apply returns it.
func ValidSetupJournal(s string) bool {
	return strings.HasPrefix(s, "restore-") && len(s) <= 200 && validSetupLabel(s)
}

func validSetupItems(items []SetupItem) bool {
	if len(items) == 0 || len(items) > MaxSetupItems {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		key := item.Agent + "\x00" + item.Kind + "\x00" + item.Name
		if !item.valid() || seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (s SetupFleet) valid() bool {
	switch s.Op {
	case SetupOpRead:
		return len(s.Items) == 0 && s.Fingerprint == "" && s.Label == "" && s.Journal == ""
	case SetupOpPlan:
		return validSetupItems(s.Items) && s.Fingerprint == "" && s.Label == "" && s.Journal == ""
	case SetupOpApply:
		return validSetupItems(s.Items) && s.Fingerprint != "" && validFingerprint(s.Fingerprint) && validSetupLabel(s.Label) && s.Journal == ""
	case SetupOpRollback:
		return len(s.Items) == 0 && s.Fingerprint == "" && s.Label == "" && ValidSetupJournal(s.Journal)
	}
	return false
}

// SetupFleetResult answers one SetupFleet. A read carries items; a plan or
// apply carries the files and the plan's fingerprint; an apply carries the
// journal id an undo names. A refusal for a credential names the agent and
// file, never the matched text.
type SetupFleetResult struct {
	Op          string          `json:"op"`
	State       string          `json:"state"`
	Reason      string          `json:"reason,omitempty"`
	Agent       string          `json:"agent,omitempty"`
	Name        string          `json:"name,omitempty"`
	Items       []SetupItem     `json:"items,omitempty"`
	Files       []SetupFilePlan `json:"files,omitempty"`
	Fingerprint string          `json:"fingerprint,omitempty"`
	Journal     string          `json:"journal,omitempty"`
}

func (SetupFleetResult) kind() string { return "setup_fleet_result" }

func (r SetupFleetResult) valid() bool {
	if SetupFeatureFor(r.Op) == "" {
		return false
	}
	switch r.State {
	case SetupOK:
		if r.Reason != "" {
			return false
		}
	case SetupRefused, SetupFailed:
		if !validSetupReason(r.Reason) || len(r.Items) > 0 || r.Journal != "" {
			return false
		}
	default:
		return false
	}
	if r.Agent != "" && !ValidSetupAgent(r.Agent) || r.Name != "" && !validSetupName(r.Name) {
		return false
	}
	if len(r.Items) > 0 && (r.Op != SetupOpRead || !validSetupItems(r.Items)) {
		return false
	}
	if len(r.Files) > MaxSetupItems || !validFingerprint(r.Fingerprint) {
		return false
	}
	for _, f := range r.Files {
		if !f.valid() {
			return false
		}
	}
	if r.Journal != "" && (r.Op != SetupOpApply || !ValidSetupJournal(r.Journal)) {
		return false
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxSetupBytes
}
