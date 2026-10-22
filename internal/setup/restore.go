package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const restorePlanSchemaVersion = 1

type RestoreDestination struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Project      string `json:"project,omitempty"`
}

type RestoreAction struct {
	AgentID     string   `json:"agent_id"`
	Components  []string `json:"components"`
	Sources     []string `json:"sources"`
	Destination string   `json:"destination"`
	Scope       string   `json:"scope"`
	Format      string   `json:"format"`
	State       string   `json:"state"` // create, conflict, blocked
	Reason      string   `json:"reason,omitempty"`
}

type RestoreUnsupported struct {
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason"`
	Count  int    `json:"count,omitempty"`
}

type RestorePlan struct {
	SchemaVersion      int                  `json:"schema_version"`
	SnapshotID         string               `json:"snapshot_id"`
	SnapshotVersion    string               `json:"snapshot_version,omitempty"`
	SnapshotTakenAt    time.Time            `json:"snapshot_taken_at"`
	SnapshotDigest     string               `json:"snapshot_digest"`
	SourceOS           string               `json:"source_os"`
	SourceArchitecture string               `json:"source_architecture"`
	Destination        RestoreDestination   `json:"destination"`
	Actions            []RestoreAction      `json:"actions"`
	Unsupported        []RestoreUnsupported `json:"unsupported"`
	Software           []VerifySoftware     `json:"software,omitempty"`
	Warnings           []string             `json:"warnings,omitempty"`
	Fingerprint        string               `json:"fingerprint"`
	Limitations        []string             `json:"limitations"`

	files map[string]restoreFile
}

type restoreFile struct {
	agentID, format, container string
	settings, mcp              map[string]any
	text                       string
}

type restoreComponent struct {
	agentID, path, kind, scope, format, digest string
}

type destinationCandidate struct {
	agentID, scope, path, state string
}

// BuildRestorePlan maps a snapshot to a destination and reports conflicts,
// unsupported items and a stable fingerprint. It writes nothing.
func BuildRestorePlan(snapshot SetupSnapshot, manifest sealed.Manifest, destination Report) RestorePlan {
	plan := RestorePlan{
		SchemaVersion: restorePlanSchemaVersion, SnapshotID: manifest.ID,
		SnapshotVersion: manifest.Version, SnapshotTakenAt: time.UnixMilli(manifest.TakenAt).UTC(),
		SnapshotDigest: manifest.SHA256,
		SourceOS:       snapshot.Preview.OS, SourceArchitecture: snapshot.Preview.Architecture,
		Destination: RestoreDestination{OS: destination.OS, Architecture: destination.Architecture, Project: destination.Project},
		Actions:     []RestoreAction{}, Unsupported: []RestoreUnsupported{},
		Limitations: []string{
			"Preview only: no files are written and no installers or hooks run.",
			"Destination discovery is metadata-only; an existing source is a conflict even when selected content looks similar.",
			"Unsupported and omitted components are listed; they are not restore actions.",
			"No cross-OS translation, dependency resolution or runtime verification has been performed.",
		},
	}
	components := snapshotRestoreComponents(snapshot)
	candidates := destinationCandidates(destination)
	actions := map[string]*RestoreAction{}
	files := map[string]restoreFile{}
	for _, source := range snapshot.Sources {
		var component restoreComponent
		found := false
		for _, agent := range snapshot.Preview.Agents {
			if candidate, ok := components[restoreKey(agent.ID, source.Path, source.Component)]; ok {
				component, found = candidate, true
				break
			}
		}
		if !found {
			plan.Unsupported = append(plan.Unsupported, RestoreUnsupported{Kind: source.Component, Source: source.Path, Reason: "component_not_in_preview"})
			continue
		}
		if component.scope == "project" && destination.Project == "" {
			plan.Unsupported = append(plan.Unsupported, RestoreUnsupported{Kind: source.Component, Source: source.Path, Reason: "project_not_selected"})
			continue
		}
		candidate, ok := matchDestination(candidates, component)
		if !ok {
			plan.Unsupported = append(plan.Unsupported, RestoreUnsupported{Kind: source.Component, Source: source.Path, Reason: "destination_location_not_found"})
			continue
		}
		key := component.agentID + "\x00" + candidate.path
		action := actions[key]
		if action == nil {
			state, reason := restoreState(candidate)
			action = &RestoreAction{
				AgentID: component.agentID, Components: []string{}, Sources: []string{},
				Destination: candidate.path, Scope: component.scope, Format: component.format,
				State: state, Reason: reason,
			}
			actions[key] = action
		}
		file := files[candidate.path]
		file.agentID, file.format, file.container = component.agentID, component.format, mcpContainer(component.agentID)
		if file.settings == nil {
			file.settings = map[string]any{}
		}
		if file.mcp == nil {
			file.mcp = map[string]any{}
		}
		switch source.Component {
		case "settings":
			for key, value := range source.Content {
				file.settings[key] = value
			}
		case "mcp":
			for name, declaration := range source.Content {
				file.mcp[name] = declaration
			}
		case "instructions":
			file.text, file.format = source.Text, "text"
		}
		files[candidate.path] = file
		action.Components = appendUnique(action.Components, component.kind)
		action.Sources = appendUnique(action.Sources, source.Path)
		state, reason := restoreState(candidate)
		if stateRank(state) > stateRank(action.State) {
			action.State, action.Reason = state, reason
		}
	}
	for _, action := range actions {
		sort.Strings(action.Components)
		sort.Strings(action.Sources)
		plan.Actions = append(plan.Actions, *action)
	}
	sort.Slice(plan.Actions, func(i, j int) bool {
		if plan.Actions[i].Destination == plan.Actions[j].Destination {
			return plan.Actions[i].AgentID < plan.Actions[j].AgentID
		}
		return plan.Actions[i].Destination < plan.Actions[j].Destination
	})
	plan.Unsupported = append(plan.Unsupported, policyUnsupported(snapshot)...)
	sort.Slice(plan.Unsupported, func(i, j int) bool {
		if plan.Unsupported[i].Kind != plan.Unsupported[j].Kind {
			return plan.Unsupported[i].Kind < plan.Unsupported[j].Kind
		}
		if plan.Unsupported[i].Source != plan.Unsupported[j].Source {
			return plan.Unsupported[i].Source < plan.Unsupported[j].Source
		}
		return plan.Unsupported[i].Reason < plan.Unsupported[j].Reason
	})
	if snapshot.Preview.OS != "" && destination.OS != "" && (snapshot.Preview.OS != destination.OS || snapshot.Preview.Architecture != destination.Architecture) {
		plan.Warnings = append(plan.Warnings, fmt.Sprintf("Source setup was captured on %s/%s; this destination is %s/%s. Path mapping uses known locations only and cross-OS restore is not verified.", snapshot.Preview.OS, snapshot.Preview.Architecture, destination.OS, destination.Architecture))
	}
	plan.files = files
	plan.Software = verifySoftware(snapshot.Software, destination.Agents)
	for _, software := range plan.Software {
		switch software.State {
		case "version_mismatch", "package_mismatch", "agent_not_detected", "destination_version_unknown", "installation_unknown":
			plan.Warnings = append(plan.Warnings, software.Name+": "+software.Guidance)
		}
	}
	plan.Fingerprint = restoreFingerprint(plan)
	return plan
}

func snapshotRestoreComponents(snapshot SetupSnapshot) map[string]restoreComponent {
	components := map[string]restoreComponent{}
	for _, agent := range snapshot.Preview.Agents {
		for _, component := range agent.Components {
			components[restoreKey(agent.ID, component.Source, component.Kind)] = restoreComponent{
				agentID: agent.ID, path: component.Source, kind: component.Kind,
				scope: component.Scope, format: component.Format, digest: component.Digest,
			}
		}
	}
	return components
}

func policyUnsupported(snapshot SetupSnapshot) []RestoreUnsupported {
	var unsupported []RestoreUnsupported
	for _, agent := range snapshot.Preview.Agents {
		for _, omitted := range agent.Omitted {
			unsupported = append(unsupported, RestoreUnsupported{
				Kind: omitted.Kind, Source: omitted.Source, Reason: omitted.Reason, Count: omitted.Count,
			})
		}
	}
	for _, class := range snapshot.Preview.Classes {
		switch class.Kind {
		case "credentials":
			unsupported = append(unsupported, RestoreUnsupported{Kind: class.Kind, Reason: "reference_names_only"})
		default:
			if class.Selection == "include" {
				continue
			}
			unsupported = append(unsupported, RestoreUnsupported{Kind: class.Kind, Reason: "class_" + class.Selection})
		}
	}
	return unsupported
}

func restoreKey(agentID, path, kind string) string {
	return agentID + "\x00" + path + "\x00" + kind
}

func sourceBase(path string) string {
	return pathpkg.Base(strings.ReplaceAll(path, `\`, "/"))
}

func destinationCandidates(report Report) []destinationCandidate {
	seen := map[string]bool{}
	var out []destinationCandidate
	for _, agent := range report.Agents {
		for _, loc := range agent.Locations {
			if loc.Kind != "configuration" && loc.Kind != "mcp_configuration" && loc.Kind != "instructions" && loc.Kind != "shared_instructions" {
				continue
			}
			for _, path := range configPaths(agent.ID, loc) {
				key := agent.ID + "\x00" + loc.Scope + "\x00" + path
				if seen[key] {
					continue
				}
				seen[key] = true
				state := loc.State
				if state == "present" {
					state = observeSource(path)
				}
				out = append(out, destinationCandidate{agentID: agent.ID, scope: loc.Scope, path: path, state: state})
			}
		}
	}
	return out
}

func matchDestination(candidates []destinationCandidate, component restoreComponent) (destinationCandidate, bool) {
	base := sourceBase(component.path)
	var matches []destinationCandidate
	for _, candidate := range candidates {
		if candidate.agentID == component.agentID && candidate.scope == component.scope && sourceBase(candidate.path) == base {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return destinationCandidate{}, false
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].path < matches[j].path })
	return matches[0], true
}

func restoreState(candidate destinationCandidate) (string, string) {
	switch candidate.state {
	case "missing":
		return "create", ""
	case "present":
		return "conflict", "destination_source_exists"
	default:
		return "blocked", "destination_" + candidate.state
	}
}

func stateRank(state string) int {
	switch state {
	case "blocked":
		return 3
	case "conflict":
		return 2
	case "create":
		return 1
	default:
		return 0
	}
}

func restoreFingerprint(plan RestorePlan) string {
	input := struct {
		SnapshotID     string               `json:"snapshot_id"`
		SnapshotDigest string               `json:"snapshot_digest"`
		Destination    RestoreDestination   `json:"destination"`
		Actions        []RestoreAction      `json:"actions"`
		Unsupported    []RestoreUnsupported `json:"unsupported"`
	}{plan.SnapshotID, plan.SnapshotDigest, plan.Destination, plan.Actions, plan.Unsupported}
	data, err := json.Marshal(input)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func loadSetupSnapshot(path, password, dir string) (SetupSnapshot, sealed.Manifest, error) {
	path, err := resolveSetupSnapshotPath(path, dir)
	if err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, fmt.Errorf("inspect setup snapshot: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return SetupSnapshot{}, sealed.Manifest{}, fmt.Errorf("the setup snapshot must be a regular file, not a symlink")
	}
	if info.Size() > sealed.MaxObjectBytes {
		return SetupSnapshot{}, sealed.Manifest{}, fmt.Errorf("the setup snapshot is larger than this build can open")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return SetupSnapshot{}, sealed.Manifest{}, fmt.Errorf("read setup snapshot: %w", err)
	}
	return openSetupSnapshot(raw, password)
}

func renderRestorePlan(out io.Writer, plan RestorePlan) error {
	if _, err := fmt.Fprintf(out, "Setup restore plan for %s\nSource: %s/%s; destination: %s/%s", plan.SnapshotID, plan.SourceOS, plan.SourceArchitecture, plan.Destination.OS, plan.Destination.Architecture); err != nil {
		return err
	}
	if plan.Destination.Project != "" {
		if _, err := fmt.Fprintf(out, "; project %q", plan.Destination.Project); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if len(plan.Actions) == 0 {
		if _, err := fmt.Fprintln(out, "No supported files would be restored."); err != nil {
			return err
		}
	}
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(out, "  %s %q from %s", action.State, action.Destination, strings.Join(action.Components, ", ")); err != nil {
			return err
		}
		if action.Reason != "" {
			if _, err := fmt.Fprintf(out, " (%s)", action.Reason); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	if len(plan.Software) > 0 {
		if _, err := fmt.Fprintln(out, "Application compatibility:"); err != nil {
			return err
		}
	}
	for _, software := range plan.Software {
		if _, err := fmt.Fprintf(out, "  %s: %s", software.Name, software.State); err != nil {
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
	for _, warning := range plan.Warnings {
		if _, err := fmt.Fprintf(out, "  warning: %s\n", warning); err != nil {
			return err
		}
	}
	if len(plan.Unsupported) > 0 {
		if _, err := fmt.Fprintln(out, "Unsupported or omitted:"); err != nil {
			return err
		}
	}
	for _, unsupported := range plan.Unsupported {
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
	if _, err := fmt.Fprintf(out, "\nPlan fingerprint: %s\n", plan.Fingerprint); err != nil {
		return err
	}
	for _, note := range plan.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}
