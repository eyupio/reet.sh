package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"github.com/eyupio/reet.sh/internal/webpush"
)

// pushFixture is an accounts instance with one machine and one signed-in
// owner, whose wake-ups are recorded instead of sent.
type pushFixture struct {
	srv    *Server
	st     *store.Store
	agent  store.SyncAgent
	uid    int64
	cookie string
	gen    int64
	woken  []webpush.Message
	fail   error
}

func newPushFixture(t *testing.T) *pushFixture {
	t.Helper()
	srv, st, a, _, _ := privateFixture(t, AuthAccounts, "")
	uid, _ := st.DefaultOwner()
	f := &pushFixture{srv: srv, st: st, agent: a, uid: uid, cookie: terminalLogin(t, st, uid)}
	srv.s.publicURL = "https://reet.example"
	srv.s.push.send = func(_ context.Context, m webpush.Message) error {
		f.woken = append(f.woken, m)
		return f.fail
	}
	gen, err := st.BeginRemoteConnectionFor(uid, a.ID, terminalbroker.ID(), terminalproto.Capabilities{
		ProtocolVersion: 1, OS: "linux", OSUser: "tester", TerminalEnabled: true, TerminalControl: true, HealthEnabled: true, Readiness: terminalproto.Ready, Attention: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.gen = gen.Generation
	return f
}

func (f *pushFixture) do(t *testing.T, method, path, body string, headers ...string) (int, string) {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Origin", "https://reet.example")
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Set(headers[i], headers[i+1])
	}
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: f.cookie})
	w := httptest.NewRecorder()
	f.srv.ServeHTTP(w, r)
	out, _ := io.ReadAll(w.Body)
	return w.Code, string(out)
}

// session starts a running session whose bell rang `ago` ago.
func (f *pushFixture) session(t *testing.T, title string, ago time.Duration) store.TerminalSession {
	t.Helper()
	sess, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent.ID, terminalbroker.ID(), f.gen, terminalproto.Create{Title: title, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, sess.ID, f.gen, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	if ago >= 0 {
		f.bell(t, sess.ID, ago)
	}
	return sess
}

func (f *pushFixture) bell(t *testing.T, id string, ago time.Duration) {
	t.Helper()
	at := time.Now().Add(-ago).UnixMilli()
	if err := f.st.ObserveTerminalAttentionFor(f.uid, f.agent.ID, id, f.gen, at, at); err != nil {
		t.Fatal(err)
	}
}

func (f *pushFixture) pending(t *testing.T, headers ...string) []pushNote {
	t.Helper()
	code, body := f.do(t, "GET", "/api/v2/push/pending", "", headers...)
	var out struct {
		Notifications []pushNote `json:"notifications"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &out) != nil {
		t.Fatalf("pending: %d %s", code, body)
	}
	return out.Notifications
}

const testEndpoint = "https://fcm.googleapis.com/fcm/send/phone"

func TestPushDeviceIsRegisteredListedAndRemoved(t *testing.T) {
	f := newPushFixture(t)
	code, body := f.do(t, "GET", "/api/v2/push", "")
	var status pushStatusView
	if code != 200 || json.Unmarshal([]byte(body), &status) != nil || !status.Available || status.PublicKey == "" || len(status.Devices) != 0 {
		t.Fatalf("status: %d %s", code, body)
	}
	// The identity is made once and kept.
	_, again := f.do(t, "GET", "/api/v2/push", "")
	var second pushStatusView
	_ = json.Unmarshal([]byte(again), &second)
	saved, _ := f.st.GetSetting(pushKeySetting)
	if second.PublicKey != status.PublicKey || saved == "" {
		t.Fatal("the push identity changed between two reads")
	}
	for _, bad := range []string{"http://fcm.googleapis.com/x", "https://example.org/hook", "https://127.0.0.1:8080/x", ""} {
		if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+bad+`"}`); code != 400 {
			t.Fatalf("endpoint %q was accepted: %d %s", bad, code, body)
		}
	}
	code, body = f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`","label":"Chrome on Android"}`)
	var device store.PushSubscription
	if code != 200 || json.Unmarshal([]byte(body), &device) != nil || device.Label != "Chrome on Android" || !device.Waiting || !device.Ended {
		t.Fatalf("subscribe: %d %s", code, body)
	}
	if code, body := f.do(t, "GET", "/api/v2/push", "", "X-Reet-Push-Endpoint", testEndpoint); code != 200 || !strings.Contains(body, `"current":`+jsonInt(device.ID)) {
		t.Fatalf("the device asking was not named: %d %s", code, body)
	}
	if strings.Contains(body, "fcm.googleapis.com") {
		t.Fatalf("the device's push address was sent back: %s", body)
	}
	if code, body = f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`","label":"Chrome on Android","ended":false}`); code != 200 || !strings.Contains(body, `"ended":false`) {
		t.Fatalf("choices: %d %s", code, body)
	}
	if code, body = f.do(t, "POST", "/api/v2/push/test", "{}"); code != 200 || !strings.Contains(body, `"sent":1`) || len(f.woken) != 1 || f.woken[0].Endpoint != testEndpoint {
		t.Fatalf("test: %d %s, woken %v", code, body, f.woken)
	}
	if notes := f.pending(t); len(notes) != 1 || notes[0].Tag != "reet-test" {
		t.Fatalf("pending after a test: %+v", notes)
	}
	if code, body = f.do(t, "DELETE", "/api/v2/push/subscriptions", `{"id":`+jsonInt(device.ID)+`}`); code != 200 || !strings.Contains(body, `"removed":true`) {
		t.Fatalf("remove: %d %s", code, body)
	}
	if code, _ = f.do(t, "POST", "/api/v2/push/test", "{}"); code != 409 {
		t.Fatalf("a test with no device: %d", code)
	}
}

