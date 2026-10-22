package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const snapshotPreviewSchemaVersion = 1

// SnapshotPreview describes a policy-driven selection without writing an
// archive. It contains names and counts only, never selected values.
type SnapshotPreview struct {
	SchemaVersion int                 `json:"schema_version"`
	PolicyVersion int                 `json:"policy_version"`
	CapturedAt    time.Time           `json:"captured_at"`
	OS            string              `json:"os"`
	Architecture  string              `json:"architecture"`
	Project       string              `json:"project,omitempty"`
	Agents        []SnapshotAgent     `json:"agents"`
	Classes       []ClassExportPolicy `json:"classes"`
	Limitations   []string            `json:"limitations"`
}

type SnapshotAgent struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	State      string              `json:"state"`
	Components []SnapshotComponent `json:"components"`
	Omitted    []SnapshotOmission  `json:"omitted,omitempty"`
}

type SnapshotComponent struct {
	Kind                 string   `json:"kind"`
	Source               string   `json:"source"`
	Scope                string   `json:"scope"`
	Format               string   `json:"format"`
	Observed             string   `json:"observed"`
	Selection            string   `json:"selection"`
	Declarations         int      `json:"declarations,omitempty"`
	SelectedDeclarations int      `json:"selected_declarations,omitempty"`
	IncludedFields       []string `json:"included_fields,omitempty"`
	EncryptedFields      []string `json:"encrypted_fields,omitempty"`
	ReferenceFields      []string `json:"reference_fields,omitempty"`
	ExcludedFields       []string `json:"excluded_fields,omitempty"`
	UnknownFields        int      `json:"unknown_fields,omitempty"`
	Digest               string   `json:"digest,omitempty"`
}

type SnapshotOmission struct {
	Kind   string `json:"kind"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason"`
	Count  int    `json:"count,omitempty"`
}

// SnapshotSelection keeps the selected content in memory for a future archive
// writer. It is deliberately not serialised by the preview command.
type SnapshotSelection struct {
	Preview SnapshotPreview
	sources []selectedSnapshotSource
}

type selectedSnapshotSource struct {
	Path      string
	Component string
	Content   map[string]any
}

// BuildSnapshotSelection applies the declared export policies to supported
// sources. It reads bounded known files but returns no values in the preview.
func BuildSnapshotSelection(r Report) (SnapshotSelection, error) {
	cp := r
	cp.Agents = append([]Agent(nil), r.Agents...)
	ApplyExportPolicies(&cp)
	preview := SnapshotPreview{
		SchemaVersion: snapshotPreviewSchemaVersion,
		PolicyVersion: exportPolicyVersion,
		CapturedAt:    time.Now().UTC(),
		OS:            cp.OS,
		Architecture:  cp.Architecture,
		Project:       cp.Project,
		Agents:        []SnapshotAgent{},
		Classes:       classExportPolicies(),
		Limitations: []string{
			"Preview reads supported configuration files into memory to apply policy; selected values are never returned or written.",
			"Credential-bearing fields are counted but not selected; portable credentials remain unimplemented.",
			"Opt-in sources are not read. Unsupported classes and unknown fields are reported, not collected.",
			"This is a preview, not a backup: archive writing, encryption, key management, restore and cross-OS translation are not implemented.",
		},
	}
	selection := SnapshotSelection{Preview: preview}
	for i := range cp.Agents {
		a := &cp.Agents[i]
		sa := SnapshotAgent{ID: a.ID, Name: a.Name, State: a.State, Components: []SnapshotComponent{}, Omitted: []SnapshotOmission{}}
		if a.ExportPolicy != nil {
			for _, source := range a.ExportPolicy.Sources {
				addSourceSelection(&sa, &selection, a.ID, source)
			}
		}
		preview.Agents = append(preview.Agents, sa)
	}
	selection.Preview = preview
	return selection, nil
}

func addSourceSelection(agent *SnapshotAgent, selection *SnapshotSelection, agentID string, source SourceExportPolicy) {
	if source.Observed != "present" {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: "source", Source: source.Path, Reason: "source_" + source.Observed})
		return
	}
	allOptIn := len(source.Components) > 0
	for _, component := range source.Components {
		if component.Selection != "opt_in" {
			allOptIn = false
			break
		}
	}
	if allOptIn {
		for _, component := range source.Components {
			agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "source_opt_in", Count: 1})
		}
		return
	}
	object, state, issue := readConfigObject(source.Path, source.Format, agentID)
	if state != "parsed" {
		reason := "source_" + state
		if issue != "" {
			reason = issue
		}
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: "source", Source: source.Path, Reason: reason})
		return
	}
	for _, component := range source.Components {
		switch component.Selection {
		case "include":
			addIncludedComponent(agent, selection, source, component, object)
		case "opt_in":
			agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "source_opt_in", Count: 1})
		case "unsupported":
			agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "component_unsupported"})
		default:
			agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "component_unsupported"})
		}
	}
}

