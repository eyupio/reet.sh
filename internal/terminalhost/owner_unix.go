//go:build linux || darwin

package terminalhost

import (
	"fmt"
	"os"
	"syscall"
)

func ownedByCurrentUser(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: cannot verify file ownership", ErrUnsafePath)
	}
	if int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("%w: file is owned by another user", ErrUnsafePath)
	}
	return nil
}
