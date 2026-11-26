// SPDX-License-Identifier: AGPL-3.0-only
// Adapted from eyupio/zoomies, 3c4d09627f78afab72a6575e94624fb0e7710f3f.
// Original source: internal/assistant/. See ../NOTICE.md for provenance.
package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/elicore"
)

func TestAnthropicSendsTheMessagesRequestWithItsHeaders(t *testing.T) {
	srv := newAnthropicServer(t)
	p := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "sk-ant", Model: "m"})
	tool := elicore.Tool{Name: "echo", Description: "Echo", Parameters: json.RawMessage(`{"type":"object"}`)}
	req := elicore.Request{
		System: "Be brief.",
		Messages: []elicore.Message{
			{Role: elicore.RoleUser, Content: "hi"},
			{Role: elicore.RoleAssistant, ToolCalls: []elicore.ToolCall{{ID: "toolu_0", Name: "echo", Arguments: json.RawMessage(`{"text":"x"}`)}}},
			{Role: elicore.RoleTool, ToolCallID: "toolu_0", Content: "x"},
			{Role: elicore.RoleUser, Content: "Say hello"},
		},
		Tools: []elicore.Tool{tool},
	}
	drain(t, p, req)
	rec := srv.Requests()[0]
	if rec.Path != "/v1/messages" || rec.Header.Get("x-api-key") != "sk-ant" || rec.Header.Get("anthropic-version") != "2023-06-01" {
		t.Errorf("request %s %v", rec.Path, rec.Header)
	}
	system, _ := rec.Body["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["text"] != "Be brief." || rec.Body["stream"] != true || rec.Body["model"] != "m" {
		t.Errorf("body %v", rec.Body)
	}
	tools, _ := rec.Body["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["input_schema"] == nil {
		t.Errorf("tools %v", rec.Body["tools"])
	}
	msgs, _ := rec.Body["messages"].([]any)
	if len(msgs) != 4 {
		t.Fatalf("messages %v", msgs)
	}
	second, _ := msgs[1].(map[string]any)
	blocks, _ := second["content"].([]any)
	if second["role"] != "assistant" || len(blocks) != 1 || blocks[0].(map[string]any)["type"] != "tool_use" {
		t.Errorf("assistant tool turn %v", second)
	}
	third, _ := msgs[2].(map[string]any)
	blocks, _ = third["content"].([]any)
	if third["role"] != "user" || len(blocks) != 1 || blocks[0].(map[string]any)["type"] != "tool_result" || blocks[0].(map[string]any)["tool_use_id"] != "toolu_0" {
		t.Errorf("tool result turn %v", third)
	}
}

// The API refuses a request that carries tool_use or tool_result blocks and
// defines no tools. A caller that stops offering tools after one call still
// sends that call and its answer, so they go as plain text.
func TestAnthropicSendsNoToolBlocksInARequestWithoutTools(t *testing.T) {
	p := NewAnthropic(Config{BaseURL: "https://api.anthropic.com", Model: "m"})
	raw, err := p.body(elicore.Request{Messages: []elicore.Message{
		{Role: elicore.RoleUser, Content: "Tell the worker to fix the tests."},
		{Role: elicore.RoleAssistant, Content: "On it.", ToolCalls: []elicore.ToolCall{{ID: "toolu_0", Name: "message_worker", Arguments: json.RawMessage(`{"text":"Fix the tests."}`)}}},
		{Role: elicore.RoleTool, ToolCallID: "toolu_0", Content: `{"state":"queued"}`},
	}}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Tools    any `json:"tools"`
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.Tools != nil || strings.Contains(string(raw), `"tool_use"`) || strings.Contains(string(raw), `"tool_result"`) || strings.Contains(string(raw), "toolu_0") {
		t.Fatalf("tool blocks without tools: %s", raw)
	}
	said, _ := body.Messages[1].Content.(string)
	if len(body.Messages) != 3 || body.Messages[1].Role != "assistant" || !strings.Contains(said, "On it.") || !strings.Contains(said, "message_worker") || !strings.Contains(said, "Fix the tests.") {
		t.Fatalf("the call was not kept as text: %s", raw)
	}
	if answer, _ := json.Marshal(body.Messages[2].Content); body.Messages[2].Role != "user" || !strings.Contains(string(answer), "queued") {
		t.Fatalf("the answer was not kept as text: %s", raw)
	}
}

func TestAnthropicStreamsContentBlockEventsIntoDeltasToolCallsAndUsage(t *testing.T) {
	srv := newAnthropicServer(t)
	p := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	var text strings.Builder
	var usage *elicore.Usage
	for _, e := range drain(t, p, hello()) {
		text.WriteString(e.Delta)
		if e.Usage != nil {
			usage = e.Usage
		}
	}
	if text.String() != "Hello from the fake" {
		t.Errorf("text %q", text.String())
	}
	if usage == nil || !usage.Reported || usage.InputTokens != 7 || usage.OutputTokens != 4 {
		t.Errorf("usage %+v", usage)
	}
	tool := elicore.Tool{Name: "echo", Parameters: json.RawMessage(`{"type":"object"}`)}
	req := elicore.Request{Messages: []elicore.Message{{Role: elicore.RoleUser, Content: ContractPrompt}}, Tools: []elicore.Tool{tool}}
	var call *elicore.ToolCall
	for _, e := range drain(t, p, req) {
		if e.ToolCall != nil {
			call = e.ToolCall
		}
	}
	if call == nil || call.Name != "echo" || call.ID != "toolu_1" || string(call.Arguments) != `{"text":"ping"}` {
		t.Errorf("tool call %+v", call)
	}
}

func TestAnthropicCheckIsOneRequestOfOneToken(t *testing.T) {
	srv := newAnthropicServer(t)
	res, err := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"}).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Body["max_tokens"] != float64(1) {
		t.Errorf("requests %+v", reqs)
	}
	if res.Model != "m" || !res.UsageReported {
		t.Errorf("result %+v", res)
	}
}

