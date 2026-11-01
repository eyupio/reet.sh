package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func chatCaps() terminalproto.Capabilities {
	caps := launchCaps("opencode")
	caps.ACP = true
	caps.ACPHistory = true
	caps.ACPImages = true
	caps.ACPApproval = true
	caps.ShellDir = true
	return caps
}

func TestChatCreatePromptAndSSE(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session, _ := out["id"].(string)
	if session == "" || out["structured"] != true {
		t.Fatalf("session: %v", out)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]string{"text": "hello"}); code != 200 {
		t.Fatalf("prompt: %d %s", code, body)
	}
	if err := f.peer.Receive(terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), SessionID: session, Generation: f.peer.Generation,
		Body: &terminalproto.ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "hi"}}); err != nil {
		t.Fatal(err)
	}
	if err := f.peer.Receive(terminalproto.Message{Version: 1, RequestID: terminalbroker.ID(), SessionID: session, Generation: f.peer.Generation,
		Body: &terminalproto.ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", Title: "run", Command: "echo ok"}}); err != nil {
		t.Fatal(err)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/permissions/permission-0123456789", map[string]bool{"allow": false}); code != 200 {
		t.Fatalf("deny: %d %s", code, body)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/permissions/permission-0123456789", map[string]any{"allow": true, "forSession": true}); code != 200 {
		t.Fatalf("allow for session: %d %s", code, body)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/approval-mode", map[string]string{"mode": "yolo"}); code != http.StatusForbidden {
		t.Fatalf("yolo without the account setting: %d %s", code, body)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/approval-mode", map[string]string{"mode": "ask"}); code != 200 {
		t.Fatalf("ask mode: %d %s", code, body)
	}
	if code, _, _ := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/approval-mode", map[string]string{"mode": "sometimes"}); code != http.StatusBadRequest {
		t.Fatalf("invalid mode: %d", code)
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/cancel", map[string]bool{}); code != 200 {
		t.Fatalf("cancel: %d %s", code, body)
	}
	server := httptest.NewServer(f.srv)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/api/v2/terminal/sessions/"+session+"/chat", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	resp, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("content type %q", resp.Header.Get("Content-Type"))
	}
	reader := bufio.NewReader(resp.Body)
	var text strings.Builder
	for i := 0; i < 40 && !strings.Contains(text.String(), `"kind":"agent"`); i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		text.WriteString(line)
	}
	if !strings.Contains(text.String(), "event: reset") || !strings.Contains(text.String(), `"kind":"user"`) || !strings.Contains(text.String(), `"kind":"agent"`) {
		t.Fatalf("stream:\n%s", text.String())
	}
}

func TestChatPromptImagesNeedTheMachineAndTheLimit(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session, _ := out["id"].(string)
	path := "/home/dev/launch-uploads/agent/shot.png"
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]any{"text": "", "images": []string{path}}); code != 200 {
		t.Fatalf("image-only prompt: %d %s", code, body)
	}
	f.mu.Lock()
	prompts := append([]terminalproto.ACPPrompt(nil), f.prompts...)
	f.mu.Unlock()
	if len(prompts) != 1 || len(prompts[0].Images) != 1 || prompts[0].Images[0] != path || prompts[0].Text != "" {
		t.Fatalf("prompt wire: %+v", prompts)
	}
	many := make([]string, terminalproto.MaxACPImages+1)
	for i := range many {
		many[i] = path
	}
	if code, _, _ := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]any{"text": "look", "images": many}); code != 400 {
		t.Fatalf("too many images: %d", code)
	}
	if code, _, _ := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]any{"text": "look", "images": []string{"relative.png"}}); code != 400 {
		t.Fatalf("relative image: %d", code)
	}

	oldCaps := chatCaps()
	oldCaps.ACPImages = false
	g := launchAPIFixture(t, oldCaps)
	code, out, body = g.create(t, launchBody(g.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 {
		t.Fatalf("create without acp-image-v1: %d %s", code, body)
	}
	session, _ = out["id"].(string)
	if code, _, body := g.do(t, g.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]any{"text": "", "images": []string{path}}); code != http.StatusConflict {
		t.Fatalf("image without acp-image-v1: %d %s", code, body)
	}
}

