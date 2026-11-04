package terminalbroker

import (
	"context"
	"errors"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrLiveControlsUnsupported = errors.New("Update reet on this machine to use live chat controls.")

// ChatControl uses session runtime capabilities, never static option guesses.
// Uncertain steering results are returned unchanged for queue reconciliation.
func (b *Broker) ChatControl(ctx context.Context, uid int64, session string, in terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	if in.Action == "get" || in.Action == "set" {
		log := b.chatLog(uid, session)
		log.controlMu.Lock()
		defer log.controlMu.Unlock()
	}
	s, err := b.structuredSession(uid, session)
	if err != nil {
		return terminalproto.ACPControlResult{}, err
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return terminalproto.ACPControlResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil || caps.Generation != p.Generation {
		return terminalproto.ACPControlResult{}, ErrOffline
	}
	if _, ok := caps.Capabilities.ChatAgent(s.Tool); !ok || !caps.Capabilities.ACPLiveControls {
		return terminalproto.ACPControlResult{}, ErrLiveControlsUnsupported
	}
	if len(in.Images) > 0 && !caps.Capabilities.ACPImages {
		return terminalproto.ACPControlResult{}, ErrACPImagesUnsupported
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: ID(), SessionID: session, Body: &in}, chatCallTimeout)
	if err != nil {
		if in.Action == "set" {
			_ = b.st.RecordTerminalControlsFor(uid, session, "", "")
		}
		return terminalproto.ACPControlResult{}, err
	}
	answer, ok := reply.Body.(*terminalproto.ACPControlResult)
	if !ok {
		if err := chatAck(reply, nil); err != nil {
			return terminalproto.ACPControlResult{}, err
		}
		return terminalproto.ACPControlResult{}, ErrACPBusy
	}
	if (in.Action == "set" || in.Action == "get") && !answer.Controls.Configuring {
		effort := ""
		for _, o := range answer.Controls.Options {
			if o.Category == "thought_level" {
				effort = o.Current
			}
		}
		if err = b.st.RecordTerminalControlsFor(uid, session, answer.Controls.Model, effort); err != nil {
			return *answer, err
		}
	}
	return *answer, nil
}
