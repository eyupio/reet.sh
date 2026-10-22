package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupHealthReportGivesConservativeGuidance(t *testing.T) {
	e := fixture(t)
	mkdir(t, filepath.Join(e.home, ".claude"))
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	report := BuildSetupHealthReport(r)
	if report.SchemaVersion != setupHealthSchemaVersion || report.OS == "" || len(report.Agents) == 0 || report.Summary.Warn == 0 {
		t.Fatalf("incomplete health report: %+v", report)
	}
	var claude SetupAgentHealth
	for _, agent := range report.Agents {
		if agent.ID == "claude" {
			claude = agent
		}
	}
	if len(claude.Checks) != 3 || claude.Checks[0].Code != "executable" || claude.Checks[1].Code != "application_version" || claude.Checks[2].Code != "installation" {
		t.Fatalf("wrong checks: %+v", claude.Checks)
	}
	if claude.Checks[0].State != "warn" || claude.Checks[1].State != "info" || claude.Checks[2].State != "info" {
		t.Fatalf("wrong states: %+v", claude.Checks)
	}
	var text bytes.Buffer
	if err := renderSetupHealth(&text, report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "AI setup health") || !strings.Contains(text.String(), "Credential stores") {
		t.Fatalf("bad health text: %s", text.String())
	}
}

func TestSetupHealthCLIAndProbes(t *testing.T) {
	home := isolatedCLI(t)
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"SETUP-HEALTH"}`)
	var out bytes.Buffer
	if err := Run([]string{"health", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	var report SetupHealthReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != setupHealthSchemaVersion || len(report.Agents) == 0 {
		t.Fatalf("wrong CLI health: %+v", report)
	}
	var text bytes.Buffer
	if err := Run([]string{"health"}, &text); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "AI setup health") {
		t.Fatalf("bad health text: %s", text.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude.json")); err != nil {
		t.Fatal("health changed the configuration")
	}
}
