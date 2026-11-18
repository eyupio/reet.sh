//go:build unix

package agentaccounts

import "syscall"

// openNoWait opens a named pipe without waiting for a writer. A plain file
// is read as usual.
const openNoWait = syscall.O_NONBLOCK
