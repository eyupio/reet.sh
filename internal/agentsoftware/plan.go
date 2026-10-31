// Package agentsoftware turns observed agent installation evidence into a
// reviewable maintenance plan and stages the one verified artifact reet may
// install. A plan is text for a person to approve; it never runs by itself.
package agentsoftware

import (
	"errors"
	"fmt"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// OpenCodePackage and OpenCodeReviewedVersion are the only package and
	// version reet has verified for an OpenCode install. A newer release is
	// not assumed compatible with the ACP connection reet pins.
	OpenCodePackage         = "opencode-ai"
	OpenCodeReviewedVersion = "1.19.5"
	// OpenCodeTool is the launch catalogue id this package installs.
	OpenCodeTool = "opencode"
	// SupportedPlatform is the first operating system whose layout reet has
	// verified.
	SupportedPlatform = "linux"
)

// Action is the maintenance decision a plan names.
type Action = terminalproto.SoftwareAction

const (
	Install  = terminalproto.SoftwareActionInstall
	Update   = terminalproto.SoftwareActionUpdate
	Rollback = terminalproto.SoftwareActionRollback
	Remove   = terminalproto.SoftwareActionRemove
)

var (
	// ErrUnsupported means this observation has no reviewed plan: another
	// tool, platform, package manager or installation kind.
	ErrUnsupported = errors.New("no reviewed maintenance plan for this installation")
	// ErrAction means the action does not match the observed state.
	ErrAction = errors.New("that action does not match the installed state")
)

// Plan is a reviewable set of steps. Executes is always false: reet applies a
// plan only after a separate confirmation, and only from the reviewed artifact.
type Plan struct {
	Tool     string   `json:"tool"`
	Action   Action   `json:"action"`
	State    string   `json:"state"`
	Package  string   `json:"package"`
	Current  string   `json:"current"`
	Target   string   `json:"target"`
	Source   string   `json:"source"`
	Command  string   `json:"command"`
	Steps    []string `json:"steps"`
	Effects  []string `json:"effects"`
	Executes bool     `json:"executes"`
	Sentence string   `json:"sentence"`
}

// For builds a plan for one observed OpenCode installation. running is the
// number of live sessions of this tool on the machine; the controller refuses
// to apply while any run, and the plan says so.
func For(entry terminalproto.SoftwareEntry, action Action, host string, running int) (Plan, error) {
	if !action.Valid() || entry.Tool != OpenCodeTool || entry.Platform != SupportedPlatform {
		return Plan{}, ErrUnsupported
	}
	if entry.State != terminalproto.SoftwareInstalled && entry.State != terminalproto.SoftwareMissing {
		return Plan{}, ErrUnsupported
	}
	if entry.State == terminalproto.SoftwareInstalled && entry.Managed {
		if entry.Manager != "reet" || entry.Source != "verified_tarball" || entry.Package == "" {
			return Plan{}, ErrUnsupported
		}
	}
	plan := Plan{
		Tool: entry.Tool, Action: action, Package: OpenCodePackage, Current: entry.Version,
		Target: OpenCodeReviewedVersion, Source: OpenCodeLinuxX64.URL,
	}
	switch action {
	case Install:
		if entry.State == terminalproto.SoftwareInstalled && entry.Managed {
			return Plan{}, ErrAction
		}
		plan.Steps = []string{
			fmt.Sprintf("Download the reviewed opencode-linux-x64@%s artifact on %s and verify its sha512.", OpenCodeReviewedVersion, host),
			"Install it under reet's own folder (~/.reet/software/opencode); configuration, history and credentials are untouched.",
			"Verify the installed binary's sha256 before it becomes current.",
		}
		plan.Effects = []string{"Nothing on this machine changes until you confirm."}
		if entry.State == terminalproto.SoftwareInstalled {
			plan.Effects = append(plan.Effects, fmt.Sprintf("The existing OpenCode at %s is not changed; new sessions prefer reet's copy.", entry.Path))
			plan.Sentence = fmt.Sprintf("OpenCode %s is installed outside reet on %s. This plan adds a reet-managed %s alongside it and leaves the existing installation alone.", entry.Version, host, OpenCodeReviewedVersion)
		} else {
			plan.Sentence = fmt.Sprintf("OpenCode is not installed on %s. This plan installs the reviewed version %s in reet's own folder.", host, OpenCodeReviewedVersion)
		}
	case Update:
		if entry.State != terminalproto.SoftwareInstalled || !entry.Managed {
			return Plan{}, ErrAction
		}
		if entry.Version == OpenCodeReviewedVersion {
			plan.State = "noop"
			plan.Steps = []string{fmt.Sprintf("Nothing to do: reet's managed OpenCode on %s is already the reviewed %s.", host, OpenCodeReviewedVersion)}
			plan.Sentence = fmt.Sprintf("reet's managed OpenCode on %s is already the reviewed version %s.", host, OpenCodeReviewedVersion)
			return plan, nil
		}
		plan.Steps = []string{
			fmt.Sprintf("Download the reviewed opencode-linux-x64@%s artifact on %s and verify its sha512.", OpenCodeReviewedVersion, host),
			"Install it under reet's own folder and verify the binary's sha256.",
			fmt.Sprintf("Keep %s so the update can be rolled back.", entry.Version),
		}
		plan.Effects = runningEffects(host, running, "update")
		plan.Sentence = fmt.Sprintf("reet will replace its managed OpenCode %s on %s with the reviewed %s; the previous version stays for rollback.", entry.Version, host, OpenCodeReviewedVersion)
	case Rollback:
		if entry.State != terminalproto.SoftwareInstalled || !entry.Managed {
			return Plan{}, ErrAction
		}
		if entry.Previous == "" {
			return Plan{}, ErrNoRollback
		}
		plan.Target = entry.Previous
		plan.Steps = []string{
			fmt.Sprintf("Make the kept OpenCode %s current again on %s.", entry.Previous, host),
			"Verify the kept binary's sha256 before it becomes current.",
			fmt.Sprintf("Keep %s so the rollback can be reversed by an update.", entry.Version),
		}
		plan.Effects = runningEffects(host, running, "roll back")
		plan.Sentence = fmt.Sprintf("reet will roll its managed OpenCode on %s back from %s to %s. The earlier version is verified first.", host, entry.Version, entry.Previous)
	case Remove:
		if entry.State != terminalproto.SoftwareInstalled || !entry.Managed {
			return Plan{}, ErrAction
		}
		plan.Target = ""
		plan.Steps = []string{
			fmt.Sprintf("Remove reet's managed OpenCode %s from %s.", entry.Version, host),
			"Configuration, history and credentials stay on the machine.",
		}
		plan.Effects = runningEffects(host, running, "remove")
		plan.Effects = append(plan.Effects,
			"Any OpenCode installed outside reet is untouched.",
			"reet's kept previous versions are removed with its own copy.")
		plan.Sentence = fmt.Sprintf("reet's managed OpenCode %s would be removed from %s. Configuration and history stay, and a system copy is untouched.", entry.Version, host)
	default:
		return Plan{}, ErrUnsupported
	}
	plan.State = "ready"
	return plan, nil
}

func runningEffects(host string, running int, action string) []string {
	if running <= 0 {
		return []string{"No OpenCode sessions are running on " + host + "."}
	}
	if running == 1 {
		return []string{fmt.Sprintf("1 OpenCode session is running on %s; reet refuses to %s while it runs.", host, action)}
	}
	return []string{fmt.Sprintf("%d OpenCode sessions are running on %s; reet refuses to %s while they run.", running, host, action)}
}
