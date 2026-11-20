package setup

import (
	"encoding/hex"
	"fmt"

	"golang.org/x/crypto/blake2b"
)

const maxSkillBytes = 8 << 20

// SnapshotSkill is credential-free preview metadata for one installed skill
// folder. File contents stay inside the sealed snapshot. Setups no longer
// capture skill folders; these types read snapshots saved before.
type SnapshotSkill struct {
	Name       string `json:"name"`
	AgentID    string `json:"agent_id"`
	Scope      string `json:"scope"`
	Source     string `json:"source"`
	Origin     string `json:"origin,omitempty"`
	OriginType string `json:"origin_type,omitempty"`
	Files      int    `json:"files"`
	Bytes      int64  `json:"bytes"`
	Digest     string `json:"digest"`
}

// SetupSkill is one whole skill folder in the encrypted payload.
type SetupSkill struct {
	Name           string           `json:"name"`
	AgentID        string           `json:"agent_id"`
	Scope          string           `json:"scope"`
	Root           string           `json:"root"`
	LocationKind   string           `json:"location_kind,omitempty"`
	LocationSource string           `json:"location_source,omitempty"`
	Origin         string           `json:"origin,omitempty"`
	OriginType     string           `json:"origin_type,omitempty"`
	OriginPath     string           `json:"origin_path,omitempty"`
	Files          []SetupSkillFile `json:"files"`
	Digest         string           `json:"digest"`
}

type SetupSkillFile struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode,omitempty"`
	Data []byte `json:"data,omitempty"`
}

func skillBytes(files []SetupSkillFile) int64 {
	var total int64
	for _, file := range files {
		total += int64(len(file.Data))
	}
	return total
}

func setupSkillDigest(files []SetupSkillFile) string {
	var manifest []byte
	for _, file := range files {
		manifest = append(manifest, []byte(fmt.Sprintf("%s\x00%o\x00%d\x00", file.Path, file.Mode, len(file.Data)))...)
		manifest = append(manifest, file.Data...)
		manifest = append(manifest, 0)
	}
	sum := blake2b.Sum256(manifest)
	return hex.EncodeToString(sum[:])
}
