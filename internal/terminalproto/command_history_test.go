package terminalproto

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCommandHistoryExtensionPreservesStrictV1AndLegacyCapabilityJSON(t *testing.T) {
	for _, value := range []string{"", "other", "command-history-v10", "prefix-command-history-v1"} {
		if SupportsCommandHistory(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsCommandHistory("other, command-history-v1") {
		t.Fatal("feature not recognized")
	}
	legacy, _ := json.Marshal(testCapabilities())
	if bytes.Contains(legacy, []byte("commandHistory")) {
		t.Fatal("new field sent to old server", string(legacy))
	}
	message := testMessage(History{Lines: 1})
	message.Body = CommandHistory{StreamToken: testID}
	encoded, err := Encode(message)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil || decoded.Version != 1 || decoded.Body.kind() != "command_history" {
		t.Fatal(decoded, err)
	}
	invalid := bytes.Replace(encoded, []byte(`"streamToken":`), []byte(`"path":"/arbitrary","streamToken":`), 1)
	if _, err = Decode(invalid); err == nil {
		t.Fatal("history accepted arbitrary paths")
	}
}

func TestCommandHistorySnapshotRejectsAmbiguousOrExecutableMetadata(t *testing.T) {
	good := CommandHistorySnapshot{Entries: []CommandHistoryEntry{{Command: "df -h", Source: "bash"}}, Sources: []CommandHistorySource{{Source: "bash", Available: true}}}
	encoded, err := EncodeCommandHistory(good)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeCommandHistory(encoded); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{[]byte(`{}`), []byte(`{"entries":null,"sources":[]}`), bytes.Replace(encoded, []byte(`"command":"df -h"`), []byte(`"command":"df -h","command":"other"`), 1), bytes.Replace(encoded, []byte(`"command":"df -h"`), []byte(`"command":"df -h","execute":true`), 1), bytes.Replace(encoded, []byte(`"available":true`), []byte(`"available":false`), 1)} {
		if _, err = DecodeCommandHistory(invalid); err == nil {
			t.Fatal("invalid snapshot accepted", string(invalid))
		}
	}
}

// A desk pane's own history is a third source, "pane"; one marked unavailable
// carries a reason and no entries.
func TestCommandHistoryPaneSource(t *testing.T) {
	good := CommandHistorySnapshot{
		Entries: []CommandHistoryEntry{{Command: "make", Source: PaneHistorySource}, {Command: "ls", Source: "bash"}},
		Sources: []CommandHistorySource{{Source: PaneHistorySource, Available: true}, {Source: "bash", Available: true}, {Source: "zsh", Reason: "No saved history file yet."}},
	}
	if _, err := EncodeCommandHistory(good); err != nil {
		t.Fatal(err)
	}
	none := CommandHistorySnapshot{Entries: []CommandHistoryEntry{}, Sources: []CommandHistorySource{{Source: PaneHistorySource, Reason: "This pane's own history needs bash or zsh."}}}
	if _, err := EncodeCommandHistory(none); err != nil {
		t.Fatal(err)
	}
	unavailable := none
	unavailable.Entries = []CommandHistoryEntry{{Command: "make", Source: PaneHistorySource}}
	if _, err := EncodeCommandHistory(unavailable); err == nil {
		t.Fatal("entries accepted from an unavailable pane source")
	}
	twice := good
	twice.Sources = append(append([]CommandHistorySource{}, good.Sources...), CommandHistorySource{Source: PaneHistorySource})
	if _, err := EncodeCommandHistory(twice); err == nil {
		t.Fatal("a fourth, repeated source accepted")
	}
}