func TestChatCreateVersionAndTrustMapped(t *testing.T) {
	for name, code := range map[string]terminalproto.ResultCode{
		"version": terminalproto.ACPVersion,
		"trust":   terminalproto.ACPTrustRequired,
	} {
		t.Run(name, func(t *testing.T) {
			f := launchAPIFixture(t, chatCaps())
			f.reply = func(terminalproto.Create) terminalproto.Body {
				return terminalproto.Result{Code: code, Detail: "detail"}
			}
			status, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
				"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
			}))
			if status != http.StatusConflict {
				t.Fatalf("status %d %s", status, body)
			}
			want := "acp_version"
			if code == terminalproto.ACPTrustRequired {
				want = "acp_trust_required"
			}
			if out["code"] != want {
				t.Fatalf("code %v %s", out["code"], body)
			}
		})
	}
}

func TestChatPlainSessionIsNotFound(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), nil))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session, _ := out["id"].(string)
	for _, path := range []string{"/chat", "/chat/prompt", "/chat/cancel"} {
		method := "POST"
		if path == "/chat" {
			method = "GET"
		}
		code, _, _ := f.do(t, f.cookie, method, "/api/v2/terminal/sessions/"+session+path, map[string]string{"text": "hello"})
		if code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, code)
		}
	}
}

func TestChatResumeNeedsAClosedChatOwnedHere(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	id := "ses_fake000000000000000000000001"
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: id}
	}
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 || out["acpSessionId"] != id {
		t.Fatalf("create: %d %v %s", code, out, body)
	}
	session, _ := out["id"].(string)
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/end", map[string]string{"requestId": terminalbroker.ID()}); code != 200 {
		t.Fatalf("end: %d %s", code, body)
	}
	code, out, body = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "resumeAcp": id, "dir": "/tmp",
	}))
	if code != 200 || out["acpSessionId"] != id {
		t.Fatalf("resume: %d %v %s", code, out, body)
	}
	code, out, body = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "resumeAcp": "ses_nosuch000000000000000000001", "dir": "/tmp",
	}))
	if code != http.StatusNotFound || out["code"] != "acp_resume_not_found" {
		t.Fatalf("crafted: %d %v %s", code, out, body)
	}
}

func TestPlainTerminalInAFolderNeedsTheFeature(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("shell dir: %d %v %s", code, out, body)
	}
	if created := f.creates[len(f.creates)-1]; created.Dir != "/tmp" || created.Tool != "" {
		t.Fatalf("create %+v", created)
	}
	plain := launchAPIFixture(t, launchCaps("claude"))
	code, out, body = plain.create(t, launchBody(plain.agent.ID, terminalbroker.ID(), map[string]any{"dir": "/tmp"}))
	if code != http.StatusConflict || out["code"] != "shell_dir_unsupported" {
		t.Fatalf("old machine: %d %v %s", code, out, body)
	}
}

func TestChatModelSwitch(t *testing.T) {
	for _, supported := range []bool{true, false} {
		caps := chatCaps()
		caps.ACPModel = supported
		f := launchAPIFixture(t, caps)
		code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
		if code != 200 {
			t.Fatalf("create: %d %s", code, body)
		}
		session := out["id"].(string)
		path := "/api/v2/terminal/sessions/" + session + "/chat/model"
		code, _, body = f.do(t, f.cookie, "POST", path, map[string]string{"model": "provider/other"})
		if supported && code != 200 {
			t.Fatalf("switch: %d %s", code, body)
		}
		if !supported && (code != 409 || !strings.Contains(body, "Update reet")) {
			t.Fatalf("old machine: %d %s", code, body)
		}
		if code, _, body := f.do(t, f.cookie, "POST", path, map[string]string{"model": ""}); code != 400 {
			t.Fatalf("empty model: %d %s", code, body)
		}
	}
}

func TestChatDescriptorFeaturesOverrideMachineWideFlags(t *testing.T) {
	caps := chatCaps()
	caps.ACPModel = true
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	session := out["id"].(string)
	for _, tc := range []struct {
		path string
		body any
	}{
		{"prompt", map[string]any{"text": "look", "images": []string{"/home/dev/launch-uploads/shot.png"}}},
		{"model", map[string]string{"model": "provider/model"}},
		{"approval-mode", map[string]string{"mode": "ask"}},
	} {
		code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/"+tc.path, tc.body)
		if code != http.StatusConflict {
			t.Fatalf("%s ignored descriptor: %d %s", tc.path, code, body)
		}
	}
	if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]string{"text": "plain prompt"}); code != 200 {
		t.Fatalf("plain prompt: %d %s", code, body)
	}
}

