package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// SetupCallTimeout bounds one setup read, plan, apply or undo as the
// controller sees it. The connector answers timed_out a little sooner.
var SetupCallTimeout = time.Minute

var (
	// ErrSetupUnsupported means the machine did not negotiate the feature the
	// operation needs; nothing was sent.
	ErrSetupUnsupported = errors.New("machine does not support this setup operation")
	// ErrSetupAnswer means the machine sent a reply of another kind or for
	// another operation.
	ErrSetupAnswer = errors.New("machine sent an unexpected setup result")
)

// SetupFleet sends one setup operation to a ready connection. ErrNotSent says
// nothing reached the machine; plain ErrOffline says the connection went after
// the request was sent, so an apply may have happened.
func (b *Broker) SetupFleet(ctx context.Context, uid int64, agentID string, in terminalproto.SetupFleet) (terminalproto.SetupFleetResult, error) {
	p, err := b.peer(uid, agentID, true)
	if errors.Is(err, ErrOffline) {
		return terminalproto.SetupFleetResult{}, ErrNotSent
	}
	if err != nil {
		return terminalproto.SetupFleetResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agentID)
	if err != nil || caps.Generation != p.Generation {
		return terminalproto.SetupFleetResult{}, ErrNotSent
	}
	supported := map[string]bool{
		terminalproto.SetupOpRead:     caps.Capabilities.SetupRead,
		terminalproto.SetupOpPlan:     caps.Capabilities.SetupApply,
		terminalproto.SetupOpApply:    caps.Capabilities.SetupApply,
		terminalproto.SetupOpRollback: caps.Capabilities.SetupRollback,
	}
	if !supported[in.Op] {
		return terminalproto.SetupFleetResult{}, ErrSetupUnsupported
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{Body: in}, SetupCallTimeout)
	if err == ErrOffline {
		return terminalproto.SetupFleetResult{}, ErrNotSent
	}
	body, err := p.answer(ctx, reply, err)
	if err != nil {
		return terminalproto.SetupFleetResult{}, err
	}
	result, ok := body.(*terminalproto.SetupFleetResult)
	if !ok || result.Op != in.Op {
		return terminalproto.SetupFleetResult{}, ErrSetupAnswer
	}
	return *result, nil
}
