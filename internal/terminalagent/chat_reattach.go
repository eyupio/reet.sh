//go:build linux || darwin

package terminalagent

import (
	"fmt"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// errReattachStarting refuses a reattach of a chat this machine is still
// starting (a create, a restart restore or another reattach). It is an
// ErrOperationConflict that result answers with the detail
// terminalproto.ConflictStarting, so the controller can say to try again in a
// moment rather than that another session owns the conversation.
var errReattachStarting = fmt.Errorf("%w: the chat is still starting", terminalhost.ErrOperationConflict)

// reattach brings a lost or crashed chat back under the controller's existing
// session id (chat_reattach). A reviewed adapter that sets Spec.Reattach keeps
// the conversation, so the chat is started again with session/load of
// in.ACPSessionID; the replay that produces is dropped because the controller
// keeps its own transcript.
//
// A chat this machine still runs under sessionID is adopted as it is: no
// restart and no second session/load. An exited receipt left for sessionID is
// replaced. A conversation another session still owns and a retired process
// that may still be alive are refused with ErrOperationConflict; a start in
// flight for sessionID with errReattachStarting. A retry with the same requestID answers
// from the first attempt's receipt.
func (h *chatHost) reattach(requestID, sessionID string, in terminalproto.ChatReattach) (terminalproto.SessionChanged, error) {
	if spec, ok := acpagent.Lookup(in.Tool); !ok || !spec.Reattach {
		return terminalproto.SessionChanged{}, acpagent.ErrUnsupported
	}
	h.sup.mu.Lock()
	if h.sup.closed {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
	}
	if _, ok := h.sup.requests[requestID]; !ok {
		if cs := h.sup.sessions[sessionID]; cs != nil {
			switch {
			case cs.stray || cs.quiet:
				h.sup.mu.Unlock()
				return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
			case cs.state == terminalproto.Running && cs.process != nil:
				h.sup.requests[requestID] = sessionID
				acpID, tool := cs.acpID, cs.tool
				history := h.link.history
				h.sup.mu.Unlock()
				return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: reportedACP(tool, acpID, history)}, nil
			case cs.state.ReservesQuota():
				// Still starting (a create, a restart restore or another
				// reattach): the controller retries once it settles.
				h.sup.mu.Unlock()
				return terminalproto.SessionChanged{}, errReattachStarting
			default:
				// An ended chat keeps its receipt to answer a lost reply. The
				// controller is asking for this id again, so that receipt is
				// stale: drop it and its request ids before starting.
				h.sup.dropLocked(sessionID)
			}
		}
		if h.sup.conversationInUseLocked(in.Tool, in.ACPSessionID) {
			h.sup.mu.Unlock()
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
	}
	h.sup.mu.Unlock()
	// openChat checks again under its own lock and answers a known requestID
	// from its receipt, so a race since the checks above is still refused.
	return h.openChat(requestID, sessionID, terminalproto.Create{
		Tool: in.Tool, Dir: in.Dir, Model: in.Model, Effort: in.Effort, Yolo: in.Yolo, Trust: in.Trust,
		Structured: true, Title: in.Title, ResumeACP: in.ACPSessionID,
	}, true)
}
