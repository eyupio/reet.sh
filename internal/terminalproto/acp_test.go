package terminalproto

import (
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
		"prompt":           &ACPPrompt{Text: "fix the failing build"},
		"cancel":           &ACPCancel{},
		"permission":       &ACPPermission{PermissionID: "permission-0123456789", Allow: true},
		"agent":            &ACPEvent{Kind: "agent", MessageID: "msg_1", Text: "hello"},
		"tool":             &ACPEvent{Kind: "tool", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Status: "in_progress", Text: "done", Locations: []string{"/home/u/a"}},
		"permission event": &ACPEvent{Kind: "permission", PermissionID: "permission-0123456789", ToolCallID: "call_1", Title: "write", ToolKind: "edit", Command: "echo ok", Path: "/home/u/a", Diff: "@@ -0,0 +1 @@\n+x", Truncated: false},
		"resolved":         &ACPEvent{Kind: "permission_resolved", PermissionID: "permission-0123456789", Allow: &allow},
		"cancelled":        &ACPEvent{Kind: "permission_cancelled", PermissionID: "permission-0123456789", Reason: "timeout"},
		"status":           &ACPEvent{Kind: "status", State: "working", Detail: "Still working"},
		"notice":           &ACPEvent{Kind: "notice", Text: "hello", Bad: pending},
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
	for name, body := range map[string]Body{
		"empty prompt":     &ACPPrompt{Text: " "},
		"long prompt":      &ACPPrompt{Text: tooLong},
		"nul prompt":       &ACPPrompt{Text: "a\x00b"},
		"unknown event":    &ACPEvent{Kind: "raw"},
		"agent no id":      &ACPEvent{Kind: "agent", Text: "x"},
		"permission empty": &ACPEvent{Kind: "permission", PermissionID: "permission-0123456789"},
		"bad status":       &ACPEvent{Kind: "status", State: "nope"},
		"bad permission":   &ACPPermission{PermissionID: "x"},
	} {
		if _, err := Encode(acpMessage(body)); err == nil {
			t.Errorf("%s accepted", name)
		}
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
	for name, mutate := range map[string]func(*Create){
		"other tool":  func(c *Create) { c.Tool = "claude" },
		"worktree":    func(c *Create) { c.Worktree = "run" },
		"resume":      func(c *Create) { c.Resume = true },
		"yolo":        func(c *Create) { c.Yolo = true },
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
}
