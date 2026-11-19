package scan

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// renameNoReplace renames src to dest and never replaces what is at dest: if
// anything is there, even in the instant before the rename, it fails with an
// error that is fs.ErrExist and both paths are left as they were. Where the
// system has a rename that refuses to replace (Linux renameat2 with
// RENAME_NOREPLACE, macOS renamex_np with RENAME_EXCL) the refusal is made by
// the system. Where it has none, or the kernel or file system does not take
// the flag, a check that dest is free stands in for it, and a process that
// puts something at dest between the check and the rename can still be
// replaced.
func renameNoReplace(src, dest string) error {
	var err error
	for {
		if err = renameExcl(src, dest); !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	switch {
	case err == nil:
		return nil
	case errors.Is(err, syscall.EEXIST):
		return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EEXIST}
	case errors.Is(err, syscall.ENOSYS), errors.Is(err, syscall.EINVAL),
		errors.Is(err, syscall.ENOTSUP), errors.Is(err, syscall.EOPNOTSUPP):
		// The system cannot refuse for us.
		if _, statErr := os.Lstat(dest); statErr == nil {
			return &os.LinkError{Op: "rename", Old: src, New: dest, Err: syscall.EEXIST}
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		return os.Rename(src, dest)
	}
	return &os.LinkError{Op: "rename", Old: src, New: dest, Err: err}
}

// renameExcl is the system's rename that refuses to replace; tests replace
// it to stand for a system that has none.
var renameExcl = renameExclSystem

// copyRename is how copyEntry moves its finished copy into place; tests
// replace it to stand for another process at work meanwhile.
var copyRename = renameNoReplace
