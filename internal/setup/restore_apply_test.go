package setup

import (
	"bytes"
	"encoding/json"
	"errors"
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

func fixtureWithHome(t *testing.T, home string) environment {
	t.Helper()
	e := fixture(t)
	e.home = home
	return e
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
	before, err := os.ReadFile(filepath.Join(destination, ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	var skipped bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--on-conflict", "skip", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &skipped); err != nil {
		t.Fatal(err)
	}
	var skipResult RestoreApplyResult
	if err := json.Unmarshal(skipped.Bytes(), &skipResult); err != nil || len(skipResult.Skipped) != 1 {
		t.Fatalf("skip result: %+v %v", skipResult, err)
	}
	afterSkip, err := os.ReadFile(filepath.Join(destination, ".claude.json"))
	if err != nil || !bytes.Equal(before, afterSkip) {
		t.Fatal("skipped conflict changed the destination")
	}
	var replaced bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--on-conflict", "replace", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &replaced); err != nil {
		t.Fatal(err)
	}
	var replaceResult RestoreApplyResult
	if err := json.Unmarshal(replaced.Bytes(), &replaceResult); err != nil || len(replaceResult.Replaced) != 1 || len(replaceResult.Backups) != 1 {
		t.Fatalf("replace result: %+v %v", replaceResult, err)
	}
	if err := Run([]string{"restore", "--on-conflict", "skip", "--password-file", passFile, "--dir", dir, snapshot.ID}, &bytes.Buffer{}); err == nil {
		t.Fatal("on-conflict without apply was accepted")
	}
}

func TestApplyRestorePlanSkipsConflicts(t *testing.T) {
	selection, report, home := applyFixture(t)
	path := filepath.Join(home, ".claude.json")
	original := `{"model":"KEEP-DESTINATION"}`
	writeConfig(t, path, original)
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, updated)
	if action := findRestoreAction(t, plan, path); action.State != "conflict" {
		t.Fatalf("destination not a conflict: %+v", action)
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "skip"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0] != path || len(result.Created) != 0 || len(result.Replaced) != 0 {
		t.Fatalf("conflict was not skipped: %+v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != original {
		t.Fatalf("skipped conflict changed the destination: %q %v", after, err)
	}
	var record restoreJournal
	data, err := os.ReadFile(result.JournalPath)
	if err != nil || json.Unmarshal(data, &record) != nil || record.State != "completed" || record.Actions[0].State != "skipped" {
		t.Fatalf("bad skip journal: %+v %v", record, err)
	}
	_ = report
}

func TestApplyRestorePlanReplacesWithRollbackCopy(t *testing.T) {
	selection, report, home := applyFixture(t)
	path := filepath.Join(home, ".claude.json")
	original := `{"model":"KEEP-DESTINATION"}`
	writeConfig(t, path, original)
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, updated)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "replace"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replaced) != 1 || result.Replaced[0] != path || len(result.Backups) != 1 {
		t.Fatalf("conflict was not replaced: %+v", result)
	}
	backup, err := os.ReadFile(result.Backups[0])
	if err != nil || string(backup) != original {
		t.Fatalf("rollback copy wrong: %q %v", backup, err)
	}
	info, err := os.Stat(result.Backups[0])
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("rollback copy permissions: %+v %v", info, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := parseSource(data, "json")
	if err != nil || object["model"] != "MODEL" {
		t.Fatalf("replacement content wrong: %#v %v", object, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), "SYNTHETIC-APPLY-SECRET-DO-NOT-RETURN") {
		t.Fatal("replace result exposed a value")
	}
	_ = report
}