const modeTestACPID = "ses_fake000000000000000000000001"

func modeCaps() terminalproto.Capabilities {
	caps := chatCaps()
	caps.SessionMode = true
	return caps
}

// modeChat starts an OpenCode chat that carries an OpenCode session id.
func modeChat(t *testing.T, f *launchAPI) string {
	t.Helper()
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: modeTestACPID}
	}
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{
		"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp",
	}))
	if code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	return out["id"].(string)
}

func modePath(session string) string { return "/api/v2/terminal/sessions/" + session + "/mode" }

func TestSessionModeEndpointSwitchesAndReturnsNoContent(t *testing.T) {
	f := launchAPIFixture(t, modeCaps())
	session := modeChat(t, f)
	code, _, body := f.do(t, f.cookie, "POST", modePath(session), map[string]any{"mode": "terminal"})
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("to terminal: %d %q", code, body)
	}
	row, err := f.st.TerminalSessionFor(f.uid, session)
	if err != nil || row.Structured {
		t.Fatalf("row after switch: %+v %v", row, err)
	}
	// A terminal-mode session is not a chat, and must still switch back.
	code, _, body = f.do(t, f.cookie, "POST", modePath(session), map[string]any{"mode": "chat", "trust": true})
	if code != http.StatusNoContent {
		t.Fatalf("to chat: %d %s", code, body)
	}
	row, err = f.st.TerminalSessionFor(f.uid, session)
	if err != nil || !row.Structured {
		t.Fatalf("row after switch back: %+v %v", row, err)
	}
	f.mu.Lock()
	modes := append([]terminalproto.SessionMode(nil), f.modes...)
	f.mu.Unlock()
	if len(modes) != 2 || modes[0].To != "terminal" || modes[1].To != "chat" || !modes[1].Trust {
		t.Fatalf("machine requests: %+v", modes)
	}
	if code, _, _ := f.do(t, f.cookie, "POST", modePath("nosuchsession0000000"), map[string]any{"mode": "chat"}); code != http.StatusNotFound {
		t.Fatalf("unknown session: %d", code)
	}
}

