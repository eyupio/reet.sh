package setup

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

func verifySetupSkills(snapshot SetupSnapshot, destination Report, result *SetupVerifyResult) {
	candidates := destinationSkillCandidates(destination)
	for _, skill := range snapshot.Skills {
		source := filepath.Join(skill.Root, skill.Name)
		entry := VerifyComponent{
			AgentID: skill.AgentID, Kind: "skills", Source: source,
			Scope: skill.Scope, SelectedFields: len(skill.Files),
		}
		if skill.Scope == "project" && destination.Project == "" {
			entry.State, entry.Reason = "unsupported", "project_not_selected"
			addVerifyComponent(result, entry)
			continue
		}
		root, ok := matchSkillRoot(candidates, skill)
		if !ok {
			entry.State, entry.Reason = "unsupported", "destination_location_not_found"
			addVerifyComponent(result, entry)
			continue
		}
		entry.Destination = filepath.Join(root.root, skill.Name)
		switch root.state {
		case "missing", "present":
		default:
			entry.State, entry.Reason = "blocked", "destination_skill_root_"+root.state
			addVerifyComponent(result, entry)
			continue
		}
		missing, changed, blocked := false, false, false
		for _, file := range skill.Files {
			path := filepath.Join(root.root, skill.Name, filepath.FromSlash(file.Path))
			info, err := os.Lstat(path)
			switch {
			case os.IsNotExist(err):
				missing = true
				entry.Mismatches = append(entry.Mismatches, file.Path)
				continue
			case err != nil:
				blocked = true
				entry.Reason = "destination_metadata_unavailable"
				continue
			case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
				blocked = true
				entry.Reason = "destination_unsupported"
				continue
			}
			data, reason := readSkillFile(path, info)
			if reason != "" || !bytes.Equal(data, file.Data) {
				changed = true
				entry.Mismatches = append(entry.Mismatches, file.Path)
			}
		}
		switch {
		case blocked:
			entry.State = "blocked"
		case missing:
			entry.State = "missing"
		case changed:
			entry.State, entry.Reason = "changed", "skill_content_differs"
		default:
			entry.State = "verified"
		}
		addVerifyComponent(result, entry)
	}
}

func skillVerificationNote() string {
	return fmt.Sprintf("Skill folders are verified file-by-file up to %d files and %d MiB each; symlinked or special files are omitted.", maxSkillFiles, maxSkillBytes>>20)
}
