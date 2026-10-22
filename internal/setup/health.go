package setup

import (
	"fmt"
	"io"
	"time"
)

const setupHealthSchemaVersion = 1

type SetupHealthCheck struct {
	Code    string `json:"code"`
	State   string `json:"state"` // pass, warn, fail, info
	Message string `json:"message"`
}

type SetupAgentHealth struct {
	ID     string             `json:"id"`
	Name   string             `json:"name"`
	State  string             `json:"state"`
	Checks []SetupHealthCheck `json:"checks"`
}

type SetupHealthSummary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Info int `json:"info"`
}

type SetupHealthReport struct {
	SchemaVersion int                `json:"schema_version"`
	CapturedAt    time.Time          `json:"captured_at"`
	OS            string             `json:"os"`
	Architecture  string             `json:"architecture"`
	Project       string             `json:"project,omitempty"`
	Agents        []SetupAgentHealth `json:"agents"`
	Summary       SetupHealthSummary `json:"summary"`
	Limitations   []string           `json:"limitations"`
}

// BuildSetupHealthReport turns discovered local evidence into conservative
// guidance. It never authenticates, executes agents or repairs anything.
func BuildSetupHealthReport(r Report) SetupHealthReport {
	report := SetupHealthReport{
		SchemaVersion: setupHealthSchemaVersion, CapturedAt: r.CapturedAt.UTC(),
		OS: r.OS, Architecture: r.Architecture, Project: r.Project,
		Agents: []SetupAgentHealth{},
		Limitations: []string{
			"Local evidence only: executable presence, optional version reports and optional package metadata are not runtime health.",
			"Credential stores and OS keychains are not checked; authenticate again after restoring to another machine.",
			"Memory, history, hooks and unsupported components are not checked or repaired.",
			"No installer, hook, package manager or authentication flow runs.",
		},
	}
	for _, agent := range r.Agents {
		health := SetupAgentHealth{ID: agent.ID, Name: agent.Name, State: agent.State, Checks: []SetupHealthCheck{}}
		health.Checks = append(health.Checks, executableHealth(agent))
		health.Checks = append(health.Checks, versionHealth(agent))
		health.Checks = append(health.Checks, installationHealth(agent))
		for _, check := range health.Checks {
			switch check.State {
			case "pass":
				report.Summary.Pass++
			case "warn":
				report.Summary.Warn++
			case "fail":
				report.Summary.Fail++
			default:
				report.Summary.Info++
			}
		}
		report.Agents = append(report.Agents, health)
	}
	return report
}

func executableHealth(agent Agent) SetupHealthCheck {
	check := SetupHealthCheck{Code: "executable"}
	switch agent.Executable.State {
	case "present":
		check.State = "pass"
		check.Message = fmt.Sprintf("%s is on PATH", agent.Executable.Command)
	case "missing":
		check.State = "warn"
		check.Message = fmt.Sprintf("%s was not found on PATH; the agent may be uninstalled or installed elsewhere", agent.Executable.Command)
	default:
		check.State = "fail"
		check.Message = "The executable lookup failed."
	}
	return check
}

func versionHealth(agent Agent) SetupHealthCheck {
	check := SetupHealthCheck{Code: "application_version"}
	if agent.ApplicationVersion == nil {
		check.State = "info"
		check.Message = "No version probe was requested; use --probe-versions for a bounded report."
		return check
	}
	switch agent.ApplicationVersion.State {
	case "reported":
		check.State = "pass"
		check.Message = fmt.Sprintf("Reported version %s.", agent.ApplicationVersion.Version)
	case "not_detected":
		check.State = "info"
		check.Message = "No executable version was reported."
	default:
		check.State = "warn"
		check.Message = "The version probe did not produce a recognised report."
	}
	return check
}

func installationHealth(agent Agent) SetupHealthCheck {
	check := SetupHealthCheck{Code: "installation"}
	if agent.Installation == nil {
		check.State = "info"
		check.Message = "No installation inspection was requested; use --inspect-installation for local package evidence."
		return check
	}
	switch agent.Installation.State {
	case "matched":
		check.State = "pass"
		check.Message = fmt.Sprintf("Local package evidence matches %s %s.", agent.Installation.Package, agent.Installation.Version)
	case "not_detected":
		check.State = "info"
		check.Message = "No supported local package metadata was found."
	default:
		check.State = "warn"
		check.Message = "Installation provenance is unknown or could not be inspected."
	}
	return check
}

func renderSetupHealth(out io.Writer, report SetupHealthReport) error {
	if _, err := fmt.Fprintf(out, "AI setup health (%s/%s)\npass=%d warn=%d fail=%d info=%d\n", report.OS, report.Architecture, report.Summary.Pass, report.Summary.Warn, report.Summary.Fail, report.Summary.Info); err != nil {
		return err
	}
	for _, agent := range report.Agents {
		if _, err := fmt.Fprintf(out, "\n%s: %s\n", agent.Name, agent.State); err != nil {
			return err
		}
		for _, check := range agent.Checks {
			if _, err := fmt.Fprintf(out, "  [%s] %s: %s\n", check.State, check.Code, check.Message); err != nil {
				return err
			}
		}
	}
	for _, note := range report.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
