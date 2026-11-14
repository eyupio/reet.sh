//go:build linux || darwin

package terminalhost

import (
	"os"
	"syscall"
)

// deviceOf is the filesystem (device) a file or folder is on; ok is false
// when it cannot be told.
func deviceOf(info os.FileInfo) (dev uint64, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	// Dev is an int32 on macOS and a uint64 on Linux.
	return uint64(stat.Dev), true
}