func addIncludedComponent(agent *SnapshotAgent, selection *SnapshotSelection, source SourceExportPolicy, component ComponentExportPolicy, object map[string]any) {
	preview := SnapshotComponent{
		Kind: component.Kind, Source: source.Path, Scope: source.Scope, Format: source.Format,
		Observed: source.Observed, Selection: component.Selection,
	}
	var content map[string]any
	var stats snapshotFieldStats
	switch component.Kind {
	case "settings":
		content, stats = selectSettingsContent(object, component)
	case "mcp":
		var mcp snapshotMCPSelection
		mcp = selectMCPContent(object, component, agent.ID)
		content, stats, preview.Declarations, preview.SelectedDeclarations = mcp.content, mcp.stats, mcp.declarations, mcp.selected
		if !mcp.valid {
			agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "invalid_mcp_declarations", Count: 1})
		}
	default:
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: component.Kind, Source: source.Path, Reason: "component_unsupported"})
		return
	}
	preview.IncludedFields = stats.included
	preview.EncryptedFields = stats.encrypted
	preview.ReferenceFields = stats.references
	preview.ExcludedFields = stats.excluded
	preview.UnknownFields = stats.unknown
	preview.Digest = contentDigest(content)
	agent.Components = append(agent.Components, preview)

	if len(content) > 0 {
		selection.sources = append(selection.sources, selectedSnapshotSource{Path: source.Path, Component: component.Kind, Content: content})
	}
	addFieldOmissions(agent, source.Path, component.Kind, stats, len(content) == 0, preview.Declarations, preview.SelectedDeclarations)
}

type snapshotFieldStats struct {
	included, encrypted, references, excluded []string
	unknown                                   int
}

func (s *snapshotFieldStats) add(state, path string) {
	switch state {
	case "include":
		s.included = appendUnique(s.included, path)
	case "encrypted_only":
		s.encrypted = appendUnique(s.encrypted, path)
	case "reference_only":
		s.references = appendUnique(s.references, path)
	case "exclude":
		s.excluded = appendUnique(s.excluded, path)
	}
}

func (s *snapshotFieldStats) sort() {
	sort.Strings(s.included)
	sort.Strings(s.encrypted)
	sort.Strings(s.references)
	sort.Strings(s.excluded)
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func selectSettingsContent(object map[string]any, policy ComponentExportPolicy) (map[string]any, snapshotFieldStats) {
	selected := map[string]any{}
	stats := snapshotFieldStats{}
	known := map[string]bool{}
	for _, field := range policy.Fields {
		if strings.Contains(field.Path, ".") {
			continue
		}
		known[field.Path] = true
		value, exists := object[field.Path]
		if !exists {
			continue
		}
		stats.add(field.State, field.Path)
		if field.State == "include" || field.State == "reference_only" {
			selected[field.Path] = value
		}
	}
	for key := range object {
		if !known[key] {
			stats.unknown++
		}
	}
	stats.sort()
	return selected, stats
}

type snapshotMCPSelection struct {
	content                map[string]any
	stats                  snapshotFieldStats
	declarations, selected int
	valid                  bool
}

func selectMCPContent(object map[string]any, policy ComponentExportPolicy, agent string) snapshotMCPSelection {
	container := mcpContainer(agent)
	result := snapshotMCPSelection{content: map[string]any{}, valid: true}
	raw, exists := object[container]
	if !exists {
		return result
	}
	servers, ok := raw.(map[string]any)
	if !ok || len(servers) > 256 {
		result.valid = false
		return result
	}
	fields := map[string]string{}
	for _, field := range policy.Fields {
		if strings.HasPrefix(field.Path, "*.") {
			fields[strings.TrimPrefix(field.Path, "*.")] = field.State
		}
	}
	for name, rawDeclaration := range servers {
		result.declarations++
		declaration, ok := rawDeclaration.(map[string]any)
		if !ok {
			result.stats.unknown++
			continue
		}
		selected := map[string]any{}
		for field, value := range declaration {
			state, known := fields[field]
			if !known {
				result.stats.unknown++
				continue
			}
			result.stats.add(state, field)
			if state == "include" || state == "reference_only" {
				selected[field] = value
			}
		}
		if len(selected) > 0 {
			result.content[name] = selected
			result.selected++
		}
	}
	result.stats.sort()
	return result
}

func contentDigest(content map[string]any) string {
	if len(content) == 0 {
		return ""
	}
	data, err := json.Marshal(content)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func addFieldOmissions(agent *SnapshotAgent, source, kind string, stats snapshotFieldStats, noSelected bool, declarations, selectedDeclarations int) {
	if len(stats.encrypted) > 0 {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: kind, Source: source, Reason: "credentials_not_selected", Count: len(stats.encrypted)})
	}
	if len(stats.excluded) > 0 {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: kind, Source: source, Reason: "excluded_by_policy", Count: len(stats.excluded)})
	}
	if stats.unknown > 0 {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: kind, Source: source, Reason: "unlisted_fields", Count: stats.unknown})
	}
	if declarations > selectedDeclarations {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: kind, Source: source, Reason: "declarations_without_selected_fields", Count: declarations - selectedDeclarations})
	}
	if noSelected {
		agent.Omitted = append(agent.Omitted, SnapshotOmission{Kind: kind, Source: source, Reason: "no_selected_fields", Count: 1})
	}
}

