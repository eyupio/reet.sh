package skillbundle

import (
	"encoding/hex"
	"path"
	"sort"
	"strings"
)

type Change struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"` // added, removed or modified; rename is remove + add.
	Before *File  `json:"before,omitempty"`
	After  *File  `json:"after,omitempty"`
}

type Diff struct {
	State   string   `json:"state"` // equal, changed or unknown.
	Changes []Change `json:"changes"`
}

// Compare is pure and directional: previous -> current. Missing, incomplete,
// incompatible or malformed observations are unknown, never empty/equal trees.
// Validation checks manifest consistency, not publisher authenticity or ownership.
func Compare(previous, current *Manifest) Diff {
	result := Diff{State: "unknown", Changes: []Change{}}
	if !validManifest(previous) || !validManifest(current) {
		return result
	}
	result.State = "equal"
	a, b := 0, 0
	for a < len(previous.Files) || b < len(current.Files) {
		switch {
		case b == len(current.Files) || a < len(previous.Files) && previous.Files[a].Path < current.Files[b].Path:
			file := previous.Files[a]
			result.Changes = append(result.Changes, Change{Path: file.Path, Kind: "removed", Before: &file})
			a++
		case a == len(previous.Files) || current.Files[b].Path < previous.Files[a].Path:
			file := current.Files[b]
			result.Changes = append(result.Changes, Change{Path: file.Path, Kind: "added", After: &file})
			b++
		default:
			before, after := previous.Files[a], current.Files[b]
			if before != after {
				result.Changes = append(result.Changes, Change{Path: before.Path, Kind: "modified", Before: &before, After: &after})
			}
			a++
			b++
		}
	}
	if len(result.Changes) > 0 {
		result.State = "changed"
	}
	return result
}

func validManifest(m *Manifest) bool {
	if m == nil || !m.Complete || len(m.Issues) > 0 || m.SchemaVersion != SchemaVersion || m.Algorithm != Algorithm || m.ModeSemantics != ModeSemantics || len(m.Files) == 0 || len(m.Files) > MaxFiles || !sort.SliceIsSorted(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path }) {
		return false
	}
	var size int64
	found := false
	seen := map[string]bool{}
	for _, file := range m.Files {
		if !validPath(file.Path, MaxPathBytes) || file.Bytes < 0 || file.Bytes > MaxBytes || len(file.Digest) != 64 || strings.ToLower(file.Digest) != file.Digest {
			return false
		}
		if _, err := hex.DecodeString(file.Digest); err != nil {
			return false
		}
		if strings.Count(file.Path, "/") > MaxDepth {
			return false
		}
		names := []string{file.Path}
		for parent := path.Dir(file.Path); parent != "."; parent = path.Dir(parent) {
			names = append(names, parent)
		}
		for i, name := range names {
			isDirectory := i > 0
			for existing := range seen {
				if existing != name && strings.EqualFold(existing, name) {
					return false
				}
			}
			if priorDirectory, exists := seen[name]; exists && (!isDirectory || !priorDirectory) {
				return false
			}
			seen[name] = isDirectory
			if len(seen) > MaxEntries {
				return false
			}
		}
		size += file.Bytes
		if size > MaxBytes {
			return false
		}
		if file.Path == "SKILL.md" {
			found = true
		}
	}
	return found && size == m.TotalBytes && m.Digest == manifestDigest(*m)
}
