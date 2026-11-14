package terminalproto

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCreateNewFieldsOmittedWhenEmpty(t *testing.T) {
	b, err := json.Marshal(Create{Title: "test", Cols: 80, Rows: 24})
	if err != nil || string(b) != `{"title":"test","cols":80,"rows":24}` {
		t.Fatalf("old shape: %s, %v", b, err)
	}
}
func TestCreateBaseRequiresWorktree(t *testing.T) {
	c := Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex", Dir: "/tmp", Base: "main"}
	if c.Validate() == nil {
		t.Fatal("base without worktree accepted")
	}
	c.Worktree = "0123456789abcdef"
	c.Project = "project"
	if c.Validate() != nil {
		t.Fatal("valid base refused")
	}
	for _, name := range []string{"-bad", "a..b", "a.lock"} {
		c.Base = name
		if c.Validate() == nil {
			t.Fatalf("accepted %q", name)
		}
	}
}
func TestCreateEffortValues(t *testing.T) {
	c := Create{Title: "test", Cols: 80, Rows: 24, Tool: "codex"}
	for _, e := range []string{"", "low", "medium", "high"} {
		c.Effort = e
		if c.Validate() != nil {
			t.Fatalf("refused %q", e)
		}
	}
	c.Effort = "extreme"
	if c.Validate() == nil {
		t.Fatal("invalid effort accepted")
	}
	c.Tool = ""
	c.Effort = "low"
	if c.Validate() == nil {
		t.Fatal("effort without tool accepted")
	}
	c.Effort = ""
	c.Yolo = true
	if c.Validate() == nil {
		t.Fatal("yolo without tool accepted")
	}
}
func TestCreateStillComparable(t *testing.T) {
	if !reflect.TypeOf(Create{}).Comparable() {
		t.Fatal("Create must remain comparable")
	}
}
func TestCapabilitiesNewToolIDsNeedAgentsV2(t *testing.T) {
	for _, tool := range []string{"cline", "antigravity", "copilot"} {
		c := Capabilities{Launch: true, TerminalControl: true, Tools: []string{tool}, ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, Readiness: Ready}
		if c.Validate() == nil || c.ValidateFor("") == nil {
			t.Fatal("old peer accepts new tool")
		}
		features := "launch-v1, agents-v2"
		if tool == "antigravity" {
			features += "," + AntigravityFeature
		}
		if tool == "copilot" {
			features += "," + CopilotFeature
		}
		if c.ValidateFor(features) != nil {
			t.Fatal("new peer refuses tool")
		}
		if !SupportsAgents(" launch-v1, agents-v2 ") || SupportsAgents("agents-v2-extra") || !ValidToolV2(tool) || ValidTool(tool) {
			t.Fatal("feature or tool check failed")
		}
	}
}

func TestAntigravityCapabilityRequiresItsNegotiatedFeature(t *testing.T) {
	c := Capabilities{AgentsV2: true, Launch: true, TerminalControl: true, Tools: []string{"antigravity"}, ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, Readiness: Ready}
	if c.ValidateFor(AgentsFeature) == nil {
		t.Fatal("AGY claimed to a controller with only the older agents-v2 catalogue")
	}
	if c.Validate() != nil || c.ValidateFor(AgentsFeature+","+AntigravityFeature) != nil {
		t.Fatal("negotiated AGY refused")
	}
	if !SupportsAntigravity(" launch-v1, "+AntigravityFeature+" ") || SupportsAntigravity(AntigravityFeature+"0") {
		t.Fatal("AGY token matching changed")
	}
}

