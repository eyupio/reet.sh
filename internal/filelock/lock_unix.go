//go:build linux || darwin

// Package filelock provides crash-released exclusive locks for private state.
package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

var ErrLocked = errors.New("another process holds the private state lock")
var ErrUnsafe = errors.New("private state lock must be a private regular file owned by this user")

// Acquire returns an open, close-on-exec descriptor. Closing it or process death
// releases the lock. Never unlink the persistent file: that creates two lock
// identities and permits overlapping owners. Legacy empty lock files are safe.
func Acquire(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	var stat unix.Stat_t
	statErr := unix.Fstat(fd, &stat)
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || statErr != nil || stat.Uid != uint32(os.Geteuid()) {
		f.Close()
		return nil, ErrUnsafe
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}
