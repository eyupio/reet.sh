package setup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxSkillFiles      = 256
	maxSkillBytes      = 8 << 20
	maxSetupSkillFiles = 256
	maxSetupSkillBytes = 64 << 20
)

// SnapshotSkill is credential-free preview metadata for one installed skill
// folder. File contents stay inside the sealed snapshot.
type SnapshotSkill struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Scope   string `json:"scope"`
	Source  string `json:"source"`
	Files   int    `json:"files"`
	Bytes   int64  `json:"bytes"`
	Digest  string `json:"digest"`
}

// SetupSkill is one whole skill folder in the encrypted payload.
type SetupSkill struct {
	Name           string           `json:"name"`
	AgentID        string           `json:"agent_id"`
	Scope          string           `json:"scope"`
	Root           string           `json:"root"`
	LocationKind   string           `json:"location_kind,omitempty"`
	LocationSource string           `json:"location_source,omitempty"`
	Files          []SetupSkillFile `json:"files"`
	Digest         string           `json:"digest"`
}

type SetupSkillFile struct {
	Path string `json:"path"`
	Mode uint32 `json:"mode,omitempty"`
	Data []byte `json:"data,omitempty"`
}

func isSkillLocation(kind string) bool { return kind == "skills" || kind == "shared_skills" }

func selectSkillContent(agents []Agent, preview *SnapshotPreview, selection *SnapshotSelection) {
	seenRoots := map[string]bool{}
	totalFiles, totalBytes := 0, int64(0)
	for i := range agents {
		for _, loc := range agents[i].Locations {
			if !isSkillLocation(loc.Kind) || loc.Path == "" {
				continue
			}
			if loc.State != "present" {
				if loc.State == "symlink" || loc.State == "error" || loc.State == "unsupported" {
					preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, SnapshotOmission{Kind: "skills", Source: loc.Path, Reason: "skill_root_" + loc.State})
				}
				continue
			}
			root := filepath.Clean(loc.Path)
			if seenRoots[root] {
				continue
			}
			seenRoots[root] = true
			skills, omissions := collectSetupSkills(root, agents[i].ID, loc.Scope, loc.Kind, loc.Source)
			preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, omissions...)
			for _, skill := range skills {
				if totalFiles+len(skill.Files) > maxSetupSkillFiles || totalBytes+skillBytes(skill.Files) > maxSetupSkillBytes {
					preview.Agents[i].Omitted = append(preview.Agents[i].Omitted, SnapshotOmission{Kind: "skills", Source: filepath.Join(root, skill.Name), Reason: "setup_skill_limit_reached", Count: 1})
					continue
				}
				totalFiles += len(skill.Files)
				totalBytes += skillBytes(skill.Files)
				preview.Skills = append(preview.Skills, snapshotSkill(skill, root))
				selection.skills = append(selection.skills, skill)
			}
		}
	}
	sort.Slice(preview.Skills, func(i, j int) bool {
		if preview.Skills[i].AgentID != preview.Skills[j].AgentID {
			return preview.Skills[i].AgentID < preview.Skills[j].AgentID
		}
		if preview.Skills[i].Scope != preview.Skills[j].Scope {
			return preview.Skills[i].Scope < preview.Skills[j].Scope
		}
		return preview.Skills[i].Name < preview.Skills[j].Name
	})
}

