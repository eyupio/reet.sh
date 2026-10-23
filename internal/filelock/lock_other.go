//go:build !linux && !darwin

package filelock

import (
	"errors"
	"os"
)

var ErrLocked = errors.New("another process holds the private state lock")
var ErrUnsafe = errors.New("private state locking requires Linux or macOS")

func Acquire(string) (*os.File, error) { return nil, ErrUnsafe }
