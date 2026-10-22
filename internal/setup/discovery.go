// Package setup inventories AI agent setups independently of a reet server.
package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Report describes only known locations and executable availability. It does not
// assert that configuration is readable, valid or sufficient for recovery.
type Report struct {
	Home                   string    `json:"-"`
	SchemaVersion          int       `json:"schema_version"`
	CapturedAt             time.Time `json:"captured_at"`
	OS                     string    `json:"os"`
	Architecture           string    `json:"architecture"`
	Project                string    `json:"project,omitempty"`
	Agents                 []Agent   `json:"agents"`
	Limitations            []string  `json:"limitations"`
	InstallationInspection bool      `json:"installation_inspection,omitempty"`
	VersionProbes          bool      `json:"version_probes,omitempty"`
	ExportPolicies         bool      `json:"export_policies,omitempty"`
}

type Agent struct {
	ID                 string                   `json:"id"`
	Name               string                   `json:"name"`
	State              string                   `json:"state"` // detected, not_detected, unknown
	Executable         Executable               `json:"executable"`
	Locations          []Location               `json:"locations"`
	Capabilities       Capabilities             `json:"capabilities"`
	Components         *ComponentInventory      `json:"components,omitempty"`
	Installation       *InstallationObservation `json:"installation,omitempty"`
	ApplicationVersion *VersionObservation      `json:"application_version,omitempty"`
	ExportPolicy       *ExportPolicy            `json:"export_policy,omitempty"`
}

type Executable struct {
	Command string `json:"command"`
	Path    string `json:"path,omitempty"`
	State   string `json:"state"` // present, missing, error
	Issue   string `json:"issue,omitempty"`
}

type Location struct {
	Scope    string `json:"scope"`
	Kind     string `json:"kind"`
	Path     string `json:"path,omitempty"`
	Source   string `json:"source"`
	Expected string `json:"expected"` // directory or file
	State    string `json:"state"`    // present, missing, symlink, error, unsupported
	Issue    string `json:"issue,omitempty"`
}

type Capabilities struct {
	Discovery    string `json:"discovery"`
	Backup       string `json:"backup"`
	Restore      string `json:"restore"`
	Version      string `json:"version,omitempty"`
	Installation string `json:"installation,omitempty"`
	ExportPolicy string `json:"export_policy,omitempty"`
}

type environment struct {
	home, project string
	getenv        func(string) string
	lookPath      func(string) (string, error)
	lstat         func(string) (os.FileInfo, error)
}

// Discover checks metadata only. It never reads configuration contents, walks
// directories, executes agents or sends a report to the controller.
func Discover(home, project string) (Report, error) {
	return discover(environment{home, project, os.Getenv, exec.LookPath, os.Lstat})
}

func discover(e environment) (Report, error) {
	if !filepath.IsAbs(e.home) {
		return Report{}, fmt.Errorf("home directory must be absolute")
	}
	if e.project != "" {
		if !filepath.IsAbs(e.project) {
			return Report{}, fmt.Errorf("project directory must be absolute")
		}
		info, err := e.lstat(e.project)
		if err != nil || !info.IsDir() {
			return Report{}, fmt.Errorf("project must be an accessible directory, not a file or symlink")
		}
	}
	r := Report{
		Home:          e.home,
		SchemaVersion: 1, CapturedAt: time.Now().UTC(), OS: runtime.GOOS,
		Architecture: runtime.GOARCH, Project: e.project,
		Agents: []Agent{},
		Limitations: []string{
			"Metadata-only discovery of known paths; configuration contents and nested files are not read.",
			noVersionProbeNote,
			"Configuration may remain after uninstalling an agent; a detected setup is not proof of a working installation.",
			"Configuration contents are not read by discovery; MCP definitions, credentials, memory and history are collected only by explicit snapshot commands or remain unsupported.",
			"Symlink locations are reported without inspecting their targets; missing known paths do not prove an agent is absent.",
		},
	}
	for _, adapter := range adapters() {
		a := Agent{
			ID: adapter.id, Name: adapter.name, State: "not_detected",
			Executable:   Executable{Command: adapter.command, State: "missing"},
			Locations:    []Location{},
			Capabilities: Capabilities{Discovery: "metadata_only", Backup: "not_implemented", Restore: "not_implemented"},
		}
		path, err := e.lookPath(adapter.command)
		switch {
		case err == nil:
			a.Executable.Path, a.Executable.State = path, "present"
		case errors.Is(err, exec.ErrNotFound):
		default:
			a.Executable.State, a.Executable.Issue = "error", "executable_lookup_failed"
		}
		for _, spec := range adapter.locations {
			if spec.scope == "project" && e.project == "" {
				continue
			}
			if spec.path == "" && e.getenv(spec.env) == "" {
				continue // An optional override is not a default home location.
			}
			a.Locations = append(a.Locations, inspect(e, spec))
		}
		uncertain := a.Executable.State == "error"
		detected := a.Executable.State == "present"
		for _, loc := range a.Locations {
			detected = detected || (loc.State == "present" && loc.Kind != "skills" && loc.Kind != "shared_skills" && loc.Kind != "shared_instructions")
			uncertain = uncertain || (loc.State != "present" && loc.State != "missing")
		}
		memoryLocations(e, &a)
		if detected {
			a.State = "detected"
		} else if uncertain {
			a.State = "unknown"
		}
		r.Agents = append(r.Agents, a)
	}
	return r, nil
}

func inspect(e environment, spec locationSpec) Location {
	loc := Location{Scope: spec.scope, Kind: spec.kind, Expected: spec.expected, Source: "default"}
	root := e.home
	if spec.scope == "project" {
		root = e.project
	}
	if spec.env != "" && e.getenv(spec.env) != "" {
		root = e.getenv(spec.env)
		loc.Source = spec.env
		if !filepath.IsAbs(root) {
			loc.State, loc.Issue = "error", "override_must_be_absolute"
			return loc
		}
		loc.Path = filepath.Join(root, spec.suffix)
	} else {
		loc.Path = filepath.Join(root, spec.path)
	}
	info, err := e.lstat(loc.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		loc.State = "missing"
	case err != nil:
		loc.State, loc.Issue = "error", "metadata_unavailable"
	case info.Mode()&os.ModeSymlink != 0:
		loc.State, loc.Issue = "symlink", "target_not_inspected"
	case (spec.expected == "directory" && info.IsDir()) || (spec.expected == "file" && info.Mode().IsRegular()):
		loc.State = "present"
	default:
		loc.State, loc.Issue = "unsupported", "unexpected_file_type"
	}
	return loc
}
