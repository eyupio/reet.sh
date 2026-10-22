package setup

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupSnapshotRecordsSoftwareEvidence(t *testing.T) {
	e := fixture(t)
	writeConfig(t, filepath.Join(e.home, ".claude.json"), `{"model":"MODEL"}`)
	r, err := discover(e)
	if err != nil {
		t.Fatal(err)
	}
	r.Agents[0].ApplicationVersion = &VersionObservation{State: "reported", Version: "1.2.3", Source: "executable --version", InstallMethod: "unknown"}
	r.Agents[0].Installation = &InstallationObservation{State: "matched", Kind: "node_package", Manager: "unknown", Package: "@anthropic-ai/claude-code", Version: "1.2.3", MetadataPath: "/private/package.json"}
	selection, err := BuildSnapshotSelection(r)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := BuildSetupSnapshot(selection)
	if len(snapshot.Software) != 1 || snapshot.Software[0].Version != "1.2.3" || snapshot.Software[0].Package != "@anthropic-ai/claude-code" || snapshot.Software[0].Manager != "unknown" {
		t.Fatalf("software evidence missing: %+v", snapshot.Software)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "/private/package.json") || strings.Contains(string(encoded), filepath.Join(e.home, "bin")) {
		t.Fatal("software evidence exposed executable or metadata paths")
	}
	dir := t.TempDir()
	result, err := WriteSetupSnapshot(selection, SetupWriteOptions{Dir: dir, Password: testSetupPassphrase})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Software) != 1 || result.Software[0].Version != "1.2.3" {
		t.Fatalf("write result lost software evidence: %+v", result.Software)
	}
	inspection, err := InspectSetupSnapshot(result.ID, testSetupPassphrase, dir)
	if err != nil || len(inspection.Software) != 1 || inspection.Software[0].Package != "@anthropic-ai/claude-code" {
		t.Fatalf("inspection lost software evidence: %+v %v", inspection.Software, err)
	}
	var text bytes.Buffer
	if err := renderSnapshotInspection(&text, inspection); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "software Claude Code") || strings.Contains(text.String(), "/private/package.json") {
		t.Fatalf("bad software inspection text: %s", text.String())
	}
}

func TestVerifySoftwareEvidenceGuidance(t *testing.T) {
	source := []SetupSoftware{{
		AgentID: "claude", Name: "Claude Code", Version: "1.2.3", VersionState: "reported",
		InstallState: "matched", Package: "@anthropic-ai/claude-code", InstallVersion: "1.2.3", Manager: "unknown",
	}}
	destination := Report{Agents: []Agent{{
		ID: "claude", ApplicationVersion: &VersionObservation{State: "reported", Version: "1.2.3"},
		Installation: &InstallationObservation{State: "matched", Package: "@anthropic-ai/claude-code"},
	}}}
	result := VerifySetupSnapshot(SetupSnapshot{Software: source}, testManifest("setup-software"), destination)
	if len(result.Software) != 1 || result.Software[0].State != "matches" || result.Summary.SoftwareMatches != 1 {
		t.Fatalf("version match not reported: %+v", result.Software)
	}
	destination.Agents[0].ApplicationVersion.Version = "2.0.0"
	result = VerifySetupSnapshot(SetupSnapshot{Software: source}, testManifest("setup-software"), destination)
	if result.Software[0].State != "version_mismatch" || !strings.Contains(result.Software[0].Guidance, "differs") || result.Summary.SoftwareWarnings != 1 {
		t.Fatalf("version mismatch not reported: %+v", result.Software)
	}
	result = VerifySetupSnapshot(SetupSnapshot{Software: source}, testManifest("setup-software"), Report{})
	if result.Software[0].State != "agent_not_detected" || result.Summary.SoftwareWarnings != 1 {
		t.Fatalf("missing agent not reported: %+v", result.Software)
	}
	var text bytes.Buffer
	if err := renderSetupVerify(&text, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text.String(), "software Claude Code: agent_not_detected") {
		t.Fatalf("software guidance missing: %s", text.String())
	}
}