func TestAnthropicPassesTheContract(t *testing.T) {
	runContractTests(t, "anthropic", func(t *testing.T) elicore.Provider {
		return NewAnthropic(Config{BaseURL: newAnthropicServer(t).URL, APIKey: "k", Model: "m"})
	})
}

// Review Focus 1 on the second adapter: a gateway in front of the API
// usually publishes its address ending in /v1, and the Messages path must
// not double it.
func TestAnthropicAppendsTheMessagesPathOnceWhateverThePathGiven(t *testing.T) {
	srv := newAnthropicServer(t)
	for given, wantPath := range map[string]string{
		"":       "/v1/messages",
		"/":      "/v1/messages",
		"/v1":    "/v1/messages",
		"/v1/":   "/v1/messages",
		"/gw":    "/gw/v1/messages",
		"/gw/v1": "/gw/v1/messages",
	} {
		before := len(srv.Requests())
		p := NewAnthropic(Config{BaseURL: srv.URL + given, APIKey: "k", Model: "m"})
		if s, err := p.Chat(context.Background(), hello()); err == nil {
			s.Close()
		}
		reqs := srv.Requests()
		if len(reqs) != before+1 || reqs[before].Path != wantPath {
			t.Errorf("base %q: requested %q, want %q", given, reqs[len(reqs)-1].Path, wantPath)
		}
	}
}

// With cache marks Anthropic's input_tokens is only the uncached remainder.
// The input a turn used is that plus what was written to and read from the cache.
func TestAnthropicCountsCachedInputAsInput(t *testing.T) {
	decode := newAnthropicDecoder()
	if _, err := decode(ServerEvent{Data: `{"type":"message_start","message":{"usage":{"input_tokens":3,"cache_creation_input_tokens":100,"cache_read_input_tokens":2000}}}`}); err != nil {
		t.Fatal(err)
	}
	events, err := decode(ServerEvent{Data: `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`})
	if err != nil || len(events) != 1 || events[0].Usage == nil || events[0].Usage.InputTokens != 2103 || events[0].Usage.OutputTokens != 9 {
		t.Fatalf("usage: %+v %v", events, err)
	}
}

func TestAnthropicSaysAKeyIsNeededWhenNoneIsConfigured(t *testing.T) {
	srv := newAnthropicServer(t)
	srv.Status, srv.Body = 401, `{"type":"error","error":{"message":"missing key"}}`
	_, err := NewAnthropic(Config{BaseURL: srv.URL, Model: "m"}).Check(context.Background())
	if err == nil || !strings.Contains(err.Error(), "needs an API key") {
		t.Errorf("err %v", err)
	}
}

// Anthropic says an answer stopped at its ceiling with stop_reason max_tokens;
// the adapter says it the one way the loop reads, so the page is never shown
// a half answer as a whole one.
func TestAnthropicSaysWhenTheProviderCutTheAnswer(t *testing.T) {
	srv := newAnthropicServer(t)
	srv.Cut = true
	p := NewAnthropic(Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
	var done *elicore.Event
	for _, e := range drain(t, p, hello()) {
		if e.Done {
			d := e
			done = &d
		}
	}
	if done == nil || !done.Cut {
		t.Errorf("the end of a cut answer does not say it was cut: %+v", done)
	}
}

// The prefix every turn repeats is the system prompt, the tool list and the
// conversation so far. Marking those three breakpoints by default is what
// turns repeated input into cached input; the next request can then reuse
// everything up to the newest turn.
func TestAnthropicMarksCacheBreakpointsByDefault(t *testing.T) {
	srv := newAnthropicServer(t)
	p := NewAnthropic(Config{BaseURL: srv.URL, APIKey: "k", Model: "m"})
	tool := elicore.Tool{Name: "echo", Description: "Echo", Parameters: json.RawMessage(`{"type":"object"}`)}
	drain(t, p, elicore.Request{
		System: "Be brief.",
		Messages: []elicore.Message{
			{Role: elicore.RoleUser, Content: "hi"},
			{Role: elicore.RoleAssistant, Content: "hello"},
			{Role: elicore.RoleUser, Content: "Say hello"},
		},
		Tools: []elicore.Tool{tool},
	})
	body := srv.Requests()[0].Body
	system, _ := body["system"].([]any)
	if len(system) != 1 || system[0].(map[string]any)["cache_control"] == nil {
		t.Errorf("system is not a cached block: %v", body["system"])
	}
	tools, _ := body["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["cache_control"] == nil {
		t.Errorf("the last tool is not cached: %v", body["tools"])
	}
	msgs, _ := body["messages"].([]any)
	last, _ := msgs[len(msgs)-1].(map[string]any)
	blocks, _ := last["content"].([]any)
	if len(blocks) != 1 || blocks[0].(map[string]any)["cache_control"] == nil || blocks[0].(map[string]any)["text"] != "Say hello" {
		t.Errorf("the newest turn is not a cached block: %v", last["content"])
	}
}
