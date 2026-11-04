package terminalbroker

import (
	"context"
	"errors"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (b *Broker) queueSupported(uid int64, session string) error {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	if archived, err := b.st.TerminalArchivedFor(uid, session); err != nil {
		return err
	} else if archived {
		return store.ErrRemoteNotFound
	}
	if !s.Structured {
		return ErrACPUnsupported
	}
	caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
	if err != nil {
		return err
	}
	if !caps.Capabilities.ACPLiveControls {
		return ErrLiveControlsUnsupported
	}
	return nil
}

func (b *Broker) ChatQueue(uid int64, session string) ([]store.TerminalQueuedMessage, error) {
	if err := b.queueSupported(uid, session); err != nil {
		return nil, err
	}
	if err := b.st.RecoverTerminalQueueFor(uid, session, b.queueOwner); err != nil {
		return nil, err
	}
	b.startChatQueue(uid, session)
	return b.st.TerminalQueueFor(uid, session)
}

func (b *Broker) EnqueueChat(uid int64, session, id, text string, images []string) error {
	if err := b.queueSupported(uid, session); err != nil {
		return err
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return err
	}
	if s.State != terminalproto.Running {
		return ErrACPClosed
	}
	if err := b.st.EnqueueTerminalFor(uid, session, id, text, images); err != nil {
		return err
	}
	b.startChatQueue(uid, session)
	return nil
}

func (b *Broker) ChangeChatQueue(uid int64, session, id string, revision int64, action, text string, acknowledge bool) error {
	if err := b.queueSupported(uid, session); err != nil {
		return err
	}
	if err := b.st.ChangeTerminalQueueFor(uid, session, id, revision, action, text, acknowledge); err != nil {
		return err
	}
	b.startChatQueue(uid, session)
	return nil
}

// One worker per session outlives every browser. Claim revisions are stored
// before handing any bytes to the machine; a lost answer never causes a retry.
func (b *Broker) startChatQueue(uid int64, session string) {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil || s.State != terminalproto.Running {
		return
	}
	b.mu.Lock()
	k := chatKey{uid, session}
	if b.closed || b.queueWorkers[k] {
		b.mu.Unlock()
		return
	}
	b.queueWorkers[k] = true
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		defer func() { b.mu.Lock(); delete(b.queueWorkers, k); b.mu.Unlock() }()
		if b.st.RecoverTerminalQueueFor(uid, session, b.queueOwner) != nil {
			return
		}
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-b.queueStop:
				return
			default:
			}
			b.stepChatQueue(uid, session)
			s, err := b.st.TerminalSessionFor(uid, session)
			if err != nil || s.State != terminalproto.Running {
				return
			}
			select {
			case <-b.queueStop:
				return
			case <-ticker.C:
			}
		}
	}()
}

func (b *Broker) stepChatQueue(uid int64, session string) {
	list, err := b.st.TerminalQueueFor(uid, session)
	if err != nil || len(list) == 0 {
		return
	}
	q := list[0]
	if q.State != "queued" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	state, err := b.ChatControl(ctx, uid, session, terminalproto.ACPControl{Action: "get"})
	if err != nil {
		if errors.Is(err, ErrACPClosed) {
			_, _ = b.st.TransitionTerminalQueueFor(uid, session, q, "failed", q.Intent, "The session ended before this message was sent. Reconnect it, then retry.", b.queueOwner)
		}
		return
	}
	c := state.Controls
	if c.Configuring {
		return
	}
	if q.Intent == "stop" && c.Working {
		// Persist the cancellation intent first. On a controller restart we
		// wait for the actual turn to end rather than issuing another stop.
		ok, err := b.st.TransitionTerminalQueueFor(uid, session, q, "queued", "stopping", "Waiting for the current turn to stop.", b.queueOwner)
		if err != nil || !ok {
			return
		}
		q.Revision++
		q.Intent = "stopping"
		if err := b.CancelChat(ctx, uid, session); err != nil {
			_, _ = b.st.TransitionTerminalQueueFor(uid, session, q, "failed", "wait", "The stop was not confirmed. Check the conversation before retrying.", b.queueOwner)
		}
		return
	}
	if c.Waiting || c.Compacting {
		return
	}
	steer := q.Intent == "send" && c.Working
	if c.Working && !steer {
		return
	}
	if steer && !c.Steering {
		_, _ = b.st.TransitionTerminalQueueFor(uid, session, q, "failed", "wait", "This agent cannot accept a message during its turn. Retry between turns, or use Stop and send.", b.queueOwner)
		return
	}
	ok, err := b.st.TransitionTerminalQueueFor(uid, session, q, "sending", q.Intent, "Delivering…", b.queueOwner)
	if err != nil || !ok {
		return
	}
	q.Revision++
	q.State = "sending"
	finish := func(state, note string) {
		_, _ = b.st.TransitionTerminalQueueFor(uid, session, q, state, "wait", note, b.queueOwner)
	}
	if steer {
		answer, err := b.ChatControl(ctx, uid, session, terminalproto.ACPControl{Action: "steer", Text: q.Text, Images: q.Images})
		if err != nil {
			finish("uncertain", "Delivery was not confirmed. Check the conversation before retrying.")
			return
		}
		switch answer.Outcome {
		case "injected":
			b.chatLog(uid, session).publish(terminalproto.ACPEvent{Kind: "user", Text: queueEcho(q)})
			finish("sent", "")
			return
		case "promptRequired": // Explicitly not consumed; safe to submit once as a new turn.
		case "rejected":
			finish("failed", "The agent refused this message during its turn. Retry between turns, or use Stop and send.")
			return
		default:
			finish("uncertain", "The agent did not confirm delivery. Check the conversation before retrying.")
			return
		}
	}
	err = b.PromptChat(ctx, uid, session, q.Text, q.Images)
	if err == nil {
		finish("sent", "")
		return
	}
	if errors.Is(err, ErrACPBusy) || errors.Is(err, ErrACPClosed) || errors.Is(err, ErrACPModeUnknown) || errors.Is(err, ErrACPImageUnavailable) || errors.Is(err, ErrACPImagesUnsupported) {
		finish("failed", err.Error())
		return
	}
	finish("uncertain", "Delivery was not confirmed. Check the conversation before retrying.")
}

func queueEcho(q store.TerminalQueuedMessage) string {
	if q.Text == "" && len(q.Images) > 0 {
		return "Image attached."
	}
	return q.Text
}
