package terminalproto

import "strings"

// AgentsFeature negotiates the expanded agent catalogue and launch fields.
const AgentsFeature = "agents-v2"

func SupportsAgents(features string) bool {
	for _, feature := range strings.Split(features, ",") {
		if strings.TrimSpace(feature) == AgentsFeature {
			return true
		}
	}
	return false
}

// agentOptions lists the agents a connector starts under agents-v2 and the
// launch options each preset maps to arguments. It mirrors the enabled rows of
// terminalhost.Catalogue (a test there keeps them equal) so the controller can
// refuse an option before it reserves a session. Disabled presets (pi, cursor)
// are absent until the connector starts them.
var agentOptions = map[string]struct{ effort, model, yolo bool }{
	"claude":   {model: true},
	"codex":    {model: true, yolo: true},
	"opencode": {model: true, yolo: true},
	"gemini":   {model: true},
	"cline":    {effort: true, model: true, yolo: true},
}

// ValidToolV2 accepts the agents started under agents-v2.
func ValidToolV2(id string) bool {
	_, ok := agentOptions[id]
	return ok
}

// ToolTakesEffort reports whether the agent's preset maps an effort level.
func ToolTakesEffort(id string) bool { return agentOptions[id].effort }

// ToolTakesModel reports whether the agent's preset takes a model.
func ToolTakesModel(id string) bool { return agentOptions[id].model }

// ToolTakesYolo reports whether the agent's preset has a reviewed option that
// skips its own approval prompts. The connector still refuses when the machine
// has YOLO switched off.
func ToolTakesYolo(id string) bool { return agentOptions[id].yolo }
