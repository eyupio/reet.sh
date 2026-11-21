package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Run init-env.sh in a copy of the layout it expects, so the repository's own
// .env is never touched.
func runInitEnv(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{filepath.Join(root, "scripts", "init-env.sh")}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("init-env.sh %v: %v\n%s", args, err, out)
	}
}

func initEnvFixture(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shell script targets Linux and macOS")
	}
	for _, dependency := range []string{"sh", "awk", "sed"} {
		if _, err := exec.LookPath(dependency); err != nil {
			t.Skipf("%s unavailable", dependency)
		}
	}
	script, err := os.ReadFile("init-env.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err = os.Mkdir(filepath.Join(root, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "scripts", "init-env.sh"), script, 0755); err != nil {
		t.Fatal(err)
	}
	example := "REET_DATA_PATH=./data\nREET_UID=1000\nREET_GID=1000\nREET_TOKEN=\n"
	if err = os.WriteFile(filepath.Join(root, ".env.example"), []byte(example), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func envValue(t *testing.T, root, key string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimPrefix(line, key+"=")
		}
	}
	t.Fatalf("%s not in .env:\n%s", key, b)
	return ""
}

func TestInitEnvKeepsTheEnvFilePrivate(t *testing.T) {
	root := initEnvFixture(t)
	check := func(when string) {
		t.Helper()
		info, err := os.Stat(filepath.Join(root, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != 0600 {
			t.Fatalf("%s: .env mode %o, want 600", when, mode)
		}
	}
	runInitEnv(t, root)
	check("after creation")
	if envValue(t, root, "REET_TOKEN") == "" {
		t.Fatal("no token generated")
	}
	// A file left open by an earlier version is tightened on the next run.
	if err := os.Chmod(filepath.Join(root, ".env"), 0644); err != nil {
		t.Fatal(err)
	}
	runInitEnv(t, root, "--set", "REET_PORT=9090")
	check("after a rewrite")
}

func TestInitEnvWritesBackslashesAsGiven(t *testing.T) {
	root := initEnvFixture(t)
	const value = `a\tb\\c\n&d`
	// The first run appends the key and the second replaces it; both must
	// write the same bytes.
	runInitEnv(t, root, "--set", "EXTRA="+value)
	if got := envValue(t, root, "EXTRA"); got != value {
		t.Fatalf("appended %q, want %q", got, value)
	}
	runInitEnv(t, root, "--set", "EXTRA="+value)
	if got := envValue(t, root, "EXTRA"); got != value {
		t.Fatalf("replaced %q, want %q", got, value)
	}
}
