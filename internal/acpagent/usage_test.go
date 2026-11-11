package acpagent

import (
	"encoding/json"
	"testing"
)

func TestTurnUsageReadsWhatTheAgentReportsAndNothingElse(t *testing.T) {
	got := turnUsage(json.RawMessage(`{"stopReason":"end_turn","usage":{"inputTokens":1200,"outputTokens":340,"cachedReadTokens":9000,"cachedWriteTokens":100,"totalTokens":10640}}`))
	if got == nil || *got != (Usage{InputTokens: 1200, OutputTokens: 340, CachedTokens: 9100}) {
		t.Fatalf("camelCase: %+v", got)
	}
	got = turnUsage(json.RawMessage(`{"usage":{"input_tokens":5,"output_tokens":7}}`))
	if got == nil || *got != (Usage{InputTokens: 5, OutputTokens: 7}) {
		t.Fatalf("snake_case: %+v", got)
	}
	for name, raw := range map[string]string{
		"no usage":   `{"stopReason":"end_turn"}`,
		"all zero":   `{"usage":{"inputTokens":0,"outputTokens":0}}`,
		"negative":   `{"usage":{"inputTokens":-4,"outputTokens":-1}}`,
		"too large":  `{"usage":{"inputTokens":99999999999999999}}`,
		"not json":   `{"usage":`,
		"wrong type": `{"usage":{"inputTokens":"many"}}`,
	} {
		if got := turnUsage(json.RawMessage(raw)); got != nil {
			t.Fatalf("%s reported %+v", name, got)
		}
	}
}

func TestContextUsageNeedsAWindowSizeAndNeverExceedsIt(t *testing.T) {
	got := contextUsage(json.RawMessage(`{"sessionId":"s","update":{"sessionUpdate":"usage_update","used":53000,"size":200000,"cost":{"amount":0.04,"currency":"USD"}}}`))
	if got == nil || *got != (Usage{ContextUsed: 53000, ContextSize: 200000}) {
		t.Fatalf("usage_update: %+v", got)
	}
	if got := contextUsage(json.RawMessage(`{"update":{"used":300000,"size":200000}}`)); got == nil || got.ContextUsed != 200000 {
		t.Fatalf("over the window: %+v", got)
	}
	for name, raw := range map[string]string{"no size": `{"update":{"used":10}}`, "zero size": `{"update":{"used":10,"size":0}}`, "negative": `{"update":{"used":1,"size":-5}}`} {
		if got := contextUsage(json.RawMessage(raw)); got != nil {
			t.Fatalf("%s reported %+v", name, got)
		}
	}
}
