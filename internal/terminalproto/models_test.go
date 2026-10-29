package terminalproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func validModels(n int) []ModelEntry {
	out := make([]ModelEntry, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, ModelEntry{ID: fmt.Sprintf("provider/model-%03d", i), Provider: "provider"})
	}
	return out
}

func TestModelsFeatureToken(t *testing.T) {
	for _, value := range []string{"", "models-v10", "models-v1x", "prefix-models-v1", "Models-v1"} {
		if SupportsModels(value) {
			t.Fatal("unadvertised extension enabled", value)
		}
	}
	if !SupportsModels("browse-v1, models-v1") || !SupportsModels(" models-v1 ") || ModelsFeature != "models-v1" {
		t.Fatal("feature not recognized")
	}
}

func TestModelsRequestRoundTrip(t *testing.T) {
	m := testMessage(Models{Tool: "opencode"})
	encoded, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.Body.(*Models)
	if !ok || got.Tool != "opencode" {
		t.Fatalf("body %#v", decoded.Body)
	}
	again, err := Encode(decoded)
	if err != nil || !bytes.Equal(encoded, again) {
		t.Fatal("round trip changed", err)
	}
	for _, tool := range []string{"", "pi", "cursor", "unknown"} {
		if (Models{Tool: tool}).valid() {
			t.Fatal("invalid tool accepted", tool)
		}
	}
}

func TestModelsResultRoundTripAndLimits(t *testing.T) {
	r := ModelsResult{Tool: "opencode", Models: []ModelEntry{{ID: "anthropic/claude-sonnet-4-5", Provider: "anthropic"}, {ID: "orphan", Provider: ""}}, Version: "1.19.5"}
	m := testMessage(r)
	encoded, err := Encode(m)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.Body.(*ModelsResult)
	if !ok || got.Tool != "opencode" || len(got.Models) != 2 || got.Version != "1.19.5" {
		t.Fatalf("body %#v", decoded.Body)
	}
	cases := map[string]ModelsResult{
		"unknown reason":         {Tool: "opencode", Reason: "nonsense"},
		"reason with models":     {Tool: "opencode", Reason: ModelsUnsupported, Models: []ModelEntry{{ID: "a/b", Provider: "a"}}},
		"reason with version":    {Tool: "opencode", Reason: ModelsTimedOut, Version: "1"},
		"duplicate id":           {Tool: "opencode", Models: []ModelEntry{{ID: "a/b", Provider: "a"}, {ID: "a/b", Provider: "a"}}},
		"invalid id":             {Tool: "opencode", Models: []ModelEntry{{ID: "-bad", Provider: ""}}},
		"provider mismatch":      {Tool: "opencode", Models: []ModelEntry{{ID: "a/b", Provider: "b"}}},
		"provider without slash": {Tool: "opencode", Models: []ModelEntry{{ID: "orphan", Provider: "orphan"}}},
		"version too long":       {Tool: "opencode", Version: strings.Repeat("v", 65)},
		"too many models":        {Tool: "opencode", Models: validModels(501)},
		"empty tool":             {Tool: "", Models: []ModelEntry{{ID: "a/b", Provider: "a"}}},
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if err := (Message{Version: Version, RequestID: testID, Generation: 1, Body: result}).Validate(); err == nil {
				t.Fatal("invalid result accepted")
			}
			if name == "too many models" {
				return
			}
			if _, err := Encode(testMessage(result)); err == nil {
				t.Fatal("invalid result encoded")
			}
		})
	}
	accepted := []ModelsResult{
		{Tool: "opencode", Models: []ModelEntry{}},
		{Tool: "opencode", Models: []ModelEntry{{ID: "a/b", Provider: "a"}}, More: true},
		{Tool: "opencode", Reason: ModelsUnsupported},
		{Tool: "opencode", Reason: ModelsOff},
	}
	for _, result := range accepted {
		if _, err := Encode(testMessage(result)); err != nil {
			t.Fatalf("valid result refused: %+v %v", result, err)
		}
	}
}

func TestModelsResultRejectsAnOversizeAcceptedList(t *testing.T) {
	entries := make([]ModelEntry, 0, MaxModels)
	for i := 0; i < MaxModels; i++ {
		id := fmt.Sprintf("provider-%03d/%s", i, strings.Repeat("x", 60))
		entries = append(entries, ModelEntry{ID: id, Provider: fmt.Sprintf("provider-%03d", i)})
	}
	result := ModelsResult{Tool: "opencode", Models: entries}
	if (Message{Version: Version, RequestID: testID, Generation: 1, Body: result}).Validate() == nil {
		t.Fatal("an encoded result over MaxModelsReplyBytes was accepted")
	}
}

func TestModelsCapabilities(t *testing.T) {
	base := Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "u", TerminalEnabled: true, HealthEnabled: true, Readiness: Ready, TerminalControl: true}
	base.Models = true
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.ModelsOff = true
	if err := base.Validate(); err == nil {
		t.Fatal("models and modelsOff together accepted")
	}
	base.Models, base.ModelsOff = false, true
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	base.TerminalControl, base.ModelsOff = false, true
	if err := base.Validate(); err == nil {
		t.Fatal("modelsOff without terminal control accepted")
	}
	legacy, _ := json.Marshal(Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "u", TerminalEnabled: true, HealthEnabled: true, Readiness: Ready})
	if bytes.Contains(legacy, []byte(`"models"`)) || bytes.Contains(legacy, []byte(`"modelsOff"`)) {
		t.Fatal("models fields in legacy JSON", string(legacy))
	}
}

func TestModelsStrictnessAndBinding(t *testing.T) {
	encoded, err := Encode(testMessage(Models{Tool: "opencode"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(bytes.Replace(encoded, []byte(`"tool"`), []byte(`"extra":1,"tool"`), 1)); err == nil {
		t.Fatal("unknown request field accepted")
	}
	bound := testMessage(Models{Tool: "opencode"})
	bound.SessionID = testID
	if bound.Validate() == nil {
		t.Fatal("models request with a session id accepted")
	}
	result, err := Encode(testMessage(ModelsResult{Tool: "opencode", Reason: ModelsUnsupported}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(bytes.Replace(result, []byte(`"reason":"unsupported"`), []byte(`"reason":"other"`), 1)); err == nil {
		t.Fatal("unknown reason decoded")
	}
}
