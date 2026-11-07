//go:build linux || darwin

package terminalagent

import (
	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// A native terminal spec is obtainable only from this supervisor's exact live
// chat. It never comes from a controller, session title or terminal output.
func (h *chatHost) nativeTerminal(sessionID string) (acpagent.NativeTerminalSpec, bool) {
	h.sup.mu.Lock()
	cs := h.sup.sessions[sessionID]
	if cs == nil || cs.state != terminalproto.Running || cs.quiet || h.sup.closed {
		h.sup.mu.Unlock()
		return acpagent.NativeTerminalSpec{}, false
	}
	process := cs.process
	h.sup.mu.Unlock()
	adapter, ok := process.(interface {
		NativeTerminal() (acpagent.NativeTerminalSpec, bool)
	})
	if !ok {
		return acpagent.NativeTerminalSpec{}, false
	}
	return adapter.NativeTerminal()
}
