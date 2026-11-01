package agenthealth

import (
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestACPAgentsOnlyObservedAfterNegotiation(t *testing.T) {
	calls := 0
	cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }, StructuredAgents: func() []terminalproto.ACPAgent {
		calls++
		return []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true, Images: true, Model: true, Approval: true}}
	}}
	legacy := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature
	negotiateFeatures(&cfg, legacy)
	if c := capabilitiesFor(cfg, true, terminalproto.Ready); c.ACPAgents != nil || calls != 0 {
		t.Fatal("observed or sent descriptor to old peer")
	}
	negotiateFeatures(&cfg, legacy+","+terminalproto.ACPAgentsFeature)
	c := capabilitiesFor(cfg, true, terminalproto.Ready)
	if c.ACPAgents == nil || len(*c.ACPAgents) != 1 || calls != 1 {
		t.Fatalf("descriptor: %+v calls=%d", c, calls)
	}
	a := (*c.ACPAgents)[0]
	if a.History || a.Images || a.Model || a.Approval {
		t.Fatal("unnegotiated feature leaked")
	}
	if c.Validate() != nil {
		t.Fatal("invalid capabilities")
	}
	if c := capabilitiesFor(cfg, false, terminalproto.DisabledLocally); c.ACPAgents != nil || calls != 1 {
		t.Fatal("observed while disabled")
	}
	cfg.StructuredAgents = nil
	c = capabilitiesFor(cfg, true, terminalproto.Ready)
	if c.ACPAgents == nil || len(*c.ACPAgents) != 0 {
		t.Fatal("unknown new observation became legacy fallback")
	}
	if !terminalproto.SupportsACPAgents(terminalFeatures(cfg)) {
		t.Fatal("dispatcher did not negotiate descriptor contract")
	}
}

// A machine with Claude's adapter but no OpenCode still offers Claude chats: chat
// availability follows the reviewed adapters that are available, not OpenCode alone.
func TestClaudeChatDoesNotNeedOpenCodeInstalled(t *testing.T) {
	claude := terminalproto.ACPAgent{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1", Model: true, Approval: true}
	cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"claude"} }, StructuredAgents: func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{claude}
	}}
	features := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPAgentsFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.ACPApprovalFeature + "," + terminalproto.ACPModelFeature
	negotiateFeatures(&cfg, features)
	c := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !c.ACP || c.ACPAgents == nil || len(*c.ACPAgents) != 1 {
		t.Fatalf("claude was dropped without opencode: %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if a, ok := c.ChatAgent("claude"); !ok || !a.Approval || !a.Model {
		t.Fatalf("claude descriptor %+v %v", a, ok)
	}
	if _, ok := c.ChatAgent("opencode"); ok {
		t.Fatal("opencode became available without being installed")
	}
	// An older controller (no descriptors) still needs OpenCode, because it assumes it.
	old := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: cfg.LaunchTools, StructuredAgents: cfg.StructuredAgents}
	negotiateFeatures(&old, terminalproto.LaunchFeature+","+terminalproto.ACPFeature)
	if c := capabilitiesFor(old, true, terminalproto.Ready); c.ACP {
		t.Fatal("an old controller was told chats are available on a machine without opencode")
	}
	// A descriptor that is not available never turns chats on.
	dark := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: cfg.LaunchTools, StructuredAgents: func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "claude", Adapter: "claude-agent-acp", State: "unverified", MinimumVersion: "0.99.1"}}
	}}
	negotiateFeatures(&dark, features)
	if c := capabilitiesFor(dark, true, terminalproto.Ready); c.ACP {
		t.Fatal("an unverified adapter turned chats on")
	}
}
