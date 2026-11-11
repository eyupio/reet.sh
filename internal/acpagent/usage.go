package acpagent

import "encoding/json"

// Usage is what an agent reports about its own consumption. Tokens are for
// one finished turn; the context figures are the agent's current state.
type Usage struct {
	InputTokens, OutputTokens, CachedTokens int64
	ContextUsed, ContextSize                int64
}

const maxUsageTokens = 1 << 40

func usageNumber(values ...*int64) int64 {
	for _, v := range values {
		if v != nil && *v > 0 && *v <= maxUsageTokens {
			return *v
		}
	}
	return 0
}

// turnUsage reads the optional usage a prompt result carries. Agents name the
// fields in camelCase; some adapters pass their provider's snake_case through.
// A result without usage, or with nothing but zeros, reports nothing.
func turnUsage(result json.RawMessage) *Usage {
	var in struct {
		Usage *struct {
			InputTokens       *int64 `json:"inputTokens"`
			OutputTokens      *int64 `json:"outputTokens"`
			CachedReadTokens  *int64 `json:"cachedReadTokens"`
			CachedWriteTokens *int64 `json:"cachedWriteTokens"`
			InputSnake        *int64 `json:"input_tokens"`
			OutputSnake       *int64 `json:"output_tokens"`
			CachedReadSnake   *int64 `json:"cached_read_tokens"`
			CachedWriteSnake  *int64 `json:"cached_write_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(result, &in) != nil || in.Usage == nil {
		return nil
	}
	u := in.Usage
	out := &Usage{InputTokens: usageNumber(u.InputTokens, u.InputSnake), OutputTokens: usageNumber(u.OutputTokens, u.OutputSnake)}
	out.CachedTokens = usageNumber(u.CachedReadTokens, u.CachedReadSnake) + usageNumber(u.CachedWriteTokens, u.CachedWriteSnake)
	if out.InputTokens+out.OutputTokens+out.CachedTokens == 0 {
		return nil
	}
	return out
}

// contextUsage reads a usage_update: how many tokens are in the agent's
// context and how large the window is.
func contextUsage(update json.RawMessage) *Usage {
	var in struct {
		Update struct {
			Used *int64 `json:"used"`
			Size *int64 `json:"size"`
		} `json:"update"`
	}
	if json.Unmarshal(update, &in) != nil {
		return nil
	}
	size := usageNumber(in.Update.Size)
	if size == 0 {
		return nil
	}
	used := usageNumber(in.Update.Used)
	if used > size {
		used = size
	}
	return &Usage{ContextUsed: used, ContextSize: size}
}
