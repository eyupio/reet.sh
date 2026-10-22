package clientconfig

import (
	"bytes"

	"os"
	"path/filepath"
	"runtime"
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("APPDATA", t.TempDir())
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

func TestSavedTokenRoundTripAndForget(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("REET_TOKEN", "")
	path, err := SaveToken("saved-token")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("saved token permissions: %o", info.Mode().Perm())
	}
	var out bytes.Buffer
	got, err := ResolveToken("", "", strings.NewReader(""), &out)
	if err != nil || got != "saved-token" {
		t.Fatalf("saved token not used: %q %v", got, err)
	}
	t.Setenv("REET_TOKEN", "env-token")
	if got, err := ResolveToken("", "", strings.NewReader(""), &out); err != nil || got != "env-token" {
		t.Fatalf("environment did not override saved token: %q %v", got, err)
	}
	t.Setenv("REET_TOKEN", "")
	if _, err := ForgetToken(); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveToken("", "", strings.NewReader(""), &out); err != nil || got != "" {
		t.Fatalf("forgotten token still used: %q %v", got, err)
	}
}

func TestSavedTokenRejectsOtherUserReadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows does not expose unix permissions")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("REET_TOKEN", "")
	path, err := SaveToken("saved-token")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveToken("", "", strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("accepted a saved token readable by other users")
	}
}
