//go:build linux || darwin

package terminalagent

import (
	"context"
	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type liveChatControls interface {
	Controls() terminalproto.ACPControls
	SetOption(context.Context, string, string) error
	Steer(context.Context, string, []acpagent.Attachment) (string, error)
}

func (h *chatHost) control(ctx context.Context, session string, in terminalproto.ACPControl) (terminalproto.ACPControlResult, error) {
	cs := h.find(session)
	if cs == nil {
		return terminalproto.ACPControlResult{}, terminalhost.ErrSessionMissing
	}
	h.sup.mu.Lock()
	process, state, quiet := cs.process, cs.state, cs.quiet
	h.sup.mu.Unlock()
	if quiet || state != terminalproto.Running {
		return terminalproto.ACPControlResult{}, terminalhost.ErrOperationConflict
	}
	controls, ok := process.(liveChatControls)
	if !ok {
		return terminalproto.ACPControlResult{}, terminalhost.ErrLaunchUnavailable
	}
	answer := terminalproto.ACPControlResult{}
	var err error
	switch in.Action {
	case "get":
	case "set":
		wasWorking := process.Working()
		h.sup.mu.Lock()
		busyBlocking := cs.busy && !wasWorking
		inUse := cs.changing || cs.quiet
		if inUse || busyBlocking {
			h.sup.mu.Unlock()
			return answer, terminalhost.ErrOperationConflict
		}
		cs.changing = true
		h.sup.mu.Unlock()
		defer func() { h.sup.mu.Lock(); cs.changing = false; h.sup.mu.Unlock() }()

		err = controls.SetOption(ctx, in.ConfigID, in.Value)
		if err != nil {
			answer.Error = "The agent did not confirm that setting. Check its current value before trying again."
			err = nil
		}
		readback := controls.Controls()
		h.sup.mu.Lock()
		cs.model = readback.Model
		cs.effort = ""
		for _, o := range readback.Options {
			if o.Category == "thought_level" {
				cs.effort = o.Current
			}
		}
		h.sup.mu.Unlock()
		if h.sup.registry != nil {
			h.sup.registry.setControls(session, readback)
		}
		h.sup.mu.Lock()
		cs.changing = false
		h.sup.mu.Unlock()
	case "steer":
		var images []acpagent.Attachment
		if len(in.Images) > 0 {
			if !process.ImagesSupported() {
				return answer, errAttachmentUnavailable
			}
			images, err = h.sup.readAttachments(in.Images)
			if err != nil {
				return answer, err
			}
		}
		answer.Outcome, err = controls.Steer(ctx, in.Text, images)
		// An uncertain steer needs a typed outcome, not an error that a caller
		// might mistake for proof that the message was never consumed.
		if answer.Outcome == "unknown" {
			err = nil
		}
	}
	answer.Controls = controls.Controls()
	h.sup.mu.Lock()
	answer.Controls.Working = answer.Controls.Working || cs.busy
	answer.Controls.Configuring = answer.Controls.Configuring || cs.changing
	h.sup.mu.Unlock()
	return answer, err
}
