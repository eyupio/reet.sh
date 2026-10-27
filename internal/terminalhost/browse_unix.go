//go:build linux || darwin

package terminalhost

import (
	"os"
	"strconv"
	"syscall"
)

// dirOpenFlags makes a browse's open fail on anything but a directory, so a
// file or FIFO swapped in is never opened. (Through an os.Root a final link
// inside the root is still followed; one leading out is refused by the root.)
const dirOpenFlags = syscall.O_DIRECTORY | syscall.O_NOFOLLOW

// nonBlockFlag makes an open return at once on a FIFO or device, which a
// plain open for reading would wait on for ever; the caller then refuses
// anything that is not a regular file. regularOpenFlags adds O_NOFOLLOW.
const (
	nonBlockFlag     = syscall.O_NONBLOCK
	regularOpenFlags = syscall.O_NONBLOCK | syscall.O_NOFOLLOW
)

// openRootNoBlock opens dir as an os.Root without ever waiting on it.
// os.OpenRoot opens its path with neither O_DIRECTORY nor O_NONBLOCK, so a
// FIFO swapped in for dir would hold it (and its goroutine) for ever. Here
// dir is opened first as a directory only, without blocking, and the root
// is opened through that descriptor (/dev/fd/N reopens the same folder, not
// the path), so a swap after the check cannot reach it either. Where
// /dev/fd cannot be opened (no /proc in a sandbox), dir is opened by path
// after the check, which leaves only the moment between the two opens.
func openRootNoBlock(dir string) (*os.Root, error) {
	fd, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer syscall.Close(fd)
	if root, err := os.OpenRoot("/dev/fd/" + strconv.Itoa(fd)); err == nil {
		return root, nil
	}
	return os.OpenRoot(dir)
}
