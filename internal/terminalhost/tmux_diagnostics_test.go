//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTmuxDiagnosticsExplainMissingBrokenAndOldExecutable(t *testing.T) {
	for _, tc := range []struct{ name, program, want string }{{"missing", "", "not found on PATH"}, {"broken", "#!/bin/sh\nexit 1\n", "could not run tmux -V"}, {"old", "#!/bin/sh\nprintf 'tmux 3.2a\\n'\n", "3.3 or newer"}, {"invalid", "#!/bin/sh\nprintf '\\033[2Jnot a version\\n'\n", "3.3 or newer"}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("PATH", dir)
			if tc.program != "" {
				if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(tc.program), 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := FindTmux(context.Background())
			if !errors.Is(err, ErrTmuxUnavailable) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "reet agent terminal setup") {
				t.Fatal(err)
			}
			if strings.Contains(err.Error(), "\x1b") {
				t.Fatal("untrusted version output reached the diagnostic")
			}
		})
	}
}
