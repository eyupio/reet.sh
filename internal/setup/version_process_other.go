//go:build !unix

package setup

import "os/exec"

// CommandContext retains its default direct-process cancellation elsewhere.
func configureVersionProcess(cmd *exec.Cmd) {}
