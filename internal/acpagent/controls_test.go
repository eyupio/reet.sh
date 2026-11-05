//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
	"time"
)

func TestSettingReplyCannotOverwriteALaterConfigurationUpdate(t *testing.T) {
	s, done := runningControlSession(t, "live-controls")
	id := s.nextRequestID()
	reply := make(chan rpcResult, 1)
	s.mu.Lock()
	s.calls[id] = rpcCall{reply: reply, apply: func(raw json.RawMessage) {
		_ = s.applySettingReply(terminalproto.ACPSetting{ID: "effort", Category: "thought_level"}, "high", raw)
	}}
	s.mu.Unlock()
	encodedID, _ := json.Marshal(id)
	s.handle(rpcMessage{ID: encodedID, Result: json.RawMessage(`{"configOptions":[{"id":"effort","category":"thought_level","currentValue":"high","options":[{"value":"high"},{"value":"low"}]}]}`)})
	s.handleUpdate(json.RawMessage(`{"update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"effort","category":"thought_level","currentValue":"low","options":[{"value":"high"},{"value":"low"}]}]}}`))
	<-reply // The caller is deliberately scheduled after the following update.
	if current := s.Controls().Options[0].Current; current != "low" {
		t.Fatalf("late caller restored stale effort %q", current)
	}
	s.Cancel()
	<-done
}

func TestLiveSelectorsInvalidateUnsafeOrMalformedReplacements(t *testing.T) {
	s, done := runningControlSession(t, "live-controls")
	s.updateSettings(json.RawMessage(`{"configOptions":[{"id":"mode","category":"thought_level","currentValue":"yolo","options":[]}]}`))
	if len(s.Controls().Options) != 0 {
		t.Fatal("permission selector admitted")
	}
	s.updateSettings(json.RawMessage(`{"configOptions":[{"id":"effort","category":"thought_level","currentValue":"low","options":[{"value":"low"}]}]}`))
	s.updateSettings(json.RawMessage(`{"configOptions":"invalid"}`))
	if len(s.Controls().Options) != 0 || s.Controls().Model != "" {
		t.Fatal("malformed replacement retained stale settings")
	}
	s.Cancel()
	<-done
}

func TestClineSettingsSkipValuesReetCannotName(t *testing.T) {
	s, done := runningControlSession(t, "live-controls")
	s.updateSettings(json.RawMessage(`{"configOptions":[
		{"type":"select","id":"provider","category":"_provider","currentValue":"cline","options":[{"value":"cline"}]},
		{"type":"select","id":"model","category":"model","currentValue":"anthropic/claude-sonnet-5.5","options":[{"value":"~alias/latest"},{"value":"anthropic/claude-sonnet-5.5"}]},
		{"type":"select","id":"mode","category":"mode","currentValue":"act","options":[{"value":"act"},{"value":"plan"}]},
		{"type":"boolean","id":"auto_approve","currentValue":false}
	]}`))
	controls := s.Controls()
	if len(controls.Options) != 1 || controls.Options[0].Category != "model" || controls.Model != "anthropic/claude-sonnet-5.5" {
		t.Fatalf("controls: %+v", controls)
	}
	if len(controls.Options[0].Values) != 1 || controls.Options[0].Values[0] != "anthropic/claude-sonnet-5.5" {
		t.Fatalf("values: %+v", controls.Options[0].Values)
	}
	// An unrepresentable current value keeps the selector but claims no value.
	s.updateSettings(json.RawMessage(`{"configOptions":[{"id":"model","category":"model","currentValue":"~alias/latest","options":[{"value":"~alias/latest"},{"value":"anthropic/claude-sonnet-5.5"}]}]}`))
	if controls := s.Controls(); len(controls.Options) != 1 || controls.Options[0].Current != "" || controls.Model != "" {
		t.Fatalf("unrepresentable current: %+v", controls)
	}
	s.Cancel()
	<-done
}

