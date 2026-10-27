//go:build linux || darwin

package terminalhost

import "syscall"

// dirOpenFlags makes a browse's open fail on anything but a directory, so a
// file or FIFO swapped in is never opened. (Through an os.Root a final link
// inside the root is still followed; one leading out is refused by the root.)
const dirOpenFlags = syscall.O_DIRECTORY | syscall.O_NOFOLLOW
