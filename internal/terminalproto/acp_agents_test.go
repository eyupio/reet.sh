package terminalproto

import (
	"encoding/json"
	"slices"
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
	const cline = "1793848855723_8zvJS_cli"
	for id, want := range map[string]map[string]bool{
		uuid:                                    {"opencode": false, "claude": true, "codex": true, "cline": false},
		"ses_fake0001":                          {"opencode": true, "claude": false, "codex": false, "cline": false},
		cline:                                   {"opencode": false, "claude": false, "codex": false, "cline": true},
		"1793850943237_9D-BF_cli":               {"cline": true},
		"1793850943237_9_-BF_cli":               {"cline": true},
		"1793848303972_xxmw4":                   {"cline": false},
		"1793848855723_8zvJS":                   {"cline": false},
		"1793848855723_8zvJS_cli2":              {"cline": false},
		"1793848855723_8zvJS_cli\n":             {"cline": false},
		"1793848855723_8zvJS_cli/../x":          {"cline": false},
		"../1793848855723_8zvJS_cli":            {"cline": false},
		"1793848855723_8zvJ_cli":                {"cline": false},
		"1793848855723_8zvJSS_cli":              {"cline": false},
		"17938488557238zvJS_cli":                {"cline": false},
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
	// A session snapshot or change carries no tool, so its handle check accepts
	// every reviewed namespace; a caller that knows the tool stays strict.
	for _, handle := range []string{"ses_fake0001", uuid, cline} {
		if !ValidACPHandle(handle) {
			t.Fatalf("ValidACPHandle(%q) refused a reviewed handle", handle)
		}
	}
	for _, handle := range []string{"", "ses_", "1793848303972_xxmw4", "../" + cline} {
		if ValidACPHandle(handle) {
			t.Fatalf("ValidACPHandle(%q) accepted an unknown handle", handle)
		}
	}
	// Claude, Codex and Cline are reviewed (their descriptors still gate
	// availability); the other measured candidates are not, so none of them may
	// validate.
	if !ValidACPSessionIDFor("claude", uuid) || ValidACPSessionIDFor("claude", "ses_fake0001") {
		t.Fatal("claude identifier namespace")
	}
	if !ValidACPSessionIDFor("codex", uuid) || ValidACPSessionIDFor("codex", "ses_fake0001") {
		t.Fatal("codex identifier namespace")
	}
	if !ValidACPSessionIDFor("cline", cline) || ValidACPSessionIDFor("cline", "ses_fake0001") || ValidACPSessionIDFor("cline", uuid) {
		t.Fatal("cline identifier namespace")
	}
	if nativeIDMatches("gemini", uuid) {
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

// A descriptor may list the modes a person can choose, but only on a connection that
// negotiated acp-agent-mode-v1 (the Capabilities flag), only a few, only well-formed
// and distinct, and never for an agent that is not available.
func TestACPAgentModesInDescriptors(t *testing.T) {
	ok := chatCapabilities()
	ok.ACPAgentMode = true
	(*ok.ACPAgents)[0].Modes = []string{"read-only", "agent"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid modes: %v", err)
	}
	cases := map[string]func(*Capabilities){
		"flag missing":  func(c *Capabilities) { c.ACPAgentMode = false },
		"no ACP":        func(c *Capabilities) { c.ACP = false; c.ACPHistory = false },
		"too many":      func(c *Capabilities) { (*c.ACPAgents)[0].Modes = []string{"a", "b", "c", "d", "e"} },
		"duplicate":     func(c *Capabilities) { (*c.ACPAgents)[0].Modes = []string{"agent", "agent"} },
		"malformed":     func(c *Capabilities) { (*c.ACPAgents)[0].Modes = []string{"Agent Mode"} },
		"empty name":    func(c *Capabilities) { (*c.ACPAgents)[0].Modes = []string{""} },
		"missing agent": func(c *Capabilities) { (*c.ACPAgents)[0].State = "missing" },
		"incompatible":  func(c *Capabilities) { (*c.ACPAgents)[0].State = "incompatible" },
		"unverified":    func(c *Capabilities) { (*c.ACPAgents)[0].State = "unverified" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := ok
			agents := append([]ACPAgent(nil), *ok.ACPAgents...)
			agents[0].Modes = append([]string(nil), agents[0].Modes...)
			c.ACPAgents = &agents
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid modes")
			}
		})
	}
	c := ok
	agents := append([]ACPAgent(nil), *ok.ACPAgents...)
	c.ACPAgents = &agents
	if a, found := c.ChatAgent("opencode"); !found || len(a.Modes) != 2 {
		t.Fatalf("descriptor lost its modes: %+v %v", a, found)
	}
}

// A descriptor may describe the other selectable options it was measured to
// offer. They are bounded, well formed, distinct, and never claimed without
// acp-agent-options-v1 or for an agent that is not available.
func TestACPAgentOptionsInDescriptors(t *testing.T) {
	ok := chatCapabilities()
	ok.ACPAgentOptions = true
	(*ok.ACPAgents)[0].Options = []ACPAgentOption{
		{ID: "effort", Category: AgentOptionThoughtLevel, Values: []string{"default", "low", "high"}},
		{ID: "fast-mode", Category: AgentOptionModelConfig, Values: []string{"off", "on"}},
	}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid options: %v", err)
	}
	cases := map[string]func(*Capabilities){
		"flag missing": func(c *Capabilities) { c.ACPAgentOptions = false },
		"no ACP":       func(c *Capabilities) { c.ACP = false; c.ACPHistory = false },
		"too many options": func(c *Capabilities) {
			options := make([]ACPAgentOption, MaxACPAgentOptions+1)
			for i := range options {
				options[i] = ACPAgentOption{ID: "option" + string(rune('a'+i)), Category: AgentOptionModelConfig, Values: []string{"off"}}
			}
			(*c.ACPAgents)[0].Options = options
		},
		"duplicate id":     func(c *Capabilities) { (*c.ACPAgents)[0].Options[1].ID = "effort" },
		"missing id":       func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].ID = "" },
		"malformed id":     func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].ID = "Reasoning Effort" },
		"unknown category": func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].Category = "mode" },
		"no values":        func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].Values = nil },
		"too many values": func(c *Capabilities) {
			values := make([]string, MaxACPAgentOptionValues+1)
			for i := range values {
				values[i] = "value" + string(rune('a'+i))
			}
			(*c.ACPAgents)[0].Options[0].Values = values
		},
		"duplicate value": func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].Values = []string{"low", "low"} },
		"malformed value": func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].Values = []string{"High"} },
		"empty value":     func(c *Capabilities) { (*c.ACPAgents)[0].Options[0].Values = []string{""} },
		"missing agent":   func(c *Capabilities) { (*c.ACPAgents)[0].State = "missing" },
		"incompatible":    func(c *Capabilities) { (*c.ACPAgents)[0].State = "incompatible" },
		"unverified":      func(c *Capabilities) { (*c.ACPAgents)[0].State = "unverified" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := ok
			agents := append([]ACPAgent(nil), *ok.ACPAgents...)
			agents[0].Options = append([]ACPAgentOption(nil), agents[0].Options...)
			agents[0].Options[0].Values = append([]string(nil), agents[0].Options[0].Values...)
			agents[0].Options[1].Values = append([]string(nil), agents[0].Options[1].Values...)
			c.ACPAgents = &agents
			mutate(&c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid options")
			}
		})
	}
	c := ok
	agents := append([]ACPAgent(nil), *ok.ACPAgents...)
	c.ACPAgents = &agents
	if a, found := c.ChatAgent("opencode"); !found || len(a.Options) != 2 || len(a.Options[0].Values) != 3 {
		t.Fatalf("descriptor lost its options: %+v %v", a, found)
	}
}

