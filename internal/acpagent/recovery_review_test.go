//go:build linux || darwin

package acpagent

import (
	"context"
	"testing"
	"time"
)

func TestBrokenOutputStreamEndsChat(t *testing.T) {
	for _, scenario := range []string{"stdout-eof", "stdout-overflow"} {
		t.Run(scenario, func(t *testing.T) {
			opts, _ := options(t, scenario, testVersion)
			opts.TurnTimeout = time.Minute
			s, err := Start(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			prompt := make(chan error, 1)
			go func() { prompt <- s.Prompt(context.Background(), "go") }()
			select {
			case <-s.Done():
			case <-time.After(time.Second):
				t.Fatal("agent with an unusable output stream remained alive")
			}
			select {
			case err := <-prompt:
				if err == nil {
					t.Fatal("broken stream reported a successful prompt")
				}
			case <-time.After(time.Second):
				t.Fatal("prompt did not unblock after stream failure")
			}
		})
	}
}

func TestRecoveredProviderErrorRetiresUnresponsiveAgent(t *testing.T) {
	opts, got := options(t, "ignore-cancel", testVersion)
	opts.TurnTimeout = time.Minute
	opts.ForceStopGrace = 40 * time.Millisecond
	opts.Agent.RecoverLive = func(_, _ string, _ int64) string { return "provider unavailable" }
	s, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	prompt := make(chan error, 1)
	go func() { prompt <- s.Prompt(context.Background(), "go") }()
	select {
	case <-s.Done():
	case <-time.After(time.Second):
		t.Fatal("provider error left an unresponsive turn alive")
	}
	select {
	case <-prompt:
	case <-time.After(time.Second):
		t.Fatal("provider error left Prompt blocked")
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	found := false
	for _, e := range got.all {
		if e.Kind == "status" && e.State == "error" && e.Detail == "provider unavailable" {
			found = true
		}
		if e.Kind == "status" && e.State == "cancelled" {
			t.Fatal("provider error replaced by cancellation")
		}
	}
	if !found {
		t.Fatal("provider failure was not reported")
	}
}
