//go:build unix

package main

import "syscall"

func reexecBinary(path string, args, env []string) error { return syscall.Exec(path, args, env) }
