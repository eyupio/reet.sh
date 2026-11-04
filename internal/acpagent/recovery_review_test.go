//go:build linux || darwin

package acpagent

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAgentExitDrainsFinalFrames(t *testing.T) {
	opts, got := options(t, "exit-final-frames", testVersion)
	opts.TurnTimeout = time.Minute
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	opts.OnEvent = func(e Event) {
		got.add(e)
		if e.Kind == "agent" && e.Text == "first final chunk" {
			<-release
		}
	}
	s, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	timer := time.AfterFunc(200*time.Millisecond, unblock)
	defer timer.Stop()
	_ = s.Prompt(t.Context(), "finish then exit")
	select {
	case <-s.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("agent did not finish")
	}
	if !got.text("first final chunk") || !got.text("second final chunk") {
		t.Fatal("agent exit discarded a flushed final chunk")
	}
}

func TestAgentExitBoundsBlockedOutputDelivery(t *testing.T) {
	opts, _ := options(t, "exit-final-frames", testVersion)
	opts.TurnTimeout = time.Minute
	release := make(chan struct{})
	defer close(release)
	opts.OnEvent = func(e Event) {
		if e.Kind == "agent" && e.Text == "first final chunk" {
			<-release
		}
	}
	s, err := Start(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	go func() { _ = s.Prompt(t.Context(), "finish then exit") }()
	select {
	case <-s.Done():
	case <-time.After(4 * time.Second):
		t.Fatal("blocked event delivery hid agent exit")
	}
}

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

func TestLeaderExitWithInheritedOutputEndsChat(t *testing.T) {
	for _, scenario := range []string{"leader-exit-open-stdout", "leader-exit-open-stderr"} {
		t.Run(scenario, func(t *testing.T) {
			opts, _ := options(t, scenario, testVersion)
			opts.TurnTimeout = time.Minute
			s, err := Start(t.Context(), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			go func() { _ = s.Prompt(t.Context(), "exit with a child") }()
			select {
			case <-s.Done():
			case <-time.After(3 * time.Second):
				t.Fatal("leader exit was hidden by a child holding output open")
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
