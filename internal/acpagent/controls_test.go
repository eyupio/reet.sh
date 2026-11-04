//go:build linux || darwin

package acpagent

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

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
	opts.Effort = "invented"
	if other, err := Start(t.Context(), opts); err == nil {
		other.Close()
		t.Fatal("unsupported saved effort admitted")
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
