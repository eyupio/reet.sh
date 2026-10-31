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
)

func (r SoftwareReason) Valid() bool {
	switch r {
	case SoftwareNotInstalled, SoftwareUnsupportedTool, SoftwareUnreadable, SoftwarePackageUnknown, SoftwarePlatformUnsupported:
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
	Tool       string              `json:"tool"`
	State      SoftwareState       `json:"state"`
	Version    string              `json:"version,omitempty"`
	Manager    string              `json:"manager,omitempty"`
	Package    string              `json:"package,omitempty"`
	Path       string              `json:"path,omitempty"`
	Platform   string              `json:"platform,omitempty"`
	Arch       string              `json:"arch,omitempty"`
	Source     string              `json:"source,omitempty"`
	Evidence   string              `json:"evidence,omitempty"`
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
