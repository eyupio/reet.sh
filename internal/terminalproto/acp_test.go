package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

func acpMessage(body Body) Message {
	return Message{Version: Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1, Body: body}
}

func TestACPBodiesRoundTrip(t *testing.T) {
	allow := true
	pending := true
	for name, body := range map[string]Body{
		"prompt":            &ACPPrompt{Text: "fix the failing build"},
		"prompt images":     &ACPPrompt{Text: "what is wrong?", Images: []string{"/home/u/.reet/launch-uploads/a/shot.png"}},
		"prompt image only": &ACPPrompt{Images: []string{"/home/u/.reet/launch-uploads/a/shot.png"}},
		"cancel":            &ACPCancel{},
		"model":             &ACPModel{Model: "ollama-cloud/deepseek-v4"},
		"permission":        &ACPPermission{PermissionID: "permission-0123456789", Allow: true},
		"approval mode":     &ACPMode{Yolo: true},
		"agent":             &ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "hello"},
		"tool":              &ACPEvent{Kind: "tool", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Status: "completed", Text: "done", Command: "echo ok", Path: "/home/u/a", Diff: "@@ -1 +1 @@\n-a\n+b", Additions: 1, Deletions: 1, DurationMs: 12, Locations: []string{"/home/u/a"}},
		"permission event":  &ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Command: "echo ok", Path: "/home/u/a", Diff: "@@ -0,0 +1 @@\n+x", Truncated: false},
		"resolved":          &ACPEvent{Kind: "permission_resolved", PermissionID: "permission-0123456789", Allow: &allow},
		"cancelled":         &ACPEvent{Kind: "permission_cancelled", PermissionID: "permission-0123456789", Reason: "timeout"},
		"status":            &ACPEvent{Kind: "status", State: "working", Detail: "Still working"},
		"notice":            &ACPEvent{Kind: "notice", Text: "hello", Bad: pending},
	} {
		data, err := Encode(acpMessage(body))
		if err != nil {
			t.Fatalf("%s: encode: %v", name, err)
		}
		decoded, err := Decode(data)
		if err != nil || decoded.Body.kind() != body.kind() {
			t.Fatalf("%s: decode: %v %+v", name, err, decoded.Body)
		}
	}
}

