package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func credentialFixture(t *testing.T) (environment, SnapshotSelection) {
	t.Helper()
	e := fixture(t)
	const secret = "SYNTHETIC-CREDENTIAL-SECRET-DO-NOT-RETURN"
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{
  "model":"MODEL",
  "env":{"TOKEN":"`+secret+`"},
  "mcpServers":{
    "local":{"command":"server","env":{"TOKEN":"`+secret+`"},"headers":{"Authorization":"`+secret+`"}},
    "remote":{"type":"http","url":"https://example.test/?key=`+secret+`"}
  }
}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := BuildSnapshotSelectionWithOptions(r, SnapshotBuildOptions{IncludeCredentials: true})
	if err != nil {
		t.Fatal(err)
	}
	return e, selection
}

func TestCredentialsOnlyWithExplicitOptIn(t *testing.T) {
	e, selection := credentialFixture(t)
	const secret = "SYNTHETIC-CREDENTIAL-SECRET-DO-NOT-RETURN"
	if !selection.Preview.CredentialsIncluded {
		t.Fatal("selection did not mark credentials")
	}
	var settings, mcp map[string]any
	for _, source := range selection.sources {
		switch source.Component {
		case "settings":
			settings = source.Content
		case "mcp":
			mcp = source.Content
		}
	}
	if settings["env"] == nil {
		t.Fatalf("settings credentials missing: %#v", settings)
	}
	local := mcp["local"].(map[string]any)
	remote := mcp["remote"].(map[string]any)
	if local["env"] == nil || local["headers"] == nil || remote["url"] == nil {
		t.Fatalf("MCP credentials missing: %#v", mcp)
	}
	defaultReport, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	defaultSelection, err := BuildSnapshotSelection(defaultReport)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range defaultSelection.sources {
		if source.Component == "settings" && source.Content["env"] != nil {
			t.Fatal("credential selected without opt-in")
		}
	}
	dir := t.TempDir()
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	if !result.CredentialsIncluded {
		t.Fatal("write result did not mark credentials")
	}
	raw, err := os.ReadFile(result.Path)
	if err != nil || bytes.Contains(raw, []byte(secret)) {
		t.Fatal("credential value was not sealed")
	}
	encoded, err := json.Marshal(result)
	if err != nil || strings.Contains(string(encoded), secret) {
		t.Fatal("write result exposed credentials")
	}
	for _, component := range result.Preview.Agents[0].Components {
		if component.Digest != "" {
			t.Fatalf("credential-bearing preview kept a digest: %+v", component)
		}
	}
	inspection, err := InspectSetupSnapshot(result.ID, testSetupPassphrase, dir)
	if err != nil {
		t.Fatal(err)
	}
	inspectionJSON, err := json.Marshal(inspection)
	if err != nil || strings.Contains(string(inspectionJSON), secret) {
		t.Fatal("inspection exposed credentials")
	}
	for _, source := range inspection.Sources {
		if source.Digest == "" {
			t.Fatalf("inspection lost the encrypted source digest: %+v", source)
		}
	}
	destination := fixture(t)
	report, err := discover(destination)
	if err != nil {
		t.Fatal(err)
	}
	plan := BuildRestorePlan(BuildSetupSnapshot(selection), testManifest("setup-credentials"), report)
	resultApply, err := ApplyRestorePlan(plan, RestoreApplyOptions{JournalDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(resultApply.Created) != 1 {
		t.Fatalf("wrong apply result: %+v", resultApply)
	}
	restored, err := os.ReadFile(resultApply.Created[0])
	if err != nil || !bytes.Contains(restored, []byte(secret)) {
		t.Fatal("credential value was not restored")
	}
	updated, err := discover(fixtureWithHome(t, destination.home))
	if err != nil {
		t.Fatal(err)
	}
	verified := VerifySetupSnapshot(BuildSetupSnapshot(selection), testManifest("setup-credentials"), updated)
	if verified.Summary.Verified != 2 {
		t.Fatalf("credential snapshot not verified: %+v", verified)
	}
	verifyJSON, err := json.Marshal(verified)
	if err != nil || strings.Contains(string(verifyJSON), secret) {
		t.Fatal("verification exposed credentials")
	}
}

func TestCredentialValuesRequireWriteCLI(t *testing.T) {
	dir := t.TempDir()
	passFile := writePassphrase(t, t.TempDir(), testSetupPassphrase)
	var preview bytes.Buffer
	if err := Run([]string{"backup", "--preview", "--include-credentials", "--json", "--dir", dir, "--password-file", passFile}, &preview); err == nil {
		t.Fatal("preview accepted credential inclusion")
	}
	sourceHome := isolatedCLI(t)
	const secret = "SYNTHETIC-CREDENTIAL-CLI-SECRET"
	writeConfig(t, filepath.Join(sourceHome, ".claude.json"), `{"model":"MODEL","env":{"TOKEN":"`+secret+`"}}`)
	var written bytes.Buffer
	if err := Run([]string{"backup", "--include-credentials", "--password-file", passFile, "--dir", dir, "--json"}, &written); err != nil {
		t.Fatal(err)
	}
	var result SetupWriteResult
	if err := json.Unmarshal(written.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.CredentialsIncluded || strings.Contains(written.String(), secret) {
		t.Fatalf("bad credential CLI result: %+v", result)
	}
	destination := t.TempDir()
	t.Setenv("HOME", destination)
	t.Setenv("USERPROFILE", destination)
	var applied bytes.Buffer
	if err := Run([]string{"restore", "--apply", "--password-file", passFile, "--dir", dir, "--json", result.ID}, &applied); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(applied.String(), secret) {
		t.Fatal("restore output exposed credentials")
	}
	data, err := os.ReadFile(filepath.Join(destination, ".claude.json"))
	if err != nil || !bytes.Contains(data, []byte(secret)) {
		t.Fatal("restored credential missing")
	}
	var verified bytes.Buffer
	if err := Run([]string{"verify", "--password-file", passFile, "--dir", dir, "--json", result.ID}, &verified); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(verified.String(), secret) {
		t.Fatal("verify output exposed credentials")
	}
}

func TestCredentialSnapshotContentTamperIsRejected(t *testing.T) {
	_, selection := credentialFixture(t)
	snapshot := BuildSetupSnapshot(selection)
	if err := verifySetupSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Sources[0].Content["env"] = map[string]any{"TOKEN": "TAMPERED"}
	if err := verifySetupSnapshot(snapshot); err == nil {
		t.Fatal("tampered credential content accepted")
	}
}
