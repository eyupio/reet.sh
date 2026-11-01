package terminalproto

import (
	"encoding/json"
	"strings"
	"testing"
)

func chatCapabilities() Capabilities {
	entries := []ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5", History: true}}
	return Capabilities{ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, HealthEnabled: true, TerminalControl: true, Readiness: Ready, Launch: true, Tools: []string{"opencode"}, ACP: true, ACPHistory: true, ACPAgents: &entries}
}
func TestACPAgentsNegotiationAndRoundTrip(t *testing.T) {
	c := chatCapabilities()
	if c.ValidateFor(ACPFeature) == nil {
		t.Fatal("accepted descriptors without negotiation")
	}
	if c.ValidateFor(ACPAgentsFeature) != nil {
		t.Fatal("refused negotiated descriptor")
	}
	h := Handshake{ProtocolVersion: Version, RequestID: "request-0123456789", AgentInstanceID: "instance-0123456789", Capabilities: c}
	wire, err := EncodeHandshake(h)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeHandshake(wire)
	if err != nil || got.Capabilities.ACPAgents == nil || len(*got.Capabilities.ACPAgents) != 1 {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	c.ACPAgents = nil
	wire, _ = json.Marshal(c)
	if strings.Contains(string(wire), "acpAgents") {
		t.Fatal("legacy wire includes descriptor field")
	}
	entries := []ACPAgent{}
	c.ACPAgents = &entries
	wire, _ = json.Marshal(c)
	if !strings.Contains(string(wire), `"acpAgents":[]`) {
		t.Fatalf("empty list lost: %s", wire)
	}
	if !SupportsACPAgents("other, acp-agents-v1 ") || SupportsACPAgents("acp-agents-v10") {
		t.Fatal("feature token match")
	}
}
func TestACPAgentsAuthoritativeAndLegacy(t *testing.T) {
	c := chatCapabilities()
	if a, ok := c.ChatAgent("opencode"); !ok || !a.History {
		t.Fatal("available descriptor refused")
	}
	(*c.ACPAgents)[0].State = "missing"
	(*c.ACPAgents)[0].History = false
	if _, ok := c.ChatAgent("opencode"); ok {
		t.Fatal("missing descriptor fell back to legacy ACP")
	}
	empty := []ACPAgent{}
	c.ACPAgents = &empty
	if _, ok := c.ChatAgent("opencode"); ok {
		t.Fatal("empty descriptor fell back")
	}
	c.ACPAgents = nil
	if a, ok := c.ChatAgent("opencode"); !ok || !a.History {
		t.Fatal("legacy OpenCode fallback lost")
	}
	if _, ok := c.ChatAgent("codex"); ok {
		t.Fatal("legacy flag enabled another tool")
	}
}
func TestACPAgentsRejectInvalidClaims(t *testing.T) {
	cases := map[string]func(*Capabilities){
		"no control":            func(c *Capabilities) { c.TerminalControl = false },
		"disabled":              func(c *Capabilities) { c.TerminalEnabled = false; c.Readiness = DisabledLocally },
		"no ACP":                func(c *Capabilities) { c.ACP = false; c.ACPHistory = false },
		"not installed":         func(c *Capabilities) { c.Tools = nil },
		"duplicate":             func(c *Capabilities) { *c.ACPAgents = append(*c.ACPAgents, (*c.ACPAgents)[0]) },
		"unreviewed available":  func(c *Capabilities) { (*c.ACPAgents)[0].Tool = "codex" },
		"missing with features": func(c *Capabilities) { (*c.ACPAgents)[0].State = "missing" },
		"unknown state":         func(c *Capabilities) { (*c.ACPAgents)[0].State = "ready" },
		"unnegotiated history":  func(c *Capabilities) { c.ACPHistory = false },
		"too many": func(c *Capabilities) {
			for len(*c.ACPAgents) <= MaxACPAgents {
				*c.ACPAgents = append(*c.ACPAgents, (*c.ACPAgents)[0])
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := chatCapabilities()
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid descriptor")
			}
		})
	}
	if ValidACPSessionIDFor("codex", "ses_fake0001") || !ValidACPSessionIDFor("opencode", "ses_fake0001") {
		t.Fatal("tool identifier namespace")
	}
}

func TestNativeIDNamespacesAreDisjointAndGated(t *testing.T) {
	const uuid = "0b9f6a1c-3d2e-4f5a-8b7c-1234567890ab"
	for id, want := range map[string]map[string]bool{
		uuid:                                    {"opencode": false, "claude": true, "codex": true},
		"ses_fake0001":                          {"opencode": true, "claude": false, "codex": false},
		strings.ToUpper(uuid):                   {"claude": false, "codex": false},
		"0b9f6a1c-3d2e-4f5a-8b7c-1234567890a":   {"claude": false},
		"0b9f6a1c-3d2e-4f5a-8b7c-1234567890abc": {"claude": false},
		"0b9f6a1c_3d2e-4f5a-8b7c-1234567890ab":  {"claude": false},
		"0b9f6a1c-3d2e-4f5a-8b7c-1234567890ag":  {"codex": false},
		"../" + uuid[3:]:                        {"claude": false},
	} {
		for tool, ok := range want {
			if nativeIDMatches(tool, id) != ok {
				t.Fatalf("nativeIDMatches(%q, %q) != %v", tool, id, ok)
			}
		}
	}
	// Claude is reviewed (its descriptor still gates availability); the other
	// measured candidates are not, so none of them may validate.
	if !ValidACPSessionIDFor("claude", uuid) || ValidACPSessionIDFor("claude", "ses_fake0001") {
		t.Fatal("claude identifier namespace")
	}
	if ValidACPSessionIDFor("codex", uuid) || nativeIDMatches("gemini", uuid) {
		t.Fatal("unreviewed tool accepted")
	}
}

// A reviewed tool is available only with the adapter that was reviewed for it,
// and a peer that sent no descriptors is an OpenCode-only peer.
func TestChatAgentDescriptorsAreBoundToTheirAdapter(t *testing.T) {
	claude := func(adapter string) Capabilities {
		c := chatCapabilities()
		c.Tools = []string{"claude", "opencode"}
		entries := []ACPAgent{
			{Tool: "claude", Adapter: adapter, State: "available", MinimumVersion: "0.99.1", Model: true, Approval: true},
			{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"},
		}
		c.ACPAgents = &entries
		c.ACPModel, c.ACPApproval = true, true
		return c
	}
	good := claude("claude-agent-acp")
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	if a, ok := good.ChatAgent("claude"); !ok || !a.Approval || a.History || a.Images || a.Questions {
		t.Fatalf("claude descriptor %+v %v", a, ok)
	}
	for _, adapter := range []string{"opencode", "codex-acp", "claude"} {
		bad := claude(adapter)
		if bad.Validate() == nil {
			t.Fatalf("claude accepted with adapter %q", adapter)
		}
	}
	// opencode with claude's adapter name is refused too.
	swapped := claude("claude-agent-acp")
	(*swapped.ACPAgents)[1].Adapter = "claude-agent-acp"
	if swapped.Validate() == nil {
		t.Fatal("opencode accepted with claude's adapter")
	}
	// No descriptors at all: only OpenCode is assumed.
	legacy := chatCapabilities()
	legacy.ACPAgents = nil
	if _, ok := legacy.ChatAgent("claude"); ok {
		t.Fatal("a legacy peer was assumed to support claude")
	}
	if _, ok := legacy.ChatAgent("opencode"); !ok {
		t.Fatal("a legacy peer lost opencode")
	}
}