func collectSetupSkills(root, agentID, scope, locationKind, locationSource string) ([]SetupSkill, []SnapshotOmission) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, []SnapshotOmission{{Kind: "skills", Source: root, Reason: "skill_root_unreadable"}}
	}
	var skills []SetupSkill
	var omissions []SnapshotOmission
	for _, entry := range entries {
		name := entry.Name()
		source := filepath.Join(root, name)
		if !validSkillName(name) {
			omissions = append(omissions, SnapshotOmission{Kind: "skills", Source: source, Reason: "invalid_skill_name"})
			continue
		}
		info, err := os.Lstat(source)
		if err != nil {
			omissions = append(omissions, SnapshotOmission{Kind: "skills", Source: source, Reason: "skill_metadata_unavailable"})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			omissions = append(omissions, SnapshotOmission{Kind: "skills", Source: source, Reason: "skill_symlink"})
			continue
		}
		if !info.IsDir() {
			continue
		}
		manifest := filepath.Join(source, "SKILL.md")
		manifestInfo, err := os.Lstat(manifest)
		if err != nil || manifestInfo.Mode()&os.ModeSymlink != 0 || !manifestInfo.Mode().IsRegular() {
			omissions = append(omissions, SnapshotOmission{Kind: "skills", Source: source, Reason: "skill_manifest_missing"})
			continue
		}
		files, reason := collectSkillFiles(source)
		if reason != "" {
			omissions = append(omissions, SnapshotOmission{Kind: "skills", Source: source, Reason: reason})
			continue
		}
		skill := SetupSkill{Name: name, AgentID: agentID, Scope: scope, Root: root, LocationKind: locationKind, LocationSource: locationSource, Files: files}
		skill.Digest = setupSkillDigest(files)
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, omissions
}

func collectSkillFiles(dir string) ([]SetupSkillFile, string) {
	var files []SetupSkillFile
	var total int64
	var walk func(string) string
	walk = func(current string) string {
		entries, err := os.ReadDir(current)
		if err != nil {
			return "skill_directory_unreadable"
		}
		for _, entry := range entries {
			path := filepath.Join(current, entry.Name())
			info, err := os.Lstat(path)
			if err != nil {
				return "skill_file_metadata_unavailable"
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "skill_symlink_file"
			}
			if info.IsDir() {
				if reason := walk(path); reason != "" {
					return reason
				}
				continue
			}
			if !info.Mode().IsRegular() {
				return "skill_special_file"
			}
			rel, err := filepath.Rel(dir, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return "skill_path_escape"
			}
			if len(files) >= maxSkillFiles {
				return "skill_too_many_files"
			}
			if info.Size() > maxSkillBytes || total+info.Size() > maxSkillBytes {
				return "skill_too_large"
			}
			data, reason := readSkillFile(path, info)
			if reason != "" {
				return reason
			}
			total += int64(len(data))
			mode := uint32(0o644)
			if info.Mode().Perm()&0o111 != 0 {
				mode = 0o755
			}
			files = append(files, SetupSkillFile{Path: filepath.ToSlash(rel), Mode: mode, Data: data})
		}
		return ""
	}
	if reason := walk(dir); reason != "" {
		return nil, reason
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(files) == 0 {
		return nil, "skill_empty"
	}
	return files, ""
}

func readSkillFile(path string, before os.FileInfo) ([]byte, string) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "skill_file_unreadable"
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return nil, "skill_file_changed"
	}
	data, err := io.ReadAll(io.LimitReader(f, maxSkillBytes+1))
	if err != nil {
		return nil, "skill_file_unreadable"
	}
	if len(data) > maxSkillBytes {
		return nil, "skill_too_large"
	}
	after, err := f.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != after.Size() {
		return nil, "skill_file_changed"
	}
	return data, ""
}

func validSkillName(name string) bool {
	return name != "" && name != "." && name != ".." && len(name) <= 128 &&
		filepath.Base(name) == name && !strings.ContainsAny(name, `/\`) && !strings.ContainsRune(name, 0)
}

func snapshotSkill(skill SetupSkill, root string) SnapshotSkill {
	return SnapshotSkill{
		Name: skill.Name, AgentID: skill.AgentID, Scope: skill.Scope,
		Source: filepath.Join(root, skill.Name), Files: len(skill.Files),
		Bytes: skillBytes(skill.Files), Digest: skill.Digest,
	}
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
	sum := sha256.Sum256(manifest)
	return hex.EncodeToString(sum[:])
}
