//go:build linux || darwin

package terminalhost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// fakeModelBinary writes an executable shell script and returns a lookPath
// that always resolves the opencode binary name to it.
func fakeModelBinary(t *testing.T, script string) string {
	t.Helper()
	dir := shortTempDir(t)
	path := filepath.Join(dir, "opencode")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func modelLookPath(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func listModelsForTest(t *testing.T, script string) terminalproto.ModelsResult {
	t.Helper()
	path := fakeModelBinary(t, script)
	home := shortTempDir(t)
	return ListModels(t.Context(), "opencode", modelLookPath(path), home)
}

func TestListModelsNormalAndProvider(t *testing.T) {
	r := listModelsForTest(t, "printf '%s\\n' 'anthropic/claude-sonnet-4-5' 'openai/gpt-5' 'noslash'\n")
	if r.Reason != "" || r.Tool != "opencode" || r.More {
		t.Fatalf("result: %+v", r)
	}
	want := []terminalproto.ModelEntry{
		{ID: "anthropic/claude-sonnet-4-5", Provider: "anthropic"},
		{ID: "openai/gpt-5", Provider: "openai"},
		{ID: "noslash", Provider: ""},
	}
	if len(r.Models) != len(want) {
		t.Fatalf("models: %+v", r.Models)
	}
	for i := range want {
		if r.Models[i] != want[i] {
			t.Fatalf("model %d: %+v, want %+v", i, r.Models[i], want[i])
		}
	}
}

func TestListModelsEmptyOutputIsAnEmptyList(t *testing.T) {
	r := listModelsForTest(t, ": # no models\n")
	if r.Reason != "" || len(r.Models) != 0 || r.More {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsSkipsInvalidLines(t *testing.T) {
	r := listModelsForTest(t, "printf '%s\\n' 'anthropic/good' '-bad' 'foo bar' 'openai/gpt-5'\n")
	if r.Reason != "" || len(r.Models) != 2 || r.Models[0].ID != "anthropic/good" || r.Models[1].ID != "openai/gpt-5" {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsRejectsEscAndControlOutput(t *testing.T) {
	for name, script := range map[string]string{
		"esc":     "printf 'good\\n\\033[31mbad\\n'\n",
		"control": "printf 'good\\n\\001bad\\n'\n",
	} {
		t.Run(name, func(t *testing.T) {
			if r := listModelsForTest(t, script); r.Reason != terminalproto.ModelsUnreadable {
				t.Fatalf("result: %+v", r)
			}
		})
	}
}

func TestListModelsTooLarge(t *testing.T) {
	r := listModelsForTest(t, "yes aaaa\n")
	if r.Reason != terminalproto.ModelsTooLarge {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsTimeoutKillsTheProcessGroup(t *testing.T) {
	original := modelListTimeout
	modelListTimeout = 200 * time.Millisecond
	t.Cleanup(func() { modelListTimeout = original })
	dir := shortTempDir(t)
	pidFile := filepath.Join(dir, "child.pid")
	script := "sleep 30 &\necho $! > '" + pidFile + "'\nprintf 'a/b\\n'\nwait\n"
	path := fakeModelBinary(t, script)
	start := time.Now()
	r := ListModels(t.Context(), "opencode", modelLookPath(path), dir)
	if r.Reason != terminalproto.ModelsTimedOut {
		t.Fatalf("result: %+v", r)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %s", elapsed)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("child %d survived the group kill", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestListModelsNonZeroExit(t *testing.T) {
	r := listModelsForTest(t, "printf 'a/b\\n'\nexit 1\n")
	if r.Reason != terminalproto.ModelsExited {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsNeverRunsUnsupportedOrDisabledTools(t *testing.T) {
	dir := shortTempDir(t)
	marker := filepath.Join(dir, "ran")
	path := fakeModelBinary(t, "touch '"+marker+"'\nprintf 'a/b\\n'\n")
	home := shortTempDir(t)
	for _, tool := range []string{"claude", "pi"} {
		if r := ListModels(t.Context(), tool, modelLookPath(path), home); r.Reason != terminalproto.ModelsUnsupported {
			t.Fatalf("%s: result: %+v", tool, r)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("%s ran the binary", tool)
		}
	}
}

func TestListModelsEnvironmentIsMinimal(t *testing.T) {
	dir := shortTempDir(t)
	envFile := filepath.Join(dir, "env.txt")
	path := fakeModelBinary(t, "env > '"+envFile+"'\nprintf 'a/b\\n'\n")
	home := shortTempDir(t)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("OPENCODE_PERMISSION", `{"*":"allow"}`)
	r := ListModels(t.Context(), "opencode", modelLookPath(path), home)
	if r.Reason != "" {
		t.Fatalf("result: %+v", r)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{}
	count := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[key] = value
		count[key]++
	}
	for _, key := range []string{"TERM", "SHELL", "OPENCODE_PERMISSION"} {
		if _, ok := env[key]; ok {
			t.Fatalf("%s leaked into the listing environment", key)
		}
	}
	for key, want := range map[string]string{
		"HOME": home, "OPENCODE_DISABLE_MODELS_FETCH": "1", "OPENCODE_DISABLE_AUTOUPDATE": "1", "NO_COLOR": "1", "LANG": "C.UTF-8", "CI": "1",
	} {
		if env[key] != want {
			t.Fatalf("%s=%q, want %q", key, env[key], want)
		}
	}
	for key, n := range count {
		if n != 1 {
			t.Fatalf("%s appears %d times", key, n)
		}
	}
}

func TestListModelsNeverReturnsStderr(t *testing.T) {
	r := listModelsForTest(t, "printf 'secret /root/path\\n' >&2\nprintf 'a/b\\n'\n")
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "/root/path") {
		t.Fatalf("stderr leaked: %s", encoded)
	}
	if r.Reason != "" || len(r.Models) != 1 {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsSkipsAProviderPastTheProtocolBound(t *testing.T) {
	r := listModelsForTest(t, "printf '%s/model\\n' '"+strings.Repeat("a", 65)+"'\n")
	if r.Reason != "" || len(r.Models) != 0 {
		t.Fatalf("result: %+v", r)
	}
}

func TestListModelsNotInstalledAndDuplicateLines(t *testing.T) {
	missing := func(string) (string, error) { return "", os.ErrNotExist }
	if r := ListModels(t.Context(), "opencode", missing, shortTempDir(t)); r.Reason != terminalproto.ModelsNotInstalled {
		t.Fatalf("not installed: %+v", r)
	}
	r := listModelsForTest(t, "printf 'a/b\\na/b\\na/c\\n'\n")
	if r.Reason != "" || len(r.Models) != 2 || r.Models[0].ID != "a/b" || r.Models[1].ID != "a/c" {
		t.Fatalf("duplicates: %+v", r)
	}
}

func TestListModelsCapsAtFiveHundred(t *testing.T) {
	// 501 entries are read; the list is capped at MaxModels before the encoded
	// answer is trimmed to MaxModelsReplyBytes (which is smaller than 500
	// minimum entries), so More is set either way.
	r := listModelsForTest(t, "i=0\nwhile [ $i -lt 501 ]; do printf 'model%04d\\n' $i; i=$((i+1)); done\n")
	if r.Reason != "" || len(r.Models) > terminalproto.MaxModels || len(r.Models) == 0 || !r.More {
		t.Fatalf("result: %+v more=%t len=%d", r.Reason, r.More, len(r.Models))
	}
}

func TestListModelsTrimsToTheReplyLimit(t *testing.T) {
	names := strings.Repeat("a", 80)
	script := "i=0\nwhile [ $i -lt 400 ]; do printf 'm%04d%s\\n' $i '" + names + "'; i=$((i+1)); done\n"
	r := listModelsForTest(t, script)
	if r.Reason != "" || !r.More {
		t.Fatalf("result: %+v", r)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > terminalproto.MaxModelsReplyBytes {
		t.Fatalf("encoded result is %d bytes, limit %d", len(data), terminalproto.MaxModelsReplyBytes)
	}
	if len(r.Models) == 0 || len(r.Models) >= 400 {
		t.Fatalf("trimmed to %d entries", len(r.Models))
	}
}
