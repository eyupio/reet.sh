// Package agentsoftware turns observed agent installation evidence into a
// reviewable maintenance plan. It never downloads, installs, updates or
// removes anything; a plan is text for a person to review.
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
	// SupportedPlatform is the first operating system whose global Node
	// package layout reet has verified.
	SupportedPlatform = "linux"
)

type Action string

const (
	Install Action = "install"
	Update  Action = "update"
	Remove  Action = "remove"
)

func (a Action) Valid() bool {
	return a == Install || a == Update || a == Remove
}

var (
	// ErrUnsupported means this observation has no reviewed plan: another
	// tool, platform, package manager or installation kind.
	ErrUnsupported = errors.New("no reviewed maintenance plan for this installation")
	// ErrAction means the action does not match the observed state.
	ErrAction = errors.New("that action does not match the installed state")
)

// Plan is a reviewable set of steps. Executes is always false in this slice.
type Plan struct {
	Tool     string   `json:"tool"`
	Action   Action   `json:"action"`
	State    string   `json:"state"`
	Package  string   `json:"package"`
	Current  string   `json:"current"`
	Target   string   `json:"target"`
	Command  string   `json:"command"`
	Steps    []string `json:"steps"`
	Effects  []string `json:"effects"`
	Executes bool     `json:"executes"`
	Sentence string   `json:"sentence"`
}

// For builds a plan for one observed OpenCode installation. running is the
// number of sessions of this tool on the machine; it changes the effects, not
// the command.
func For(entry terminalproto.SoftwareEntry, action Action, host string, running int) (Plan, error) {
	if !action.Valid() || entry.Tool != OpenCodeTool || entry.Platform != SupportedPlatform {
		return Plan{}, ErrUnsupported
	}
	if entry.State == terminalproto.SoftwareInstalled {
		if entry.Manager != "node_package" || entry.Source != "package.json" || entry.Package == "" {
			return Plan{}, ErrUnsupported
		}
	}
	if entry.State != terminalproto.SoftwareInstalled && entry.State != terminalproto.SoftwareMissing {
		return Plan{}, ErrUnsupported
	}
	plan := Plan{Tool: entry.Tool, Action: action, Package: OpenCodePackage, Current: entry.Version, Target: OpenCodeReviewedVersion}
	switch action {
	case Install:
		if entry.State != terminalproto.SoftwareMissing {
			return Plan{}, ErrAction
		}
		plan.Command = "npm install -g " + OpenCodePackage + "@" + OpenCodeReviewedVersion
		plan.Steps = []string{
			fmt.Sprintf("On %s, run: %s", host, plan.Command),
			fmt.Sprintf("Then run `%s --version` and expect %s.", OpenCodeTool, OpenCodeReviewedVersion),
		}
		plan.Effects = []string{"Nothing on this machine changes until a person runs the command."}
		plan.Sentence = fmt.Sprintf("OpenCode is not installed on %s. This plan installs the reviewed version %s. reet has not run it.", host, OpenCodeReviewedVersion)
	case Update:
		if entry.State != terminalproto.SoftwareInstalled {
			return Plan{}, ErrAction
		}
		if entry.Version == OpenCodeReviewedVersion {
			plan.State = "noop"
			plan.Command = ""
			plan.Steps = []string{fmt.Sprintf("Nothing to do: %s already has the reviewed OpenCode %s.", host, OpenCodeReviewedVersion)}
			plan.Sentence = fmt.Sprintf("OpenCode on %s is already the reviewed version %s.", host, OpenCodeReviewedVersion)
			return plan, nil
		}
		plan.Command = "npm install -g " + OpenCodePackage + "@" + OpenCodeReviewedVersion
		plan.Steps = []string{
			fmt.Sprintf("On %s, run: %s", host, plan.Command),
			fmt.Sprintf("Then run `%s --version` and expect %s.", OpenCodeTool, OpenCodeReviewedVersion),
		}
		plan.Effects = runningEffects(host, running, "update")
		plan.Sentence = fmt.Sprintf("OpenCode %s on %s would be replaced by the reviewed version %s. reet has not run this plan.", entry.Version, host, OpenCodeReviewedVersion)
	case Remove:
		if entry.State != terminalproto.SoftwareInstalled {
			return Plan{}, ErrAction
		}
		plan.Command = "npm uninstall -g " + OpenCodePackage
		plan.Target = ""
		plan.Steps = []string{
			fmt.Sprintf("On %s, run: %s", host, plan.Command),
			"Configuration, history and credentials stay on the machine.",
		}
		plan.Effects = runningEffects(host, running, "remove")
		plan.Sentence = fmt.Sprintf("OpenCode %s would be removed from %s. Configuration and history stay. reet has not run this plan.", entry.Version, host)
	}
	plan.State = "ready"
	if len(plan.Effects) == 0 {
		plan.Effects = []string{"Nothing on this machine changes until a person runs the command."}
	}
	return plan, nil
}

func runningEffects(host string, running int, action string) []string {
	if running <= 0 {
		return []string{"No OpenCode sessions are running on " + host + "."}
	}
	if running == 1 {
		return []string{fmt.Sprintf("1 OpenCode session is running on %s; finish or close it before you %s.", host, action)}
	}
	return []string{fmt.Sprintf("%d OpenCode sessions are running on %s; finish or close them before you %s.", running, host, action)}
}
