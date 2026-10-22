package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func isolatedCLI(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", t.TempDir())
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "XDG_CONFIG_HOME", "PI_CODING_AGENT_DIR", "OPENCODE_CONFIG_DIR", "OPENCODE_CONFIG"} {
		t.Setenv(key, "")
	}
	return home
}

func TestCLIJSONWithoutServerOrCredentials(t *testing.T) {
	home := isolatedCLI(t)
	t.Setenv("REET_URL", "http://127.0.0.1:1")
	t.Setenv("REET_TOKEN", "")
	mkdir(t, filepath.Join(home, ".pi", "agent"))
	var out bytes.Buffer
	if err := Run([]string{"scan", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("not a JSON-only response: %v", err)
	}
	if findAgent(t, r, "pi").State != "detected" {
		t.Fatalf("Pi setup missing: %s", out.String())
	}
}

func TestCLITextAndProject(t *testing.T) {
	isolatedCLI(t)
	project := t.TempDir()
	mkdir(t, filepath.Join(project, ".gemini"))
	var out bytes.Buffer
	if err := Run([]string{"scan", "--project", project}, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Gemini CLI: detected", "project configuration: present", "no values are read by this inventory"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in output: %s", want, out.String())
		}
	}
}

func TestCLIHelpAndInvalidArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"scan", "--help"}} {
		var out bytes.Buffer
		if err := Run(args, &out); err != nil || out.Len() == 0 {
			t.Fatalf("help %v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"restore"}, {"scan", "extra"}, {"scan", "--invalid"}} {
		var out bytes.Buffer
		if err := Run(args, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestCLIReportsExecutableWithoutRunningIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is POSIX-specific")
	}
	home := isolatedCLI(t)
	marker := filepath.Join(home, "executed")
	binary := filepath.Join(os.Getenv("PATH"), "claude")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf executed > '"+marker+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run([]string{"scan", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if findAgent(t, r, "claude").Executable.State != "present" {
		t.Fatal("executable was not found")
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("discovery executed an agent: %v", err)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestCLIPropagatesOutputFailure(t *testing.T) {
	isolatedCLI(t)
	for _, args := range [][]string{nil, {"scan"}, {"scan", "--json"}} {
		if err := Run(args, failedWriter{}); err == nil {
			t.Fatalf("ignored write error: %v", args)
		}
	}
}
