package setup

import (
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const setupVerifySchemaVersion = 1

type SetupVerifySummary struct {
	Verified         int `json:"verified"`
	Missing          int `json:"missing"`
	Changed          int `json:"changed"`
	Blocked          int `json:"blocked"`
	Unsupported      int `json:"unsupported"`
	SoftwareMatches  int `json:"software_matches"`
	SoftwareWarnings int `json:"software_warnings"`
}

type VerifySoftware struct {
	AgentID            string `json:"agent_id"`
	Name               string `json:"name"`
	SourceVersion      string `json:"source_version,omitempty"`
	DestinationVersion string `json:"destination_version,omitempty"`
	SourcePackage      string `json:"source_package,omitempty"`
	DestinationPackage string `json:"destination_package,omitempty"`
	State              string `json:"state"`
	Guidance           string `json:"guidance"`
}

type VerifyComponent struct {
	AgentID        string   `json:"agent_id"`
	Kind           string   `json:"kind"`
	Source         string   `json:"source"`
	Destination    string   `json:"destination,omitempty"`
	Scope          string   `json:"scope"`
	State          string   `json:"state"` // verified, missing, changed, blocked, unsupported
	Reason         string   `json:"reason,omitempty"`
	SelectedFields int      `json:"selected_fields"`
	Mismatches     []string `json:"mismatches,omitempty"`
}

type SetupVerifyResult struct {
	SchemaVersion  int                  `json:"schema_version"`
	SnapshotID     string               `json:"snapshot_id"`
	SnapshotDigest string               `json:"snapshot_digest"`
	Destination    RestoreDestination   `json:"destination"`
	Summary        SetupVerifySummary   `json:"summary"`
	Components     []VerifyComponent    `json:"components"`
	Software       []VerifySoftware     `json:"software,omitempty"`
	Unsupported    []RestoreUnsupported `json:"unsupported"`
	Limitations    []string             `json:"limitations"`
}

// VerifySetupSnapshot checks the selected fields in the destination without
// repairing them. It never runs agents and never returns values.
func VerifySetupSnapshot(snapshot SetupSnapshot, manifest sealed.Manifest, destination Report) SetupVerifyResult {
	result := SetupVerifyResult{
		SchemaVersion: setupVerifySchemaVersion, SnapshotID: manifest.ID, SnapshotDigest: manifest.SHA256,
		Destination: RestoreDestination{OS: destination.OS, Architecture: destination.Architecture, Project: destination.Project},
		Components:  []VerifyComponent{}, Unsupported: policyUnsupported(snapshot),
		Limitations: []string{
			"File-level verification only: selected fields and MCP declarations are compared; extra destination fields are ignored.",
			"No agent executable is run; application versions, authentication, MCP health and runtime behaviour are not verified.",
			"Verification never repairs, rewrites or rolls back anything.",
			"This is not proof that the setup works on this machine or that it is portable to another OS.",
		},
	}
	components := snapshotRestoreComponents(snapshot)
	candidates := destinationCandidates(destination)
	for _, source := range snapshot.Sources {
		var component restoreComponent
		found := false
		for _, agent := range snapshot.Preview.Agents {
			if candidate, ok := components[restoreKey(agent.ID, source.Path, source.Component)]; ok {
				component, found = candidate, true
				break
			}
		}
		entry := VerifyComponent{
			AgentID: component.agentID, Kind: source.Component, Source: source.Path,
			Scope: component.scope, SelectedFields: len(source.Content),
		}
		if !found {
			entry.State, entry.Reason = "unsupported", "component_not_in_preview"
			addVerifyComponent(&result, entry)
			continue
		}
		if component.scope == "project" && destination.Project == "" {
			entry.State, entry.Reason = "unsupported", "project_not_selected"
			addVerifyComponent(&result, entry)
			continue
		}
		candidate, ok := matchDestination(candidates, component)
		if !ok {
			entry.State, entry.Reason = "unsupported", "destination_location_not_found"
			addVerifyComponent(&result, entry)
			continue
		}
		entry.Destination = candidate.path
		switch candidate.state {
		case "missing":
			entry.State = "missing"
			addVerifyComponent(&result, entry)
			continue
		case "present":
		default:
			entry.State, entry.Reason = "blocked", "destination_"+candidate.state
			addVerifyComponent(&result, entry)
			continue
		}
		if source.Component == "instructions" {
			text, state, issue := readInstructionText(candidate.path)
			if state != "parsed" {
				entry.State, entry.Reason = "blocked", issue
				if entry.Reason == "" {
					entry.Reason = state
				}
				addVerifyComponent(&result, entry)
				continue
			}
			if text != source.Text {
				entry.Mismatches = []string{"text"}
			}
		} else {
			object, state, issue := readConfigObject(candidate.path, component.format, component.agentID)
			if state != "parsed" {
				entry.State, entry.Reason = "blocked", issue
				if entry.Reason == "" {
					entry.Reason = state
				}
				addVerifyComponent(&result, entry)
				continue
			}
			entry.Mismatches = verifySelectedContent(object, component.agentID, source)
		}
		if len(entry.Mismatches) == 0 {
			entry.State = "verified"
		} else {
			entry.State, entry.Reason = "changed", "selected_content_differs"
		}
		addVerifyComponent(&result, entry)
	}
	sort.Slice(result.Components, func(i, j int) bool {
		if result.Components[i].Source == result.Components[j].Source {
			return result.Components[i].Kind < result.Components[j].Kind
		}
		return result.Components[i].Source < result.Components[j].Source
	})
	result.Software = verifySoftware(snapshot.Software, destination.Agents)
	for _, software := range result.Software {
		if software.State == "matches" {
			result.Summary.SoftwareMatches++
		} else {
			result.Summary.SoftwareWarnings++
		}
	}
	return result
}

func verifySoftware(source []SetupSoftware, destination []Agent) []VerifySoftware {
	agents := map[string]Agent{}
	for _, agent := range destination {
		agents[agent.ID] = agent
	}
	var out []VerifySoftware
	for _, recorded := range source {
		entry := VerifySoftware{
			AgentID: recorded.AgentID, Name: recorded.Name,
			SourceVersion: recorded.Version, SourcePackage: recorded.Package,
		}
		agent, ok := agents[recorded.AgentID]
		if !ok {
			entry.State = "agent_not_detected"
			entry.Guidance = "Install the agent application or make its command available on PATH."
			out = append(out, entry)
			continue
		}
		if agent.ApplicationVersion != nil {
			entry.DestinationVersion = agent.ApplicationVersion.Version
		}
		if agent.Installation != nil {
			entry.DestinationPackage = agent.Installation.Package
		}
		switch {
		case recorded.Version == "" && entry.DestinationVersion == "":
			entry.State = "not_recorded"
			entry.Guidance = "Neither the snapshot nor the destination reported an application version."
		case recorded.Version == "":
			entry.State = "source_version_not_recorded"
			entry.Guidance = "The snapshot did not record an application version; retake it with --probe-versions to compare later."
		case entry.DestinationVersion == "":
			entry.State = "destination_version_unknown"
			entry.Guidance = "Run reet setup verify --probe-versions or check the agent installation."
		case recorded.Version != entry.DestinationVersion:
			entry.State = "version_mismatch"
			entry.Guidance = "The recorded application version differs from the destination; ask before substituting or updating."
		case recorded.Package != "" && entry.DestinationPackage != "" && recorded.Package != entry.DestinationPackage:
			entry.State = "package_mismatch"
			entry.Guidance = "The local package identity differs from the snapshot; confirm the installation source."
		case recorded.InstallState != "" && recorded.InstallState != "matched" && entry.DestinationPackage == "":
			entry.State = "installation_unknown"
			entry.Guidance = "Installation provenance is unknown on the destination; confirm how the agent was installed."
		default:
			entry.State = "matches"
			entry.Guidance = "The recorded application version matches the destination."
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}

func addVerifyComponent(result *SetupVerifyResult, component VerifyComponent) {
	switch component.State {
	case "verified":
		result.Summary.Verified++
	case "missing":
		result.Summary.Missing++
	case "changed":
		result.Summary.Changed++
	case "blocked":
		result.Summary.Blocked++
	default:
		result.Summary.Unsupported++
	}
	result.Components = append(result.Components, component)
}

func verifySelectedContent(object map[string]any, agentID string, source SetupSnapshotSource) []string {
	var mismatches []string
	switch source.Component {
	case "settings":
		for key, value := range source.Content {
			if !reflect.DeepEqual(object[key], value) {
				mismatches = append(mismatches, key)
			}
		}
	case "mcp":
		container := mcpContainer(agentID)
		servers, ok := object[container].(map[string]any)
		if !ok {
			return []string{container}
		}
		for name, declaration := range source.Content {
			if !reflect.DeepEqual(servers[name], declaration) {
				mismatches = append(mismatches, name)
			}
		}
	}
	sort.Strings(mismatches)
	return mismatches
}

func renderSetupVerify(out io.Writer, result SetupVerifyResult) error {
	if _, err := fmt.Fprintf(out, "Setup verification for %s\nDestination: %s/%s", result.SnapshotID, result.Destination.OS, result.Destination.Architecture); err != nil {
		return err
	}
	if result.Destination.Project != "" {
		if _, err := fmt.Fprintf(out, "; project %q", result.Destination.Project); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "\nverified=%d missing=%d changed=%d blocked=%d unsupported=%d\n",
		result.Summary.Verified, result.Summary.Missing, result.Summary.Changed, result.Summary.Blocked, result.Summary.Unsupported); err != nil {
		return err
	}
	if len(result.Software) > 0 {
		if _, err := fmt.Fprintf(out, "software matches=%d warnings=%d\n", result.Summary.SoftwareMatches, result.Summary.SoftwareWarnings); err != nil {
			return err
		}
	}
	for _, component := range result.Components {
		if _, err := fmt.Fprintf(out, "  %s %s %q", component.State, component.Kind, component.Source); err != nil {
			return err
		}
		if component.Destination != "" {
			if _, err := fmt.Fprintf(out, " -> %q", component.Destination); err != nil {
				return err
			}
		}
		if component.Reason != "" {
			if _, err := fmt.Fprintf(out, " (%s)", component.Reason); err != nil {
				return err
			}
		}
		if len(component.Mismatches) > 0 {
			if _, err := fmt.Fprintf(out, " mismatches: %s", strings.Join(component.Mismatches, ", ")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	for _, software := range result.Software {
		if _, err := fmt.Fprintf(out, "  software %s: %s", software.Name, software.State); err != nil {
			return err
		}
		if software.Guidance != "" {
			if _, err := fmt.Fprintf(out, " (%s)", software.Guidance); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if len(result.Unsupported) > 0 {
		if _, err := fmt.Fprintln(out, "Unsupported or omitted:"); err != nil {
			return err
		}
	}
	for _, unsupported := range result.Unsupported {
		if _, err := fmt.Fprintf(out, "  %s %q: %s", unsupported.Kind, unsupported.Source, unsupported.Reason); err != nil {
			return err
		}
		if unsupported.Count > 0 {
			if _, err := fmt.Fprintf(out, " (%d)", unsupported.Count); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	for _, note := range result.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
