//go:build linux || darwin

package terminalcontrol

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func checkPath(path string, dir, private bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	var owned unix.Stat_t
	if unix.Lstat(path, &owned) != nil || owned.Uid != uint32(os.Geteuid()) || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != dir || !dir && !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || private && info.Mode().Perm()&0077 != 0 {
		return errors.New("terminal control paths must be owned by this user, private, and not symlinks")
	}
	return nil
}
func checkSocket(path string) error {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Geteuid()) || stat.Mode&unix.S_IFMT != unix.S_IFSOCK || stat.Mode&0077 != 0 {
		return errors.New("unsafe terminal control socket")
	}
	return nil
}