func TestApplyRestorePlanRollsBackReplacedFilesOnFailure(t *testing.T) {
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"SOURCE-CLAUDE"}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = 'SOURCE-CODEX'\n")
	source, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelection(source)
	if err != nil {
		t.Fatal(err)
	}
	destination := fixture(t)
	first := filepath.Join(destination.home, ".claude.json")
	second := filepath.Join(destination.home, ".codex", "config.toml")
	writeConfig(t, first, `{"model":"KEEP-CLAUDE"}`)
	writeConfig(t, second, "model = 'KEEP-CODEX'\n")
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, report)
	if len(plan.Actions) != 2 {
		t.Fatalf("expected two conflicts: %+v", plan.Actions)
	}
	journalDir := t.TempDir()
	_, err = ApplyRestorePlan(plan, RestoreApplyOptions{
		JournalDir: journalDir, Conflict: "replace",
		beforeWrite: func(path string) error {
			if path == second {
				return errors.New("injected replace failure")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("injected replace failure was accepted")
	}
	afterFirst, err := os.ReadFile(first)
	if err != nil || string(afterFirst) != `{"model":"KEEP-CLAUDE"}` {
		t.Fatalf("replaced file not rolled back: %q %v", afterFirst, err)
	}
	afterSecond, err := os.ReadFile(second)
	if err != nil || string(afterSecond) != "model = 'KEEP-CODEX'\n" {
		t.Fatalf("second destination changed: %q %v", afterSecond, err)
	}
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		t.Fatal(err)
	}
	var journalPath string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "restore-") && strings.HasSuffix(entry.Name(), ".json") {
			journalPath = filepath.Join(journalDir, entry.Name())
		}
	}
	if journalPath == "" {
		t.Fatal("journal missing")
	}
	data, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	var record restoreJournal
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record.State != "failed" || record.Actions[0].State != "rolled_back" || record.Actions[1].State != "failed" {
		t.Fatalf("bad replace failure journal: %+v", record)
	}
}

func TestApplyRestorePlanRefusesBlockedActionWithSkip(t *testing.T) {
	selection, report, home := applyFixture(t)
	path := filepath.Join(home, ".claude.json")
	target := filepath.Join(t.TempDir(), "target.json")
	writeConfig(t, target, `{}`)
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, updated)
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "skip"}); err == nil {
		t.Fatal("blocked action was skipped")
	}
	_ = report
}

func TestApplyRestorePlanRejectsUnknownConflictPolicy(t *testing.T) {
	selection, report, _ := applyFixture(t)
	plan := planForSelection(t, selection, report)
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflict: "merge"}); err == nil {
		t.Fatal("unknown conflict policy accepted")
	}
}

func TestApplyRestorePlanPerFileConflictChoices(t *testing.T) {
	selection, _, home, claude := rollbackFixture(t)
	codex := filepath.Join(home, ".codex", "config.toml")
	writeConfig(t, codex, "model = 'KEEP-CODEX'\n")
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, updated)
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{
		JournalDir: t.TempDir(),
		Conflicts:  map[string]string{claude: "replace", codex: "skip"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Replaced) != 1 || result.Replaced[0] != claude || len(result.Skipped) != 1 || result.Skipped[0] != codex {
		t.Fatalf("per-file choices not applied: %+v", result)
	}
	claudeData, err := os.ReadFile(claude)
	var claudeObject map[string]any
	if err != nil || json.Unmarshal(claudeData, &claudeObject) != nil || claudeObject["model"] != "SOURCE-CLAUDE" {
		t.Fatalf("replaced file content wrong: %q %v", claudeData, err)
	}
	codexData, err := os.ReadFile(codex)
	if err != nil || string(codexData) != "model = 'KEEP-CODEX'\n" {
		t.Fatalf("skipped file changed: %q %v", codexData, err)
	}
}

func TestApplyRestorePlanRejectsBadPerFileChoices(t *testing.T) {
	selection, report, home := applyFixture(t)
	path := filepath.Join(home, ".claude.json")
	writeConfig(t, path, `{"model":"KEEP"}`)
	updated, err := discover(fixtureWithHome(t, home))
	if err != nil {
		t.Fatal(err)
	}
	plan := planForSelection(t, selection, updated)
	for _, choices := range []map[string]string{
		{filepath.Join(home, "unknown.json"): "skip"},
		{path: "merge"},
		{filepath.Join(home, ".codex", "config.toml"): "skip"},
	} {
		if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir(), Conflicts: choices}); err == nil {
			t.Fatalf("bad per-file choice accepted: %+v", choices)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != `{"model":"KEEP"}` {
		t.Fatal("refused choices changed the destination")
	}
	_ = report
}

