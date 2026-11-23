package api

import (
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestChatRuntimeControlsAndDurableQueue(t *testing.T) {
	caps := chatCaps()
	caps.ACPLiveControls = true
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	id := out["id"].(string)
	base := "/api/v2/terminal/sessions/" + id + "/chat"
	for _, method := range []string{"GET", "POST"} {
		var in any
		if method == "POST" {
			in = map[string]string{"action": "set", "configId": "effort", "value": "high"}
		}
		if code, out, body := f.do(t, f.cookie, method, base+"/controls", in); code != 200 || out["controls"] == nil {
			t.Fatalf("controls %d %s", code, body)
		}
	}
	if code, _, _ := f.do(t, f.cookie, "POST", base+"/controls", map[string]string{"action": "steer", "text": "bypass queue"}); code != 400 {
		t.Fatalf("direct steering bypassed durable queue: %d", code)
	}
	message := terminalbroker.ID()
	if code, _, body := f.do(t, f.cookie, "POST", base+"/queue", map[string]string{"id": message, "text": "shared message"}); code != 200 {
		t.Fatalf("enqueue %d %s", code, body)
	}
	code, out, body = f.do(t, f.cookie, "GET", base+"/queue", nil)
	if code != 200 {
		t.Fatalf("read %d %s", code, body)
	}
	queue := out["queue"].([]any)
	if len(queue) != 1 {
		t.Fatal(queue)
	}
	row := queue[0].(map[string]any)
	if code, _, _ := f.do(t, f.cookie, "PATCH", base+"/queue", map[string]any{"id": message, "revision": row["revision"], "action": "edit", "text": "edited"}); code != 200 {
		t.Fatal("edit", code)
	}
	if code, _, _ := f.do(t, f.cookie, "PATCH", base+"/queue", map[string]any{"id": message, "revision": row["revision"], "action": "remove"}); code != 409 {
		t.Fatal("stale delete", code)
	}
	if code, _, _ := f.do(t, "", "GET", base+"/queue", nil); code != http.StatusUnauthorized {
		t.Fatal("anonymous queue", code)
	}
}

func TestOldConnectorRefusesRuntimeControls(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatal(code, body)
	}
	base := "/api/v2/terminal/sessions/" + out["id"].(string) + "/chat"
	if code, out, _ := f.do(t, f.cookie, "GET", base+"/controls", nil); code != 409 || out["code"] != "chat_controls_unsupported" {
		t.Fatal(code, out)
	}
}

func TestChatCommandsReadWhatTheAgentAnnounced(t *testing.T) {
	caps := chatCaps()
	caps.ACPLiveControls, caps.ACPCommands = true, true
	f := launchAPIFixture(t, caps)
	var listed []terminalproto.ACPCommand
	f.controls = func(in *terminalproto.ACPControl) terminalproto.ACPControlResult {
		if in.Action != "commands" {
			t.Errorf("action %q", in.Action)
		}
		return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Options: []terminalproto.ACPSetting{}}, Commands: listed}
	}
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	url := "/api/v2/terminal/sessions/" + out["id"].(string) + "/chat/commands"
	if code, _, body := f.do(t, f.cookie, "GET", url, nil); code != 200 || body != `{"commands":[]}` {
		t.Fatalf("an agent that announced nothing: %d %s", code, body)
	}
	f.mu.Lock()
	listed = []terminalproto.ACPCommand{{Name: "review", Description: "Review changes", Hint: "branch"}, {Name: "init"}}
	f.mu.Unlock()
	code, _, body = f.do(t, f.cookie, "GET", url, nil)
	if want := `{"commands":[{"name":"review","description":"Review changes","hint":"branch"},{"name":"init","description":"","hint":""}]}`; code != 200 || body != want {
		t.Fatalf("commands %d %s", code, body)
	}
	if code, _, _ := f.do(t, "", "GET", url, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", code)
	}
	member, err := f.st.CreateUser("commands-member@example.test", "Member", "member", "hash")
	if err != nil {
		t.Fatal(err)
	}
	other := terminalLogin(t, f.st, member.ID)
	code, _, real := f.do(t, other, "GET", url, nil)
	missing, _, unknown := f.do(t, other, "GET", "/api/v2/terminal/sessions/nosuchsession0000000/chat/commands", nil)
	if code != http.StatusNotFound || missing != http.StatusNotFound || real != unknown {
		t.Fatalf("foreign id %d %q, unknown id %d %q", code, real, missing, unknown)
	}
}

func TestOldConnectorAnswersNoChatCommands(t *testing.T) {
	caps := chatCaps()
	caps.ACPLiveControls = true
	f := launchAPIFixture(t, caps)
	asked := false
	f.controls = func(*terminalproto.ACPControl) terminalproto.ACPControlResult {
		asked = true
		return terminalproto.ACPControlResult{Controls: terminalproto.ACPControls{Options: []terminalproto.ACPSetting{}}}
	}
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatal(code, body)
	}
	code, _, body = f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions/"+out["id"].(string)+"/chat/commands", nil)
	if want := `{"commands":[],"code":"chat_commands_unsupported","error":"Update reet on this machine to see the agent's commands."}`; code != 200 || body != want || asked {
		t.Fatalf("%d %s asked=%v", code, body, asked)
	}
}
