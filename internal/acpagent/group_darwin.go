//go:build darwin

package acpagent

import "syscall"

// groupAlive reports whether the process group still has a member. Without
// /proc a zombie still counts, so a stuck reaper can make retirement report
// false; that is the safe direction.
func groupAlive(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }
