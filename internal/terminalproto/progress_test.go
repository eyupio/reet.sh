package terminalproto

import "testing"

func TestLaunchProgressRoundTripsAndRejectsUnknownStage(t *testing.T) {
	if !SupportsProgress("launch-v1, "+ProgressFeature) || SupportsProgress("launch-v1") {
		t.Fatal("feature negotiation is wrong")
	}
	if !(LaunchProgress{Stage: StageStarting}).valid() || (LaunchProgress{Stage: "bogus"}).valid() {
		t.Fatal("stage validation is wrong")
	}
}

// A stage note travels with its create's request id and nothing else: with a
// session id it is not a valid message and would never be sent.
func TestLaunchProgressMessageCarriesNoSessionID(t *testing.T) {
	note := Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: LaunchProgress{Stage: StageWorktree}}
	if _, err := Encode(note); err != nil {
		t.Fatal("a stage note with a request id alone is refused:", err)
	}
	note.SessionID = "session-0123456789"
	if _, err := Encode(note); err == nil {
		t.Fatal("a stage note carrying a session id was accepted")
	}
}

// What the machine encodes is what the controller's reader gets back: a
// pointer body it recognises as a stage note, under the create's request id.
func TestLaunchProgressSurvivesTheWire(t *testing.T) {
	raw, err := Encode(Message{Version: Version, RequestID: "request-0123456789", Generation: 1, Body: LaunchProgress{Stage: StageStarting}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	note, ok := got.Body.(*LaunchProgress)
	if !ok || note.Stage != StageStarting || got.RequestID != "request-0123456789" || got.SessionID != "" {
		t.Fatalf("decoded %+v with body %#v", got, got.Body)
	}
}
