package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/clientconfig"
)

func TestConfigureSavesDefaultsForLaterCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("REET_VERSION", "")
	t.Setenv("REET_URL", "")
	var out bytes.Buffer
	if err := runConfigure([]string{"--version", "dev", "--url", "https://example.test"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if err := clientconfig.Apply(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("REET_VERSION") != "dev" || os.Getenv("REET_URL") != "https://example.test" {
		t.Fatal("saved defaults not applied")
	}
	path, _ := clientconfig.Path()
	before, _ := os.ReadFile(path)
	if err := runConfigure([]string{"--url", "https://user:secret@example.test"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("accepted URL credentials")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid configuration changed file")
	}
	if strings.Contains(out.String(), "secret") {
		t.Fatal("printed rejected credentials")
	}
}
func TestLocalAndInformationalCommandsDoNotLoadClientDefaults(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		args []string
	}{{"serve", nil}, {"version", nil}, {"healthcheck", []string{"--local"}}, {"upgrade", []string{"--help"}}} {
		if usesClientDefaults(tc.cmd, tc.args) {
			t.Fatalf("loaded config for %s", tc.cmd)
		}
	}
	for _, cmd := range []string{"configure", "upgrade", "doctor", "scan", "sync", "agent", "healthcheck", "setup"} {
		if !usesClientDefaults(cmd, nil) {
			t.Fatal(cmd)
		}
	}
}

func TestConfigureForgetsSavedToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	if _, err := clientconfig.SaveToken("secret-token"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runConfigure([]string{"--forget-token"}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	path, _ := clientconfig.TokenPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("saved token still present")
	}
	if err := runConfigure([]string{"--forget-token", "--url", "https://example.test"}, strings.NewReader(""), &out); err == nil {
		t.Fatal("accepted conflicting flags")
	}
}
