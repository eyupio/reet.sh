package scan

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnrollmentRetryReusesOnlySameMachineAndCreatesSharedFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	hostname, _ := os.Hostname()
	config := filepath.Join(home, "agent.json")
	c := agentConfig{Server: "https://reet.example", Host: "studio", Hostname: hostname, Home: home, ID: "unchanged", Secret: "unchanged"}
	if err := writePrivate(config, c); err != nil {
		t.Fatal(err)
	}
	args := []string{"enroll", "--config", config, "--server", c.Server, "--env", c.Host, "--if-needed", "--allow-apply"}
	for range 2 {
		if err := RunAgentInput(args, strings.NewReader(""), io.Discard, "test"); err != nil {
			t.Fatal(err)
		}
	}
	got, err := loadAgentConfig(config)
	if err != nil || !got.AllowApply || got.ID != c.ID || got.Secret != c.Secret {
		t.Fatalf("enrollment changed: %+v %v", got, err)
	}
	if st, err := os.Stat(filepath.Join(home, ".agents", "skills")); err != nil || !st.IsDir() {
		t.Fatal("shared folder unavailable", err)
	}
	for _, args := range [][]string{
		{"enroll", "--config", config, "--server", "https://another.example", "--env", c.Host, "--if-needed"},
		{"enroll", "--config", config, "--server", c.Server, "--env", "different-machine", "--if-needed"},
	} {
		if err := RunAgentInput(args, strings.NewReader(""), io.Discard, "test"); err == nil {
			t.Fatal("reused an unrelated enrollment")
		}
	}
}