func renderSnapshotPreview(out io.Writer, preview SnapshotPreview) error {
	if _, err := fmt.Fprintf(out, "AI setup backup preview (%s/%s)\nPolicy version %d; no archive written.\n", preview.OS, preview.Architecture, preview.PolicyVersion); err != nil {
		return err
	}
	for _, agent := range preview.Agents {
		if _, err := fmt.Fprintf(out, "\n%s: %s\n", agent.Name, agent.State); err != nil {
			return err
		}
		if len(agent.Components) == 0 {
			if _, err := fmt.Fprintln(out, "  No components selected."); err != nil {
				return err
			}
		}
		for _, component := range agent.Components {
			if _, err := fmt.Fprintf(out, "  %s %q (%s): %s", component.Kind, component.Source, component.Scope, component.Selection); err != nil {
				return err
			}
			if component.Declarations > 0 {
				if _, err := fmt.Fprintf(out, "; declarations %d/%d", component.SelectedDeclarations, component.Declarations); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
			for _, group := range []struct {
				name, state string
			}{
				{"included", "include"}, {"encrypted", "encrypted_only"},
				{"references", "reference_only"}, {"excluded", "exclude"},
			} {
				fields := previewFieldNames(component, group.state)
				if len(fields) > 0 {
					if _, err := fmt.Fprintf(out, "    %s: %s\n", group.name, strings.Join(fields, ", ")); err != nil {
						return err
					}
				}
			}
			if component.UnknownFields > 0 {
				if _, err := fmt.Fprintf(out, "    unlisted fields omitted: %d\n", component.UnknownFields); err != nil {
					return err
				}
			}
			if component.Digest != "" {
				if _, err := fmt.Fprintf(out, "    selection digest: %s\n", component.Digest); err != nil {
					return err
				}
			}
		}
		for _, omitted := range agent.Omitted {
			if _, err := fmt.Fprintf(out, "  omit %s %q: %s", omitted.Kind, omitted.Source, omitted.Reason); err != nil {
				return err
			}
			if omitted.Count > 0 {
				if _, err := fmt.Fprintf(out, " (%d)", omitted.Count); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(out); err != nil {
				return err
			}
		}
	}
	for _, class := range preview.Classes {
		if _, err := fmt.Fprintf(out, "\nClass %s: %s, %s\n  %s\n", class.Kind, class.Selection, class.Handling, class.Reason); err != nil {
			return err
		}
	}
	for _, note := range preview.Limitations {
		if _, err := fmt.Fprintf(out, "\n%s\n", note); err != nil {
			return err
		}
	}
	return nil
}

func previewFieldNames(component SnapshotComponent, state string) []string {
	switch state {
	case "include":
		return component.IncludedFields
	case "encrypted_only":
		return component.EncryptedFields
	case "reference_only":
		return component.ReferenceFields
	case "exclude":
		return component.ExcludedFields
	default:
		return nil
	}
}
