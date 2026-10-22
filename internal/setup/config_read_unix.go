//go:build unix

package setup

import "syscall"

// Do not block if a regular configuration file is replaced with a FIFO between
// metadata lookup and open. The opened descriptor is checked before any read.
const configReadFlags = syscall.O_NONBLOCK
