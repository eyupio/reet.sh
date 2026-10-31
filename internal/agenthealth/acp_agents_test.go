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
