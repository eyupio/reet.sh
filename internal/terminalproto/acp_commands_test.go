package terminalproto

import (
	"fmt"
	"strings"
	"testing"
)

func TestCommandsCapabilityRequiresItsNegotiatedFeature(t *testing.T) {
	c := Capabilities{ProtocolVersion: 1, OS: "linux", OSUser: "dev", TerminalEnabled: true, TerminalControl: true, ACP: true, ACPLiveControls: true, ACPCommands: true, Readiness: Ready}
	live := ACPFeature + "," + ACPLiveControlsFeature
	if c.ValidateFor(live) == nil {
		t.Fatal("commands advertised on a connection that did not negotiate them")
	}
	if err := c.ValidateFor(live + "," + ACPCommandsFeature); err != nil {
		t.Fatal(err)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.ACPLiveControls = false
	if c.Validate() == nil {
		t.Fatal("commands admitted without live controls")
	}
	if !SupportsACPCommands("a,"+ACPCommandsFeature) || SupportsACPCommands("a,b") {
		t.Fatal("feature detection is wrong")
	}
}

func TestCommandEntriesAreBoundedText(t *testing.T) {
	good := ACPCommand{Name: "review:pr_2-x", Description: "Review a pull request", Hint: "number"}
	if !good.Valid() {
		t.Fatal("a plain command was refused")
	}
	for name, c := range map[string]ACPCommand{
		"empty name":        {},
		"leading slash":     {Name: "/review"},
		"space in name":     {Name: "re view"},
		"long name":         {Name: strings.Repeat("a", 65)},
		"long description":  {Name: "a", Description: strings.Repeat("d", MaxACPCommandDescriptionBytes+1)},
		"long hint":         {Name: "a", Hint: strings.Repeat("h", MaxACPCommandHintBytes+1)},
		"NUL description":   {Name: "a", Description: "x\x00y"},
		"invalid utf8 hint": {Name: "a", Hint: "\xff"},
	} {
		if c.Valid() {
			t.Fatalf("%s admitted", name)
		}
	}
	if !(ACPCommand{Name: strings.Repeat("a", 64), Description: strings.Repeat("d", MaxACPCommandDescriptionBytes), Hint: strings.Repeat("h", MaxACPCommandHintBytes)}).Valid() {
		t.Fatal("a command at every limit was refused")
	}
}

func TestCommandListsAreBoundedAndUnique(t *testing.T) {
	if !ValidACPCommands(nil) || !ValidACPCommands([]ACPCommand{}) {
		t.Fatal("an empty list was refused")
	}
	list := make([]ACPCommand, MaxACPCommands)
	for i := range list {
		list[i] = ACPCommand{Name: fmt.Sprintf("c%d", i)}
	}
	if !ValidACPCommands(list) {
		t.Fatal("a full list was refused")
	}
	if ValidACPCommands(append(list, ACPCommand{Name: "extra"})) {
		t.Fatal("a list over the count limit was admitted")
	}
	if ValidACPCommands([]ACPCommand{{Name: "a"}, {Name: "a"}}) {
		t.Fatal("duplicate names admitted")
	}
	if ValidACPCommands([]ACPCommand{{Name: "a"}, {Name: "/b"}}) {
		t.Fatal("an invalid entry admitted")
	}
	// Escaping can multiply text six-fold, so the list also has a byte budget.
	heavy := make([]ACPCommand, MaxACPCommands)
	for i := range heavy {
		heavy[i] = ACPCommand{Name: fmt.Sprintf("c%d", i), Description: strings.Repeat("<", MaxACPCommandDescriptionBytes), Hint: strings.Repeat("<", MaxACPCommandHintBytes)}
	}
	if ValidACPCommands(heavy) {
		t.Fatal("a list over the byte budget was admitted")
	}
}

func TestCommandsActionCarriesOnlyItsList(t *testing.T) {
	commands := []ACPCommand{{Name: "review", Description: "Review changes", Hint: "branch"}}
	for _, body := range []Body{ACPControl{Action: "commands"}, ACPControlResult{Controls: ACPControls{Options: []ACPSetting{}}, Commands: commands}} {
		m := Message{Version: Version, RequestID: "request-commands0123456789", SessionID: "session-commands0123456789", Generation: 1, Body: body}
		raw, err := Encode(m)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Decode(raw); err != nil {
			t.Fatal(err)
		}
	}
	if (ACPControl{Action: "commands", Text: "hidden"}).valid() || (ACPControl{Action: "commands", ConfigID: "effort"}).valid() {
		t.Fatal("commands carried mutation fields")
	}
	if (ACPControlResult{Commands: []ACPCommand{{Name: "/bad"}}}).valid() {
		t.Fatal("an invalid command list was admitted in a result")
	}
	raw, err := Encode(Message{Version: Version, RequestID: "request-commands0123456789", SessionID: "session-commands0123456789", Generation: 1, Body: ACPControlResult{Controls: ACPControls{Options: []ACPSetting{}}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"commands"`) {
		t.Fatalf("a result without commands names them: %s", raw)
	}
}
