package api

import (
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The folder a desk-shell-v1 machine reports reaches the session listing as
// liveDir, which is absent until there is one; the machine listing says the
// machine reports folders.
func TestSessionListCarriesLiveDir(t *testing.T) {
	caps := launchCaps()
	caps.DeskShell = true
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), nil))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session, _ := out["id"].(string)

	_, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
	if strings.Contains(raw, `"liveDir"`) {
		t.Fatalf("liveDir before any report: %s", raw)
	}
	if err := f.peer.Receive(terminalproto.Message{Version: terminalproto.Version, RequestID: terminalbroker.ID(), SessionID: session, Generation: f.peer.Generation,
		Body: &terminalproto.SessionChanged{State: terminalproto.Running, Dir: `/home/tester/it's "here"`}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, _, raw = f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions", nil)
		if strings.Contains(raw, `"liveDir":"/home/tester/it's \"here\""`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("liveDir not listed: %s", raw)
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, _, machines := f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if !strings.Contains(machines, `"deskShell":true`) {
		t.Fatalf("machine listing lacks deskShell: %s", machines)
	}
}
