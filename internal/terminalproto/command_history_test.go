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
