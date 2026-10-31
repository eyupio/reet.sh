//go:build linux

package acpagent

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// groupAlive reports whether any live (non-zombie) process remains in the
// process group. A zombie has exited and holds no resources a replacement could
// collide with, so it does not count; kill(-pgid, 0) would count it.
func groupAlive(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return syscall.Kill(-pgid, 0) == nil
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + entry.Name() + "/stat")
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp ...; comm may contain spaces and parens.
		text := string(data)
		end := strings.LastIndexByte(text, ')')
		if end < 0 {
			continue
		}
		fields := strings.Fields(text[end+1:])
		if len(fields) < 3 || fields[0] == "Z" || fields[0] == "X" {
			continue
		}
		if group, err := strconv.Atoi(fields[2]); err == nil && group == pgid {
			return true
		}
	}
	return false
}
