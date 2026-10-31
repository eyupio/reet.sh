package terminalproto

import (
	"encoding/json"
	"strings"
)

// SoftwareFeature lets a connector report the evidence it has about the agent
// software installed on its machine. It is an observation, never an
// instruction to install, update or remove anything.
const SoftwareFeature = "software-v1"

const (
	// MaxSoftwareEntries bounds one machine's software inventory.
	MaxSoftwareEntries = 8
	// MaxSoftwareReplyBytes keeps the whole inventory inside a control frame
	// with its envelope.
	MaxSoftwareReplyBytes = 8 << 10
)

// SupportsSoftware reports whether the peer advertised software-v1.
func SupportsSoftware(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == SoftwareFeature {
			return true
		}
	}
	return false
}

// SoftwareState is what the connector observed, not what it can do about it.
type SoftwareState string

const (
	SoftwareInstalled   SoftwareState = "installed"
	SoftwareMissing     SoftwareState = "missing"
	SoftwareUnknown     SoftwareState = "unknown"
	SoftwareUnsupported SoftwareState = "unsupported"
)

func (s SoftwareState) Valid() bool {
	switch s {
	case SoftwareInstalled, SoftwareMissing, SoftwareUnknown, SoftwareUnsupported:
		return true
	}
	return false
}

// SoftwareReason is a closed explanation for a state that is not installed.
type SoftwareReason string

const (
	SoftwareNotInstalled        SoftwareReason = "not_installed"
	SoftwareUnsupportedTool     SoftwareReason = "unsupported"
	SoftwareUnreadable          SoftwareReason = "unreadable"
	SoftwarePackageUnknown      SoftwareReason = "package_unknown"
	SoftwarePlatformUnsupported SoftwareReason = "platform_unsupported"
	SoftwareManagedBroken       SoftwareReason = "managed_broken"
)

func (r SoftwareReason) Valid() bool {
	switch r {
	case SoftwareNotInstalled, SoftwareUnsupportedTool, SoftwareUnreadable, SoftwarePackageUnknown, SoftwarePlatformUnsupported, SoftwareManagedBroken:
		return true
	}
	return false
}

// SoftwareAction is the maintenance decision a plan can name. It is not a
// command the controller may supply to the connector.
type SoftwareAction string

const (
	SoftwareActionInstall  SoftwareAction = "install"
	SoftwareActionUpdate   SoftwareAction = "update"
	SoftwareActionRollback SoftwareAction = "rollback"
	SoftwareActionRemove   SoftwareAction = "remove"
)

func (a SoftwareAction) Valid() bool {
	switch a {
	case SoftwareActionInstall, SoftwareActionUpdate, SoftwareActionRollback, SoftwareActionRemove:
		return true
	}
	return false
}

// SoftwareOperation is a maintenance decision this evidence supports. It names
// a plan reet can show, not an action it has taken or will take by itself.
type SoftwareOperation string

const (
	SoftwarePlanInstall SoftwareOperation = "plan_install"
	SoftwarePlanUpdate  SoftwareOperation = "plan_update"
	SoftwarePlanRemove  SoftwareOperation = "plan_remove"
)

func (o SoftwareOperation) Valid() bool {
	switch o {
	case SoftwarePlanInstall, SoftwarePlanUpdate, SoftwarePlanRemove:
		return true
	}
	return false
}

// SoftwareEntry is one tool's installation evidence. It deliberately carries no
// package-manager output, command history or configuration contents.
type SoftwareEntry struct {
	Tool     string        `json:"tool"`
	State    SoftwareState `json:"state"`
	Version  string        `json:"version,omitempty"`
	Manager  string        `json:"manager,omitempty"`
	Package  string        `json:"package,omitempty"`
	Path     string        `json:"path,omitempty"`
	Platform string        `json:"platform,omitempty"`
	Arch     string        `json:"arch,omitempty"`
	Source   string        `json:"source,omitempty"`
	Evidence string        `json:"evidence,omitempty"`
	// Managed is true only for an installation reet staged itself. An
	// installation reet did not make stays inspect-only.
	Managed bool `json:"managed,omitempty"`
	// Previous is the earlier managed version kept for rollback, when one is.
	Previous   string              `json:"previous,omitempty"`
	Operations []SoftwareOperation `json:"operations,omitempty"`
	Reason     SoftwareReason      `json:"reason,omitempty"`
	CheckedAt  int64               `json:"checkedAt"`
}

