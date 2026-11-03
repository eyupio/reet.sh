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

// Modes are listed, and the capability flag set, only on a connection that negotiated
// acp-agent-mode-v1 and only when an available agent offers one.
func TestACPAgentModesOnlyAfterNegotiation(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }, StructuredAgents: func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Approval: true, Modes: []string{"read-only", "agent"}}}
	}}
	base := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPAgentsFeature + "," + terminalproto.ACPApprovalFeature
	negotiateFeatures(&cfg, base)
	c := capabilitiesFor(cfg, true, terminalproto.Ready)
	if c.ACPAgentMode || c.ACPAgents == nil || len((*c.ACPAgents)[0].Modes) != 0 {
		t.Fatalf("modes leaked to a peer that did not negotiate them: %+v", c)
	}
	if terminalproto.SupportsACPAgentMode(terminalFeatures(cfg)) {
		t.Fatal("advertised the feature the peer did not offer")
	}
	negotiateFeatures(&cfg, base+","+terminalproto.ACPAgentModeFeature)
	c = capabilitiesFor(cfg, true, terminalproto.Ready)
	if !c.ACPAgentMode || len((*c.ACPAgents)[0].Modes) != 2 || c.Validate() != nil {
		t.Fatalf("negotiated modes: %+v %v", c, c.Validate())
	}
	if !terminalproto.SupportsACPAgentMode(terminalFeatures(cfg)) {
		t.Fatal("did not advertise the negotiated feature")
	}
	// No agent offers a mode: the flag stays off even when negotiated.
	cfg.StructuredAgents = func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Approval: true}}
	}
	if c = capabilitiesFor(cfg, true, terminalproto.Ready); c.ACPAgentMode {
		t.Fatal("flag set with no mode to choose")
	}
}

// Other selectable options are listed, and the capability flag set, only on a
// connection that negotiated acp-agent-options-v1 and only when an available
// agent offers one.
func TestACPAgentOptionsOnlyAfterNegotiation(t *testing.T) {
	cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }, StructuredAgents: func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Approval: true, Options: []terminalproto.ACPAgentOption{
			{ID: "effort", Category: terminalproto.AgentOptionThoughtLevel, Values: []string{"low", "high"}},
		}}}
	}}
	base := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPAgentsFeature + "," + terminalproto.ACPApprovalFeature
	negotiateFeatures(&cfg, base)
	c := capabilitiesFor(cfg, true, terminalproto.Ready)
	if c.ACPAgentOptions || c.ACPAgents == nil || len((*c.ACPAgents)[0].Options) != 0 {
		t.Fatalf("options leaked to a peer that did not negotiate them: %+v", c)
	}
	if terminalproto.SupportsACPAgentOptions(terminalFeatures(cfg)) {
		t.Fatal("advertised the feature the peer did not offer")
	}
	negotiateFeatures(&cfg, base+","+terminalproto.ACPAgentOptionsFeature)
	c = capabilitiesFor(cfg, true, terminalproto.Ready)
	if !c.ACPAgentOptions || len((*c.ACPAgents)[0].Options) != 1 || c.Validate() != nil {
		t.Fatalf("negotiated options: %+v %v", c, c.Validate())
	}
	if !terminalproto.SupportsACPAgentOptions(terminalFeatures(cfg)) {
		t.Fatal("did not advertise the negotiated feature")
	}
	// No agent offers another option: the flag stays off even when negotiated.
	cfg.StructuredAgents = func() []terminalproto.ACPAgent {
		return []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", Approval: true}}
	}
	if c = capabilitiesFor(cfg, true, terminalproto.Ready); c.ACPAgentOptions {
		t.Fatal("flag set with no option to describe")
	}
}

// Capabilities.ChatReattach needs the negotiated feature, chats with their
// history, and OpenCode among the chat tools.
func TestChatReattachDetailIsEchoedOnlyWhenAdvertised(t *testing.T) {
	base := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.ChatReattachFeature
	for _, tc := range []struct {
		name     string
		features string
		want     bool
	}{
		{"advertised", base + "," + terminalproto.ChatReattachDetailFeature, true},
		{"not advertised", base, false},
		{"detail without reattach", terminalproto.LaunchFeature + "," + terminalproto.ACPFeature + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.ChatReattachDetailFeature, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }}
			negotiateFeatures(&cfg, tc.features)
			if got := terminalproto.SupportsChatReattachDetail(terminalFeatures(cfg)); got != tc.want {
				t.Fatalf("detail = %v, want %v: %q", got, tc.want, terminalFeatures(cfg))
			}
			if err := capabilitiesFor(cfg, true, terminalproto.Ready).Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestChatReattachCapabilityNeedsNegotiationAndHistory(t *testing.T) {
	base := terminalproto.LaunchFeature + "," + terminalproto.ACPFeature
	for _, tc := range []struct {
		name     string
		features string
		want     bool
	}{
		{"negotiated with history", base + "," + terminalproto.ACPHistoryFeature + "," + terminalproto.ChatReattachFeature, true},
		{"negotiated without history", base + "," + terminalproto.ChatReattachFeature, false},
		{"history without negotiation", base + "," + terminalproto.ACPHistoryFeature, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return []string{"opencode"} }}
			negotiateFeatures(&cfg, tc.features)
			c := capabilitiesFor(cfg, true, terminalproto.Ready)
			if c.ChatReattach != tc.want {
				t.Fatalf("ChatReattach = %v, want %v: %+v", c.ChatReattach, tc.want, c)
			}
			if got := terminalproto.SupportsChatReattach(terminalFeatures(cfg)); got != tc.want {
				t.Fatalf("advertised = %v, want %v: %q", got, tc.want, terminalFeatures(cfg))
			}
			if err := c.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
	noOpenCode := AgentConfig{TerminalSession: testTerminalFactory, LaunchTools: func() []string { return nil }}
	negotiateFeatures(&noOpenCode, base+","+terminalproto.ACPHistoryFeature+","+terminalproto.ChatReattachFeature)
	if c := capabilitiesFor(noOpenCode, true, terminalproto.Ready); c.ChatReattach {
		t.Fatal("chat reattach reported without OpenCode")
	}
}
