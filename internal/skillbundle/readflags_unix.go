//go:build linux || darwin

package skillbundle

import "syscall"

// No-follow rejects a replacement symlink; nonblocking prevents a replacement
// FIFO from hanging before the descriptor's regular-file identity is checked.
const readFlags = syscall.O_NOFOLLOW | syscall.O_NONBLOCK
const supportedModes = true
