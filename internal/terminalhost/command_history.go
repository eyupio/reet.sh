//go:build linux || darwin

package terminalhost

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/eyupio/reet.sh/internal/terminalproto"
	"golang.org/x/sys/unix"
)

// CommandHistory reads known shell-owned files, never terminal input or output.
// The caller cannot choose a path, user, shell command or history configuration.
func (m *Manager) CommandHistory(ctx context.Context, sessionID string) ([]byte, error) {
	if !terminalproto.ValidID(sessionID) {
		return nil, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return nil, err
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	registry, err := m.loadRegistry()
	if err != nil {
		return nil, err
	}
	s, ok := registry.Sessions[sessionID]
	if !ok || s.State != terminalproto.Running {
		return nil, ErrSessionMissing
	}
	history := readShellHistory(ctx, m.home)
	return terminalproto.EncodeCommandHistory(history)
}

type rankedCommand struct {
	entry terminalproto.CommandHistoryEntry
	rank  int64
	order int
}

func readShellHistory(ctx context.Context, home string) terminalproto.CommandHistorySnapshot {
	result := terminalproto.CommandHistorySnapshot{Entries: []terminalproto.CommandHistoryEntry{}, Sources: []terminalproto.CommandHistorySource{}}
	var all []rankedCommand
	for _, source := range []struct{ name, file string }{{"bash", ".bash_history"}, {"zsh", ".zsh_history"}} {
		if ctx.Err() != nil {
			break
		}
		data, modified, truncated, reason := readHistoryTail(filepath.Join(home, source.file))
		status := terminalproto.CommandHistorySource{Source: source.name, Available: reason == "", Reason: reason}
		result.Sources = append(result.Sources, status)
		if reason != "" {
			continue
		}
		result.Truncated = result.Truncated || truncated
		entries, skipped := parseShellHistory(data, source.name)
		result.Skipped += skipped
		for i, entry := range entries {
			rank := entry.Timestamp
			if rank == 0 {
				rank = modified
			}
			all = append(all, rankedCommand{entry: entry, rank: rank, order: i})
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].rank != all[j].rank {
			return all[i].rank > all[j].rank
		}
		if all[i].entry.Source == all[j].entry.Source {
			return all[i].order > all[j].order
		}
		return all[i].entry.Source < all[j].entry.Source
	})
	budget := terminalproto.MaxCommandHistoryBytes - 4096
	for _, candidate := range all {
		encoded, _ := json.Marshal(candidate.entry)
		if len(result.Entries) >= terminalproto.MaxCommandHistoryEntries || len(encoded)+1 > budget {
			result.Truncated = true
			break
		}
		result.Entries = append(result.Entries, candidate.entry)
		budget -= len(encoded) + 1
	}
	return result
}

func readHistoryTail(path string) ([]byte, int64, bool, string) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, false, "No saved history file yet."
		}
		return nil, 0, false, "History file is unavailable or not a regular owned file."
	}
	file := os.NewFile(uintptr(fd), "shell-history")
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || ownedByCurrentUser(info) != nil {
		return nil, 0, false, "History file is not a regular file writable only by its OS user."
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return nil, 0, false, "Linked history files are not read."
	}
	offset := max(int64(0), info.Size()-terminalproto.MaxCommandHistoryBytes)
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, false, "History file could not be read."
	}
	data, err := io.ReadAll(io.LimitReader(file, terminalproto.MaxCommandHistoryBytes))
	if err != nil {
		return nil, 0, false, "History file could not be read."
	}
	if offset > 0 {
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		} else {
			data = nil
		}
	}
	return data, info.ModTime().UnixMilli(), offset > 0, ""
}

func historyTime(value string) int64 {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 || n > 253402300799 {
		return 0
	}
	return n * 1000
}

// Bash timestamp markers delimit multiline entries. Zsh extended records carry
// a timestamp/duration prefix and backslash-newline continuation. Plain files
// retain line entries; the shell's history settings control what it has saved.
func parseShellHistory(data []byte, source string) ([]terminalproto.CommandHistoryEntry, int) {
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	var entries []terminalproto.CommandHistoryEntry
	skipped := 0
	appendEntry := func(text string, timestamp int64) {
		if strings.TrimSpace(text) == "" {
			return
		}
		if !terminalproto.ValidCommandText(text) {
			skipped++
			return
		}
		entries = append(entries, terminalproto.CommandHistoryEntry{Command: text, Source: source, Timestamp: timestamp})
	}
	if source == "bash" {
		var pending []string
		var timestamp int64
		marked := false
		flush := func() {
			if len(pending) > 0 {
				appendEntry(strings.Join(pending, "\n"), timestamp)
				pending = nil
			}
		}
		for _, line := range lines {
			if strings.HasPrefix(line, "#") {
				if at := historyTime(strings.TrimPrefix(line, "#")); at > 0 {
					flush()
					timestamp = at
					marked = true
					continue
				}
			}
			if marked {
				pending = append(pending, line)
			} else {
				appendEntry(line, 0)
			}
		}
		flush()
		return entries, skipped
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		var timestamp int64
		if strings.HasPrefix(line, ": ") {
			if end := strings.IndexByte(line, ';'); end > 2 {
				parts := strings.SplitN(line[2:end], ":", 2)
				if len(parts) == 2 {
					if at := historyTime(parts[0]); at > 0 {
						if _, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
							timestamp = at
							line = line[end+1:]
						}
					}
				}
			}
		}
		for strings.HasSuffix(line, "\\") && i+1 < len(lines) {
			slashes := 0
			for j := len(line) - 1; j >= 0 && line[j] == '\\'; j-- {
				slashes++
			}
			if slashes%2 == 0 {
				break
			}
			i++
			line = strings.TrimSuffix(line, "\\") + "\n" + lines[i]
		}
		appendEntry(line, timestamp)
	}
	return entries, skipped
}
