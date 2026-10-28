package terminalproto

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestClipboardFeatureToken(t *testing.T) {
	for _, value := range []string{"", "launch-v1", "clipboard-v10", "clipboard-v1x", "prefix-clipboard-v1"} {
		if SupportsClipboard(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsClipboard("launch-v1, clipboard-v1") || ClipboardFeature != "clipboard-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestClipboardCapabilityNeedsTerminalControl(t *testing.T) {
	c := testCapabilities()
	c.Clipboard = true
	if err := c.Validate(); err == nil {
		t.Fatal("clipboard without terminal control accepted")
	}
	c.TerminalControl = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	// The negotiated token is checked where capabilities are built, not in
	// Validate or ValidateFor: capability validation runs before a feature
	// list exists on several paths.
	if err := c.ValidateFor(""); err != nil {
		t.Fatal("ValidateFor enforced the clipboard token", err)
	}
}

func TestClipboardIsOmittedWhenOffAndRoundTripsStrictly(t *testing.T) {
	off, err := json.Marshal(testCapabilities())
	if err != nil || strings.Contains(string(off), `"clipboard"`) {
		t.Fatal(string(off), err)
	}
	on := testCapabilities()
	on.TerminalControl, on.Clipboard = true, true
	encoded, err := Encode(testMessage(Hello{AgentInstanceID: testID, Capabilities: on}))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	hello, ok := decoded.Body.(*Hello)
	if !ok || !hello.Capabilities.Clipboard || !hello.Capabilities.TerminalControl {
		t.Fatalf("clipboard lost: %+v", decoded.Body)
	}
	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("round trip changed", string(encoded), string(again), err)
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(`"clipboard":true`), []byte(`"clipboard":true,"clipboardSize":1`), 1)); err == nil {
		t.Fatal("unknown clipboard field accepted")
	}
	// A strict encoder still refuses a capability combination Validate rejects.
	bad := testCapabilities()
	bad.Clipboard = true
	if _, err := Encode(testMessage(Hello{AgentInstanceID: testID, Capabilities: bad})); err == nil {
		t.Fatal("encoded clipboard without terminal control")
	}
}
