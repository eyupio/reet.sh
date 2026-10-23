//go:build !linux && !darwin

package terminalhost

import (
	"fmt"
	"os"
)

func ownedByCurrentUser(os.FileInfo) error {
	return fmt.Errorf("%w: terminal services support Linux and macOS only", ErrUnsafePath)
}
