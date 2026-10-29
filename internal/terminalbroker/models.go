package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ModelsCallTimeout bounds one controller-initiated model listing. The
// connector's own listing deadline is shorter, so its answer arrives in time.
var ModelsCallTimeout = 12 * time.Second

var (
	// ErrModelsUnsupported means the machine did not negotiate models-v1 (an
	// older reet there); nothing was sent.
	ErrModelsUnsupported = errors.New("machine does not support reading its models")
	// ErrModelsOff means a current machine has model listing switched off.
	ErrModelsOff = errors.New("model listing is switched off on the machine")
	// ErrModelsAnswer means the machine sent a reply of another kind, or for
	// another tool, than the request.
	ErrModelsAnswer = errors.New("machine sent an unexpected model list")
)

// Models asks a machine to list its models for tool. It is sent only to a
// ready connection that reported the capability in its current generation.
// A refusal result is returned as an answer; the caller words and caches it.
// Nothing is stored here.
func (b *Broker) Models(ctx context.Context, uid int64, agentID, tool string) (terminalproto.ModelsResult, error) {
	if !terminalproto.ValidToolV2(tool) {
		return terminalproto.ModelsResult{}, terminalproto.ErrInvalid
	}
	p, err := b.peer(uid, agentID, true)
	if err != nil {
		return terminalproto.ModelsResult{}, err
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, agentID)
	if err != nil || caps.Generation != p.Generation {
		return terminalproto.ModelsResult{}, ErrOffline
	}
	if !caps.Capabilities.Models {
		// A connector that negotiated models-v1 but has it switched off says
		// so; without either it is an older reet.
		if caps.Capabilities.ModelsOff {
			return terminalproto.ModelsResult{}, ErrModelsOff
		}
		return terminalproto.ModelsResult{}, ErrModelsUnsupported
	}
	body, err := p.ask(ctx, terminalproto.Models{Tool: tool}, ModelsCallTimeout)
	if err != nil {
		return terminalproto.ModelsResult{}, err
	}
	r, ok := body.(*terminalproto.ModelsResult)
	if !ok || r.Tool != tool {
		return terminalproto.ModelsResult{}, ErrModelsAnswer
	}
	return *r, nil
}