func (e SoftwareEntry) Valid() bool {
	if !ValidTool(e.Tool) || !e.State.Valid() || !plainText(e.Version, 64) || !plainText(e.Manager, 32) ||
		!plainText(e.Package, 128) || !plainText(e.Path, 4096) || !plainText(e.Platform, 16) ||
		!plainText(e.Arch, 16) || !plainText(e.Source, 32) || !plainText(e.Evidence, 128) ||
		!ValidCounter(e.CheckedAt) {
		return false
	}
	if e.Reason != "" && (!e.Reason.Valid() || e.State == SoftwareInstalled) {
		return false
	}
	if e.Managed && e.State != SoftwareInstalled {
		return false
	}
	if e.Previous != "" && (!e.Managed || e.State != SoftwareInstalled || !validVersionText(e.Previous)) {
		return false
	}
	if len(e.Operations) > 3 {
		return false
	}
	seen := make(map[SoftwareOperation]bool, len(e.Operations))
	for _, op := range e.Operations {
		if !op.Valid() || seen[op] {
			return false
		}
		seen[op] = true
	}
	switch e.State {
	case SoftwareInstalled:
		return e.Reason == "" && e.Version != "" && e.Manager != "" && e.Package != "" && e.Path != "" &&
			e.Platform != "" && e.Arch != "" && e.Source != ""
	case SoftwareMissing:
		return e.Reason == SoftwareNotInstalled && e.Version == "" && e.Manager == "" && e.Path == "" && e.Evidence == ""
	case SoftwareUnknown, SoftwareUnsupported:
		return e.Reason != "" && e.Reason != SoftwareNotInstalled
	}
	return false
}

func validVersionText(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 64 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '+':
		default:
			return false
		}
	}
	return true
}

// ValidSoftware checks a whole inventory and its encoded size.
func ValidSoftware(entries []SoftwareEntry) bool {
	if len(entries) > MaxSoftwareEntries {
		return false
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if !entry.Valid() || seen[entry.Tool] {
			return false
		}
		seen[entry.Tool] = true
	}
	data, err := json.Marshal(entries)
	return err == nil && len(data) <= MaxSoftwareReplyBytes
}

// SoftwareApplyFeature lets a connector accept one reviewed install or update.
// It is separate from software-v1: observing evidence and changing a machine
// are different permissions.
const SoftwareApplyFeature = "software-apply-v1"

// MaxSoftwareApplyReplyBytes keeps the result, including the new entry, inside
// a control frame with its envelope.
const MaxSoftwareApplyReplyBytes = 8 << 10

func SupportsSoftwareApply(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == SoftwareApplyFeature {
			return true
		}
	}
	return false
}

// SoftwareApply asks for one reviewed action. The controller supplies only the
// tool, action and the evidence the person approved; the connector owns the
// artifact URL, checksums and command.
type SoftwareApply struct {
	Tool             string         `json:"tool"`
	Action           SoftwareAction `json:"action"`
	ExpectedState    SoftwareState  `json:"expectedState"`
	ExpectedVersion  string         `json:"expectedVersion,omitempty"`
	ExpectedPrevious string         `json:"expectedPrevious,omitempty"`
	ExpectedManaged  bool           `json:"expectedManaged,omitempty"`
}

func (SoftwareApply) kind() string { return "software_apply" }

func (s SoftwareApply) valid() bool {
	if !ValidTool(s.Tool) || !s.Action.Valid() || !s.ExpectedState.Valid() || !plainText(s.ExpectedVersion, 64) || !validVersionText(s.ExpectedPrevious) {
		return false
	}
	switch s.Action {
	case SoftwareActionInstall:
		return s.ExpectedState == SoftwareMissing || s.ExpectedState == SoftwareInstalled && !s.ExpectedManaged
	case SoftwareActionUpdate:
		return s.ExpectedState == SoftwareInstalled && s.ExpectedManaged && s.ExpectedVersion != ""
	case SoftwareActionRollback:
		return s.ExpectedState == SoftwareInstalled && s.ExpectedManaged && s.ExpectedVersion != "" && s.ExpectedPrevious != ""
	case SoftwareActionRemove:
		return s.ExpectedState == SoftwareInstalled && s.ExpectedManaged && s.ExpectedVersion != ""
	}
	return false
}

