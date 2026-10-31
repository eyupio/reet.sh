//go:build linux || darwin

package scan

import (
	"bytes"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
	"github.com/eyupio/reet.sh/internal/api"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/store/storetest"
)

func TestStandaloneTerminalSetupUsesRealTTYAndPreservesDeclinedConsent(t *testing.T) {
	home, _ := os.UserHomeDir()
	hostname, _ := os.Hostname()
	config := filepath.Join(shortScanDir(t), "agent.json")
	original := agentConfig{Server: "https://reet.example", Token: "never-print-token", ID: "agent0123456789abcdef", Secret: "secret0123456789abcdef", Host: "TTY host", Hostname: hostname, Home: home}
	if err := writePrivate(config, original); err != nil {
		t.Fatal(err)
	}
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	if _, err = master.Write([]byte("no\n")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = runAgentTerminalInput([]string{"setup", "--config", config}, slave, &out, "test"); err != nil {
		t.Fatal(err)
	}
	saved, err := loadAgentConfig(config)
	if err != nil || saved.TerminalEnabled || saved.ID != original.ID || saved.Secret != original.Secret {
		t.Fatal("declined setup changed enrollment", err)
	}
	if !strings.Contains(out.String(), "Allow your enrolling account") || strings.Contains(out.String(), original.Token) {
		t.Fatal(out.String())
	}
	out.Reset()
	if err = runAgentTerminalInput([]string{"setup", "--config", config}, strings.NewReader("yes\n"), &out, "test"); err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatal("noninteractive consent accepted", err)
	}
}

func TestEnrollmentActuallyOffersTerminalSetupAndCanSkipIt(t *testing.T) {
	for _, skip := range []bool{false, true} {
		t.Run(map[bool]string{false: "guided", true: "skip"}[skip], func(t *testing.T) {
			t.Setenv("REET_TOKEN", "")
			st, err := storetest.Open(t)
			if err != nil {
				t.Fatal(err)
			}
			defer st.Close()
			server := api.New(st, meta.Fetcher{}, "", api.WithAuthMode(api.AuthNone))
			defer server.Close()
			site := httptest.NewServer(server)
			defer site.Close()
			config := filepath.Join(shortScanDir(t), "agent.json")
			master, slave, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			defer slave.Close()
			if _, err = master.Write([]byte("no\n")); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			args := []string{"enroll", "--server", site.URL, "--env", "Guided machine", "--config", config, "--token", "fixture-enrollment-token"}
			if skip {
				args = append(args, "--skip-terminal-setup")
			}
			if err = runAgentInput(args, slave, &out, "test"); err != nil {
				t.Fatal(err)
			}
			saved, err := loadAgentConfig(config)
			if err != nil || saved.TerminalEnabled || saved.Secret == "" {
				t.Fatal("enrollment did not survive declined setup", err)
			}
			prompted := strings.Contains(out.String(), "Allow your enrolling account")
			if prompted == skip {
				t.Fatal("wrong wizard behavior", skip, out.String())
			}
			uid, _ := st.DefaultOwner()
			agents, err := st.SyncAgentsFor(uid)
			if err != nil || len(agents) != 1 {
				t.Fatal("wizard re-enrolled the machine", agents, err)
			}
		})
	}
}
