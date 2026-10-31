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
	p, err := b.peer(uid, agentID, true)
	if err != nil {
		return terminalproto.SoftwareApplyResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agentID)
	if err != nil || caps.Generation != p.Generation {
		return terminalproto.SoftwareApplyResult{}, ErrOffline
	}
	if !caps.Capabilities.SoftwareApply {
		return terminalproto.SoftwareApplyResult{}, ErrSoftwareApplyUnsupported
	}
	body, err := p.ask(ctx, in, SoftwareApplyTimeout)
	if err != nil {
		return terminalproto.SoftwareApplyResult{}, err
	}
	result, ok := body.(*terminalproto.SoftwareApplyResult)
	if !ok || result.Tool != in.Tool || result.Action != in.Action {
		return terminalproto.SoftwareApplyResult{}, ErrSoftwareApplyAnswer
	}
	return *result, nil
}
