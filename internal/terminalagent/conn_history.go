//go:build linux || darwin

package terminalagent

import (
	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// handleHistory serves a terminal's scrollback or a shell's command history:
// op is a *terminalproto.History or a *terminalproto.CommandHistory.
func (r *agentReq) handleHistory(op terminalproto.Body) error {
	msg, callCtx, m := r.msg, r.callCtx, r.m
	var token string
	switch request := op.(type) {
	case *terminalproto.History:
		token = request.StreamToken
	case *terminalproto.CommandHistory:
		token = request.StreamToken
	}
	if !terminalproto.ValidID(token) {
		return terminalproto.ErrInvalid
	}
	// Reading the terminal and dialing the stream can wait on tmux
	// or the network; do both off the dispatcher. The answer is sent
	// before the bytes so the controller is already reading.
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		var data []byte
		var err error
		switch request := op.(type) {
		case *terminalproto.History:
			data, err = m.History(callCtx, msg.SessionID, request.Lines)
		case *terminalproto.CommandHistory:
			data, err = m.CommandHistoryFor(callCtx, msg.SessionID, r.features.Has(terminalproto.DeskShellFeature))
		}
		if err != nil {
			r.reply.Body = result(err)
			_ = r.send(callCtx, r.reply)
			return
		}
		c, err := stream(callCtx, r.client, r.headers, msg, token, "history")
		if err != nil {
			r.reply.Body = result(terminalhost.ErrNotRunning)
			_ = r.send(callCtx, r.reply)
			return
		}
		defer c.CloseNow()
		r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
		if err := r.send(callCtx, r.reply); err != nil {
			return
		}
		for len(data) > 0 {
			n := min(len(data), terminalproto.MaxBinaryBytes)
			if write(callCtx, c, data[:n]) != nil {
				return
			}
			data = data[n:]
		}
		_ = c.Close(websocket.StatusNormalClosure, "")
	}()
	return nil
}
