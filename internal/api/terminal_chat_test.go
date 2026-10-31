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
