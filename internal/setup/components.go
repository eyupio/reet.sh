package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxConfigBytes = 1 << 20

// ComponentInventory is a summary of explicitly supported files, not an export
// of settings. Unknown settings and credential values never enter the report.
type ComponentInventory struct {
	Sources     []ConfigSource `json:"sources"`
	Limitations []string       `json:"limitations"`
}

type ConfigSource struct {
	Path     string           `json:"path"`
	Scope    string           `json:"scope"`
	Format   string           `json:"format"`
	State    string           `json:"state"`
	Issue    string           `json:"issue,omitempty"`
	Settings *SettingsSummary `json:"settings,omitempty"`
	MCPState string           `json:"mcp_state,omitempty"`
	MCP      []MCPSummary     `json:"mcp,omitempty"`
}

type SettingsSummary struct {
	ModelConfigured       bool `json:"model_configured"`
	HooksConfigured       bool `json:"hooks_configured"`
	PermissionsConfigured bool `json:"permissions_configured"`
	PackagesConfigured    bool `json:"packages_configured"`
	EnvironmentFields     int  `json:"environment_fields"`
	ReferenceFields       int  `json:"reference_fields"`
	LiteralFields         int  `json:"literal_fields"`
}

// Names are configuration identifiers; all command, URL, argument, environment
// and header values are deliberately omitted, including seemingly harmless ones.
type MCPSummary struct {
	Name              string `json:"name"`
	Transport         string `json:"transport"`
	State             string `json:"state"`
	Issue             string `json:"issue,omitempty"`
	CommandConfigured bool   `json:"command_configured"`
	URLConfigured     bool   `json:"url_configured"`
	ArgumentCount     int    `json:"argument_count"`
	EnvironmentFields int    `json:"environment_fields"`
	HeaderFields      int    `json:"header_fields"`
	ReferenceFields   int    `json:"reference_fields"`
	LiteralFields     int    `json:"literal_fields"`
}

// InspectComponents opts into bounded content reads. The original metadata-only
// Discover contract and schema stay unchanged unless this function is called.
func InspectComponents(r *Report) {
	r.SchemaVersion = 2
	r.Limitations = []string{
		"Component summaries read only known configuration files; values are omitted, but paths and MCP names may identify private projects or services.",
		noVersionProbeNote,
		"Files are observed separately; agent merge precedence and effective settings are not resolved.",
		"Setup backup and restore are not implemented; this report cannot recreate a setup.",
	}
	for i := range r.Agents {
		a := &r.Agents[i]
		a.Capabilities.Discovery = "metadata_and_config_summary"
		a.Components = &ComponentInventory{
			Sources: []ConfigSource{},
			Limitations: []string{
				"Supported formats: strict JSON, OpenCode JSONC and Codex TOML; other source formats remain unsupported.",
				"Only selected settings and MCP declarations are summarised; other fields, nested project settings, memory, history and credential stores are not inventoried.",
				"Credential-bearing values are counted, never returned; literal fields may be secrets and require explicit encrypted-backup selection later.",
				"Each file is limited to 1 MiB; symlink files and non-regular files are not read.",
			},
		}
		seen := map[string]bool{}
		for _, loc := range a.Locations {
			if loc.Kind != "configuration" && loc.Kind != "mcp_configuration" {
				continue
			}
			for _, path := range configPaths(a.ID, loc) {
				key := loc.Scope + "\x00" + path
				if seen[key] {
					continue
				}
				seen[key] = true
				source := ConfigSource{Path: path, Scope: loc.Scope, Format: strings.TrimPrefix(filepath.Ext(path), ".")}
				if loc.State != "present" {
					source.State, source.Issue = loc.State, loc.Issue
				} else {
					inspectConfig(&source, a.ID)
				}
				a.Components.Sources = append(a.Components.Sources, source)
			}
		}
	}
}

