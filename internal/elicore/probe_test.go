package elicore

import (
	"context"
	"encoding/json"
	"testing"
)

func TestReusedEliProviderContract(t *testing.T) {
	RunContractTests(t, "reet-spike", func(t *testing.T) Provider { return NewFake(FakeOptions{}) })
}

// This disposable probe demonstrates a read result returning through Eli's
// contract. It grants no browser, MCP or machine access and dispatches no work.
func TestPermittedReadReturnsThroughTheProviderContract(t *testing.T) {
	reads := 0
	provider := NewFake(FakeOptions{Script: func(req Request) []Event {
		if len(req.Messages) == 1 {
			return []Event{{ToolCall: &ToolCall{ID: "read-1", Name: "read_permitted_work", Arguments: json.RawMessage(`{}`)}}}
		}
		last := req.Messages[len(req.Messages)-1]
		if last.Role != RoleTool || last.ToolCallID != "read-1" || last.Content != `{"work":[]}` {
			t.Fatalf("unbound tool result: %+v", last)
		}
		return []Event{{Delta: "No work is available."}}
	}})
	req := Request{Messages: []Message{{Role: RoleUser, Content: "Read my work"}}, Tools: []Tool{{Name: "read_permitted_work", Parameters: json.RawMessage(`{"type":"object"}`)}}}
	events := drain(t, provider, req)
	var call *ToolCall
	for _, event := range events {
		if event.ToolCall != nil {
			call = event.ToolCall
		}
	}
	if call == nil || call.Name != req.Tools[0].Name || string(call.Arguments) != "{}" {
		t.Fatalf("unoffered or invalid read: %+v", call)
	}
	// The harness executes only its fixed, synthetic read, after checking the offer.
	reads++
	req.Messages = append(req.Messages, Message{Role: RoleAssistant, ToolCalls: []ToolCall{*call}}, Message{Role: RoleTool, ToolCallID: call.ID, Content: `{"work":[]}`})
	req.Tools = nil
	result := drain(t, provider, req)
	if reads != 1 || result[0].Delta != "No work is available." {
		t.Fatalf("read result did not return: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := provider.Chat(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	cancel()
	event, ok := stream.Next(ctx)
	if !ok || event.Err != context.Canceled {
		t.Fatalf("cancelled stream: %+v, %v", event, ok)
	}
}
