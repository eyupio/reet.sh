//go:build linux || darwin

package terminalagent

import (
	"context"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// handleShellCreate starts a terminal session. The caller holds the session id
// in r.held; the goroutine takes the hold over.
func (r *agentReq) handleShellCreate(op *terminalproto.Create) error {
	msg, callCtx, chats, m := r.msg, r.callCtx, r.chats, r.m
	features, send := r.features, r.send
	// Starting tmux can wait; answer from its own goroutine so the
	// dispatcher keeps serving other requests meanwhile.
	r.wg.Add(1)
	r.held = false
	go func(in terminalproto.Create) {
		defer r.wg.Done()
		defer chats.sup.unhold(msg.SessionID)
		createCtx := callCtx
		if features.Has(terminalproto.BaseFreshFeature) {
			// A worktree this create makes starts from the base as
			// GitHub has it; without the feature it starts where a
			// create always did and nothing is fetched.
			createCtx = terminalhost.WithBaseFresh(createCtx)
		}
		if features.Has(terminalproto.ProgressFeature) {
			// Each stage is told to the controller as it begins, before this create's one answer.
			// Built on createCtx, never on callCtx: a context built afresh from the call would
			// drop what this one already carries, such as the negotiated base-fresh above.
			createCtx = terminalhost.WithProgress(createCtx, func(stage terminalproto.LaunchStage) {
				// A stage note is matched to its create by request id alone. It must not
				// carry the create's session id: a launch_progress with one is not a valid
				// message, so it would be refused when encoded and never sent.
				note := terminalproto.Message{Version: msg.Version, RequestID: msg.RequestID, Generation: msg.Generation,
					Body: terminalproto.LaunchProgress{Stage: stage}}
				noteCtx, cancel := context.WithTimeout(callCtx, time.Second)
				defer cancel()
				_ = send(noteCtx, note)
			})
		}
		if in.Tool == agentsoftware.OpenCodeTool {
			release, err := lockSoftwareRead(createCtx)
			if err != nil {
				r.reply.Body = result(err)
				_ = send(callCtx, r.reply)
				return
			}
			defer release()
		}
		s, err := m.Create(createCtx, msg.RequestID, msg.SessionID, in)
		if callCtx.Err() != nil {
			return
		}
		if err != nil {
			r.reply.Body = result(err)
		} else {
			// Only a worktree create's session has a worktree to report.
			r.reply.Body = terminalproto.SessionChanged{
				State:        s.State,
				ExitCode:     s.ExitCode,
				ActivityAt:   s.ActivityAt,
				BellAt:       s.BellAt,
				ExitStatus:   s.ExitCode,
				WorktreePath: s.WorktreePath,
				WorktreeRepo: s.WorktreeRepo,
				Base:         s.Base,
			}.ForFeatures(r.featureHeader)
		}
		_ = send(callCtx, r.reply)
	}(*op)
	return nil
}

func (r *agentReq) handleShellEnd(op *terminalproto.End) error {
	msg, callCtx, m := r.msg, r.callCtx, r.m
	s, err := m.End(callCtx, msg.RequestID, msg.SessionID)
	if err != nil {
		return r.respond(err)
	}
	if op.ForgetDesk {
		// The pane is gone for good: its history file goes too. A
		// failed removal never fails the end.
		_ = m.ForgetDeskSession(callCtx, msg.SessionID)
	}
	r.reply.Body = terminalproto.SessionChanged{
		State:      s.State,
		ExitCode:   s.ExitCode,
		ActivityAt: s.ActivityAt,
		BellAt:     s.BellAt,
		ExitStatus: s.ExitCode,
	}.ForFeatures(r.featureHeader)
	return r.respond(nil)
}

func (r *agentReq) handleAttach(op *terminalproto.Attach) error {
	msg, callCtx, chats, m := r.msg, r.callCtx, r.chats, r.m
	ctx := r.ctx
	if !terminalproto.ValidID(op.StreamToken) {
		return terminalproto.ErrInvalid
	}
	attachOpts := terminalhost.AttachOptions{ID: msg.AttachmentID, Generation: msg.AttachmentGeneration, Cols: op.Cols, Rows: op.Rows, Takeover: op.Takeover, Lease: time.Duration(op.LeaseSeconds) * time.Second}
	var a *terminalhost.Attachment
	var err error
	var stopNative context.CancelFunc = func() {}
	if op.NativeView {
		native, ok := chats.nativeTerminal(msg.SessionID)
		if !ok {
			return r.respond(terminalhost.ErrSessionMissing)
		}
		var nativeCtx context.Context
		nativeCtx, stopNative = context.WithCancel(ctx)
		go func() {
			select {
			case <-native.Done:
				stopNative()
			case <-nativeCtx.Done():
			}
		}()
		a, err = m.AttachNative(nativeCtx, msg.SessionID, attachOpts, terminalhost.NativeCommand{Binary: native.Binary, Dir: native.Dir, Args: native.Args, Env: native.Env})
	} else {
		if _, chat := chats.snapshot(msg.SessionID); chat {
			return r.respond(terminalhost.ErrSessionMissing)
		}
		a, err = m.Attach(ctx, msg.SessionID, attachOpts)
	}
	if err != nil {
		stopNative()
		return r.respond(err)
	}
	r.attachments[msg.SessionID] = a
	c, err := stream(callCtx, r.client, r.headers, msg, op.StreamToken, "attach")
	if err != nil {
		a.Close()
		stopNative()
		return r.respond(terminalhost.ErrNotRunning)
	}
	r.wg.Add(1)
	go func() { defer r.wg.Done(); defer stopNative(); defer a.Close(); defer c.CloseNow(); bridge(ctx, c, a) }()
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.respond(nil)
}

func (r *agentReq) handleRenewAttachment(op *terminalproto.RenewAttachment) error {
	msg := r.msg
	err := r.m.RenewAttachment(msg.SessionID, msg.AttachmentID, msg.AttachmentGeneration, time.Duration(op.LeaseSeconds)*time.Second)
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.respond(err)
}

func (r *agentReq) handleResize(op *terminalproto.Resize) error {
	msg := r.msg
	a := r.attachments[msg.SessionID]
	valid := a != nil && a.ID == msg.AttachmentID && a.Generation == msg.AttachmentGeneration
	if !valid {
		return r.respond(terminalhost.ErrAttachmentStale)
	}
	err := a.Resize(op.Cols, op.Rows)
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.respond(err)
}

func (r *agentReq) handleDetach() error {
	msg := r.msg
	a := r.attachments[msg.SessionID]
	valid := a != nil && a.ID == msg.AttachmentID && a.Generation == msg.AttachmentGeneration
	if valid {
		a.Close()
		delete(r.attachments, msg.SessionID)
	}
	r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
	return r.respond(nil)
}
