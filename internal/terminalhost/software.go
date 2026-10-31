//go:build linux || darwin

package terminalhost

import (
	"runtime"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// InspectSoftware reports the local installation evidence for one agent CLI.
// A reet-managed copy is preferred and is the only kind reet will update or
// remove. Any other installation is read through a bounded package.json beside
// the executable and stays inspect-only. It never runs the agent, a package
// manager or an installer.
func InspectSoftware(home, tool string, lookPath func(string) (string, error)) terminalproto.SoftwareEntry {
	now := time.Now().UnixMilli()
	entry := terminalproto.SoftwareEntry{Tool: tool, Platform: runtime.GOOS, Arch: runtime.GOARCH, CheckedAt: now}
	if tool != "opencode" {
		entry.State, entry.Reason = terminalproto.SoftwareUnsupported, terminalproto.SoftwareUnsupportedTool
		return entry
	}
	if path, receipt, found, err := agentsoftware.Current(home, tool); err != nil {
		entry.State, entry.Reason = terminalproto.SoftwareUnknown, terminalproto.SoftwareManagedBroken
		return entry
	} else if found {
		entry.State, entry.Version = terminalproto.SoftwareInstalled, receipt.Version
		entry.Manager, entry.Package, entry.Source = "reet", "opencode-ai", "verified_tarball"
		entry.Path, entry.Evidence, entry.Managed = path, "verified_sha256", true
		entry.Previous = receipt.Previous
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanUpdate, terminalproto.SoftwarePlanRemove}
		return entry
	}

	p, ok := launchPresets[tool]
	if !ok {
		entry.State, entry.Reason = terminalproto.SoftwareUnsupported, terminalproto.SoftwareUnsupportedTool
		return entry
	}
	path, found := resolveBinary(p, lookPath)
	if !found {
		entry.State, entry.Reason = terminalproto.SoftwareMissing, terminalproto.SoftwareNotInstalled
		entry.Package, entry.Source = "opencode-ai", "npm"
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
		return entry
	}

	report := setup.Report{
		SchemaVersion: 2,
		OS:            runtime.GOOS,
		Architecture:  runtime.GOARCH,
		Agents: []setup.Agent{{
			ID:           tool,
			Name:         "OpenCode",
			State:        "detected",
			Executable:   setup.Executable{Command: p.binary, Path: path, State: "present"},
			Capabilities: setup.Capabilities{Installation: "local_node_package_evidence"},
		}},
	}
	setup.InspectInstallations(&report)
	observation := report.Agents[0].Installation
	if observation == nil {
		entry.State, entry.Reason = terminalproto.SoftwareUnknown, terminalproto.SoftwareUnreadable
		return entry
	}
	switch observation.State {
	case "matched":
		entry.State = terminalproto.SoftwareInstalled
		entry.Version, entry.Package = observation.Version, observation.Package
		entry.Manager, entry.Path, entry.Source = "node_package", path, "package.json"
		entry.Evidence = observation.Evidence
		// reet did not make this installation, so it is never updated or
		// removed here. It may still install its own verified copy alongside.
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
	case "not_detected":
		entry.State, entry.Reason = terminalproto.SoftwareMissing, terminalproto.SoftwareNotInstalled
		entry.Package, entry.Source = "opencode-ai", "npm"
		entry.Operations = []terminalproto.SoftwareOperation{terminalproto.SoftwarePlanInstall}
	default:
		entry.State = terminalproto.SoftwareUnknown
		entry.Reason = terminalproto.SoftwareUnreadable
		switch observation.Issue {
		case "package_metadata_not_found_within_limit", "package_metadata_not_regular",
			"unrecognised_package", "unsupported_package_bin", "package_bin_mismatch":
			entry.Reason = terminalproto.SoftwarePackageUnknown
		case "installation_adapter_not_supported":
			entry.State, entry.Reason = terminalproto.SoftwareUnsupported, terminalproto.SoftwareUnsupportedTool
		}
	}
	return entry
}
