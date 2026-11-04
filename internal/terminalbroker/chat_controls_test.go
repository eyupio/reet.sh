package terminalbroker

import (
	"errors"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"testing"
)

func TestLiveControlsRequireNegotiationAndPersistReadback(t *testing.T) {
	b, _, uid, agent := chatFixture(t)
	s := chatSession(t, b, uid, agent)
	if _, err := b.ChatControl(t.Context(), uid, s.ID, terminalproto.ACPControl{Action: "get"}); !errors.Is(err, ErrLiveControlsUnsupported) {
		t.Fatalf("old connector admitted controls: %v", err)
	}
	b, _, uid, agent = chatFixtureCaps(t, nil, func(c *terminalproto.Capabilities) { c.ACPLiveControls = true })
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
