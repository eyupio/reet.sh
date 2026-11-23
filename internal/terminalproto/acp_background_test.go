package terminalproto

import (
	"strings"
	"testing"
)

func encodedStatus(t *testing.T, event ACPEvent) []byte {
	t.Helper()
	b, err := Encode(Message{Version: Version, RequestID: "request-0000000001", SessionID: "session-000000001", Generation: 1, Body: &event})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return b
}

func TestACPBackgroundOnlyOnIdleStatus(t *testing.T) {
	cases := []struct {
		event ACPEvent
		ok    bool
	}{
		{ACPEvent{Kind: "status", State: "idle", Background: 2}, true},
		{ACPEvent{Kind: "status", State: "idle"}, true},
		{ACPEvent{Kind: "status", State: "working", Background: 1}, false},
		{ACPEvent{Kind: "tool", ToolCallID: "t1", Status: "in_progress", ToolKind: "execute", Background: 1}, false},
		{ACPEvent{Kind: "status", State: "idle", Background: -1}, false},
		{ACPEvent{Kind: "status", State: "idle", Background: MaxACPBackground + 1}, false},
	}
	for i, c := range cases {
		if got := c.event.valid(); got != c.ok {
			t.Errorf("case %d: valid = %v, want %v", i, got, c.ok)
		}
	}
}

func TestACPBackgroundDecodesAndOldShapeStillDecodes(t *testing.T) {
	m, err := Decode(encodedStatus(t, ACPEvent{Kind: "status", State: "idle"}))
	if err != nil {
		t.Fatalf("old status shape: %v", err)
	}
	if e := m.Body.(*ACPEvent); e.Background != 0 {
		t.Fatalf("old shape carried background %d", e.Background)
	}
	withBackground := encodedStatus(t, ACPEvent{Kind: "status", State: "idle", Background: 3})
	if !strings.Contains(string(withBackground), `"background":3`) {
		t.Fatalf("encoded event lacks the count: %s", withBackground)
	}
	m, err = Decode(withBackground)
	if err != nil {
		t.Fatalf("background status: %v", err)
	}
	if e := m.Body.(*ACPEvent); e.Background != 3 {
		t.Fatalf("background = %d, want 3", e.Background)
	}
	working := strings.Replace(string(withBackground), `"state":"idle"`, `"state":"working"`, 1)
	if _, err := Decode([]byte(working)); err == nil {
		t.Fatal("background on a working status was accepted")
	}
}

func TestACPControlsBackgroundBound(t *testing.T) {
	if !(ACPControls{Background: 1, Options: []ACPSetting{}}).Valid() {
		t.Fatal("controls with background work were refused")
	}
	if (ACPControls{Background: -1}).Valid() || (ACPControls{Background: MaxACPBackground + 1}).Valid() {
		t.Fatal("out-of-range background count was accepted")
	}
}

func TestACPBackgroundFeatureNamed(t *testing.T) {
	if !ParseFeatures("x," + ACPBackgroundFeature).Has(ACPBackgroundFeature) {
		t.Fatal("feature not parsed")
	}
	if ACPBackgroundFeature != "acp-background-v1" || BackgroundHoldLimit.Minutes() != 30 {
		t.Fatal("feature name or hold limit changed")
	}
}

func TestACPUnwatchedCodeOnlyOnIdleStatus(t *testing.T) {
	if !(ACPEvent{Kind: "status", State: "idle", Code: ACPCodeUnwatched}).valid() {
		t.Fatal("an idle status that stopped waiting was refused")
	}
	for _, e := range []ACPEvent{{Kind: "status", State: "working", Code: ACPCodeUnwatched}, {Kind: "notice", Text: "x", Code: ACPCodeUnwatched}} {
		if e.valid() {
			t.Fatalf("accepted %+v", e)
		}
	}
	if _, err := Decode(encodedStatus(t, ACPEvent{Kind: "status", State: "idle", Code: ACPCodeUnwatched})); err != nil {
		t.Fatal(err)
	}
}
