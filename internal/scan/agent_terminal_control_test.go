//go:build linux || darwin

package scan

import (
	"os/exec"
	"testing"
)

func TestGitAvailableFollowsPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	if !gitAvailable() {
		t.Fatal("git is on PATH")
	}
	t.Setenv("PATH", t.TempDir())
	if gitAvailable() {
		t.Fatal("git is not on PATH")
	}
}
