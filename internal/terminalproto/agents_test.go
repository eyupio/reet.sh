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
	c := Capabilities{Launch: true, TerminalControl: true, Tools: []string{"cline"}, ProtocolVersion: Version, OS: "linux", OSUser: "test", TerminalEnabled: true, Readiness: Ready}
	if c.Validate() == nil || c.ValidateFor("") == nil {
		t.Fatal("old peer accepts new tool")
	}
	if c.ValidateFor("launch-v1, agents-v2") != nil {
		t.Fatal("new peer refuses tool")
	}
	if !SupportsAgents(" launch-v1, agents-v2 ") || SupportsAgents("agents-v2-extra") || !ValidToolV2("cline") || ValidTool("cline") {
		t.Fatal("feature or tool check failed")
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

func TestAgentsV2ToolsAndOptions(t *testing.T) {
	for _, id := range []string{"pi", "cursor", "nonesuch", ""} {
		if ValidToolV2(id) {
			t.Fatalf("%q accepted under agents-v2", id)
		}
	}
	for _, tc := range []struct {
		tool          string
		effort, model bool
	}{{"claude", false, true}, {"codex", false, true}, {"opencode", false, true}, {"gemini", false, true}, {"cline", true, true}, {"pi", false, false}} {
		if ToolTakesEffort(tc.tool) != tc.effort || ToolTakesModel(tc.tool) != tc.model {
			t.Fatalf("%s: effort=%v model=%v", tc.tool, ToolTakesEffort(tc.tool), ToolTakesModel(tc.tool))
		}
	}
}
