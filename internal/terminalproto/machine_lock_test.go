package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

// Locked rides on capabilities only to a controller that negotiated
// machine-lock-v1, and never beside a machine that says it is ready.
func TestCapabilitiesLockedOnlyWhenNegotiated(t *testing.T) {
	locked := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "dev", HealthEnabled: true, TerminalControl: true, Readiness: DisabledLocally, Locked: true}
	if locked.ValidateFor("") == nil || locked.ValidateFor(AttentionFeature) == nil {
		t.Fatal("Locked was accepted without machine-lock-v1")
	}
	if err := locked.ValidateFor("x, " + MachineLockFeature); err != nil {
		t.Fatalf("negotiated Locked refused: %v", err)
	}
	if err := locked.Validate(); err != nil {
		t.Fatalf("Validate refused a locked machine: %v", err)
	}
	ready := locked
	ready.TerminalEnabled, ready.Readiness = true, Ready
	if ready.ValidateFor(MachineLockFeature) == nil {
		t.Fatal("a locked machine claimed to be ready for terminals")
	}
	raw, err := json.Marshal(Capabilities{Readiness: DisabledLocally})
	if err != nil || strings.Contains(string(raw), "locked") {
		t.Fatalf("an unlocked machine sent the field: %s %v", raw, err)
	}
	if !SupportsMachineLock(AttentionFeature+","+MachineLockFeature) || SupportsMachineLock(AttentionFeature) {
		t.Fatal("SupportsMachineLock does not read the header")
	}
}

// The locked result code survives a round trip and carries no detail.
func TestLockedResultRoundTrip(t *testing.T) {
	m := Message{Version: Version, RequestID: "req_1234567890123456", Generation: 1, Body: Result{Code: Locked}}
	encoded, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Body.(*Result); got.Code != Locked {
		t.Fatalf("got %+v", got)
	}
	m.Body = Result{Code: Locked, Detail: LockedSentence}
	if _, err := Encode(m); err == nil {
		t.Fatal("a locked result carried a detail")
	}
}
