//go:build linux

package terminalhost

import (
	"errors"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames from to to, failing when to is there, even as an
// empty folder that a plain rename would replace.
func renameNoReplace(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		// A kernel or file system without RENAME_NOREPLACE.
		return renameIfAbsent(from, to)
	}
	return err
}
