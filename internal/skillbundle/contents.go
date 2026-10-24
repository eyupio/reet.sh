package skillbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

// ContentFile is an explicit upload, not a filesystem instruction. Data is never
// executed or expanded. Executable uses the same bit-presence semantics as Scan.
type ContentFile struct {
	Path       string `json:"path"`
	Data       []byte `json:"data"`
	Executable bool   `json:"executable"`
}

// FromContents computes the same canonical identity as Scan, without filesystem
// access. It rejects ambiguous file/directory names and never normalizes bytes.
func FromContents(files []ContentFile) (Manifest, error) {
	m := Manifest{SchemaVersion: SchemaVersion, Algorithm: Algorithm, ModeSemantics: ModeSemantics, Files: []File{}}
	invalid := errors.New("invalid skill bundle contents")
	if len(files) == 0 || len(files) > MaxFiles {
		return m, invalid
	}
	nodes := map[string]bool{}
	found := false
	for _, file := range files {
		if !validPath(file.Path, MaxPathBytes) || strings.Count(file.Path, "/") > MaxDepth {
			return m, invalid
		}
		parts := strings.Split(file.Path, "/")
		for i := range parts {
			name := strings.Join(parts[:i+1], "/")
			directory := i < len(parts)-1
			if wasDirectory, ok := nodes[name]; ok {
				if !directory || !wasDirectory {
					return m, invalid
				}
				continue
			}
			for seen := range nodes {
				if strings.EqualFold(seen, name) {
					return m, invalid
				}
			}
			nodes[name] = directory
			if len(nodes) > MaxEntries {
				return m, invalid
			}
		}
		m.TotalBytes += int64(len(file.Data))
		if m.TotalBytes > MaxBytes {
			return m, invalid
		}
		sum := sha256.Sum256(file.Data)
		m.Files = append(m.Files, File{Path: file.Path, Bytes: int64(len(file.Data)), Digest: hex.EncodeToString(sum[:]), Executable: file.Executable})
		found = found || file.Path == "SKILL.md"
	}
	if !found {
		return m, invalid
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	m.Complete = true
	m.Digest = manifestDigest(m)
	return m, nil
}
