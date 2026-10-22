package setup

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const maxPackageAncestors = 6

// InstallationObservation records local evidence, not publisher authenticity or
// an instruction to reinstall. Different package managers use the same layout.
type InstallationObservation struct {
	State        string `json:"state"`
	Kind         string `json:"kind"`
	Manager      string `json:"manager"`
	Package      string `json:"package,omitempty"`
	Version      string `json:"version,omitempty"`
	MetadataPath string `json:"metadata_path,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
	Issue        string `json:"issue,omitempty"`
}

func nodePackageNames(agent string) []string {
	return map[string][]string{
		"claude": {"@anthropic-ai/claude-code"}, "codex": {"@openai/codex"},
		"opencode": {"@opencode/cli", "opencode-ai"},
		"pi":       {"@earendil-works/pi-coding-agent", "@mariozechner/pi-coding-agent"},
		"gemini":   {"@google/gemini-cli"},
	}[agent]
}

// InspectInstallations is an independent opt-in. It reads bounded package.json
// files near resolved executables; no agents, package managers or scripts run.
func InspectInstallations(r *Report) {
	r.SchemaVersion = 2
	r.InstallationInspection = true
	r.Limitations = append(r.Limitations,
		"Installation inspection resolves executable symlinks and reads nearby package.json files; it never executes commands or queries registries.",
		"Matched local package metadata is not authenticated provenance. Package managers, native installers, wrappers and reproducible installation remain unverified.",
	)
	for i := range r.Agents {
		a := &r.Agents[i]
		a.Capabilities.Installation = "local_node_package_evidence"
		if len(nodePackageNames(a.ID)) == 0 {
			a.Capabilities.Installation = "not_supported"
		}
		a.Installation = inspectInstallation(*a)
	}
}

func inspectInstallation(a Agent) *InstallationObservation {
	v := &InstallationObservation{State: "unknown", Kind: "unknown", Manager: "unknown"}
	if a.Executable.State == "missing" {
		v.State = "not_detected"
		return v
	}
	if a.Executable.State != "present" || !filepath.IsAbs(a.Executable.Path) {
		v.Issue = "executable_unavailable"
		return v
	}
	if len(nodePackageNames(a.ID)) == 0 {
		v.Issue = "installation_adapter_not_supported"
		return v
	}
	resolved, err := filepath.EvalSymlinks(a.Executable.Path)
	if err != nil {
		v.State, v.Issue = "error", "executable_unavailable"
		return v
	}
	before, err := os.Stat(resolved)
	if err != nil || !before.Mode().IsRegular() {
		v.State, v.Issue = "error", "executable_unavailable"
		return v
	}
	// Stop at the nearest package boundary, even if it is malformed or unrelated.
	// An outer package must not be attributed to a nested dependency's executable.
	dir := filepath.Dir(resolved)
	for i := 0; i < maxPackageAncestors; i++ {
		path := filepath.Join(dir, "package.json")
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
			continue
		}
		if err != nil {
			v.State, v.Issue = "error", "package_metadata_unavailable"
			return v
		}
		if !info.Mode().IsRegular() {
			v.Issue = "package_metadata_not_regular"
			return v
		}
		data, issue := readConfig(path, info)
		if issue != "" {
			v.State, v.Issue = "error", "package_"+issue
			return v
		}
		object, err := parseConfig(data)
		if err != nil {
			v.State, v.Issue = "error", "invalid_package_metadata"
			return v
		}
		name, _ := object["name"].(string)
		if !slices.Contains(nodePackageNames(a.ID), name) {
			v.Issue = "unrecognised_package"
			return v
		}
		version, _ := object["version"].(string)
		if version == "" || reportedVersion("", version) != version {
			v.Issue = "invalid_package_version"
			return v
		}
		bins, _ := object["bin"].(map[string]any)
		bin, _ := bins[a.Executable.Command].(string)
		// Only the explicit per-command bin map is supported. Reject paths that
		// depend on host-specific separator/drive interpretation or escape root.
		if !filepath.IsLocal(bin) || strings.ContainsAny(bin, "\\:") {
			v.Issue = "unsupported_package_bin"
			return v
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			v.State, v.Issue = "error", "package_metadata_unavailable"
			return v
		}
		declared, statErr := root.Stat(bin)
		root.Close()
		if statErr != nil || !declared.Mode().IsRegular() || !os.SameFile(before, declared) {
			v.Issue = "package_bin_mismatch"
			return v
		}
		after, err := os.Stat(a.Executable.Path)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			v.State, v.Issue = "error", "executable_changed"
			return v
		}
		v.State, v.Kind = "matched", "node_package"
		v.Package, v.Version, v.MetadataPath = name, version, path
		v.Evidence = "package_bin_matches_executable"
		return v
	}
	v.Issue = "package_metadata_not_found_within_limit"
	return v
}
