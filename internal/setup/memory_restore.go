package setup

import (
	"bytes"
	"os"
	"path/filepath"
)

func addMemoryRestoreActions(plan *RestorePlan, snapshot SetupSnapshot, destination Report, actions map[string]*RestoreAction, files map[string]restoreFile) {
	if destination.Home == "" {
		return
	}
	for _, memory := range snapshot.Memories {
		if memory.AgentID != "codex" || !validMemoryName(memory.Name) {
			plan.Unsupported = append(plan.Unsupported, RestoreUnsupported{Kind: "memory", Source: memory.Source, Reason: "memory_destination_unsupported"})
			continue
		}
		dest := filepath.Join(destination.Home, ".codex", memory.Name)
		state, reason := memoryRestoreState(dest)
		key := memory.AgentID + "\x00" + dest
		action := &RestoreAction{
			AgentID: memory.AgentID, Components: []string{"memory"}, Sources: []string{memory.Source},
			Destination: dest, Scope: memory.Scope, Format: "sqlite", State: state, Reason: reason,
		}
		actions[key] = action
		files[dest] = restoreFile{agentID: memory.AgentID, format: "sqlite", data: memory.Data, mode: 0o600}
	}
}

func memoryRestoreState(dest string) (string, string) {
	for _, suffix := range []string{"-wal", "-shm"} {
		if info, err := os.Lstat(dest + suffix); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return "blocked", "destination_sqlite_sidecar_unsafe"
			}
			return "blocked", "destination_sqlite_sidecars_present"
		} else if !os.IsNotExist(err) {
			return "blocked", "destination_metadata_unavailable"
		}
	}
	info, err := os.Lstat(dest)
	switch {
	case os.IsNotExist(err):
		return "create", ""
	case err != nil:
		return "blocked", "destination_metadata_unavailable"
	case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
		return "blocked", "destination_sqlite_unsupported"
	default:
		return "conflict", "destination_source_exists"
	}
}

func verifySetupMemories(snapshot SetupSnapshot, destination Report, result *SetupVerifyResult) {
	if destination.Home == "" {
		return
	}
	for _, memory := range snapshot.Memories {
		entry := VerifyComponent{
			AgentID: memory.AgentID, Kind: "memory", Source: memory.Source,
			Scope: memory.Scope, SelectedFields: 1,
		}
		if memory.AgentID != "codex" || !validMemoryName(memory.Name) {
			entry.State, entry.Reason = "unsupported", "memory_destination_unsupported"
			addVerifyComponent(result, entry)
			continue
		}
		dest := filepath.Join(destination.Home, ".codex", memory.Name)
		entry.Destination = dest
		for _, suffix := range []string{"-wal", "-shm"} {
			if _, err := os.Lstat(dest + suffix); err == nil {
				entry.State, entry.Reason = "blocked", "destination_sqlite_sidecars_present"
				addVerifyComponent(result, entry)
				return
			}
		}
		info, err := os.Lstat(dest)
		if os.IsNotExist(err) {
			entry.State = "missing"
			addVerifyComponent(result, entry)
			continue
		}
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			entry.State, entry.Reason = "blocked", "destination_sqlite_unsupported"
			addVerifyComponent(result, entry)
			continue
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			entry.State, entry.Reason = "blocked", "destination_metadata_unavailable"
		} else if !bytes.Equal(data, memory.Data) {
			entry.State, entry.Reason = "changed", "memory_content_differs"
		} else {
			entry.State = "verified"
		}
		addVerifyComponent(result, entry)
	}
}
