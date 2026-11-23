//go:build linux || darwin

package acpagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func announce(s *Session, commands string) {
	s.handleUpdate(json.RawMessage(`{"sessionId":"s","update":{"sessionUpdate":"available_commands_update","availableCommands":` + commands + `}}`))
}

func TestAnnouncedCommandsAreKeptAndReplaced(t *testing.T) {
	s := &Session{}
	if got := s.Commands(); got == nil || len(got) != 0 {
		t.Fatalf("a new session has commands %#v", got)
	}
	announce(s, `[{"name":"review","description":"Review changes","input":{"hint":"branch"}},{"name":"init","description":"Write a guide"}]`)
	want := []terminalproto.ACPCommand{{Name: "review", Description: "Review changes", Hint: "branch"}, {Name: "init", Description: "Write a guide"}}
	if got := s.Commands(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %#v", got)
	}
	announce(s, `[{"name":"compact","description":"Shorten the context"}]`)
	if got := s.Commands(); len(got) != 1 || got[0].Name != "compact" {
		t.Fatalf("a later announcement did not replace the list: %#v", got)
	}
	got := s.Commands()
	got[0].Name = "changed"
	if s.Commands()[0].Name != "compact" {
		t.Fatal("a caller could change the session's list")
	}
	announce(s, `[]`)
	if got := s.Commands(); got == nil || len(got) != 0 {
		t.Fatalf("an empty announcement did not clear the list: %#v", got)
	}
	announce(s, `[{"name":"a"}]`)
	announce(s, `"not a list"`)
	if got := s.Commands(); len(got) != 0 {
		t.Fatalf("a malformed announcement kept the old list: %#v", got)
	}
}

func TestAnnouncedCommandsDropInvalidEntries(t *testing.T) {
	s := &Session{}
	long := strings.Repeat("x", terminalproto.MaxACPCommandDescriptionBytes+1)
	longHint := strings.Repeat("h", terminalproto.MaxACPCommandHintBytes+1)
	announce(s, `[
		{"name":"/slash"},
		{"name":""},
		{"name":"has space"},
		{"name":"`+strings.Repeat("n", 65)+`"},
		{"name":"long","description":"`+long+`"},
		{"name":"hint","input":{"hint":"`+longHint+`"}},
		{"name":"nul","description":"a\u0000b"},
		{"name":42},
		{"name":"ok:one_2-x","description":"kept"},
		{"name":"ok:one_2-x","description":"second copy"}
	]`)
	got := s.Commands()
	if len(got) != 1 || got[0].Description != "kept" {
		t.Fatalf("got %#v", got)
	}
}

func TestAnnouncedCommandsAreCapped(t *testing.T) {
	s := &Session{}
	entries := make([]string, 0, 70)
	for i := range 70 {
		entries = append(entries, fmt.Sprintf(`{"name":"c%d"}`, i))
	}
	announce(s, "["+strings.Join(entries, ",")+"]")
	got := s.Commands()
	if len(got) != terminalproto.MaxACPCommands || got[0].Name != "c0" || got[63].Name != "c63" {
		t.Fatalf("kept %d commands", len(got))
	}
	entries = entries[:0]
	for i := range 64 {
		entries = append(entries, fmt.Sprintf(`{"name":"c%d","description":"%s","input":{"hint":"%s"}}`, i, strings.Repeat("<", 300), strings.Repeat("<", 120)))
	}
	announce(s, "["+strings.Join(entries, ",")+"]")
	got = s.Commands()
	if len(got) == 0 || len(got) == 64 || !terminalproto.ValidACPCommands(got) {
		t.Fatalf("an escaped list was not trimmed to its byte budget: %d", len(got))
	}
}