func TestACPAgentOptionsMustHaveACP(t *testing.T) {
	c := chatCapabilities()
	c.ACPAgentOptions = true
	c.ACP = false
	c.ACPHistory = false
	if c.Validate() == nil {
		t.Fatal("options flag accepted without chat support")
	}
}

func TestACPAgentOptionsRoundTrip(t *testing.T) {
	c := chatCapabilities()
	c.ACPAgentOptions = true
	(*c.ACPAgents)[0].Options = []ACPAgentOption{{ID: "effort", Category: AgentOptionThoughtLevel, Values: []string{"low", "high"}}}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	h := Handshake{ProtocolVersion: Version, RequestID: "request-0123456789", AgentInstanceID: "instance-0123456789", Capabilities: c}
	wire, err := EncodeHandshake(h)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeHandshake(wire)
	if err != nil || got.Capabilities.ACPAgents == nil || !got.Capabilities.ACPAgentOptions {
		t.Fatalf("round trip: %+v %v", got, err)
	}
	options := (*got.Capabilities.ACPAgents)[0].Options
	if len(options) != 1 || options[0].ID != "effort" || options[0].Category != AgentOptionThoughtLevel || !slices.Equal(options[0].Values, []string{"low", "high"}) {
		t.Fatalf("options: %+v", options)
	}
}

// Cline is a reviewed chat tool: a descriptor with its own adapter name is
// available, carries the measured features and ids, and no other adapter name
// can claim it. Its reattach claim is honoured only when negotiated.
func TestClineIsAReviewedChatTool(t *testing.T) {
	c := chatCapabilities()
	c.AgentsV2 = true
	c.Tools = []string{"cline"}
	c.ACPHistory, c.ACPImages, c.ACPModel, c.ACPApproval = true, true, true, true
	entries := []ACPAgent{{
		Tool: "cline", Adapter: "cline", State: "available", MinimumVersion: "3.0.74",
		History: true, Images: true, Model: true, Approval: true, Modes: []string{"act", "plan"}, Reattach: true,
	}}
	c.ACPAgents = &entries
	c.ACPAgentMode = true
	// A descriptor claiming reattach needs the negotiated claim.
	if c.Validate() == nil {
		t.Fatal("reattach claim accepted without acp-agent-reattach-v1")
	}
	c.ACPAgentReattach = true
	c.ChatReattach = true
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	agent, ok := c.ChatAgent("cline")
	if !ok || !agent.History || !agent.Images || !agent.Model || !agent.Approval || !agent.Reattach || !slices.Equal(agent.Modes, []string{"act", "plan"}) {
		t.Fatalf("cline descriptor: %+v %v", agent, ok)
	}
	if !c.ChatReattachFor("cline") {
		t.Fatal("chat reattach not offered for a claimed agent")
	}
	// The machine-wide flag alone never enables a claim.
	c.ACPAgentReattach = false
	if c.ChatReattachFor("cline") {
		t.Fatal("claim honoured without the negotiated feature")
	}
	c.ACPAgentReattach = true
	// Reattach without history is not a descriptor.
	noHistory := []ACPAgent{entries[0]}
	noHistory[0].History = false
	c.ACPAgents = &noHistory
	if noHistory[0].Valid() {
		t.Fatal("reattach accepted without history")
	}
	c.ACPAgents = &entries
	for _, adapter := range []string{"opencode", "claude-agent-acp", "codex-acp", "cline-acp"} {
		bad := []ACPAgent{entries[0]}
		bad[0].Adapter = adapter
		c.ACPAgents = &bad
		if c.Validate() == nil {
			t.Fatalf("cline accepted with adapter %q", adapter)
		}
	}
}
