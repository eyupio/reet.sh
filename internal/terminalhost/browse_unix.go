//go:build linux || darwin

package terminalhost

import "syscall"

// dirOpenFlags makes a browse's open fail on anything but a directory, and on
// a final symbolic link, so a file or FIFO swapped in is never opened.
const dirOpenFlags = syscall.O_DIRECTORY | syscall.O_NOFOLLOW
