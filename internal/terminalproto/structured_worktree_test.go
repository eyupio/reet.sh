package terminalproto

import "testing"

func TestStructuredWorktreeCreateKeepsConversationBoundary(t *testing.T) {
	c := Create{Title: "Isolated chat", Tool: "opencode", Structured: true, Trust: true, Cols: 80, Rows: 24, Dir: "/home/test/repo", Project: "project", Worktree: "0123456789abcdef0123456789abcdef", Base: "main"}
	if err := c.Validate(); err != nil {
		t.Fatal("new structured worktree refused", err)
	}
	c.ResumeACP = "ses_existing00000000000000001"
	if err := c.Validate(); err == nil {
		t.Fatal("existing conversation moved into a fresh worktree")
	}
	c.ResumeACP = ""
	c.Resume = true
	if err := c.Validate(); err == nil {
		t.Fatal("terminal resume accepted for structured worktree")
	}
}

func TestStructuredWorktreeCapabilityIsNegotiated(t *testing.T) {
	c := Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "test", Readiness: Ready, TerminalControl: true, TerminalEnabled: true, Launch: true, Tools: []string{"opencode"}, Worktree: true, ACP: true, ACPWorktree: true}
	features := LaunchFeature + "," + WorktreeFeature + "," + ACPFeature
	if err := c.ValidateFor(features); err == nil {
		t.Fatal("new capability accepted on an old connection")
	}
	if err := c.ValidateFor(features + "," + ACPWorktreeFeature); err != nil {
		t.Fatal("negotiated capability refused", err)
	}
	c.Worktree = false
	if err := c.Validate(); err == nil {
		t.Fatal("structured worktrees advertised without worktree support")
	}
}
