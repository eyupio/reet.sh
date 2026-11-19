//go:build darwin

package scan

import "golang.org/x/sys/unix"

func renameExclSystem(src, dest string) error {
	return unix.RenamexNp(src, dest, unix.RENAME_EXCL)
}
