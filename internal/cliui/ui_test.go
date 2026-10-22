package cliui

import (
	"bytes"
	"testing"
)

func TestRedirectedOutputHasNoBrandingOrAnimation(t *testing.T) {
	var out bytes.Buffer
	out.WriteString(`{"ok":true}`)
	Banner(&out)
	stop := Start(&out, "Checking updates")
	stop()
	stop()
	if out.String() != `{"ok":true}` {
		t.Fatalf("corrupted machine output: %q", out.String())
	}
}
