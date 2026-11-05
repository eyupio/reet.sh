package api

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

// A session started without a title is named by the controller, so the API, the
// browser and project runs all call the same work the same thing, and a caller
// that leaves the title out is no longer refused.
func TestTerminalCreateWithoutATitleIsNamedAutomatically(t *testing.T) {
	caps := launchCaps("claude")
	caps.ShellDir = true // a plain shell may start in a folder
	f := launchAPIFixture(t, caps)
	for _, c := range []struct {
		name, want string
		body       map[string]any
	}{
		{"agent and folder", "Claude Code · app", map[string]any{"tool": "claude", "dir": "/home/tester/app"}},
		{"the prompt names the work", "Fix the flaky login test and explain why", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": "Please fix the flaky login test and explain why"}},
		{"a blank title is no title", "Claude Code · app", map[string]any{"tool": "claude", "dir": "/home/tester/app", "title": "   "}},
		{"a typed title is kept as typed", "release prep", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": "Please fix the flaky login test and explain why", "title": "release prep"}},
		{"a plain shell", "Terminal", map[string]any{}},
		{"a plain shell in a folder", "Terminal · api", map[string]any{"dir": "/srv/api"}},
	} {
		body := map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "cols": 80, "rows": 24}
		for k, v := range c.body {
			body[k] = v
		}
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
		if code != 200 || out["title"] != c.want {
			t.Fatalf("%s: %d %s, want the title %q", c.name, code, raw, c.want)
		}
		sent := f.sent()
		if got := sent[len(sent)-1].Title; got != c.want {
			t.Fatalf("%s: the machine was sent the title %q, want %q", c.name, got, c.want)
		}
	}
}

// The name is a pure function of the request, so a retry whose reply was lost
// finds the session it already started instead of starting a second one.
func TestTerminalCreateAutomaticNameSurvivesAReplay(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	body := map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "cols": 80, "rows": 24, "tool": "claude", "dir": "/home/tester/app", "prompt": "Hi, can you please fix the login redirect loop? It started after the deploy."}
	code, first, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
	if code != 200 || first["title"] != "Fix the login redirect loop" {
		t.Fatalf("first: %d %s", code, raw)
	}
	code, again, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
	if code != 200 || again["id"] != first["id"] || again["title"] != first["title"] {
		t.Fatalf("replay: %d %s, want the session %v", code, raw, first["id"])
	}
	if n := len(f.sent()); n != 1 {
		t.Fatalf("the machine was asked %d times, want once", n)
	}
}
