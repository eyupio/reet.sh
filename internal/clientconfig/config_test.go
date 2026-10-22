package clientconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolated(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("REET_VERSION", "")
	t.Setenv("REET_URL", "")
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func TestDefaultsRoundTripPreserveOtherSettingsAndRespectEnvironment(t *testing.T) {
	path := isolated(t)
	if err := Apply(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read created a file")
	}
	if _, err := Save(map[string]string{"REET_VERSION": "dev"}); err != nil {
		t.Fatal(err)
	}
	// Shell metacharacters in a legitimate URL path must remain inert data.
	url := "https://example.test/a'$(echo-secret)&b"
	if _, err := Save(map[string]string{"REET_URL": url}); err != nil {
		t.Fatal(err)
	}
	values, err := read(path)
	if err != nil || values["REET_VERSION"] != "dev" || values["REET_URL"] != url {
		t.Fatalf("round trip: %#v %v", values, err)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions: %v", info.Mode())
		}
	}
	t.Setenv("REET_VERSION", "latest")
	if err := Apply(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REET_VERSION") != "latest" || os.Getenv("REET_URL") != url {
		t.Fatal("environment precedence failed")
	}
}
func TestInvalidDefaultsNeverReplaceExistingFile(t *testing.T) {
	path := isolated(t)
	if _, err := Save(map[string]string{"REET_VERSION": "dev"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	for _, values := range []map[string]string{
		{"REET_TOKEN": "SECRET"}, {"REET_URL": "https://user:SECRET@example.test"}, {"REET_URL": "https://example.test/?token=SECRET"}, {"REET_URL": "file:///tmp/x"}, {"REET_VERSION": "dev\nREET_TOKEN=SECRET"},
	} {
		if _, err := Save(values); err == nil {
			t.Fatalf("accepted %#v", values)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("invalid input changed defaults")
		}
	}
	if err := os.WriteFile(path+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(map[string]string{"REET_VERSION": "latest"}); err == nil {
		t.Fatal("ignored concurrent writer lock")
	}
}
func TestMalformedAndSymlinkDefaultsAreRejected(t *testing.T) {
	path := isolated(t)
	os.MkdirAll(filepath.Dir(path), 0700)
	for _, data := range []string{
		"REET_URL=$(touch /tmp/should-not-run)\n", "REET_VERSION='dev'\nREET_VERSION='latest'\n", "REET_TOKEN='secret'\n", strings.Repeat("x", maxBytes+1),
	} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := read(path); err == nil {
			t.Fatal("accepted malformed file")
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	os.Remove(path)
	target := filepath.Join(t.TempDir(), "target")
	os.WriteFile(target, []byte("REET_VERSION='dev'\n"), 0600)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := read(path); err == nil {
		t.Fatal("followed config symlink")
	}
	if _, err := Save(map[string]string{"REET_VERSION": "latest"}); err == nil {
		t.Fatal("replaced config symlink")
	}
}
func TestPromptsAreBoundedAndDoNotConsumeLaterConfirmation(t *testing.T) {
	in := strings.NewReader("dev\nhttps://example.test\nyes\n")
	var out bytes.Buffer
	if v, err := Prompt(in, &out, "REET_VERSION", ""); err != nil || v != "dev" {
		t.Fatalf("version %q %v", v, err)
	}
	if v, err := Prompt(in, &out, "REET_URL", ""); err != nil || v != "https://example.test" {
		t.Fatalf("URL %q %v", v, err)
	}
	rest := make([]byte, 4)
	in.Read(rest)
	if string(rest) != "yes\n" {
		t.Fatalf("consumed confirmation: %q", rest)
	}
	for _, input := range []string{"", strings.Repeat("x", 4097) + "\n", "https://user:SECRET@example.test\n"} {
		if _, err := Prompt(strings.NewReader(input), &out, "REET_URL", ""); err == nil {
			t.Fatal("accepted invalid input")
		}
	}
	if _, err := ResolveURL("", strings.NewReader("https://example.test\n"), &out); err == nil {
		t.Fatal("prompted from a pipe")
	}
	out.Reset()
	if _, err := Prompt(strings.NewReader("\n"), &out, "REET_URL", "https://user:SECRET@example.test"); err == nil || strings.Contains(out.String(), "SECRET") {
		t.Fatal("echoed invalid default")
	}
}
