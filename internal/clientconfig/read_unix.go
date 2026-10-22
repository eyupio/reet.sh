//go:build unix

package clientconfig

import "syscall"

const readFlags = syscall.O_NONBLOCK
