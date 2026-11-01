package agenthealth

import (
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/terminalproto"
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

// session-mode-v1 is claimed only when the controller advertised it together
// with the features a swap stands on: acp-v1, acp-history-v1 and launch-v1.
func TestNegotiationAdvertisesSessionMode(t *testing.T) {
	full := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.SessionModeFeature
	var cfg AgentConfig
	negotiateFeatures(&cfg, full)
	if !terminalproto.SupportsSessionMode(terminalFeatures(cfg)) {
		t.Fatalf("session-mode-v1 not negotiated: %q", terminalFeatures(cfg))
	}
	for _, missing := range []string{terminalproto.LaunchFeature, terminalproto.ACPFeature, terminalproto.ACPHistoryFeature, terminalproto.SessionModeFeature} {
		var parts []string
		for _, f := range []string{terminalproto.LaunchFeature, terminalproto.ACPFeature, terminalproto.ACPHistoryFeature, terminalproto.SessionModeFeature} {
			if f != missing {
				parts = append(parts, f)
			}
		}
		var partial AgentConfig
		negotiateFeatures(&partial, strings.Join(parts, ","))
		if terminalproto.SupportsSessionMode(terminalFeatures(partial)) {
			t.Fatalf("session-mode-v1 negotiated without %s", missing)
		}
	}
}

// Capabilities.SessionMode is reported only on a connection that negotiated
// it, with chats and their history available and terminal control on.
func TestCapabilitiesReportSessionModeOnlyWithHistory(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }}
	base := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature
	negotiateFeatures(&cfg, base+","+terminalproto.ACPHistoryFeature+","+terminalproto.SessionModeFeature)
	c := capabilitiesFor(cfg, true, terminalproto.Ready)
	if !c.SessionMode || !c.ACP || !c.ACPHistory || !c.TerminalControl {
		t.Fatalf("session mode not reported: %+v", c)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	negotiateFeatures(&cfg, base+","+terminalproto.SessionModeFeature)
	if c := capabilitiesFor(cfg, true, terminalproto.Ready); c.SessionMode {
		t.Fatal("session mode reported without chat history")
	}
	negotiateFeatures(&cfg, base+","+terminalproto.ACPHistoryFeature)
	if c := capabilitiesFor(cfg, true, terminalproto.Ready); c.SessionMode {
		t.Fatal("session mode reported to a controller that did not advertise it")
	}
	negotiateFeatures(&cfg, base+","+terminalproto.ACPHistoryFeature+","+terminalproto.SessionModeFeature)
	if c := capabilitiesFor(cfg, false, terminalproto.DisabledLocally); c.SessionMode {
		t.Fatal("session mode reported with terminal access off")
	}
	noOpenCode := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return nil }}
	negotiateFeatures(&noOpenCode, base+","+terminalproto.ACPHistoryFeature+","+terminalproto.SessionModeFeature)
	if c := capabilitiesFor(noOpenCode, true, terminalproto.Ready); c.SessionMode {
		t.Fatal("session mode reported without chats")
	}
	// Chats from Claude's adapter but no OpenCode to launch: the OpenCode TUI
	// cannot be opened, so session mode is not reported.
	claudeOnly := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"claude"} }, StructuredAgents: func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "claude", Adapter: "claude-agent-acp", State: "available", MinimumVersion: "0.99.1", History: true}}
	}}
	negotiateFeatures(&claudeOnly, base+","+terminalproto.ACPAgentsFeature+","+terminalproto.ACPHistoryFeature+","+terminalproto.SessionModeFeature)
	c = capabilitiesFor(claudeOnly, true, terminalproto.Ready)
	if !c.ACP || !c.ACPHistory {
		t.Fatalf("claude chats not reported: %+v", c)
	}
	if c.SessionMode {
		t.Fatal("session mode reported without OpenCode among the launch tools")
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}
