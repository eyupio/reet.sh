//go:build !linux && !darwin

package terminalhost

import "os"

// deviceOf cannot tell a file's filesystem here: nothing is vouched for.
func deviceOf(os.FileInfo) (dev uint64, ok bool) { return 0, false }
