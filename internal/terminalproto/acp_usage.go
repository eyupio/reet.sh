package terminalproto

import "strings"

// ACPUsageFeature lets a machine report what a chat's agent says it used: the
// tokens of a finished turn and how full its context window is. A machine
// sends it only to a controller that advertises the feature, because an older
// controller refuses an event kind it does not know.
const ACPUsageFeature = "acp-usage-v1"

func SupportsACPUsage(features string) bool {
	for _, f := range strings.Split(features, ",") {
		if strings.TrimSpace(f) == ACPUsageFeature {
			return true
		}
	}
	return false
}

// ACPUsage is the agent's own report, passed on as numbers only. The token
// counts are for one finished turn and are added up by the controller; the
// context figures are the agent's current state and replace the last ones. A
// zero ContextSize means the agent did not say.
type ACPUsage struct {
	InputTokens  int64 `json:"inputTokens,omitempty"`
	OutputTokens int64 `json:"outputTokens,omitempty"`
	CachedTokens int64 `json:"cachedTokens,omitempty"`
	ContextUsed  int64 `json:"contextUsed,omitempty"`
	ContextSize  int64 `json:"contextSize,omitempty"`
}

const maxACPUsageTokens = 1 << 40

func (u ACPUsage) valid() bool {
	for _, n := range []int64{u.InputTokens, u.OutputTokens, u.CachedTokens, u.ContextUsed, u.ContextSize} {
		if n < 0 || n > maxACPUsageTokens {
			return false
		}
	}
	return u.InputTokens+u.OutputTokens+u.CachedTokens+u.ContextUsed+u.ContextSize > 0
}