func TestSteerWaitsForCompactionAndApprovals(t *testing.T) {
	s, done := runningControlSession(t, "live-controls")
	s.handleUpdate(json.RawMessage(`{"update":{"sessionUpdate":"compaction_update","status":"in_progress"}}`))
	if outcome, err := s.Steer(t.Context(), "later", nil); err != nil || outcome != "rejected" {
		t.Fatal(outcome, err)
	}
	s.handleUpdate(json.RawMessage(`{"update":{"sessionUpdate":"compaction_update","status":"completed"}}`))
	s.mu.Lock()
	s.pending["permission"] = &pendingPermission{}
	s.mu.Unlock()
	if outcome, err := s.Steer(t.Context(), "later", nil); err != nil || outcome != "rejected" {
		t.Fatal(outcome, err)
	}
	s.mu.Lock()
	delete(s.pending, "permission")
	s.mu.Unlock()
	s.Cancel()
	<-done
}

func TestSavedEffortIsConfirmedBeforeTheSessionIsReady(t *testing.T) {
	opts, _ := options(t, "live-controls", testVersion)
	opts.Effort = "high"
	s, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Controls().Options[1].Current != "high" {
		t.Fatal("saved effort was not applied")
	}
	ready := false
	opts.OnReady = func() { ready = true }
	opts.Effort = "invented"
	if other, err := Start(t.Context(), opts); err == nil {
		other.Close()
		t.Fatal("unsupported saved effort admitted")
	}
	if ready {
		t.Fatal("unconfirmed effort announced the session ready")
	}
}

func runningControlSession(t *testing.T, scenario string) (*Session, <-chan error) {
	t.Helper()
	opts, _ := options(t, scenario, testVersion)
	opts.TurnTimeout = time.Minute
	s, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	done := make(chan error, 1)
	go func() { done <- s.Prompt(t.Context(), "work") }()
	deadline := time.Now().Add(time.Second)
	for !s.Working() {
		if time.Now().After(deadline) {
			t.Fatal("turn did not start")
		}
		time.Sleep(time.Millisecond)
	}
	return s, done
}

func TestLiveSettingsDoNotEndOrReplaceTheRunningTurn(t *testing.T) {
	s, done := runningControlSession(t, "live-controls")
	before := s.SessionID()
	if err := s.SetOption(t.Context(), "model", "provider/other"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOption(t.Context(), "effort", "high"); err != nil {
		t.Fatal(err)
	}
	state := s.Controls()
	if !state.Working || state.Model != "provider/other" || state.Options[1].Current != "high" || s.SessionID() != before {
		t.Fatalf("live switch changed turn ownership: %+v", state)
	}
	if err := s.SetOption(t.Context(), "mode", "agent-full-access"); err == nil {
		t.Fatal("permission mode passed through generic settings")
	}
	if err := s.SetOption(t.Context(), "effort", "invented"); err == nil {
		t.Fatal("unadvertised effort was accepted")
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("turn did not cancel")
	}
}

func TestCancelRemainsResponsiveDuringAnUnansweredSetting(t *testing.T) {
	s, done := runningControlSession(t, "live-controls-hang")
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	changed := make(chan error, 1)
	go func() { changed <- s.SetOption(ctx, "model", "provider/other") }()
	deadline := time.Now().Add(time.Second)
	for !s.Controls().Configuring {
		if time.Now().After(deadline) {
			t.Fatal("setting did not start")
		}
		time.Sleep(time.Millisecond)
	}
	if err := s.Cancel(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("configuration blocked cancellation")
	}
	if err := <-changed; err == nil {
		t.Fatal("unanswered setting was confirmed")
	}
	if s.Model() != "" {
		t.Fatal("lost setting reply kept an obsolete confirmed model")
	}
}

func TestSteeringOwnsNoAdditionalTurnAndReportsDelivery(t *testing.T) {
	for _, tc := range []struct{ scenario, want string }{{"live-controls", "injected"}, {"live-controls-idle-race", "promptRequired"}, {"live-controls-unknown", "unknown"}} {
		t.Run(tc.scenario, func(t *testing.T) {
			s, done := runningControlSession(t, tc.scenario)
			outcome, err := s.Steer(t.Context(), "also run tests", nil)
			if err != nil || outcome != tc.want {
				t.Fatalf("outcome=%q err=%v", outcome, err)
			}
			select {
			case <-done:
				t.Fatal("steer ended the original turn")
			default:
			}
			if !s.Working() {
				t.Fatal("steer reset working state")
			}
			_ = s.Cancel()
			<-done
		})
	}
}
