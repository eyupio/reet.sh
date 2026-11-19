//go:build !linux && !darwin

package scan

import "syscall"

// renameExclSystem: no rename here refuses to replace, so renameNoReplace
// checks first.
func renameExclSystem(src, dest string) error {
	return syscall.ENOSYS
}
