package terminalproto

import (
	"strings"
	"testing"
)

func setupTestItem(content string) SetupItem {
	return SetupItem{Agent: "claude", Scope: "global", Kind: "instructions", Name: "CLAUDE.md", Format: "text", Content: content, SHA256: SetupItemSHA256(content)}
}

func TestSetupFleetMessagesRoundTripAndRefuseBadShapes(t *testing.T) {
	fp := strings.Repeat("a", 64)
	good := []Body{
		SetupFleet{Op: SetupOpRead},
		SetupFleet{Op: SetupOpPlan, Items: []SetupItem{setupTestItem("x\n")}},
		SetupFleet{Op: SetupOpApply, Items: []SetupItem{setupTestItem("x\n")}, Fingerprint: fp, Label: "fleet-abc"},
		SetupFleet{Op: SetupOpRollback, Journal: "restore-fleet-abc-20261125T000000Z-0a0b0c0d"},
		SetupFleetResult{Op: SetupOpRead, State: SetupOK, Items: []SetupItem{setupTestItem("y")}},
		SetupFleetResult{Op: SetupOpPlan, State: SetupOK, Fingerprint: fp, Files: []SetupFilePlan{{Agent: "codex", Kind: "settings", Name: "config.toml", State: "merge", Fields: []string{"sandbox_mode"}, Widens: []string{"sandbox_mode"}}}},
		SetupFleetResult{Op: SetupOpApply, State: SetupOK, Journal: "restore-fleet-abc-1"},
		SetupFleetResult{Op: SetupOpRead, State: SetupRefused, Reason: SetupReasonCredential, Agent: "codex", Name: "AGENTS.md"},
	}
	for _, body := range good {
		data, err := Encode(Message{Version: Version, RequestID: "setup-request-0001", Generation: 1, Body: body})
		if err != nil {
			t.Fatalf("%+v: %v", body, err)
		}
		if _, err := Decode(data); err != nil {
			t.Fatalf("decode %+v: %v", body, err)
		}
	}
	tampered := setupTestItem("x")
	tampered.Content = "y"
	pathName := setupTestItem("x")
	pathName.Name = "../CLAUDE.md"
	projectScope := setupTestItem("x")
	projectScope.Scope = "project"
	bad := []Body{
		SetupFleet{Op: "write"},
		SetupFleet{Op: SetupOpRead, Journal: "restore-x"},
		SetupFleet{Op: SetupOpPlan},
		SetupFleet{Op: SetupOpPlan, Items: []SetupItem{tampered}},
		SetupFleet{Op: SetupOpPlan, Items: []SetupItem{pathName}},
		SetupFleet{Op: SetupOpPlan, Items: []SetupItem{projectScope}},
		SetupFleet{Op: SetupOpPlan, Items: []SetupItem{setupTestItem("a"), setupTestItem("a")}},
		SetupFleet{Op: SetupOpApply, Items: []SetupItem{setupTestItem("x")}, Label: "fleet-abc"},
		SetupFleet{Op: SetupOpRollback, Journal: "../etc/passwd"},
		SetupFleetResult{Op: SetupOpRead, State: SetupRefused, Reason: "because"},
		SetupFleetResult{Op: SetupOpPlan, State: SetupOK, Items: []SetupItem{setupTestItem("y")}},
		SetupFleetResult{Op: SetupOpPlan, State: SetupOK, Files: []SetupFilePlan{{Agent: "claude", Kind: "settings", Name: "settings.json", State: "replace"}}},
		SetupFleetResult{Op: SetupOpRead, State: SetupOK, Items: []SetupItem{setupTestItem(strings.Repeat("z", MaxSetupBytes))}},
	}
	for _, body := range bad {
		if body.valid() {
			t.Errorf("accepted %+v", body)
		}
	}
	// Setup steps follow terminal control, like software apply.
	caps := testCapabilities()
	caps.SetupApply = true
	if caps.Validate() == nil {
		t.Fatal("setup apply without terminal control")
	}
	caps.TerminalControl = true
	if err := caps.Validate(); err != nil {
		t.Fatal(err)
	}
}