func TestACPRejectsUnboundedOrUnknownBodies(t *testing.T) {
	tooLong := strings.Repeat("a", MaxChatPromptBytes+1)
	tooManyImages := make([]string, MaxACPImages+1)
	for i := range tooManyImages {
		tooManyImages[i] = "/home/u/.reet/launch-uploads/a/shot.png"
	}
	for name, body := range map[string]Body{
		"empty model":        &ACPModel{},
		"invalid model":      &ACPModel{Model: "a\x00b"},
		"empty prompt":       &ACPPrompt{Text: " "},
		"long prompt":        &ACPPrompt{Text: tooLong},
		"nul prompt":         &ACPPrompt{Text: "a\x00b"},
		"relative image":     &ACPPrompt{Text: "look", Images: []string{"shot.png"}},
		"nul image":          &ACPPrompt{Text: "look", Images: []string{"/tmp/a\x00.png"}},
		"long image path":    &ACPPrompt{Text: "look", Images: []string{"/" + strings.Repeat("a", MaxACPImagePathBytes)}},
		"too many images":    &ACPPrompt{Text: "look", Images: tooManyImages},
		"unknown event":      &ACPEvent{Kind: "raw"},
		"agent no id":        &ACPEvent{Kind: "agent", Text: "x"},
		"permission empty":   &ACPEvent{Kind: "permission", PermissionID: "permission-0123456789"},
		"bad status":         &ACPEvent{Kind: "status", State: "nope"},
		"bad permission":     &ACPPermission{PermissionID: "x"},
		"negative additions": &ACPEvent{Kind: "tool", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Additions: -1},
		"negative duration":  &ACPEvent{Kind: "tool", ToolCallID: "call_1", Title: "write", ToolKind: "edit", DurationMs: -1},
		"long tool diff":     &ACPEvent{Kind: "tool", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Diff: strings.Repeat("d", 9<<10+1)},
	} {
		if _, err := Encode(acpMessage(body)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestACPPromptAllowsAnImageWithoutText(t *testing.T) {
	if !ValidACPPrompt("", []string{"/home/u/.reet/launch-uploads/a/shot.png"}) {
		t.Fatal("image-only prompt refused")
	}
	if ValidACPPrompt("", nil) {
		t.Fatal("empty prompt accepted")
	}
	if !SupportsACPImages(ACPFeature+","+ACPImageFeature) || SupportsACPImages(ACPFeature) {
		t.Fatal("acp-image-v1 feature parsing is wrong")
	}
}

func TestStructuredCreateRules(t *testing.T) {
	base := func() Create {
		return Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Dir: "/home/u/work", Structured: true}
	}
	c := base()
	if c.Validate() != nil {
		t.Fatal("valid structured create refused")
	}
	c.Yolo = true
	if c.Validate() != nil {
		t.Fatal("structured YOLO create refused")
	}
	for name, mutate := range map[string]func(*Create){
		"other tool":  func(c *Create) { c.Tool = "codex" },
		"worktree":    func(c *Create) { c.Worktree = "run" },
		"resume":      func(c *Create) { c.Resume = true },
		"effort":      func(c *Create) { c.Effort = "high" },
		"project run": func(c *Create) { c.Project = "p" },
	} {
		bad := base()
		mutate(&bad)
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestACPCapabilityNeedsTerminal(t *testing.T) {
	c := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "u", TerminalEnabled: true, HealthEnabled: true, Readiness: Ready, ACP: true}
	if c.Validate() == nil {
		t.Fatal("acp without terminal control accepted")
	}
	c.TerminalControl = true
	if c.Validate() != nil {
		t.Fatal("acp with terminal control refused")
	}
	c.ACPImages = true
	if c.Validate() != nil {
		t.Fatal("acp images with acp refused")
	}
	c.ACPApproval = true
	if c.Validate() != nil {
		t.Fatal("acp approval with terminal control refused")
	}
	c.ACP = false
	if c.Validate() == nil {
		t.Fatal("acp images without acp accepted")
	}
	if c.Validate() == nil {
		t.Fatal("acp approval without acp accepted")
	}
	if !SupportsACPApproval("acp-v1, acp-approval-v1") || SupportsACPApproval("acp-v1") {
		t.Fatal("approval feature negotiation")
	}
}

func TestACPResumeFields(t *testing.T) {
	id := "ses_fake000000000000000000000001"
	c := Create{Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, ResumeACP: id}
	if c.Validate() != nil {
		t.Fatal("valid resume refused")
	}
	for name, bad := range map[string]Create{
		"not structured": {Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", ResumeACP: id},
		"bad id":         {Title: "Chat", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, ResumeACP: "not-a-session"},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if !ValidACPSessionID(id) || ValidACPSessionID("ses_") || ValidACPSessionID("ses_a b") {
		t.Fatal("session id rule")
	}
	changed := SessionChanged{State: Exited, ACPSessionID: id}
	data, err := Encode(Message{Version: Version, RequestID: "request-0123456789", SessionID: "session-0123456789", Generation: 1, Body: &changed})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || decoded.Body.(*SessionChanged).ACPSessionID != id {
		t.Fatalf("session change %+v %v", decoded.Body, err)
	}
	if (Reconcile{Complete: true, Sessions: []SessionSnapshot{{ID: "session-0123456789", Title: "Chat", State: Running, ACPSessionID: "bad"}}}).valid() {
		t.Fatal("bad snapshot id accepted")
	}
}

func TestACPModelFeature(t *testing.T) {
	if SupportsACPModel(ACPFeature) || SupportsACPModel("not-"+ACPModelFeature) {
		t.Fatal("model switching must be negotiated separately")
	}
	if !SupportsACPModel(ACPFeature + "," + ACPModelFeature) {
		t.Fatal("model feature missing")
	}
}

func validSessionMode() SessionMode {
	return SessionMode{To: SessionModeChat, Tool: "opencode", Dir: "/home/u/proj", ACPSessionID: "ses_abc12345"}
}

func TestSessionModeValid(t *testing.T) {
	chat := validSessionMode()
	chat.Model, chat.Yolo, chat.Title, chat.Trust = "ollama-cloud/deepseek-v4", true, "t", true
	term := validSessionMode()
	term.To, term.Cols, term.Rows = SessionModeTerminal, 80, 24
	maxModel := validSessionMode()
	maxModel.Model = strings.Repeat("m", MaxModelBytes)
	emptyTitle := validSessionMode()
	emptyTitle.Title = ""
	for name, m := range map[string]SessionMode{"chat": chat, "terminal": term, "chat no dims": validSessionMode(), "max model": maxModel, "empty title": emptyTitle} {
		if !m.valid() {
			t.Fatalf("%s: valid body refused", name)
		}
	}
	bad := func(f func(*SessionMode)) SessionMode {
		m := term
		f(&m)
		return m
	}
	for name, m := range map[string]SessionMode{
		"bad to":           bad(func(m *SessionMode) { m.To = "x" }),
		"empty session id": bad(func(m *SessionMode) { m.ACPSessionID = "" }),
		"bad session id":   bad(func(m *SessionMode) { m.ACPSessionID = "nope" }),
		"trust terminal":   bad(func(m *SessionMode) { m.Trust = true }),
		"unreviewed tool":  bad(func(m *SessionMode) { m.Tool = "bash" }),
		"terminal no dims": bad(func(m *SessionMode) { m.Cols, m.Rows = 0, 0 }),
		"relative dir":     bad(func(m *SessionMode) { m.Dir = "relative" }),
		"NUL dir":          bad(func(m *SessionMode) { m.Dir = "/home/u/pro\x00j" }),
		"bad model":        bad(func(m *SessionMode) { m.Model = "a\x00b" }),
		"long model":       bad(func(m *SessionMode) { m.Model = strings.Repeat("m", MaxModelBytes+1) }),
		"control title":    bad(func(m *SessionMode) { m.Title = "a\x1bb" }),
		"chat with cols":   bad(func(m *SessionMode) { m.To, m.Cols, m.Rows = SessionModeChat, 100, 30 }),
		"chat negative":    bad(func(m *SessionMode) { m.To, m.Cols, m.Rows = SessionModeChat, -1, -1 }),
		"chat huge":        bad(func(m *SessionMode) { m.To, m.Cols, m.Rows = SessionModeChat, 1<<30, 1<<30 }),
		"chat only rows":   bad(func(m *SessionMode) { m.To, m.Cols, m.Rows = SessionModeChat, 0, 30 }),
	} {
		if m.valid() {
			t.Fatalf("%s: invalid body accepted", name)
		}
	}
}

func TestSessionModeRoundTrip(t *testing.T) {
	m := validSessionMode()
	data, err := Encode(acpMessage(&m))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(data)
	if err != nil || decoded.Body.kind() != "session_mode" {
		t.Fatalf("decode: %v %+v", err, decoded.Body)
	}
	extra := strings.Replace(string(data), `"to":"chat"`, `"to":"chat","extra":1`, 1)
	if extra == string(data) {
		t.Fatal("test did not inject field")
	}
	if _, err := Decode([]byte(extra)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestSessionModeFeature(t *testing.T) {
	if !SupportsSessionMode("acp-v1, "+SessionModeFeature) || SupportsSessionMode("acp-v1") {
		t.Fatal("session mode feature negotiation")
	}
}

func TestSessionModeAcceptsEightyRuneCJKTitle(t *testing.T) {
	m := validSessionMode()
	m.Title = strings.Repeat("\u4e2d", 80)
	if !m.valid() {
		t.Fatal("80-rune CJK title refused")
	}
	m.Title += "\u4e2d"
	if m.valid() {
		t.Fatal("81-rune title accepted")
	}
}

// A terminal body carries its size; omitempty must not drop cols and rows
// from the wire, and must drop the optional yolo, trust and model fields.
func TestSessionModeEncodingOmitsOptionalFieldsOnly(t *testing.T) {
	term := validSessionMode()
	term.To, term.Cols, term.Rows = SessionModeTerminal, 100, 30
	data, err := Encode(acpMessage(&term))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"cols":100`, `"rows":30`, `"to":"terminal"`, `"acpSessionId":"ses_abc12345"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing from %s", want, data)
		}
	}
	for _, absent := range []string{`"yolo"`, `"trust"`, `"model"`, `"title"`} {
		if strings.Contains(string(data), absent) {
			t.Fatalf("zero %s was encoded: %s", absent, data)
		}
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := *decoded.Body.(*SessionMode); got != term {
		t.Fatalf("round trip %+v, want %+v", got, term)
	}

	// The fields that are set are encoded and survive.
	full := validSessionMode()
	full.Model, full.Yolo, full.Trust, full.Title = "ollama-cloud/deepseek-v4", true, true, "Chat"
	data, err = Encode(acpMessage(&full))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"yolo":true`, `"trust":true`, `"model":"ollama-cloud/deepseek-v4"`, `"title":"Chat"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing from %s", want, data)
		}
	}
	decoded, err = Decode(data)
	if err != nil || *decoded.Body.(*SessionMode) != full {
		t.Fatalf("round trip %+v %v", decoded.Body, err)
	}
}

func TestCapabilitiesSessionModeSurvivesRoundTrip(t *testing.T) {
	c := chatCapabilities()
	c.ACPApproval, c.SessionMode = true, true
	wire, err := json.Marshal(c)
	if err != nil || !strings.Contains(string(wire), `"sessionMode":true`) {
		t.Fatalf("sessionMode not on the wire: %s %v", wire, err)
	}
	var back Capabilities
	if err := json.Unmarshal(wire, &back); err != nil || !back.SessionMode {
		t.Fatalf("sessionMode lost: %+v %v", back, err)
	}
	h := Handshake{ProtocolVersion: Version, RequestID: "request-0123456789", AgentInstanceID: "instance-0123456789", Capabilities: c}
	data, err := EncodeHandshake(h)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeHandshake(data)
	if err != nil || !got.Capabilities.SessionMode {
		t.Fatalf("handshake round trip: %+v %v", got.Capabilities, err)
	}
	c.SessionMode = false
	wire, _ = json.Marshal(c)
	if strings.Contains(string(wire), "sessionMode") {
		t.Fatalf("false sessionMode encoded: %s", wire)
	}
}
