package terminalproto

import (
	"slices"
	"strings"
)

// AgentsFeature negotiates the expanded agent catalogue and launch fields.
const AgentsFeature = "agents-v2"

// AntigravityFeature keeps the added tool id off the wire to controllers whose
// agents-v2 catalogue predates AGY. Their strict decoders reject unknown ids.
const AntigravityFeature = "antigravity-cli-v1"

func SupportsAntigravity(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == AntigravityFeature {
			return true
		}
	}
	return false
}

// CopilotFeature keeps the copilot tool id off the wire to controllers and
// connectors whose catalogue predates GitHub Copilot. Their strict decoders
// reject unknown ids.
const CopilotFeature = "copilot-cli-v1"

func SupportsCopilot(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == CopilotFeature {
			return true
		}
	}
	return false
}

// EffortLevelsFeature negotiates terminal effort levels beyond low, medium
// and high (xhigh, max). An older peer's validator refuses them.
const EffortLevelsFeature = "effort-levels-v1"

func SupportsEffortLevels(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == EffortLevelsFeature {
			return true
		}
	}
	return false
}

func SupportsAgents(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == AgentsFeature {
			return true
		}
	}
	return false
}

// launchAgents lists the agents a connector starts under agents-v2, in the
// catalogue's order, and the launch options each preset maps to arguments. It
// mirrors the enabled rows of terminalhost.Catalogue (a test there keeps them
// equal, order included) so the controller can refuse an option before it
// reserves a session, and can list every agent it knows in one order. Disabled
// presets (pi, cursor) are absent until the connector starts them.
var launchAgents = []agentLaunchOptions{
	{id: "claude", efforts: []string{"low", "medium", "high", "xhigh", "max"}, model: true, yolo: true},
	{id: "codex", efforts: basicEfforts, model: true, yolo: true},
	{id: "opencode", model: true, yolo: true},
	{id: "cline", efforts: basicEfforts, model: true, yolo: true},
	{id: "antigravity", efforts: basicEfforts, model: true, yolo: true},
	{id: "copilot", efforts: basicEfforts, model: true, yolo: true},
}

type agentLaunchOptions struct {
	id string
	// efforts are the levels the preset maps, lightest first.
	efforts     []string
	model, yolo bool
}

var basicEfforts = []string{"low", "medium", "high"}

// agentOptions is launchAgents by id.
var agentOptions = func() map[string]agentLaunchOptions {
	out := make(map[string]agentLaunchOptions, len(launchAgents))
	for _, a := range launchAgents {
		out[a.id] = a
	}
	return out
}()

// LaunchTools lists the agents started under agents-v2, in the catalogue's
// order. The caller owns the slice.
func LaunchTools() []string {
	out := make([]string, len(launchAgents))
	for i, a := range launchAgents {
		out[i] = a.id
	}
	return out
}

// ValidToolV2 accepts the agents started under agents-v2.
func ValidToolV2(id string) bool {
	_, ok := agentOptions[id]
	return ok
}

// ToolTakesProvider reports whether a structured chat of this agent can start
// on a chosen provider. Only Cline reports one.
func ToolTakesProvider(id string) bool { return id == "cline" }

// ToolTakesEffort reports whether the agent's preset maps an effort level.
func ToolTakesEffort(id string) bool { return len(agentOptions[id].efforts) > 0 }

// ToolEfforts lists the effort levels the agent's preset maps, lightest
// first. The caller owns the slice.
func ToolEfforts(id string) []string { return slices.Clone(agentOptions[id].efforts) }

// ToolTakesEffortLevel reports whether the agent's preset maps effort; the
// empty value, the agent's own default, always passes.
func ToolTakesEffortLevel(id, effort string) bool {
	return effort == "" || slices.Contains(agentOptions[id].efforts, effort)
}

// ToolTakesModel reports whether the agent's preset takes a model.
func ToolTakesModel(id string) bool { return agentOptions[id].model }

// ToolTakesYolo reports whether the agent's preset has a reviewed option that
// skips its own approval prompts. The connector still refuses when the machine
// has YOLO switched off.
func ToolTakesYolo(id string) bool { return agentOptions[id].yolo }
