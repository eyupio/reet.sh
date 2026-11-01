package cliui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestRedirectedOutputHasNoBrandingOrAnimation(t *testing.T) {
	var out bytes.Buffer
	out.WriteString(`{"ok":true}`)
	Banner(&out)
	if err := Task(context.Background(), strings.NewReader(""), &out, "Checking updates", TaskOptions{}, func(context.Context, func(string)) error {
		_, err := io.WriteString(&out, "\n")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out.String() != "{\"ok\":true}\n" {
		t.Fatalf("corrupted machine output: %q", out.String())
	}
}

func TestColourAndMotionPreferencesAreIndependent(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("REET_NO_ANIMATION", "")
	t.Setenv("REET_REDUCED_MOTION", "")
	t.Setenv("REDUCE_MOTION", "")
	t.Setenv("PREFERS_REDUCED_MOTION", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("CI", "")
	if Colour() || !Motion() {
		t.Fatal("NO_COLOR must include the empty value, independently of motion")
	}
	if Brand(Colour()) != "reet.sh" {
		t.Fatal("plain branding changed")
	}
	for key, value := range map[string]string{"REET_NO_ANIMATION": "1", "REET_REDUCED_MOTION": "1", "REDUCE_MOTION": "1", "PREFERS_REDUCED_MOTION": "reduce", "CI": "true", "TERM": "dumb"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if Motion() {
				t.Fatal("preference ignored")
			}
		})
	}
}