type SoftwareApplyState string

const (
	SoftwareApplyApplied SoftwareApplyState = "applied"
	SoftwareApplyRefused SoftwareApplyState = "refused"
	SoftwareApplyFailed  SoftwareApplyState = "failed"
)

func (s SoftwareApplyState) Valid() bool {
	switch s {
	case SoftwareApplyApplied, SoftwareApplyRefused, SoftwareApplyFailed:
		return true
	}
	return false
}

type SoftwareApplyReason string

const (
	SoftwareApplyUnsupported  SoftwareApplyReason = "unsupported"
	SoftwareApplyChanged      SoftwareApplyReason = "changed"
	SoftwareApplyBusy         SoftwareApplyReason = "busy"
	SoftwareApplySessions     SoftwareApplyReason = "sessions_running"
	SoftwareApplyDownload     SoftwareApplyReason = "download_failed"
	SoftwareApplyChecksum     SoftwareApplyReason = "checksum_failed"
	SoftwareApplyInstall      SoftwareApplyReason = "install_failed"
	SoftwareApplyRemoveFailed SoftwareApplyReason = "remove_failed"
	SoftwareApplyRollback     SoftwareApplyReason = "rollback_failed"
	SoftwareApplyNoRollback   SoftwareApplyReason = "no_rollback"
	SoftwareApplyNotManaged   SoftwareApplyReason = "not_managed"
	SoftwareApplyOff          SoftwareApplyReason = "off"
	SoftwareApplyTimedOut     SoftwareApplyReason = "timed_out"
	SoftwareApplyUnreadable   SoftwareApplyReason = "unreadable"
)

func (r SoftwareApplyReason) Valid() bool {
	switch r {
	case SoftwareApplyUnsupported, SoftwareApplyChanged, SoftwareApplyBusy, SoftwareApplySessions,
		SoftwareApplyDownload, SoftwareApplyChecksum, SoftwareApplyInstall, SoftwareApplyRemoveFailed, SoftwareApplyRollback, SoftwareApplyNoRollback,
		SoftwareApplyNotManaged, SoftwareApplyOff, SoftwareApplyTimedOut, SoftwareApplyUnreadable:
		return true
	}
	return false
}

// SoftwareApplyResult is the outcome of one request. An applied result carries
// the fresh evidence for the tool so the controller can replace its cached
// entry; a refused or failed result carries a closed reason and no entry.
type SoftwareApplyResult struct {
	Tool     string              `json:"tool"`
	Action   SoftwareAction      `json:"action"`
	State    SoftwareApplyState  `json:"state"`
	Version  string              `json:"version,omitempty"`
	Previous string              `json:"previous,omitempty"`
	Reason   SoftwareApplyReason `json:"reason,omitempty"`
	Entry    *SoftwareEntry      `json:"entry,omitempty"`
}

func (SoftwareApplyResult) kind() string { return "software_apply_result" }

func (r SoftwareApplyResult) valid() bool {
	if !ValidTool(r.Tool) || !r.Action.Valid() || !r.State.Valid() ||
		!plainText(r.Version, 64) || !plainText(r.Previous, 64) {
		return false
	}
	if r.State == SoftwareApplyApplied {
		if r.Reason != "" || r.Entry == nil || !r.Entry.Valid() {
			return false
		}
		switch r.Action {
		case SoftwareActionRemove:
			if r.Version != "" || r.Entry.Managed || r.Entry.State == SoftwareInstalled && r.Entry.Managed {
				return false
			}
		case SoftwareActionInstall, SoftwareActionUpdate, SoftwareActionRollback:
			if r.Entry.State != SoftwareInstalled || !r.Entry.Managed || r.Entry.Version != r.Version || r.Version == "" {
				return false
			}
		}
	} else if !r.Reason.Valid() || r.Entry != nil {
		return false
	}
	data, err := json.Marshal(r)
	return err == nil && len(data) <= MaxSoftwareApplyReplyBytes
}
