package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func usageURL(from, to int64) string { return fmt.Sprintf("/api/v2/usage?from=%d&to=%d", from, to) }

// The Usage page's read: every agent the controller knows, with what it
// reported in the range, the sessions behind it and Gaffer's own replies.
func TestUsageListsEveryAgentWithWhatItReported(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp", "title": "Fix the login"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	id := out["id"].(string)
	if err := f.st.AddTerminalSessionUsage(f.uid, id, 1000, 200, 5000, 9000, 20000); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	from, to := now-time.Hour.Milliseconds(), now+time.Hour.Milliseconds()
	code, out, body = f.do(t, f.cookie, "GET", usageURL(from, to), nil)
	if code != 200 {
		t.Fatalf("usage %d %s", code, body)
	}
	if out["from"] != float64(from) || out["to"] != float64(to) || out["more"] != false || out["sessionCount"] != float64(1) {
		t.Fatalf("range: %s", body)
	}
	if since, _ := out["since"].(float64); since <= 0 || since > float64(now) {
		t.Fatalf("since: %s", body)
	}
	agents, _ := out["agents"].([]any)
	tools := terminalproto.LaunchTools()
	if len(agents) != len(tools) {
		t.Fatalf("agents %d, want every catalogue agent (%d): %s", len(agents), len(tools), body)
	}
	for i, tool := range tools {
		a := agents[i].(map[string]any)
		if a["tool"] != tool || a["name"] != terminalproto.ToolName(tool) {
			t.Fatalf("agent %d: %v, want %s", i, a, tool)
		}
		want := "chat"
		if !terminalproto.ReviewedChatTool(tool) {
			want = "terminal_only"
		}
		if a["reports"] != want {
			t.Fatalf("%s reports %v, want %s", tool, a["reports"], want)
		}
		input := float64(0)
		if tool == "opencode" {
			input = 1000
		}
		if a["input"] != input {
			t.Fatalf("%s input %v, want %v", tool, a["input"], input)
		}
	}
	if agents[len(agents)-1].(map[string]any)["tool"] != "copilot" || agents[len(agents)-1].(map[string]any)["reports"] != "terminal_only" {
		t.Fatalf("Copilot is not listed as terminal only: %s", body)
	}
	sessions, _ := out["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("sessions: %s", body)
	}
	s := sessions[0].(map[string]any)
	for key, want := range map[string]any{"id": id, "tool": "opencode", "agentId": f.agent.ID, "structured": true, "archived": false, "factory": false, "input": float64(1000), "output": float64(200), "cached": float64(5000), "contextUsed": float64(9000), "contextSize": float64(20000)} {
		if s[key] != want {
			t.Fatalf("session %s = %v, want %v (%s)", key, s[key], want, body)
		}
	}
	if s["host"] == "" || s["title"] == "" || s["state"] == "" {
		t.Fatalf("session row lacks its machine, title or state: %s", body)
	}
	if gaffer, ok := out["gaffer"].([]any); !ok || len(gaffer) != 0 {
		t.Fatalf("gaffer: %s", body)
	}

	// A range with nothing in it still lists every agent, at zero.
	code, out, body = f.do(t, f.cookie, "GET", usageURL(now-48*time.Hour.Milliseconds(), now-24*time.Hour.Milliseconds()), nil)
	if code != 200 || len(out["agents"].([]any)) != len(tools) || len(out["sessions"].([]any)) != 0 || out["sessionCount"] != float64(0) {
		t.Fatalf("an empty range: %d %s", code, body)
	}
}

func TestUsageRefusesBadRangesAndOtherPeople(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	now := time.Now().UnixMilli()
	day := 24 * time.Hour.Milliseconds()
	for _, path := range []string{
		"/api/v2/usage",
		"/api/v2/usage?from=1",
		fmt.Sprintf("/api/v2/usage?from=%d&to=%d", now, now),
		fmt.Sprintf("/api/v2/usage?from=%d&to=%d", now, now-1),
		fmt.Sprintf("/api/v2/usage?from=%d&to=%d", now-93*day, now),
		fmt.Sprintf("/api/v2/usage?from=-5&to=%d", now),
		fmt.Sprintf("/api/v2/usage?from=x&to=%d", now),
		fmt.Sprintf("/api/v2/usage?from=%d&to=%d&from=%d", now-day, now, now-day),
		fmt.Sprintf("/api/v2/usage?from=%d&to=%d&agent=x", now-day, now),
	} {
		if code, _, body := f.do(t, f.cookie, "GET", path, nil); code != http.StatusBadRequest {
			t.Fatalf("%s: %d %s", path, code, body)
		}
	}
	// 92 days is the longest range.
	if code, _, body := f.do(t, f.cookie, "GET", usageURL(now-92*day, now), nil); code != 200 {
		t.Fatalf("92 days: %d %s", code, body)
	}
	if code, _, _ := f.do(t, "", "GET", usageURL(now-day, now), nil); code != http.StatusUnauthorized {
		t.Fatalf("anonymous read: %d", code)
	}
	// A viewer has no Sessions page, so it has no Usage page either.
	viewer, err := f.st.CreateUser("usage-viewer@example.test", "Viewer", "viewer", "hash")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := f.do(t, terminalLogin(t, f.st, viewer.ID), "GET", usageURL(now-day, now), nil); code != http.StatusForbidden {
		t.Fatalf("viewer read: %d", code)
	}
	// Another account reads only its own: none of the owner's sessions.
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	if err := f.st.AddTerminalSessionUsage(f.uid, out["id"].(string), 5, 5, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	other, err := f.st.CreateUser("usage-other@example.test", "Other", "admin", "hash")
	if err != nil {
		t.Fatal(err)
	}
	code, out, body = f.do(t, terminalLogin(t, f.st, other.ID), "GET", usageURL(now-day, now+day), nil)
	if code != 200 || len(out["sessions"].([]any)) != 0 || out["since"] != float64(0) || strings.Contains(body, `"input":5`) {
		t.Fatalf("another account: %d %s", code, body)
	}
	// A read from another site is refused; with remote terminals off the page still reads.
	r := usageURL(now-day, now+day)
	if code, _, _ := f.doSite(t, f.cookie, r, "cross-site"); code != http.StatusForbidden {
		t.Fatalf("cross-site read: %d", code)
	}
	off := false
	f.srv.s.terminalOverride.Store(&off)
	if code, _, body := f.do(t, f.cookie, "GET", r, nil); code != 200 {
		t.Fatalf("with remote terminals off: %d %s", code, body)
	}
}

// doSite is a GET that a browser marked with the given Sec-Fetch-Site.
func (f *launchAPI) doSite(t *testing.T, cookie, path, site string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Sec-Fetch-Site", site)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	return w.Code, nil, w.Body.String()
}
