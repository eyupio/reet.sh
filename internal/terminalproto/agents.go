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

// ValidToolV2 accepts the catalogue offered under agents-v2.
func ValidToolV2(id string) bool {
	return ValidTool(id) || id == "pi" || id == "cursor" || id == "cline"
}
