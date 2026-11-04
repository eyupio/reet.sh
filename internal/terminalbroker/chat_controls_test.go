package terminalbroker

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestStructuredStartupEffortRequiresStartupOptionsNotTerminalPresets(t *testing.T) {
	for _, live := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "live"}[live], func(t *testing.T) {
			b, _, uid, agent := chatFixtureCaps(t, nil, func(c *terminalproto.Capabilities) {
				c.ACPLiveControls = true
				c.ACPStartOptions = live
				c.AgentsV2 = false
			})
			s, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Effort", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", Model: "provider/model", Effort: "ultra"})
			if !live {
				if !errors.Is(err, ErrLaunchOptionUnsupported) {
					t.Fatal("legacy machine admitted effort", err)
				}
				return
			}
			if err != nil || s.Effort != "ultra" || s.Model != "provider/model" {
				t.Fatalf("settings lost at admission: %+v %v", s, err)
			}
		})
	}
}

func TestStructuredEffortRefusalClearsUnconfirmedSessionSettings(t *testing.T) {
	b, _, uid, agent := chatFixtureScript(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true; c.ACPStartOptions = true }, nil, func(m terminalproto.Message) (terminalproto.Body, bool) {
		if op, ok := m.Body.(terminalproto.Create); ok && op.Structured {
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The chosen effort is unsupported for this model."}, true
		}
		return nil, true
	})
	_, err := b.Create(t.Context(), uid, agent, ID(), terminalproto.Create{Title: "Refused effort", Cols: 100, Rows: 30, Tool: "opencode", Structured: true, Trust: true, Dir: "/tmp", Model: "provider/model", Effort: "ultra"})
	if err == nil {
		t.Fatal("unsupported effort admitted")
	}
	rows, err := b.st.TerminalSessionsFor(uid)
	if err != nil || len(rows) != 1 || rows[0].Model != "" || rows[0].Effort != "" {
		t.Fatalf("unconfirmed settings retained: %+v %v", rows, err)
	}
}

func TestLiveControlsRequireNegotiationAndPersistReadback(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if _, err := b.ChatControl(t.Context(), uid, s.ID, terminalproto.ACPControl{Action: "get"}); !errors.Is(err, ErrLiveControlsUnsupported) {
		t.Fatalf("old connector admitted controls: %v", err)
	}
	b, _, uid, agent = chatFixtureCaps(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true; c.ACPStartOptions = true })
	s = chatSession(t, b, uid, agent)
	answer, err := b.ChatControl(t.Context(), uid, s.ID, terminalproto.ACPControl{Action: "set", ConfigID: "effort", Value: "high"})
	if err != nil || !answer.Controls.Working {
		t.Fatalf("working session refused controls: %+v %v", answer, err)
	}
	fresh, err := b.st.TerminalSessionFor(uid, s.ID)
	if err != nil || fresh.Model != "provider/current" || fresh.Effort != "high" {
		t.Fatalf("readback not persisted: %+v %v", fresh, err)
	}
	if _, err := b.ChatControl(t.Context(), uid+1, s.ID, terminalproto.ACPControl{Action: "get"}); err == nil {
		t.Fatal("foreign account accessed controls")
	}
}
