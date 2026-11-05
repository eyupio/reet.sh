package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// SoftwareApplyTimeout bounds one install or update. The connector's own
// download deadline is shorter, so a timeout is reported rather than raced by
// a late install.
var SoftwareApplyTimeout = 21 * time.Minute

var (
	// ErrSoftwareApplyUnsupported means the machine did not negotiate
	// software-apply-v1; nothing was sent.
	ErrSoftwareApplyUnsupported = errors.New("machine does not support software maintenance")
	// ErrSoftwareApplyAnswer means the machine sent a reply of another kind,
	// for another tool, or for another action than the request.
	ErrSoftwareApplyAnswer = errors.New("machine sent an unexpected software result")
)

// SoftwareApply sends one reviewed install or update to a ready connection.
// The request names only the tool, action and approved evidence; the URL,
// checksums and command stay in the connector.
func (b *Broker) SoftwareApply(ctx context.Context, uid int64, agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error) {
	if !terminalproto.ValidToolV2(in.Tool) {
		return terminalproto.SoftwareApplyResult{}, terminalproto.ErrInvalid
	}
	// ErrNotSent says nothing reached the machine, which a caller may rely on to
	// leave the work for later. Plain ErrOffline says only that the connection is
	// gone, and after the request went out the machine may have acted on it.
	p, err := b.peer(uid, agentID, true)
	if errors.Is(err, ErrOffline) {
		return terminalproto.SoftwareApplyResult{}, ErrNotSent
	}
	if err != nil {
		return terminalproto.SoftwareApplyResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agentID)
	if err != nil || caps.Generation != p.Generation {
		return terminalproto.SoftwareApplyResult{}, ErrNotSent
	}
	if !caps.Capabilities.SoftwareApply {
		return terminalproto.SoftwareApplyResult{}, ErrSoftwareApplyUnsupported
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{Body: in}, SoftwareApplyTimeout)
	if err == ErrOffline {
		// callWithin reports a request it could not send as plain ErrOffline.
		return terminalproto.SoftwareApplyResult{}, ErrNotSent
	}
	body, err := p.answer(ctx, reply, err)
	if err != nil {
		return terminalproto.SoftwareApplyResult{}, err
	}
	result, ok := body.(*terminalproto.SoftwareApplyResult)
	valid := ok && result.Tool == in.Tool && result.Action == in.Action
	if !valid {
		return terminalproto.SoftwareApplyResult{}, ErrSoftwareApplyAnswer
	}
	return *result, nil
}
