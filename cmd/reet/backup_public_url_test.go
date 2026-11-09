package main

import (
	"bytes"
	"strings"
	"testing"
)

// A bad address is refused before the copy is read or any server is asked.
func TestRestoreAndImportRefuseABadPublicURLFirst(t *testing.T) {
	t.Setenv("REET_DATA_DIR", t.TempDir())
	for _, bad := range []string{"https://x.example/path", "ftp://x.example", "https://u:p@x.example"} {
		var out bytes.Buffer
		if err := backupRestore([]string{"--public-url", bad, "--yes", "instance-nope"}, &out); err == nil || !strings.Contains(err.Error(), "plain http or https origin") {
			t.Fatalf("restore %q: %v", bad, err)
		}
		if err := backupImport([]string{"--public-url", bad, "--server", "http://127.0.0.1:1", "--token-file", "-", "nofile"}, &out); err == nil || !strings.Contains(err.Error(), "plain http or https origin") {
			t.Fatalf("import %q: %v", bad, err)
		}
	}
}