func TestSessionModeEndpointRejectsBadMode(t *testing.T) {
	f := launchAPIFixture(t, modeCaps())
	session := modeChat(t, f)
	for name, body := range map[string]string{
		"unknown mode": `{"mode":"split"}`,
		"empty mode":   `{"mode":""}`,
		"missing mode": `{}`,
		"extra field":  `{"mode":"terminal","force":true}`,
		"not json":     `mode=terminal`,
	} {
		if code, _, raw := f.do(t, f.cookie, "POST", modePath(session), body); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	f.mu.Lock()
	n := len(f.modes)
	f.mu.Unlock()
	if n != 0 {
		t.Fatalf("a bad request reached the machine: %d", n)
	}
}

func TestSessionModeEndpointMapsRefusals(t *testing.T) {
	cases := []struct {
		name   string
		caps   func(*terminalproto.Capabilities)
		plain  bool // start from the OpenCode TUI without a recorded session id
		toTerm bool // switch to the OpenCode TUI first, then ask for the target
		target string
		reply  terminalproto.Body
		status int
		code   string
		// prompt starts a chat turn first, so the controller itself knows the
		// chat is busy.
		prompt bool
		// wire is true when the refusal comes from the machine; every other
		// refusal is decided by the controller and must not reach it.
		wire bool
		// noID starts a chat whose OpenCode session id was never recorded.
		noID bool
		// yolo marks the row YOLO; the account does not allow YOLO.
		yolo bool
		// ends is true when the machine ended the session: the row closes.
		ends bool
	}{
		{name: "busy decided by the controller", prompt: true, target: "terminal", status: 409, code: "session_mode_busy"},
		{name: "busy", wire: true, target: "terminal", reply: terminalproto.Result{Code: terminalproto.Conflict}, status: 409, code: "session_mode_busy"},
		{name: "no session", plain: true, target: "chat", status: 409, code: "session_mode_no_session"},
		{name: "unsupported", caps: func(c *terminalproto.Capabilities) { c.SessionMode = false }, target: "terminal", status: 409, code: "session_mode_unsupported"},
		{name: "trust", wire: true, toTerm: true, target: "chat", reply: terminalproto.Result{Code: terminalproto.ACPTrustRequired, Detail: ".opencode/plugins/x.js"}, status: 409, code: "acp_trust_required"},
		{name: "failed", wire: true, target: "terminal", reply: terminalproto.Result{Code: terminalproto.LaunchFailed}, status: 502, code: "session_mode_failed"},
		{name: "ended", wire: true, ends: true, target: "terminal", reply: terminalproto.SessionChanged{State: terminalproto.Exited}, status: 409, code: "session_mode_ended"},
		{name: "no conversation", noID: true, target: "terminal", status: 409, code: "session_mode_no_conversation"},
		{name: "yolo not allowed", yolo: true, target: "terminal", status: 403, code: "yolo_not_allowed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := modeCaps()
			if c.caps != nil {
				c.caps(&caps)
			}
			f := launchAPIFixture(t, caps)
			var session string
			if c.plain {
				code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "dir": "/tmp"}))
				if code != 200 {
					t.Fatalf("create: %d %s", code, body)
				}
				session = out["id"].(string)
			} else if c.noID {
				code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
				if code != 200 || out["acpSessionId"] != nil && out["acpSessionId"] != "" {
					t.Fatalf("create: %d %s", code, body)
				}
				session = out["id"].(string)
			} else {
				session = modeChat(t, f)
			}
			if c.yolo {
				if err := f.st.SetTerminalYoloFor(f.uid, session, true); err != nil {
					t.Fatal(err)
				}
			}
			if c.toTerm {
				if code, _, body := f.do(t, f.cookie, "POST", modePath(session), map[string]any{"mode": "terminal"}); code != http.StatusNoContent {
					t.Fatalf("setup: %d %s", code, body)
				}
			}
			if c.prompt {
				if code, _, body := f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+session+"/chat/prompt", map[string]string{"text": "work"}); code != 200 {
					t.Fatalf("prompt: %d %s", code, body)
				}
			}
			before, err := f.st.TerminalSessionFor(f.uid, session)
			if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			sentBefore := len(f.modes)
			f.mu.Unlock()
			if c.reply != nil {
				reply := c.reply
				f.mu.Lock()
				f.mode = func(terminalproto.SessionMode) terminalproto.Body { return reply }
				f.mu.Unlock()
			}
			code, out, body := f.do(t, f.cookie, "POST", modePath(session), map[string]any{"mode": c.target})
			if code != c.status || out["code"] != c.code {
				t.Fatalf("got %d %s, want %d %s", code, body, c.status, c.code)
			}
			f.mu.Lock()
			sentAfter := len(f.modes)
			f.mu.Unlock()
			if c.wire && sentAfter != sentBefore+1 || !c.wire && sentAfter != sentBefore {
				t.Fatalf("machine saw %d session_mode requests, want wire=%v", sentAfter-sentBefore, c.wire)
			}
			after, err := f.st.TerminalSessionFor(f.uid, session)
			if err != nil || after.Structured != before.Structured || after.State != before.State && !c.ends || after.ACPSessionID != before.ACPSessionID {
				t.Fatalf("row changed by a refusal: %+v -> %+v (%v)", before, after, err)
			}
			if c.ends && after.State == terminalproto.Running {
				t.Fatalf("the machine ended the session but the row runs: %+v", after)
			}
			if c.code == "session_mode_ended" && out["error"] != "The session could not be switched and has ended. Start a new session or continue it with history." {
				t.Fatalf("ended sentence: %s", body)
			}
			if c.code == "session_mode_no_conversation" && out["error"] != "This session has no saved OpenCode conversation yet, so it cannot move to the OpenCode TUI." {
				t.Fatalf("no conversation sentence: %s", body)
			}
			if c.name == "trust" && out["detail"] != ".opencode/plugins/x.js" {
				t.Fatalf("trust detail: %s", body)
			}
		})
	}
}

