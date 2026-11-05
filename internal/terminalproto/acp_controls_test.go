package terminalproto

import (
	"fmt"
	"testing"
)

func TestStructuredStartupEffortUsesAgentOptionTokens(t *testing.T) {
	for _, effort := range []string{"high", "xhigh", "max", "ultra"} {
		c := Create{Title: "Review", Cols: 100, Rows: 30, Tool: "codex", Structured: true, Effort: effort}
		if err := c.Validate(); err != nil {
			t.Fatalf("structured effort %q: %v", effort, err)
		}
	}
	c := Create{Title: "Review", Cols: 100, Rows: 30, Tool: "codex", Effort: "ultra"}
	if c.Validate() == nil {
		t.Fatal("unmapped terminal effort accepted")
	}
	c.Structured = true
	c.Effort = "--unsafe effort"
	if c.Validate() == nil {
		t.Fatal("invalid selector token accepted")
	}
}

func TestLiveControlsCapabilityRequiresItsNegotiatedFeature(t *testing.T) {
	c := Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "dev", TerminalEnabled: true, TerminalControl: true, ACP: true, ACPLiveControls: true, Readiness: Ready}
	if c.ValidateFor(ACPFeature) == nil {
		t.Fatal("live controls admitted on an older connection")
	}
	if err := c.ValidateFor(ACPFeature + "," + ACPLiveControlsFeature); err != nil {
		t.Fatal(err)
	}
	c.ACP = false
	if c.Validate() == nil {
		t.Fatal("live controls admitted without chat transport")
	}
}

func TestLiveControlWireBoundsAndPermissionSeparation(t *testing.T) {
	state := ACPControls{Model: "provider/model", Options: []ACPSetting{{ID: "effort", Category: "thought_level", Current: "low", Values: []string{"low", "high"}}}}
	for _, body := range []Body{ACPControl{Action: "get"}, ACPControl{Action: "set", ConfigID: "effort", Value: "high"}, ACPControl{Action: "steer", Text: "also check tests"}, ACPControlResult{Controls: state, Outcome: "injected"}} {
		m := Message{Version: Version, RequestID: "request-controls0123456789", SessionID: "session-controls0123456789", Generation: 1, Body: body}
		raw, err := Encode(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Decode(raw); err != nil {
			t.Fatal(err)
		}
	}
	bad := state
	bad.Options = []ACPSetting{{ID: "mode", Category: "mode", Current: "agent-full-access"}}
	if bad.Valid() {
		t.Fatal("permission options were admitted as live non-permission settings")
	}
	bad = state
	bad.Options = append(bad.Options, bad.Options[0])
	if bad.Valid() {
		t.Fatal("duplicate selectors admitted")
	}
	if (ACPControl{Action: "get", Text: "hidden mutation"}).valid() {
		t.Fatal("get carried mutation fields")
	}
	if (ACPControlResult{Controls: state, Outcome: "unrecognised"}).valid() {
		t.Fatal("unknown delivery result admitted")
	}
}

func TestStartupOptionsRequireSeparateNegotiation(t *testing.T) {
	c := Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "dev", TerminalEnabled: true, TerminalControl: true, ACP: true, ACPLiveControls: true, ACPStartOptions: true, Readiness: Ready}
	live := ACPFeature + "," + ACPLiveControlsFeature
	if c.ValidateFor(live) == nil {
		t.Fatal("startup options advertised on a live-controls-only connection")
	}
	if err := c.ValidateFor(live + "," + ACPStartOptionsFeature); err != nil {
		t.Fatal(err)
	}
	c.ACPLiveControls = false
	if c.Validate() == nil {
		t.Fatal("startup options accepted without live configuration support")
	}
}

// A model selector may carry a whole catalogue: Cline offers 307 ids. The
// effort selector keeps the shorter bound, and a snapshot still fits one frame.
func TestModelCatalogueBoundsTheModelSelectorNotEffort(t *testing.T) {
	models := make([]string, 0, MaxACPSettingValues)
	for i := 0; i < MaxACPSettingValues; i++ {
		models = append(models, fmt.Sprintf("provider/model-%03d", i))
	}
	state := ACPControls{Model: models[0], Options: []ACPSetting{{ID: "model", Category: "model", Current: models[0], Values: models}}}
	if !state.Valid() {
		t.Fatalf("a %d-value catalogue was refused", len(models))
	}
	tooMany := state
	tooMany.Options = []ACPSetting{{ID: "model", Category: "model", Current: models[0], Values: append(models, "provider/one-too-many")}}
	if tooMany.Valid() {
		t.Fatal("a selector over the catalogue bound was admitted")
	}
	efforts := make([]string, 0, MaxACPThoughtLevelValues+1)
	for i := 0; i <= MaxACPThoughtLevelValues; i++ {
		efforts = append(efforts, fmt.Sprintf("level-%02d", i))
	}
	long := ACPControls{Options: []ACPSetting{{ID: "effort", Category: "thought_level", Values: efforts}}}
	if long.Valid() {
		t.Fatal("an effort selector over its bound was admitted")
	}
	// The wire form keeps the same bounds in both directions.
	raw, err := Encode(Message{Version: Version, RequestID: "request-catalogue0123456789", SessionID: "session-catalogue0123456789", Generation: 1, Body: ACPControlResult{Controls: state}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Decode(raw); err != nil {
		t.Fatalf("a catalogue reply was refused on the wire: %v", err)
	}
}
