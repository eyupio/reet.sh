package scan

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostSetupPromptsForLocalConsentWithoutGrantingOnDecline(t *testing.T) {
	home, _ := os.UserHomeDir()
	hostname, _ := os.Hostname()
	config := filepath.Join(shortScanDir(t), "agent.json")
	original := agentConfig{Server: "https://reet.example", Token: "secret-do-not-print", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "Prompt host", Hostname: hostname, Home: home}
	if err := writePrivate(config, original); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var questions []string
	err := SetupTerminalHost(TerminalSetupOptions{Config: config, Server: original.Server, Ask: func(label, def string) (string, error) { questions = append(questions, label); return "no", nil }}, strings.NewReader(""), &out)
	if err != nil || len(questions) != 1 || !strings.Contains(questions[0], "Allow your enrolling account") {
		t.Fatal(questions, err)
	}
	saved, err := loadAgentConfig(config)
	if err != nil || saved.TerminalEnabled || saved.ID != original.ID || strings.Contains(out.String(), original.Token) {
		t.Fatal("decline changed enrollment or exposed token", err)
	}
}
func TestHostSetupOffersEnrollmentInsteadOfManualCommands(t *testing.T) {
	path := filepath.Join(shortScanDir(t), "agent.json")
	called := false
	var out bytes.Buffer
	err := SetupTerminalHost(TerminalSetupOptions{Config: path, Ask: func(label, def string) (string, error) {
		called = strings.Contains(label, "Also set up this machine")
		return "no", nil
	}}, strings.NewReader(""), &out)
	if err != nil || !called {
		t.Fatal("missing enrollment did not prompt", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("declining enrollment created configuration", err)
	}
}
