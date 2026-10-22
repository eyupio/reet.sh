package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func applyFixture(t *testing.T) (SnapshotSelection, Report, string) {
	t.Helper()
	e := fixture(t)
	const secret = "SYNTHETIC-APPLY-SECRET-DO-NOT-RETURN"
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{
  "model":"MODEL",
  "permissions":{"allow":["Read"]},
  "env":{"TOKEN":"`+secret+`"},
  "hooks":{"x":"`+secret+`"},
  "mcpServers":{"local":{"command":"server","args":["--x"],"env":{"TOKEN":"`+secret+`"}}}
}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	return selection, report, destination.home
}

func TestApplyRestorePlanCreatesVerifiedFiles(t *testing.T) {
	selection, report, home := applyFixture(t)
	plan := planForSelection(t, selection, report)
	journalDir := t.TempDir()
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: journalDir, Now: func() time.Time { return time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude.json")
	if len(result.Created) != 1 || result.Created[0] != path || result.JournalPath == "" || result.PlanFingerprint != plan.Fingerprint {
		t.Fatalf("wrong apply result: %+v", result)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("wrong restored permissions: %+v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := parseSource(data, "json")
	if err != nil {
		t.Fatal(err)
	}
	if object["model"] != "MODEL" || object["permissions"] == nil {
		t.Fatalf("settings missing: %#v", object)
	}
	if _, exists := object["env"]; exists {
		t.Fatal("credential-bearing setting was written")
	}
	servers, ok := object["mcpServers"].(map[string]any)
	if !ok || servers["local"] == nil {
		t.Fatalf("MCP content missing: %#v", object["mcpServers"])
	}
	for _, secret := range []string{"SYNTHETIC-APPLY-SECRET-DO-NOT-RETURN", "TOKEN"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("restored file contains %q", secret)
		}
	}
	journal, err := os.ReadFile(result.JournalPath)
	if err != nil {
		t.Fatal(err)
	}
	var record restoreJournal
	if err := json.Unmarshal(journal, &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "completed" || len(record.Actions) != 1 || record.Actions[0].State != "created" || bytes.Contains(journal, []byte("SYNTHETIC-APPLY-SECRET-DO-NOT-RETURN")) {
		t.Fatalf("bad journal: %+v", record)
	}
	// The destination now conflicts with the same snapshot.
	updatedEnv := fixture(t)
	updatedEnv.home = home
	updated, err := discover(updatedEnv)
	if err != nil {
		t.Fatal(err)
	}
	again := planForSelection(t, selection, updated)
	if action := findRestoreAction(t, again, path); action.State != "conflict" {
		t.Fatalf("restored file did not become a conflict: %+v", action)
	}
	if _, err := ApplyRestorePlan(again, RestoreApplyOptions{JournalDir: journalDir}); err == nil {
		t.Fatal("conflicting plan was applied")
	}
}

func TestApplyRestorePlanWritesTOML(t *testing.T) {
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = 'MODEL'\n[mcp_servers.local]\ncommand = 'server'\n")
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	if len(plan.Actions) != 1 || plan.Actions[0].Format != "toml" {
		t.Fatalf("wrong TOML plan: %+v", plan.Actions)
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Created[0])
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := toml.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	if object["model"] != "MODEL" {
		t.Fatalf("TOML model missing: %#v", object)
	}
	servers, ok := object["mcp_servers"].(map[string]any)
	if !ok || servers["local"] == nil {
		t.Fatalf("TOML MCP missing: %#v", object["mcp_servers"])
	}
}

func TestApplyRestorePlanRollsBackWhenDestinationChanges(t *testing.T) {
	selection, report, home := applyFixture(t)
	plan := planForSelection(t, selection, report)
	if len(plan.Actions) != 1 {
		t.Fatalf("unexpected actions: %+v", plan.Actions)
	}
	// Add a second source so the first write succeeds before the second fails.
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"MODEL"}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = 'MODEL'\n")
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err = BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	plan = planForSelection(t, selection, report)
	if len(plan.Actions) != 2 {
		t.Fatalf("expected two actions: %+v", plan.Actions)
	}
	journalDir := t.TempDir()
	_, err = ApplyRestorePlan(plan, RestoreApplyOptions{
		JournalDir: journalDir,
		beforeWrite: func(path string) error {
			if path == plan.Actions[1].Destination {
				return os.ErrExist
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("injected write failure was accepted")
	}
	if _, err := os.Stat(plan.Actions[0].Destination); !os.IsNotExist(err) {
		t.Fatalf("first restored file was not rolled back: %v", err)
	}
	entries, err := os.ReadDir(journalDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("journal missing: %+v %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(journalDir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var record restoreJournal
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "failed" || len(record.Actions) != 2 || record.Actions[0].State != "rolled_back" || record.Actions[1].State != "failed" {
		t.Fatalf("bad failure journal: %+v", record)
	}
	_ = home
}

func TestApplyRestorePlanCLI(t *testing.T) {
	sourceHome := isolatedCLI(t)
	const secret = "SYNTHETIC-APPLY-CLI-SECRET"
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"`+secret+`"}`)
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var snapshot SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &snapshot); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &applied); err != nil {
		t.Fatal(err)
	}
	var result RestoreApplyResult
	if err := json.Unmarshal(applied.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Created) != 1 || strings.Contains(applied.String(), secret) || strings.Contains(applied.String(), testSetupPassphrase) {
		t.Fatalf("bad apply output: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(destination, ".claude.json")); err != nil {
		t.Fatal(err)
	}
	var refused bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--password-file", passFile, "--dir", dir, snapshot.ID}, &refused); err == nil {
		t.Fatal("conflicting apply was accepted")
	}
}