func jsonInt(v int64) string { b, _ := json.Marshal(v); return string(b) }

func TestPushTellsAWaitingSessionOnce(t *testing.T) {
	f := newPushFixture(t)
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`"}`); code != 200 {
		t.Fatalf("subscribe: %d %s", code, body)
	}
	sess := f.session(t, "Refactor the store layer", 2*time.Second)
	now := time.Now()

	// Two seconds after the bell somebody may be looking at it: nothing yet.
	f.srv.s.pushSweep(now)
	if len(f.woken) != 0 {
		t.Fatalf("told %d before the moment settled", len(f.woken))
	}
	f.srv.s.pushSweep(now.Add(pushSettle))
	if len(f.woken) != 1 || f.woken[0].Endpoint != testEndpoint || f.woken[0].TTL != pushTTL || f.woken[0].Subject != "https://reet.example" {
		t.Fatalf("woken %+v", f.woken)
	}
	notes := f.pending(t)
	if len(notes) != 1 || notes[0].Title != "Refactor the store layer" || notes[0].Body != "Needs you." || notes[0].URL != "/sessions?agent="+f.agent.ID+"&session="+sess.ID {
		t.Fatalf("pending %+v", notes)
	}
	// The same bell is told once, however many times it is looked at.
	f.srv.s.pushSweep(now.Add(pushSettle + time.Minute))
	if len(f.woken) != 1 {
		t.Fatalf("the same moment was told %d times", len(f.woken))
	}
	// Seen, then a new bell: told again.
	if err := f.st.MarkSessionSeenFor(f.uid, sess.ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if notes := f.pending(t); len(notes) != 0 {
		t.Fatalf("a seen session is still pending: %+v", notes)
	}
	time.Sleep(5 * time.Millisecond)
	f.bell(t, sess.ID, 0)
	f.srv.s.pushSweep(time.Now().Add(pushSettle))
	if len(f.woken) != 2 {
		t.Fatalf("a new bell told %d in all, want 2", len(f.woken))
	}
}

func TestPushLeavesAloneWhatItShould(t *testing.T) {
	f := newPushFixture(t)
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`","waiting":false}`); code != 200 {
		t.Fatalf("subscribe: %d %s", code, body)
	}
	// A device that does not want waiting sessions is not woken for one, and is shown none.
	f.session(t, "Speed up the runner pool", time.Minute)
	f.srv.s.pushSweep(time.Now())
	if len(f.woken) != 0 {
		t.Fatalf("a device with waiting off was woken: %+v", f.woken)
	}
	if notes := f.pending(t, "X-Reet-Push-Endpoint", testEndpoint); len(notes) != 0 {
		t.Fatalf("a device with waiting off is shown %+v", notes)
	}
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`"}`); code != 200 {
		t.Fatalf("subscribe: %d %s", code, body)
	}
	// A session that has waited for an hour when the device is turned on tells nobody.
	f.session(t, "Left over the weekend", time.Hour)
	f.srv.s.pushSweep(time.Now())
	if len(f.woken) != 0 {
		t.Fatalf("an hour-old moment was told: %+v", f.woken)
	}
	// A subscription the push service no longer knows is forgotten.
	f.fail = webpush.ErrGone
	f.session(t, "Fix the failing build", time.Minute)
	f.srv.s.pushSweep(time.Now())
	if len(f.woken) != 1 {
		t.Fatalf("woken %d, want 1", len(f.woken))
	}
	if devices, _ := f.st.PushSubscriptionsFor(f.uid); len(devices) != 0 {
		t.Fatalf("a gone subscription was kept: %+v", devices)
	}
}

