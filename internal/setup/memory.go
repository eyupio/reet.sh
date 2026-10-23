package setup

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const maxMemoryBytes = 64 << 20

// SnapshotMemory is credential-free metadata for one supported memory database.
type SnapshotMemory struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Bytes   int64  `json:"bytes"`
	Digest  string `json:"digest"`
}

// SetupMemory is a consistent SQLite copy of a supported memory database.
type SetupMemory struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Data    []byte `json:"data"`
	Digest  string `json:"digest"`
}

func memoryLocations(e environment, agent *Agent) {
	if agent.ID != "codex" {
		return
	}
	root := filepath.Join(e.home, ".codex")
	if override := e.getenv("CODEX_HOME"); override != "" {
		if !filepath.IsAbs(override) {
			return
		}
		root = override
	}
	matches, err := filepath.Glob(filepath.Join(root, "memories_*.sqlite"))
	if err != nil {
		return
	}
	sort.Strings(matches)
	for _, path := range matches {
		info, err := e.lstat(path)
		if err != nil {
			continue
		}
		state := "unsupported"
		if info.Mode()&os.ModeSymlink != 0 {
			state = "symlink"
		} else if info.Mode().IsRegular() {
			state = "present"
		}
		agent.Locations = append(agent.Locations, Location{
			Scope: "global", Kind: "memory", Path: path, Source: "default",
			Expected: "file", State: state,
		})
	}
}

func selectMemoryContent(report Report, preview *SnapshotPreview, selection *SnapshotSelection) {
	for i := range report.Agents {
		for _, loc := range report.Agents[i].Locations {
			if loc.Kind != "memory" || loc.State != "present" {
				continue
			}
			name := filepath.Base(loc.Path)
			if !validMemoryName(name) {
				preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, SnapshotOmission{Kind: "memory", Source: loc.Path, Reason: "invalid_memory_name"})
				continue
			}
			data, err := snapshotSQLite(loc.Path)
			if err != nil {
				preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, SnapshotOmission{Kind: "memory", Source: loc.Path, Reason: "memory_unavailable"})
				continue
			}
			if int64(len(data)) > maxMemoryBytes {
				preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, SnapshotOmission{Kind: "memory", Source: loc.Path, Reason: "memory_too_large"})
				continue
			}
			memory := SetupMemory{Name: name, AgentID: report.Agents[i].ID, Scope: loc.Scope, Source: loc.Path, Data: data, Digest: sha256Bytes(data)}
			preview.Memories = append(preview.Memories, SnapshotMemory{
				Name: name, AgentID: memory.AgentID, Scope: memory.Scope, Source: memory.Source,
				Bytes: int64(len(data)), Digest: memory.Digest,
			})
			selection.memories = append(selection.memories, memory)
		}
	}
	sort.Slice(preview.Memories, func(i, j int) bool {
		if preview.Memories[i].AgentID != preview.Memories[j].AgentID {
			return preview.Memories[i].AgentID < preview.Memories[j].AgentID
		}
		return preview.Memories[i].Name < preview.Memories[j].Name
	})
}

func validMemoryName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 128 && filepath.Base(name) == name &&
		!strings.ContainsAny(name, `/\\`) && strings.HasSuffix(name, ".sqlite")
}

func snapshotSQLite(path string) ([]byte, error) {
	// SQLite creates the VACUUM output itself. Keep it inside a private directory.
	tmpDir, err := os.MkdirTemp("", "reet-memory-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, "memory.sqlite")
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	quoted := strings.ReplaceAll(tmpPath, "'", "''")
	if _, err := db.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return nil, fmt.Errorf("copy memory database: %w", err)
	}
	info, err := os.Lstat(tmpPath)
	if err != nil {
		return nil, err
	}
	data, issue := readRegularFile(tmpPath, info, maxMemoryBytes)
	if issue != "" {
		return nil, fmt.Errorf("read memory copy: %s", issue)
	}
	return data, nil
}

func (m SetupMemory) sha256() string { return sha256Bytes(m.Data) }
