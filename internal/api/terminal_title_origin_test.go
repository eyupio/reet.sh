package api

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Create settles who owns the title before the session is reserved, so a
// generic fallback name can later be replaced once and a typed name never is.
func TestCreateStoresResolvedTitleOrigin(t *testing.T) {
	const task = "Please fix the flaky login test and explain why"
	const taskTitle = "Fix the flaky login test and explain why"
	for _, c := range []struct {
		name  string
		body  map[string]any
		title string
		want  terminalproto.TitleOrigin
	}{
		{"blank title and prompt", map[string]any{"tool": "claude", "dir": "/home/tester/app"}, "Claude Code · app", terminalproto.TitleFallback},
		{"blank title and a task", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": task}, taskTitle, terminalproto.TitlePrompt},
		{"a greeting yields no title", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": "hey there!"}, "Claude Code · app", terminalproto.TitleFallback},
		{"image-only prompt keeps the fallback claim", map[string]any{"tool": "claude", "dir": "/home/dev/site", "prompt": "Image: /tmp/x.png", "title": "Claude Code · site", "titleOrigin": "fallback"}, "Claude Code · site", terminalproto.TitleFallback},
		{"quoted image-only prompt keeps the fallback claim", map[string]any{"tool": "claude", "dir": "/home/dev/site", "prompt": "Image: '/tmp/My Dir/x.png'", "title": "Claude Code · site", "titleOrigin": "fallback"}, "Claude Code · site", terminalproto.TitleFallback},
		{"escaped-apostrophe image-only prompt keeps the fallback claim", map[string]any{"tool": "claude", "dir": "/home/dev/site", "prompt": "Image: '/home/o'\\''neil/x.png'", "title": "Claude Code · site", "titleOrigin": "fallback"}, "Claude Code · site", terminalproto.TitleFallback},
		{"task then image line is a prompt title", map[string]any{"tool": "claude", "dir": "/home/dev/site", "prompt": "Fix the login redirect loop\n\nImage: /tmp/x.png", "title": "Fix the login redirect loop", "titleOrigin": "prompt"}, "Fix the login redirect loop", terminalproto.TitlePrompt},
		{"auto title claimed as fallback", map[string]any{"tool": "claude", "dir": "/home/tester/app", "title": "Claude Code · app", "titleOrigin": "fallback"}, "Claude Code · app", terminalproto.TitleFallback},
		{"auto title claimed as prompt", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": task, "title": taskTitle, "titleOrigin": "prompt"}, taskTitle, terminalproto.TitlePrompt},
		{"claim that does not match the prompt", map[string]any{"tool": "claude", "dir": "/home/tester/app", "prompt": task, "title": taskTitle, "titleOrigin": "fallback"}, taskTitle, terminalproto.TitleManual},
		{"claim on a typed title", map[string]any{"tool": "claude", "dir": "/home/tester/app", "title": "release prep", "titleOrigin": "prompt"}, "release prep", terminalproto.TitleManual},
		{"typed title without a claim", map[string]any{"tool": "claude", "dir": "/home/tester/app", "title": "release prep"}, "release prep", terminalproto.TitleManual},
		{"unknown claim is not an error", map[string]any{"tool": "claude", "dir": "/home/tester/app", "title": "Claude Code · app", "titleOrigin": "bogus"}, "Claude Code · app", terminalproto.TitleManual},
	} {
		// A fresh account each time: an account may hold only so many running sessions.
		f := launchAPIFixture(t, launchCaps("claude"))
		body := map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "cols": 80, "rows": 24}
		for k, v := range c.body {
			body[k] = v
		}
		code, out, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
		if code != 200 || out["title"] != c.title {
			t.Fatalf("%s: %d %s, want the title %q", c.name, code, raw, c.title)
		}
		got, err := f.st.TerminalTitleOriginFor(f.uid, out["id"].(string))
		if err != nil || got != c.want {
			t.Fatalf("%s: stored origin %q (%v), want %q", c.name, got, err, c.want)
		}
	}
}

// A retry whose reply was lost resolves the same title and origin, so it finds
// the session it already started.
func TestCreateTitleOriginSurvivesAReplay(t *testing.T) {
	f := launchAPIFixture(t, launchCaps("claude"))
	body := map[string]any{"agentId": f.agent.ID, "requestId": terminalbroker.ID(), "cols": 80, "rows": 24, "tool": "claude", "dir": "/home/tester/app", "titleOrigin": "fallback", "title": "Claude Code · app"}
	code, first, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
	if code != 200 {
		t.Fatalf("first: %d %s", code, raw)
	}
	code, again, raw := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions", body)
	if code != 200 || again["id"] != first["id"] || again["title"] != first["title"] {
		t.Fatalf("replay: %d %s, want the session %v", code, raw, first["id"])
	}
	got, err := f.st.TerminalTitleOriginFor(f.uid, again["id"].(string))
	if err != nil || got != terminalproto.TitleFallback {
		t.Fatalf("replayed origin %q (%v), want fallback", got, err)
	}
}
