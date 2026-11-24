package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/projectrun"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestPrivateControlAdvertisesMachineLock(t *testing.T) {
	srv, _, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	private := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer private.Close()
	c, response, err := websocket.Dial(t.Context(), private.URL+"/private/v1/control", &websocket.DialOptions{HTTPHeader: http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if features := response.Header.Get(terminalproto.FeatureHeader); !terminalproto.SupportsMachineLock(features) {
		t.Fatal("features", features)
	}
}

// lock records what a machine reports once its owner locks it: terminal
// access off, with Locked, in a new hello.
func (f *launchAPI) lock(t *testing.T) {
	t.Helper()
	rec, err := f.st.RemoteCapabilitiesRawFor(f.uid, f.agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	caps := rec.Capabilities
	caps.TerminalEnabled, caps.Readiness, caps.Locked = false, terminalproto.DisabledLocally, true
	caps.ACP, caps.ACPHistory, caps.ACPImages, caps.ACPApproval, caps.ShellDir, caps.Attention, caps.TerminalQuestion = false, false, false, false, false, false, false
	if err = f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent.ID, rec.InstanceID, rec.Generation, caps); err != nil {
		t.Fatal(err)
	}
	f.peer.Capabilities(false)
}

func lockedRefusal(t *testing.T, what string, code int, out map[string]any, body string) {
	t.Helper()
	if code != http.StatusConflict || out["code"] != "machine_locked" || out["error"] != terminalproto.LockedSentence {
		t.Errorf("%s on a locked machine: %d %s", what, code, body)
	}
}

func TestALockedMachineIsListedAsLocked(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	_, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if !strings.Contains(raw, `"locked":false`) {
		t.Fatalf("unlocked machine: %s", raw)
	}
	f.lock(t)
	_, _, raw = f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if !strings.Contains(raw, `"locked":true`) || !strings.Contains(raw, `"ready":false`) {
		t.Fatalf("terminal machines: %s", raw)
	}
	_, _, raw = f.do(t, f.cookie, "GET", "/api/v2/sync", nil)
	if !strings.Contains(raw, `"locked":true`) {
		t.Fatalf("sync listing: %s", raw)
	}
}

// Every route that would start, attach to, type into, answer or drive a
// session on a locked machine says so, with the one sentence, and nothing
// reaches the machine. Cancelling a chat still does.
func TestEveryRouteFamilyRefusesALockedMachine(t *testing.T) {
	caps := chatCaps()
	caps.Attention, caps.TerminalQuestion = true, true
	f := launchAPIFixture(t, caps)
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode"}))
	if code != 200 {
		t.Fatalf("shell: %d %s", code, body)
	}
	shell := out["id"].(string)
	code, out, body = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	if code != 200 {
		t.Fatalf("chat: %d %s", code, body)
	}
	chat := out["id"].(string)
	fp := strings.Repeat("ab", 32)
	f.ask(t, shell, apiQuestion(fp))
	f.lock(t)
	sent := len(f.sent())

	code, out, body = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode"}))
	lockedRefusal(t, "launch", code, out, body)
	code, out, body = f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode", "structured": true, "trust": true, "dir": "/tmp"}))
	lockedRefusal(t, "chat launch", code, out, body)
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+shell+"/attachments", map[string]any{"cols": 80, "rows": 24})
	lockedRefusal(t, "attach", code, out, body)
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+shell+"/answer", map[string]any{"fingerprint": fp, "option": 1})
	lockedRefusal(t, "answer", code, out, body)
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+chat+"/chat/prompt", map[string]string{"text": "hello"})
	lockedRefusal(t, "chat prompt", code, out, body)
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+chat+"/chat/permissions/permission-0123456789", map[string]any{"allow": true})
	lockedRefusal(t, "chat permission", code, out, body)
	code, out, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+chat+"/chat/questions/question-0123456789", map[string]any{"action": "cancel"})
	lockedRefusal(t, "chat answer", code, out, body)
	if len(f.sent()) != sent || len(f.prompts) != 0 || len(f.answers) != 0 {
		t.Fatalf("a request reached the locked machine: %d creates, %+v prompts, %+v answers", len(f.sent())-sent, f.prompts, f.answers)
	}
	if code, _, body = f.do(t, f.cookie, "POST", "/api/v2/terminal/sessions/"+chat+"/chat/cancel", nil); code != 200 {
		t.Fatalf("cancelling a chat on a locked machine: %d %s", code, body)
	}
}

// A machine that refuses as locked before the controller heard of the lock
// is answered the same way.
func TestALockedAnswerFromTheMachineIs409(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	f.reply = func(terminalproto.Create) terminalproto.Body { return terminalproto.Result{Code: terminalproto.Locked} }
	code, out, body := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "opencode"}))
	lockedRefusal(t, "launch", code, out, body)
}

// Gaffer and the Factory are told a locked machine started nothing, in the
// sentence that says what to do, not that the launch may have started.
func TestAnOutcomeStartOnALockedMachineSaysWhatToDo(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	f.lock(t)
	_, _, err := outcomeStarter{f.srv.s}.StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
	})
	if err == nil || err.Error() != terminalproto.LockedSentence || errors.Is(err, gaffer.ErrLaunchUnknown) {
		t.Fatalf("start on a locked machine: %v", err)
	}
	if len(f.sent()) != 0 {
		t.Fatal("a create reached the locked machine")
	}
}

// A project run's member on a locked machine carries the sentence.
func TestARunMemberOnALockedMachineSaysWhatToDo(t *testing.T) {
	if got := runMemberSentence(store.RunMemberFailed, projectrun.CodeMachineLocked); got != terminalproto.LockedSentence {
		t.Fatalf("sentence: %q", got)
	}
}

// When the lock went on is shown beside locked on both listings.
func TestALockedMachineSaysSinceWhen(t *testing.T) {
	f := launchAPIFixture(t, chatCaps())
	_, _, raw := f.do(t, f.cookie, "GET", "/api/v2/terminal/machines", nil)
	if strings.Contains(raw, "lockedAt") {
		t.Fatalf("an unlocked machine: %s", raw)
	}
	f.lock(t)
	rec, _ := f.st.RemoteCapabilitiesRawFor(f.uid, f.agent.ID)
	caps := rec.Capabilities
	caps.LockedAt = 1795000000
	if err := f.st.UpdateRemoteCapabilitiesFor(f.uid, f.agent.ID, rec.InstanceID, rec.Generation, caps); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v2/terminal/machines", "/api/v2/sync"} {
		if _, _, raw = f.do(t, f.cookie, "GET", path, nil); !strings.Contains(raw, `"lockedAt":1795000000`) {
			t.Fatalf("%s: %s", path, raw)
		}
	}
}

// A lock time from the future (beyond a few minutes of clock skew) is not
// believed: it is dropped, the lock kept.
func TestALockTimeFromTheFutureIsDropped(t *testing.T) {
	now := time.Unix(1795000000, 0)
	caps := terminalproto.Capabilities{Locked: true, LockedAt: now.Add(time.Hour).Unix()}
	if got := plausibleLockTime(caps, now); got.LockedAt != 0 || !got.Locked {
		t.Fatalf("future: %+v", got)
	}
	caps.LockedAt = now.Add(time.Minute).Unix()
	if got := plausibleLockTime(caps, now); got.LockedAt != caps.LockedAt {
		t.Fatalf("skew: %+v", got)
	}
}
