package terminalproto

import "testing"

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
