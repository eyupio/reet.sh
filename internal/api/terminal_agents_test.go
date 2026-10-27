package api

import (
	"context"
	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCreateNewFieldsRefusedToOldPeer(t *testing.T) {
	for _, fields := range []map[string]any{{"effort": "high"}, {"base": "main", "worktree": "0123456789abcdef0123456789abcdef", "project": "reet"}, {"yolo": true}, {"tool": "pi"}} {
		f := launchAPIFixture(t, launchCaps("claude"))
		body := launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "dir": "/home/tester/reet"})
		for k, v := range fields {
			body[k] = v
		}
		status, out, _ := f.create(t, body)
		if status != 409 || out["code"] != "launch_option_unsupported" {
			t.Fatalf("fields %v: status=%d body=%v", fields, status, out)
		}
		if len(f.sent()) != 0 {
			t.Fatal("old peer received extended create")
		}
	}
}
func TestCreateBaseMissingSentence(t *testing.T) {
	caps := launchCaps("claude")
	caps.Worktree = true
	caps.AgentsV2 = true
	f := launchAPIFixture(t, caps)
	f.reply = func(terminalproto.Create) terminalproto.Body {
		return terminalproto.Result{Code: terminalproto.WorktreeRefused, Reason: terminalproto.WorktreeBaseMissing}
	}
	status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "dir": "/home/tester/reet", "worktree": "0123456789abcdef0123456789abcdef", "project": "reet", "base": "main"}))
	if status != 422 || out["code"] != "base_missing" || !strings.Contains(out["error"].(string), "That branch isn't in the repository on ") {
		t.Fatalf("%d %v", status, out)
	}
}
func TestLaunchAuditCarriesEffortBaseYolo(t *testing.T) {
	caps := launchCaps("cline")
	caps.AgentsV2 = true
	f := launchAPIFixture(t, caps)
	status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "cline", "effort": "high", "prompt": "private prompt text"}))
	if status != 200 {
		t.Fatalf("%d %v", status, out)
	}
	if out["effort"] != "high" || out["yolo"] != false {
		t.Fatalf("session launch metadata not retained: %v", out)
	}
	if _, exists := out["prompt"]; exists {
		t.Fatal("session response exposed the opening prompt")
	}
	events := launchEvents(t, f.st, f.uid)
	if len(events) != 1 {
		t.Fatalf("%v", events)
	}
	// Metadata remains explicit even when the selected base is HEAD and approvals are asking.
	detail := events[0].Detail
	if !strings.Contains(detail, `"effort":"high"`) || !strings.Contains(detail, `"base":""`) || !strings.Contains(detail, `"yolo":"false"`) || strings.Contains(detail, "private prompt") {
		t.Fatalf("%s", detail)
	}
}
func TestCreateYoloRefusedBeforeF(t *testing.T) {
	caps := launchCaps("claude")
	caps.AgentsV2 = true
	f := launchAPIFixture(t, caps)
	status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "yolo": true}))
	if status != 409 || out["code"] != "yolo_refused" || len(f.sent()) != 0 {
		t.Fatalf("%d %v", status, out)
	}
}

func TestCreateNegotiationCannotUpgradeOnLaterHello(t *testing.T) {
	srv, st, agent, secret, _ := privateFixture(t, AuthAccounts, "")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.s.privateHandler().ServeHTTP(w, r.WithContext(privateconn.WithPrivateTransport(r.Context())))
	}))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, strings.Replace(ts.URL, "http://", "ws://", 1)+"/private/v1/control", &websocket.DialOptions{HTTPHeader: http.Header{"X-Reet-Agent": {agent.ID}, "X-Reet-Agent-Key": {secret}, "X-Reet-Version": {"test"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	instance := "instance0123456789abcdef"
	handshake, err := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: terminalproto.Version, RequestID: terminalbroker.ID(), AgentInstanceID: instance, Capabilities: healthOnlyCapabilities()})
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.Write(ctx, websocket.MessageText, handshake); err != nil {
		t.Fatal(err)
	}
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := terminalproto.DecodeHandshakeReply(data)
	if err != nil {
		t.Fatal(err)
	}
	upgraded := healthOnlyCapabilities()
	upgraded.AgentsV2 = true
	upgraded.OSVersion = "1.0"
	hello, err := terminalproto.Encode(terminalproto.Message{Version: terminalproto.Version, RequestID: terminalbroker.ID(), Generation: reply.Generation, Body: terminalproto.Hello{AgentInstanceID: instance, Capabilities: upgraded}})
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.Write(ctx, websocket.MessageText, hello); err != nil {
		t.Fatal(err)
	}
	for {
		if _, _, err = conn.Read(ctx); err != nil {
			break
		}
	}
	if ctx.Err() != nil {
		t.Fatal("connection did not close when later Hello changed negotiated features")
	}
	uid, _ := st.DefaultOwner()
	caps, err := st.RemoteCapabilitiesFor(uid, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if caps.Capabilities.AgentsV2 || caps.Capabilities.OSVersion != "" {
		t.Fatalf("later Hello upgraded negotiated capabilities: %+v", caps)
	}
}
