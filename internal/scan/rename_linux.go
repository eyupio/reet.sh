//go:build linux

package scan

import "golang.org/x/sys/unix"

func renameExclSystem(src, dest string) error {
	return unix.Renameat2(unix.AT_FDCWD, src, unix.AT_FDCWD, dest, unix.RENAME_NOREPLACE)
}
