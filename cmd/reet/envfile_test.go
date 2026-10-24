package main

import (
	"bytes"
	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerminalStartReusesEnvConfiguration(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "reet-env-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	data := filepath.Join(dir, "data")
	envFile := filepath.Join(dir, ".env")
	if err = os.WriteFile(envFile, []byte("REET_URL='https://reet.example'\nREET_DATA_DIR="+data+"\nREET_TOKEN=must-not-print-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"REET_PUBLIC_URL", "REET_URL", "REET_DATA_DIR"} {
		if old, set := os.LookupEnv(key); set {
			t.Cleanup(func() { os.Setenv(key, old) })
		} else {
			t.Cleanup(func() { os.Unsetenv(key) })
		}
		os.Unsetenv(key)
	}
	st, err := store.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthAccounts))
	defer server.Close()
	control, err := terminalcontrol.Open(data, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	if err = control.Serve(server.SetRemoteTerminals, server.RemoteTerminalsAvailable, terminalcontrol.Options{Configure: server.ConfigureTerminalOrigin, Reason: server.RemoteTerminalsReason, PublicURL: server.RemoteTerminalPublicURL, AuthMode: server.RemoteTerminalAuthMode}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = runTerminalInput([]string{"start", "--env-file", envFile}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err, out.String())
	}
	if !server.RemoteTerminalsAvailable() || server.RemoteTerminalPublicURL() != "https://reet.example" || !strings.Contains(out.String(), "Availability: enabled") || strings.Contains(out.String(), "must-not-print-secret") {
		t.Fatal("env configuration not reused safely", out.String())
	}
	control.Close()
	next, err := terminalcontrol.Open(data, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.PublicURL() != "https://reet.example" {
		t.Fatal("origin not persisted")
	}
}
func TestServerEnvLoadingPreservesExplicitOverrides(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(file, []byte("REET_PUBLIC_URL='https://from-file.example' # comment\nREET_AUTH=accounts\nIGNORED_KEY=value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REET_PUBLIC_URL", "https://explicit.example")
	t.Setenv("REET_AUTH", "accounts")
	if err := applyServerEnvFile(file); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REET_PUBLIC_URL") != "https://explicit.example" {
		t.Fatal("exported origin overwritten")
	}
	values, err := readLocalEnv(file)
	if err != nil || values["REET_PUBLIC_URL"] != "https://from-file.example" || values["IGNORED_KEY"] != "" {
		t.Fatal("literal env parsing", err)
	}
}
