package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnapshotAttentionFieldsOnlyWhenNegotiated(t *testing.T) {
	exit := 2
	snap := SessionSnapshot{
		ID:         "session_1234567890",
		Title:      "Refactor the store layer",
		State:      Exited,
		ExitCode:   &exit,
		ActivityAt: 1793376442000,
		BellAt:     1793376450000,
		ExitStatus: &exit,
	}

	without := snap.ForFeatures("command-history-v1,launch-v1")
	rawWithout, err := json.Marshal(without)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"activityAt", "bellAt", "exitStatus"} {
		if strings.Contains(string(rawWithout), field) {
			t.Fatalf("unnegotiated snapshot contained %s: %s", field, rawWithout)
		}
	}

	with := snap.ForFeatures("command-history-v1, attention-v1")
	msg := Message{
		Version:    Version,
		RequestID:  "req_1234567890123456",
		Generation: 1,
		Body:       Reconcile{Complete: true, Sessions: []SessionSnapshot{with}},
	}
	encoded, err := Encode(msg)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"activityAt", "bellAt", "exitStatus"} {
		if !strings.Contains(string(encoded), field) {
			t.Fatalf("negotiated snapshot missing %s: %s", field, encoded)
		}
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reconcile := decoded.Body.(*Reconcile)
	got := reconcile.Sessions[0]
	if got.ActivityAt != snap.ActivityAt || got.BellAt != snap.BellAt || got.ExitStatus == nil || *got.ExitStatus != exit {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Negative timestamps or mismatched ExitStatus/ExitCode are rejected.
	bad := with
	bad.ActivityAt = -1
	if (Reconcile{Complete: true, Sessions: []SessionSnapshot{bad}}).valid() {
		t.Fatal("negative activityAt accepted")
	}
	other := 1
	bad = with
	bad.ExitStatus = &other
	if (Reconcile{Complete: true, Sessions: []SessionSnapshot{bad}}).valid() {
		t.Fatal("mismatched exitStatus accepted")
	}
}
