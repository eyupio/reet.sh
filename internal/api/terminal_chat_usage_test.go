package api

import (
	"net/http"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
)

// A chat's usage is what its agent reported, read from the controller's own
// record: no machine is asked, so it answers for an ended chat too.
func TestChatUsageReadsWhatTheAgentReported(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	id := out["id"].(string)
	url := "/api/v2/terminal/sessions/" + id + "/chat/usage"

	code, out, body = f.do(t, f.cookie, "GET", url, nil)
	if code != 200 || out["reported"] != false {
		t.Fatalf("a chat whose agent said nothing: %d %s", code, body)
	}
	if out["inputTokens"] != float64(0) || out["contextSize"] != float64(0) {
		t.Fatalf("an unreported chat carries numbers: %s", body)
	}

	// Two finished turns and the context report that came with the second.
	if err := f.st.AddTerminalSessionUsage(f.uid, id, 1000, 200, 5000, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := f.st.AddTerminalSessionUsage(f.uid, id, 300, 50, 0, 90000, 200000); err != nil {
		t.Fatal(err)
	}
	code, out, body = f.do(t, f.cookie, "GET", url, nil)
	if code != 200 || out["reported"] != true {
		t.Fatalf("usage %d %s", code, body)
	}
	for key, want := range map[string]float64{"inputTokens": 1300, "outputTokens": 250, "cachedTokens": 5000, "contextUsed": 90000, "contextSize": 200000} {
		if out[key] != want {
			t.Fatalf("%s = %v, want %v (%s)", key, out[key], want, body)
		}
	}
	if at, _ := out["updatedAt"].(float64); at <= 0 {
		t.Fatalf("no time on the report: %s", body)
	}

	if code, _, _ := f.do(t, "", "GET", url, nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", code)
	}
	if code, _, _ := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions/"+terminalbroker.ID()+"/chat/usage", nil); code != http.StatusNotFound {
		t.Fatalf("a session that does not exist: %d", code)
	}
	if code, _, _ := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions/not-an-id/chat/usage", nil); code != http.StatusBadRequest {
		t.Fatalf("a malformed id: %d", code)
	}
}

// A plain terminal has no agent reporting usage, so the read is refused the
// way every other chat route refuses it.
func TestChatUsageIsForChatsOnly(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	if code, _, body := f.do(t, f.cookie, "GET", "/api/v2/terminal/sessions/"+out["id"].(string)+"/chat/usage", nil); code != http.StatusNotFound {
		t.Fatalf("a terminal answered a chat's usage: %d %s", code, body)
	}
}
