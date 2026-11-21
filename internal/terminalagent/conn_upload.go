//go:build linux || darwin

package terminalagent

import (
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func (r *agentReq) handleUpload(op *terminalproto.Upload) error {
	msg, callCtx, opts, m := r.msg, r.callCtx, r.opts, r.m
	// The local switches are checked before the manager: a machine
	// that turned uploads or terminal access off answers at once,
	// without opening a stream.
	if !allowed(opts.Consent) || !allowed(opts.UploadConsent) {
		r.reply.Body = terminalproto.Result{Code: terminalproto.UploadsOff}
		return r.send(callCtx, r.reply)
	}
	if r.managerErr != nil {
		r.reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
		return r.send(callCtx, r.reply)
	}
	if !r.uploads.acquire() {
		r.reply.Body = terminalproto.Result{Code: terminalproto.UploadBusy}
		return r.send(callCtx, r.reply)
	}
	// The running check and the stream dial touch tmux and the
	// network; keep them off the dispatcher so another request is
	// not held behind them.
	r.wg.Add(1)
	go func(in terminalproto.Upload) {
		defer r.wg.Done()
		defer r.uploads.release()
		if !in.Launch {
			sessions, err := m.List(callCtx)
			if err != nil {
				r.reply.Body = result(err)
				_ = r.send(callCtx, r.reply)
				return
			}
			running := false
			for _, s := range sessions {
				if s.ID == msg.SessionID && s.State == terminalproto.Running {
					running = true
					break
				}
			}
			if !running {
				r.reply.Body = terminalproto.Result{Code: terminalproto.NotFound}
				_ = r.send(callCtx, r.reply)
				return
			}
		}
		c, err := dialUploadStream(callCtx, r.client, r.headers, msg, in.StreamToken, terminalproto.UploadOperation)
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
		serveUpload(callCtx, c, m, msg.SessionID, in)
	}(*op)
	return nil
}
