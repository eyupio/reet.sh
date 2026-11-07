package terminalbroker

import (
	"context"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// NativeViewInfo is scoped by the same account/session and live connector as
// attachments. Availability describes this process, not just installed software.
func (b *Broker) NativeViewInfo(ctx context.Context, uid int64, session string) (terminalproto.NativeViewResult, error) {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return terminalproto.NativeViewResult{}, ErrDenied
	}
	unavailable := terminalproto.NativeViewResult{Reason: "This agent session has no shared native terminal."}
	if !s.Structured || s.State != terminalproto.Running || s.Tool != "opencode" {
		return unavailable, nil
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return terminalproto.NativeViewResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation || !caps.Capabilities.NativeView {
		return unavailable, nil
	}
	reply, err := p.call(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: terminalproto.NativeView{}})
	if err != nil {
		return terminalproto.NativeViewResult{}, err
	}
	if answer, ok := reply.Body.(*terminalproto.NativeViewResult); ok {
		return *answer, nil
	}
	return unavailable, nil
}