// A controller or connector whose catalogue predates GitHub Copilot decodes
// tool ids strictly, so the id is valid only where copilot-cli-v1 was
// negotiated: not on agents-v2 alone, and not on another agent's token.
func TestCopilotCapabilityRequiresItsNegotiatedFeature(t *testing.T) {
	c := Capabilities{AgentsV2: true, Launch: true, TerminalControl: true, Tools: []string{"copilot"}, ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, Readiness: Ready}
	for _, features := range []string{AgentsFeature, AgentsFeature + "," + AntigravityFeature, CopilotFeature} {
		if c.ValidateFor(features) == nil {
			t.Fatalf("copilot claimed to a peer that negotiated only %q", features)
		}
	}
	if c.Validate() != nil || c.ValidateFor(AgentsFeature+","+CopilotFeature) != nil {
		t.Fatal("negotiated copilot refused")
	}
	c.Tools = []string{"antigravity", "claude", "codex", "copilot", "opencode"}
	if c.ValidateFor(AgentsFeature+","+AntigravityFeature) == nil {
		t.Fatal("copilot beside other tools claimed without its feature")
	}
	if c.Validate() != nil || c.ValidateFor(AgentsFeature+","+AntigravityFeature+","+CopilotFeature) != nil {
		t.Fatal("negotiated copilot beside other tools refused")
	}
	if !SupportsCopilot(" launch-v1, "+CopilotFeature+" ") || SupportsCopilot(CopilotFeature+"0") || SupportsCopilot(AntigravityFeature) {
		t.Fatal("copilot token matching changed")
	}
	if CopilotFeature != "copilot-cli-v1" {
		t.Fatal("the copilot token is part of the wire contract", CopilotFeature)
	}
}
func TestCapabilitiesYoloNeedsAgentsV2(t *testing.T) {
	c := Capabilities{Yolo: true, Launch: true, TerminalControl: true, ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, Readiness: Ready}
	for _, features := range []string{"", "launch-v1"} {
		if c.ValidateFor(features) == nil {
			t.Fatalf("yolo capability accepted for %q", features)
		}
	}
	if c.ValidateFor(AgentsFeature) == nil {
		t.Fatal("yolo capability without agents-v2 claim accepted")
	}
	c.AgentsV2 = true
	if c.ValidateFor(AgentsFeature) != nil {
		t.Fatal("yolo capability with agents-v2 refused")
	}
}

func TestCapabilitiesAgentsV2AndOSVersion(t *testing.T) {
	c := Capabilities{AgentsV2: true, Tools: []string{"cline"}, Launch: true, TerminalControl: true, ProtocolVersion: Version, OS: "linux", OSUser: "test", OSVersion: "6.1", TerminalEnabled: true, Readiness: Ready}
	if c.Validate() != nil || c.ValidateFor(AgentsFeature) != nil {
		t.Fatal("v2 capabilities refused")
	}
	if c.ValidateFor("") == nil {
		t.Fatal("v2 claim without negotiation accepted")
	}
	c.OSVersion = "bad\nversion"
	if c.Validate() == nil {
		t.Fatal("invalid OS version accepted")
	}
}

// Every agent started under agents-v2 now has a reviewed YOLO option. The controller's "this agent has no YOLO option"
// refusals (broker CreateSession, project runs and their API tests) are therefore defensive: no enabled agent reaches
// them today. This pins that fact, so the day an agent without a YOLO option is added the person adding it sees that
// those refusals need a test of their own.
func TestEveryStartedAgentTakesYolo(t *testing.T) {
	for id := range agentOptions {
		if !ToolTakesYolo(id) {
			t.Fatalf("%s has no reviewed YOLO option: add tests for the controller's option-gate refusals", id)
		}
	}
}

func TestAgentsV2ToolsAndOptions(t *testing.T) {
	for _, id := range []string{"pi", "cursor", "gemini", "nonesuch", ""} {
		if ValidToolV2(id) {
			t.Fatalf("%q accepted under agents-v2", id)
		}
	}
	for _, tc := range []struct {
		tool          string
		effort, model bool
	}{{"claude", true, true}, {"codex", true, true}, {"opencode", false, true}, {"cline", true, true}, {"antigravity", true, true}, {"copilot", true, true}, {"pi", false, false}} {
		if ToolTakesEffort(tc.tool) != tc.effort || ToolTakesModel(tc.tool) != tc.model {
			t.Fatalf("%s: effort=%v model=%v", tc.tool, ToolTakesEffort(tc.tool), ToolTakesModel(tc.tool))
		}
	}
}