// A switch to the mode the session is already in answers 204, asks the
// machine nothing and leaves the row as it was.
func TestSessionModeEndpointAlreadyInModeIsNoContent(t *testing.T) {
	f := launchAPIFixture(t, modeCaps())
	session := modeChat(t, f)
	before, err := f.st.TerminalSessionFor(f.uid, session)
	if err != nil {
		t.Fatal(err)
	}
	code, _, body := f.do(t, f.cookie, "POST", modePath(session), map[string]any{"mode": "chat"})
	if code != http.StatusNoContent || body != "" {
		t.Fatalf("already chat: %d %q", code, body)
	}
	f.mu.Lock()
	n := len(f.modes)
	f.mu.Unlock()
	after, err := f.st.TerminalSessionFor(f.uid, session)
	if n != 0 || err != nil || after.Structured != before.Structured || after.State != before.State || after.ACPSessionID != before.ACPSessionID || after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("a no-op switch reached the machine (%d) or changed the row %+v -> %+v (%v)", n, before, after, err)
	}
}

func TestSessionModeEndpointHidesOtherAccountsSession(t *testing.T) {
	f := launchAPIFixture(t, modeCaps())
	session := modeChat(t, f)
	member, err := f.st.CreateUser("mode-member@example.test", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other := terminalLogin(t, f.st, member.ID)
	// Real but foreign id, and an id nobody has: the same status and body.
	code, _, real := f.do(t, other, "POST", modePath(session), map[string]any{"mode": "terminal"})
	missing, _, unknown := f.do(t, other, "POST", modePath("nosuchsession0000000"), map[string]any{"mode": "terminal"})
	if code != http.StatusNotFound || missing != http.StatusNotFound || real != unknown {
		t.Fatalf("foreign id %d %q, unknown id %d %q", code, real, missing, unknown)
	}
	f.mu.Lock()
	n := len(f.modes)
	f.mu.Unlock()
	row, err := f.st.TerminalSessionFor(f.uid, session)
	if n != 0 || err != nil || !row.Structured {
		t.Fatalf("another account switched the session: %d %+v %v", n, row, err)
	}
}

func TestSessionModeEndpointRequiresWriter(t *testing.T) {
	f := launchAPIFixture(t, modeCaps())
	session := modeChat(t, f)
	viewer, err := f.st.CreateUser("mode-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		cookie string
		status int
	}{"viewer": {terminalLogin(t, f.st, viewer.ID), http.StatusForbidden}, "anonymous": {"", http.StatusUnauthorized}} {
		if code, _, raw := f.do(t, c.cookie, "POST", modePath(session), map[string]any{"mode": "terminal"}); code != c.status {
			t.Fatalf("%s: %d %s", name, code, raw)
		}
	}
	f.mu.Lock()
	n := len(f.modes)
	f.mu.Unlock()
	row, err := f.st.TerminalSessionFor(f.uid, session)
	if n != 0 || err != nil || !row.Structured {
		t.Fatalf("refused request switched the session: %d %+v %v", n, row, err)
	}
}

func TestChatAgentModeSwitch(t *testing.T) {
	for _, offered := range []bool{true, false} {
		caps := chatCaps()
		entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true, Approval: true}}
		if offered {
			caps.ACPAgentMode = true
			entries[0].Modes = []string{"read-only", "agent"}
		}
		caps.ACPAgents = &entries
		f := launchAPIFixture(t, caps)
		code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
		if code != 200 {
			t.Fatalf("create: %d %s", code, body)
		}
		path := "/api/v2/terminal/sessions/" + out["id"].(string) + "/chat/agent-mode"
		code, _, body = f.do(t, f.cookie, "POST", path, map[string]string{"mode": "agent"})
		if offered && (code != 200 || !strings.Contains(body, `"mode":"agent"`)) {
			t.Fatalf("switch: %d %s", code, body)
		}
		if !offered && code != 409 {
			t.Fatalf("a machine that offers no modes: %d %s", code, body)
		}
		if offered {
			if code, _, body := f.do(t, f.cookie, "POST", path, map[string]string{"mode": "agent-full-access"}); code != 409 {
				t.Fatalf("a mode the machine does not list: %d %s", code, body)
			}
		}
		for _, bad := range []string{"", "Agent Mode"} {
			if code, _, body := f.do(t, f.cookie, "POST", path, map[string]string{"mode": bad}); code != 400 {
				t.Fatalf("invalid mode %q: %d %s", bad, code, body)
			}
		}
	}
}
