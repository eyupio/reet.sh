package api

import (
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
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
