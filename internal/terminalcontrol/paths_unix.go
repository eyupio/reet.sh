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
	if unix.Lstat(path, &owned) != nil {
		return errors.New("terminal control paths must be owned by this user, private, and not symlinks")
	}
	mode := info.Mode()
	validOwner := owned.Uid == uint32(os.Geteuid()) && mode&os.ModeSymlink == 0
	validKind := info.IsDir() == dir && (dir || mode.IsRegular())
	validPerms := mode.Perm()&0022 == 0 && (!private || mode.Perm()&0077 == 0)
	valid := validOwner && validKind && validPerms
	if !valid {
		return errors.New("terminal control paths must be owned by this user, private, and not symlinks")
	}
	return nil
}
func checkSocket(path string) error {
	var stat unix.Stat_t
	if err := unix.Lstat(path, &stat); err != nil {
		return err
	}
	valid := stat.Uid == uint32(os.Geteuid()) && stat.Mode&unix.S_IFMT == unix.S_IFSOCK &&
		stat.Mode&0077 == 0
	if !valid {
		return errors.New("unsafe terminal control socket")
	}
	return nil
}
