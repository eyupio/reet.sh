package clientconfig

import (
	"bytes"

	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokenEnvFile(t *testing.T) {
	for _, tc := range []struct {
		body, want string
		bad        bool
	}{
		{"REET_TOKEN=abc\n", "abc", false},
		{"export REET_TOKEN = 'abc' # comment\nREET_VERSION=dev\n", "abc", false},
		{"REET_TOKEN=\"abc\"\n", "abc", false},
		{"REET_TOKEN=abc # note\n", "abc", false},
		{"REET_TOKEN=$(echo SECRET)", "", true},
		{"REET_TOKEN='SECRET' junk", "", true},
		{"REET_TOKEN=one\nREET_TOKEN=SECRET", "", true},
		{"OTHER=SECRET", "", true},
		{"REET_TOKEN=", "", true},
		{"REET_TOKEN='SECRET", "", true},
		{"REET_TOKEN=" + strings.Repeat("x", 4097), "", true},
	} {
		p := filepath.Join(t.TempDir(), ".env")
		os.WriteFile(p, []byte(tc.body), 0600)
		got, err := tokenFromEnvFile(p)
		if (err != nil) != tc.bad || got != tc.want {
			t.Fatalf("unexpected parse result: error=%v", err)
		}
		if err != nil && strings.Contains(err.Error(), "SECRET") {
			t.Fatal("secret leaked")
		}
	}
}
func TestScanTokenPrecedenceAndNoninteractive(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(p, []byte("REET_TOKEN=file-token\n"), 0600)
	in, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	for _, tc := range []struct{ flag, env, path, want string }{
		{"flag", "env", "missing", "flag"}, {"", "env", "missing", "env"}, {"", "", p, "file-token"}, {"", "", "", ""},
	} {
		t.Setenv("REET_TOKEN", tc.env)
		var out bytes.Buffer
		got, err := ResolveToken(tc.flag, tc.path, in, &out)
		if err != nil || got != tc.want || out.Len() != 0 {
			t.Fatalf("resolution failed: %v", err)
		}
	}
}
