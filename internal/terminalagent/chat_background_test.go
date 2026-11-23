package terminalagent

import (
	"testing"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func TestBackgroundCountFollowsTheNegotiatedFeature(t *testing.T) {
	for _, background := range []bool{true, false} {
		h, proc, out := toolDetailHost(t, true)
		h.link.background = background
		proc.onEvent(acpagent.Event{Kind: "status", State: "idle", Background: 2})
		var seen *terminalproto.ACPEvent
		out.mu.Lock()
		for _, m := range out.msgs {
			if ev, ok := m.Body.(*terminalproto.ACPEvent); ok && ev.Kind == "status" && ev.State == "idle" {
				copy := *ev
				seen = &copy
			}
		}
		out.mu.Unlock()
		if seen == nil {
			t.Fatalf("background=%v: no idle status", background)
		}
		want := 0
		if background {
			want = 2
		}
		if seen.Background != want {
			t.Fatalf("background=%v: sent count %d, want %d", background, seen.Background, want)
		}
	}
}

func TestControlsBackgroundStrippedForAPeerWithoutTheFeature(t *testing.T) {
	state := terminalproto.ACPControls{Background: 3, Options: []terminalproto.ACPSetting{}}
	if got := (&chatHost{link: &chatLink{catalogue: true, provider: true}}).forConnection(state); got.Background != 0 {
		t.Fatalf("an old controller was sent background %d", got.Background)
	}
	if got := (&chatHost{link: &chatLink{catalogue: true, provider: true, background: true}}).forConnection(state); got.Background != 3 {
		t.Fatalf("background = %d, want 3", got.Background)
	}
}