func TestApplyRestorePlanPerFileChoicesCLI(t *testing.T) {
	sourceHome := isolatedCLI(t)
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"SOURCE-CLAUDE"}`)
	writeConfig(t, filepath.Join(sourceHome, ".codex", "config.toml"), "model = 'SOURCE-CODEX'\n")
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
	claude := filepath.Join(destination, ".claude.json")
	codex := filepath.Join(destination, ".codex", "config.toml")
	writeConfig(t, claude, `{"model":"KEEP-CLAUDE"}`)
	writeConfig(t, codex, "model = 'KEEP-CODEX'\n")
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--on-conflict", "skip", "--conflict", claude + "=replace", "--password-file", passFile, "--dir", dir, "--json", snapshot.ID}, &applied); err != nil {
		t.Fatal(err)
	}
	var result RestoreApplyResult
	if err := json.Unmarshal(applied.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Replaced) != 1 || result.Replaced[0] != claude || len(result.Skipped) != 1 || result.Skipped[0] != codex {
		t.Fatalf("per-file CLI choices wrong: %+v", result)
	}
	for _, args := range [][]string{
		{"restore", "--apply", "--conflict", "bad", "--password-file", passFile, "--dir", dir, snapshot.ID},
		{"restore", "--apply", "--conflict", claude + "=merge", "--password-file", passFile, "--dir", dir, snapshot.ID},
		{"restore", "--conflict", claude + "=skip", "--password-file", passFile, "--dir", dir, snapshot.ID},
	} {
		if err := Run(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestResumeRestorePlanContinuesPendingActions(t *testing.T) {
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"MODEL"}`)
	writeConfig(t, filepath.Join(e.home, ".codex", "config.toml"), "model = 'MODEL'\n")
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
	if len(plan.Actions) != 2 {
		t.Fatalf("expected two actions: %+v", plan.Actions)
	}
	first := plan.Actions[0]
	data, err := restoreContent(plan.files[first.Destination])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(first.Destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.Destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
	journalDir := t.TempDir()
	journalPath := filepath.Join(journalDir, "restore-running.json")
	journal := restoreJournal{
		SchemaVersion: restoreJournalSchemaVersion, SnapshotID: plan.SnapshotID,
		PlanFingerprint: plan.Fingerprint, State: "running", StartedAt: time.Now().UTC(),
		Actions: []restoreJournalAction{
			{Destination: first.Destination, State: "created", AppliedSHA256: sha256Bytes(data)},
			{Destination: plan.Actions[1].Destination, State: "pending"},
		},
	}
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		t.Fatal(err)
	}
	result, err := ApplyRestorePlan(plan, RestoreApplyOptions{ResumeJournal: journalPath})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Resumed || len(result.Created) != 2 || len(result.JournalPath) == 0 {
		t.Fatalf("wrong resume result: %+v", result)
	}
	if _, err := os.Stat(plan.Actions[1].Destination); err != nil {
		t.Fatal("pending action was not applied")
	}
	loaded, _, err := loadRestoreJournal(journalPath)
	if err != nil || loaded.State != "completed" {
		t.Fatalf("journal was not completed: %+v %v", loaded, err)
	}
}

func TestResumeRestorePlanRefusesChangedAppliedAction(t *testing.T) {
	selection, report, home := applyFixture(t)
	plan := planForSelection(t, selection, report)
	if len(plan.Actions) != 1 {
		t.Fatalf("unexpected actions: %+v", plan.Actions)
	}
	action := plan.Actions[0]
	writeConfig(t, filepath.Join(home, ".claude.json"), `{"model":"CHANGED"}`)
	journalPath := filepath.Join(t.TempDir(), "restore-running.json")
	journal := restoreJournal{
		SchemaVersion: restoreJournalSchemaVersion, SnapshotID: plan.SnapshotID,
		PlanFingerprint: plan.Fingerprint, State: "running", StartedAt: time.Now().UTC(),
		Actions: []restoreJournalAction{{Destination: action.Destination, State: "created", AppliedSHA256: strings.Repeat("0", 64)}},
	}
	if err := writeRestoreJournal(journalPath, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyRestorePlan(plan, RestoreApplyOptions{ResumeJournal: journalPath}); err == nil {
		t.Fatal("changed applied action was resumed")
	}
	if data, err := os.ReadFile(action.Destination); err != nil || string(data) != `{"model":"CHANGED"}` {
		t.Fatal("failed resume changed the destination")
	}
}
