package main

import (
	"bytes"
	"github.com/eyupio/reet.sh/internal/terminalcontrol"
	"os"
	"testing"
)

func TestGuidedSetupPromptsValidatesAndRemembers(t *testing.T) {
	answers := []string{"http://not-private.example", "https://reet.example", "yes"}
	calls := 0
	ask := func(string, string) (string, error) { answer := answers[calls]; calls++; return answer, nil }
	origin, auth, err := prepareTerminalSetup(terminalcontrol.Status{AuthMode: "token"}, "", "", ask, &bytes.Buffer{})
	if err != nil || calls != 3 || origin != "https://reet.example" || auth != "accounts" {
		t.Fatal(origin, auth, calls, err)
	}
	dir, err := os.MkdirTemp("/tmp", "reet-guide-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	saved, err := terminalcontrol.Command(t.Context(), dir, "start", false, origin, auth)
	if err != nil {
		t.Fatal(err)
	}
	calls = 0
	_, _, err = prepareTerminalSetup(saved, "", "", func(string, string) (string, error) { calls++; return "", nil }, &bytes.Buffer{})
	if err != nil || calls != 0 {
		t.Fatal("saved answers were requested again", err, calls)
	}
	next, err := terminalcontrol.Open(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.AuthMode() != "accounts" || next.PublicURL() != origin {
		t.Fatal("setup was not persisted for serve")
	}
}
func TestGuidedSetupCancellationAndNonInteractiveFailure(t *testing.T) {
	if _, _, err := prepareTerminalSetup(terminalcontrol.Status{}, "", "", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("missing URL silently accepted")
	}
	if _, _, err := prepareTerminalSetup(terminalcontrol.Status{AuthMode: "none"}, "https://reet.example", "", func(string, string) (string, error) { return "no", nil }, &bytes.Buffer{}); err == nil {
		t.Fatal("authentication change did not require consent")
	}
	calls := 0
	_, _, err := prepareTerminalSetup(terminalcontrol.Status{PublicURL: "https://reet.example", AuthMode: "accounts"}, "", "", func(string, string) (string, error) { calls++; return "", nil }, &bytes.Buffer{})
	if err != nil || calls != 0 {
		t.Fatal("working controller prompted unnecessarily", err)
	}
}