func configPaths(agent string, loc Location) []string {
	if loc.Path == "" {
		return nil // Invalid overrides are already reported by discovery.
	}
	if loc.Expected == "file" {
		return []string{loc.Path}
	}
	var names []string
	switch agent {
	case "claude":
		names = []string{"settings.json", "settings.local.json"}
	case "codex":
		names = []string{"config.toml"}
	case "opencode":
		names = []string{"opencode.json", "opencode.jsonc"}
	case "pi", "gemini":
		names = []string{"settings.json"}
	case "cursor":
		names = []string{"mcp.json"}
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(loc.Path, name))
	}
	return paths
}

func inspectConfig(source *ConfigSource, agent string) {
	object, state, issue := readConfigObject(source.Path, source.Format, agent)
	if state != "parsed" {
		source.State, source.Issue = state, issue
		return
	}
	source.State = "parsed"
	source.Settings = summariseSettings(object, agent)
	key := "mcpServers"
	if agent == "opencode" {
		key = "mcp"
	}
	if agent == "codex" {
		key = "mcp_servers"
	}
	if agent == "pi" {
		source.MCPState = "not_supported_by_adapter"
		return
	}
	value, exists := object[key]
	if !exists {
		source.MCPState = "absent_in_file"
		return
	}
	servers, ok := value.(map[string]any)
	if !ok || len(servers) > 256 {
		source.MCPState = "unsupported"
		source.Issue = "invalid_or_excessive_mcp_declarations"
		return
	}
	source.MCPState = "parsed"
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if len(name) > 256 {
			source.MCPState, source.Issue = "partial", "mcp_name_too_long"
			continue
		}
		m := summariseMCP(name, servers[name], agent)
		if m.State != "summarised" {
			source.MCPState = "partial"
		}
		source.MCP = append(source.MCP, m)
	}
}

func readConfigObject(path, format, agent string) (map[string]any, string, string) {
	info, err := os.Lstat(path)
	switch {
	case os.IsNotExist(err):
		return nil, "missing", ""
	case err != nil:
		return nil, "error", "metadata_unavailable"
	case info.Mode()&os.ModeSymlink != 0:
		return nil, "symlink", "target_not_inspected"
	case !info.Mode().IsRegular():
		return nil, "unsupported", "unexpected_file_type"
	case !supportedFormat(format, agent):
		return nil, "unsupported", "format_not_supported"
	}
	data, issue := readConfig(path, info)
	if issue != "" {
		return nil, "error", issue
	}
	object, err := parseSource(data, format)
	if err != nil {
		// Parser errors can quote credentials from the input. Emit a fixed code.
		return nil, "error", "invalid_or_complex_" + format
	}
	return object, "parsed", ""
}

func readConfig(path string, before os.FileInfo) ([]byte, string) {
	return readRegularFile(path, before, maxConfigBytes)
}

// readRegularFile confines and bounds reads, checking the opened inode and changes.
func readRegularFile(path string, before os.FileInfo, limit int64) ([]byte, string) {
	if before.Size() > limit {
		return nil, "file_too_large"
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, "read_unavailable"
	}
	defer root.Close()
	f, err := root.OpenFile(filepath.Base(path), os.O_RDONLY|configReadFlags, 0)
	if err != nil {
		return nil, "read_unavailable"
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, "file_changed"
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, "read_unavailable"
	}
	if int64(len(data)) > limit {
		return nil, "file_too_large"
	}
	after, err := f.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != after.Size() {
		return nil, "file_changed"
	}
	return data, ""
}

// Decode through tokens so duplicate keys cannot hide a declaration or cause a
// last-write-wins interpretation. Depth and node limits bound parser work.
func parseConfig(data []byte) (map[string]any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	nodes := 0
	v, err := configValue(d, 0, &nodes)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("trailing input")
	}
	object, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("object required")
	}
	return object, nil
}

func configValue(d *json.Decoder, depth int, nodes *int) (any, error) {
	*nodes++
	if depth > 64 || *nodes > 20000 {
		return nil, fmt.Errorf("complexity limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid key")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate key")
			}
			value, err := configValue(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		_, err := d.Token()
		return object, err
	case json.Delim('['):
		array := []any{}
		for d.More() {
			value, err := configValue(d, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		_, err := d.Token()
		return array, err
	default:
		return token, nil
	}
}
