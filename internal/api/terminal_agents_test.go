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
	for _, fields := range []map[string]any{{"effort": "high"}, {"base": "main", "worktree": "0123456789abcdef0123456789abcdef", "project": "reet"}, {"yolo": true}} {
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
func TestCreateYoloRefusedBeforeForwarding(t *testing.T) {
	caps := launchCaps("claude")
	caps.AgentsV2 = true
	f := launchAPIFixture(t, caps)
	status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "yolo": true}))
	if status != 409 || out["code"] != "yolo_refused" || len(f.sent()) != 0 {
		t.Fatalf("%d %v", status, out)
	}
}

// TestCreateUnsupportedAgentOptionsRefusedBeforeReserving checks every refusal
// leaves no session, no create operation and nothing sent to the machine.
func TestCreateUnsupportedAgentOptionsRefusedBeforeReserving(t *testing.T) {
	const updateSentence = "Update reet on that machine to use that option."
	const optionSentence = "That agent doesn't take that option."
	const disabledSentence = "reet doesn't start this agent yet."
	for _, tc := range []struct {
		name         string
		v2           bool
		fields       map[string]any
		status       int
		code, errMsg string
	}{
		{"effort on codex", true, map[string]any{"tool": "codex", "effort": "high"}, 409, "launch_option_unsupported", optionSentence},
		{"effort on claude", true, map[string]any{"tool": "claude", "effort": "low"}, 409, "launch_option_unsupported", optionSentence},
		{"effort on claude old peer", false, map[string]any{"tool": "claude", "effort": "low"}, 409, "launch_option_unsupported", updateSentence},
		{"tool pi", true, map[string]any{"tool": "pi"}, 422, "tool_unavailable", disabledSentence},
		{"tool cursor", true, map[string]any{"tool": "cursor"}, 422, "tool_unavailable", disabledSentence},
		{"tool pi old peer", false, map[string]any{"tool": "pi"}, 422, "tool_unavailable", disabledSentence},
		{"unknown tool", true, map[string]any{"tool": "nonesuch"}, 422, "tool_unavailable", disabledSentence},
		{"cline without agents-v2", false, map[string]any{"tool": "cline"}, 409, "launch_option_unsupported", updateSentence},
		{"tool not advertised", true, map[string]any{"tool": "gemini"}, 422, "tool_not_installed", "That agent isn't installed on that machine."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caps := launchCaps("claude", "codex")
			if tc.v2 {
				caps = launchCaps("claude", "cline", "codex")
				caps.AgentsV2 = true
			}
			f := launchAPIFixture(t, caps)
			request := terminalbroker.ID()
			status, out, _ := f.create(t, launchBody(f.agent.ID, request, tc.fields))
			if status != tc.status || out["code"] != tc.code || out["error"] != tc.errMsg {
				t.Fatalf("status=%d body=%v", status, out)
			}
			if len(f.sent()) != 0 {
				t.Fatalf("machine received %v", f.sent())
			}
			sessions, err := f.st.TerminalSessionsFor(f.uid)
			if err != nil || len(sessions) != 0 {
				t.Fatalf("session reserved: %v %v", sessions, err)
			}
			if op, err := f.st.TerminalOperationFor(f.uid, f.agent.ID, "create", request); err == nil {
				t.Fatalf("operation reserved: %+v", op)
			}
		})
	}
}

func TestCreateClineEffortAcceptedOnAgentsV2(t *testing.T) {
	caps := launchCaps("claude", "cline")
	caps.AgentsV2 = true
	f := launchAPIFixture(t, caps)
	status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "cline", "effort": "medium", "model": "sonnet"}))
	if status != 200 || out["effort"] != "medium" {
		t.Fatalf("%d %v", status, out)
	}
	if sent := f.sent(); len(sent) != 1 || sent[0].Tool != "cline" || sent[0].Effort != "medium" {
		t.Fatalf("%v", sent)
	}
}

// TestLaunchOptionRefusedByCurrentMachineSaysOption covers a refusal the
// machine itself sends: an up-to-date machine is not told to update.
func TestLaunchOptionRefusedByCurrentMachineSaysOption(t *testing.T) {
	for _, v2 := range []bool{true, false} {
		caps := launchCaps("claude")
		caps.AgentsV2 = v2
		f := launchAPIFixture(t, caps)
		f.reply = func(terminalproto.Create) terminalproto.Body {
			return terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
		}
		status, out, _ := f.create(t, launchBody(f.agent.ID, terminalbroker.ID(), map[string]any{"tool": "claude", "model": "sonnet"}))
		want := "Update reet on that machine to use that option."
		if v2 {
			want = "That agent doesn't take that option."
		}
		if status != 409 || out["code"] != "launch_option_unsupported" || out["error"] != want {
			t.Fatalf("v2=%v: %d %v", v2, status, out)
		}
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

func TestAgreedControlFeaturesTrackAgentsClaim(t *testing.T) {
	without := agreedControlFeatures(terminalproto.Capabilities{})
	with := agreedControlFeatures(terminalproto.Capabilities{AgentsV2: true})
	if terminalproto.SupportsAgents(without) || !terminalproto.SupportsAgents(with) {
		t.Fatalf("agents-v2 negotiation wrong: without=%q with=%q", without, with)
	}
	if with != privateControlFeatures || !strings.HasPrefix(with, without) {
		t.Fatalf("advertised features drifted: %q vs %q", with, privateControlFeatures)
	}
}

// The strict decoder refuses a v2 claim made without agentsV2 before the
// agreed-feature check runs; this pins that observable behaviour and reason.
func TestHandshakeWithoutAgentsV2IsRefusedAsInvalidHandshake(t *testing.T) {
	for name, mutate := range map[string]func(*terminalproto.Capabilities){
		"tool": func(c *terminalproto.Capabilities) {
			c.Launch, c.TerminalControl, c.Tools = true, true, []string{"cline"}
		},
		"osVersion": func(c *terminalproto.Capabilities) { c.OSVersion = "Ubuntu 24.04" },
	} {
		t.Run(name, func(t *testing.T) {
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
			caps := healthOnlyCapabilities()
			caps.AgentsV2 = true
			mutate(&caps)
			handshake, err := terminalproto.EncodeHandshake(terminalproto.Handshake{ProtocolVersion: terminalproto.Version, RequestID: terminalbroker.ID(), AgentInstanceID: "instance0123456789abcdef", Capabilities: caps})
			if err != nil {
				t.Fatal(err)
			}
			// The encoder would refuse a v2 claim without agents-v2, so strip the
			// flag from the wire form: this is what a hostile peer would send.
			raw := strings.Replace(string(handshake), `"agentsV2":true,`, "", 1)
			if raw == string(handshake) {
				t.Fatal("agentsV2 flag not found in handshake")
			}
			if err = conn.Write(ctx, websocket.MessageText, []byte(raw)); err != nil {
				t.Fatal(err)
			}
			_, _, err = conn.Read(ctx)
			if websocket.CloseStatus(err) != websocket.StatusPolicyViolation || !strings.Contains(err.Error(), "invalid handshake") {
				t.Fatalf("handshake not refused as an invalid handshake: %v", err)
			}
			uid, _ := st.DefaultOwner()
			if got, err := st.RemoteCapabilitiesFor(uid, agent.ID); err == nil && (len(got.Capabilities.Tools) > 0 || got.Capabilities.OSVersion != "") {
				t.Fatalf("stored v2 capabilities: %+v", got)
			}
		})
	}
}
