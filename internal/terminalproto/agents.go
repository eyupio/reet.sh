package terminalproto

import "strings"

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
var launchAgents = []struct {
	id                  string
	effort, model, yolo bool
}{
	{id: "claude", effort: true, model: true, yolo: true},
	{id: "codex", effort: true, model: true, yolo: true},
	{id: "opencode", model: true, yolo: true},
	{id: "cline", effort: true, model: true, yolo: true},
	{id: "antigravity", effort: true, model: true, yolo: true},
	{id: "copilot", effort: true, model: true, yolo: true},
}

// agentOptions is launchAgents by id.
var agentOptions = func() map[string]struct{ effort, model, yolo bool } {
	out := make(map[string]struct{ effort, model, yolo bool }, len(launchAgents))
	for _, a := range launchAgents {
		out[a.id] = struct{ effort, model, yolo bool }{a.effort, a.model, a.yolo}
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
func ToolTakesEffort(id string) bool { return agentOptions[id].effort }

// ToolTakesModel reports whether the agent's preset takes a model.
func ToolTakesModel(id string) bool { return agentOptions[id].model }

// ToolTakesYolo reports whether the agent's preset has a reviewed option that
// skips its own approval prompts. The connector still refuses when the machine
// has YOLO switched off.
func ToolTakesYolo(id string) bool { return agentOptions[id].yolo }