func TestPushNeedsAnAccount(t *testing.T) {
	srv, _, _, _, _ := privateFixture(t, AuthNone, "")
	r := httptest.NewRequest("GET", "/api/v2/push", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"available":false`) {
		t.Fatalf("status without accounts: %d %s", w.Code, w.Body.String())
	}
	r = httptest.NewRequest("POST", "/api/v2/push/subscriptions", strings.NewReader(`{"endpoint":"`+testEndpoint+`"}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	srv.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("subscribe without accounts: %d %s", w.Code, w.Body.String())
	}
}

func TestPushTellsAChatTurnThatFinishedWhileTheSessionRunsOn(t *testing.T) {
	f := newPushFixture(t)
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`"}`); code != 200 {
		t.Fatalf("subscribe: %d %s", code, body)
	}
	sess, _, err := f.st.ReserveTerminalCreateFor(f.uid, f.agent.ID, terminalbroker.ID(), f.gen, terminalproto.Create{Title: "Fix the checkout build", Tool: "opencode", Dir: "/tmp", Structured: true, Trust: true, Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.st.ObserveTerminalStateFor(f.uid, f.agent.ID, sess.ID, f.gen, terminalproto.Running, nil); err != nil {
		t.Fatal(err)
	}
	// Working: nothing to tell.
	at := time.Now().Add(-time.Minute).UnixMilli()
	if err := f.st.SetTerminalChatAttentionFor(f.uid, sess.ID, at, 0, "working", 0); err != nil {
		t.Fatal(err)
	}
	f.srv.s.pushSweep(time.Now())
	if len(f.woken) != 0 {
		t.Fatalf("a working chat was told: %+v", f.woken)
	}
	// The turn ends; the chat stays open for the next message.
	done := time.Now().Add(-30 * time.Second).UnixMilli()
	if err := f.st.SetTerminalChatAttentionFor(f.uid, sess.ID, done, 0, "finished", done); err != nil {
		t.Fatal(err)
	}
	f.srv.s.pushSweep(time.Now())
	if len(f.woken) != 1 {
		t.Fatalf("a finished turn woke %d devices, want 1", len(f.woken))
	}
	notes := f.pending(t)
	if len(notes) != 1 || notes[0].Title != "Fix the checkout build" || notes[0].Body != "Finished its turn." {
		t.Fatalf("pending %+v", notes)
	}
	f.srv.s.pushSweep(time.Now().Add(time.Minute))
	if len(f.woken) != 1 {
		t.Fatalf("the same turn was told %d times", len(f.woken))
	}
	// A device that does not want endings is not shown it.
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`","ended":false}`); code != 200 {
		t.Fatalf("choices: %d %s", code, body)
	}
	if notes := f.pending(t, "X-Reet-Push-Endpoint", testEndpoint); len(notes) != 0 {
		t.Fatalf("a device with endings off is shown %+v", notes)
	}
	// Looked at: no longer pending.
	if err := f.st.MarkSessionSeenFor(f.uid, sess.ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if notes := f.pending(t); len(notes) != 0 {
		t.Fatalf("a seen turn is still pending: %+v", notes)
	}
	if code, body := f.do(t, "POST", "/api/v2/push/subscriptions", `{"endpoint":"`+testEndpoint+`","ended":true}`); code != 200 {
		t.Fatalf("choices: %d %s", code, body)
	}
	// A turn that stops with a problem says so.
	time.Sleep(5 * time.Millisecond)
	failed := time.Now().UnixMilli()
	if err := f.st.SetTerminalChatAttentionFor(f.uid, sess.ID, failed, 0, "failed", failed); err != nil {
		t.Fatal(err)
	}
	f.srv.s.pushSweep(time.Now().Add(pushSettle))
	if notes := f.pending(t); len(f.woken) != 2 || len(notes) != 1 || notes[0].Body != "Its turn stopped with a problem." {
		t.Fatalf("a failed turn: woken %d, pending %+v", len(f.woken), notes)
	}
}
