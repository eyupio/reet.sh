//go:build linux || darwin

package terminalagent

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A connection that did not negotiate acp-model-catalogue-v1 keeps the 16
// values a live control reply was sized for, and the session's own snapshot is
// left whole for the next connection that did.
func TestModelCatalogueIsTrimmedPerConnection(t *testing.T) {
	values := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		values = append(values, "provider/model-"+string(rune('a'+i%26))+"-"+string(rune('a'+i/26)))
	}
	state := terminalproto.ACPControls{Model: values[0], Options: []terminalproto.ACPSetting{
		{ID: "model", Category: "model", Current: values[0], Values: values},
		{ID: "effort", Category: "thought_level", Current: "low", Values: []string{"low", "high"}},
	}}
	legacy := &chatHost{link: &chatLink{}}
	trimmed := legacy.forConnection(state)
	if got := len(trimmed.Options[0].Values); got != terminalproto.MaxACPThoughtLevelValues {
		t.Fatalf("legacy model values: %d", got)
	}
	if got := len(trimmed.Options[1].Values); got != 2 {
		t.Fatalf("effort values changed: %d", got)
	}
	if got := len(state.Options[0].Values); got != len(values) {
		t.Fatalf("session snapshot was mutated: %d", got)
	}
	catalogue := &chatHost{link: &chatLink{catalogue: true}}
	if got := len(catalogue.forConnection(state).Options[0].Values); got != len(values) {
		t.Fatalf("catalogue model values: %d", got)
	}
}

// A connection that did not negotiate acp-provider-v1 never sees the provider
// setting, whether or not it reads the model catalogue; the session's own
// snapshot keeps it for the next connection that did.
func TestProviderSettingIsStrippedForAPeerWithoutTheFeature(t *testing.T) {
	state := terminalproto.ACPControls{Model: "stub/model", Options: []terminalproto.ACPSetting{
		{ID: "provider", Category: "provider", Current: "cline", Values: []string{"cline", "cline-pass"}},
		{ID: "model", Category: "model", Current: "stub/model", Values: []string{"stub/model", "provider/other"}},
	}}
	for _, link := range []*chatLink{{}, {catalogue: true}} {
		got := (&chatHost{link: link}).forConnection(state)
		if len(got.Options) != 1 || got.Options[0].Category != "model" {
			t.Fatalf("link %+v kept the provider: %+v", link, got.Options)
		}
	}
	if len(state.Options) != 2 {
		t.Fatalf("session snapshot was mutated: %+v", state.Options)
	}
	both := (&chatHost{link: &chatLink{provider: true, catalogue: true}}).forConnection(state)
	if len(both.Options) != 2 || both.Options[0].Category != "provider" {
		t.Fatalf("a peer with the feature lost the provider: %+v", both.Options)
	}
}
