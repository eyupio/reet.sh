//go:build linux || darwin

// Package terminalagent dispatches typed private operations to the locally
// consented host manager. It exposes no general command or network forwarding.
package terminalagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func Factory(opts terminalhost.ManagerOptions) agenthealth.TerminalFactory {
	return func(ctx context.Context, client *http.Client, headers http.Header, generation int64, send agenthealth.Sender) (func(context.Context, terminalproto.Message) error, func()) {
		ctx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		m, managerErr := terminalhost.NewManager(ctx, opts)
		if managerErr == nil {
			wg.Add(1)
			go func() { defer wg.Done(); _ = m.Run(ctx) }()
			wg.Add(1)
			go func() {
				defer wg.Done()
				observed := map[string]terminalproto.SessionChanged{}
				initial := true
				for {
					changed := m.Changed()
					sessions, err := m.List(ctx)
					if err != nil {
						return
					}
					for _, s := range sessions {
						state := terminalproto.SessionChanged{State: s.State, ExitCode: s.ExitCode}
						old, known := observed[s.ID]
						sameCode := old.ExitCode == nil && state.ExitCode == nil || old.ExitCode != nil && state.ExitCode != nil && *old.ExitCode == *state.ExitCode
						if !initial && (!known || old.State != state.State || !sameCode) {
							if send(ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: s.ID, Generation: generation, Body: state}) != nil {
								return
							}
						}
						observed[s.ID] = state
					}
					initial = false
					select {
					case <-ctx.Done():
						return
					case <-changed:
					}
				}
			}()
		}
		attachments := map[string]*terminalhost.Attachment{}
		// Dispatch is called by one control reader; only stream workers use their
		// own immutable attachment. Cleanup happens after that reader has stopped.
		handler := func(callCtx context.Context, msg terminalproto.Message) error {
			if msg.Generation != generation {
				return terminalproto.ErrInvalid
			}
			reply := msg
			reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
			respond := func(err error) error {
				if err != nil {
					reply.Body = terminalproto.Result{Code: code(err)}
				}
				return send(callCtx, reply)
			}
			if managerErr != nil {
				return send(callCtx, reply)
			}
			switch op := msg.Body.(type) {
			case *terminalproto.Reconcile:
				if op.Complete || len(op.Sessions) != 0 {
					return terminalproto.ErrInvalid
				}
				if err := m.Reconcile(callCtx); err != nil {
					return respond(err)
				}
				sessions, err := m.List(callCtx)
				if err != nil {
					return respond(err)
				}
				// A complete snapshot covers every live shell (at most eight) and recent
				// final receipts. Older absent controller records safely become lost.
				sort.Slice(sessions, func(i, j int) bool {
					if sessions[i].State.ReservesQuota() != sessions[j].State.ReservesQuota() {
						return sessions[i].State.ReservesQuota()
					}
					return sessions[i].UpdatedAt > sessions[j].UpdatedAt
				})
				if len(sessions) > 32 {
					sessions = sessions[:32]
				}
				snapshot := terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
				for _, s := range sessions {
					snapshot.Sessions = append(snapshot.Sessions, terminalproto.SessionSnapshot{ID: s.ID, Title: s.Title, State: s.State, ExitCode: s.ExitCode})
				}
				reply.Body = snapshot
				return respond(nil)
			case *terminalproto.Create:
				s, err := m.Create(callCtx, msg.RequestID, msg.SessionID, *op)
				if err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.SessionChanged{State: s.State, ExitCode: s.ExitCode}
				return respond(nil)
			case *terminalproto.End:
				s, err := m.End(callCtx, msg.RequestID, msg.SessionID)
				if err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.SessionChanged{State: s.State, ExitCode: s.ExitCode}
				return respond(nil)
			case *terminalproto.Attach:
				if !terminalproto.ValidID(op.StreamToken) {
					return terminalproto.ErrInvalid
				}
				a, err := m.Attach(ctx, msg.SessionID, terminalhost.AttachOptions{ID: msg.AttachmentID, Generation: msg.AttachmentGeneration, Cols: op.Cols, Rows: op.Rows, Takeover: op.Takeover, Lease: time.Duration(op.LeaseSeconds) * time.Second})
				if err != nil {
					return respond(err)
				}
				attachments[msg.SessionID] = a
				c, err := stream(callCtx, client, headers, msg, op.StreamToken, "attach")
				if err != nil {
					a.Close()
					return respond(terminalhost.ErrNotRunning)
				}
				wg.Add(1)
				go func() { defer wg.Done(); defer a.Close(); defer c.CloseNow(); bridge(ctx, c, a) }()
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			case *terminalproto.RenewAttachment:
				err := m.RenewAttachment(msg.SessionID, msg.AttachmentID, msg.AttachmentGeneration, time.Duration(op.LeaseSeconds)*time.Second)
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(err)
			case *terminalproto.Resize:
				a := attachments[msg.SessionID]
				if a == nil || a.ID != msg.AttachmentID || a.Generation != msg.AttachmentGeneration {
					return respond(terminalhost.ErrAttachmentStale)
				}
				err := a.Resize(op.Cols, op.Rows)
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(err)
			case *terminalproto.Detach:
				a := attachments[msg.SessionID]
				if a != nil && a.ID == msg.AttachmentID && a.Generation == msg.AttachmentGeneration {
					a.Close()
					delete(attachments, msg.SessionID)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			case *terminalproto.History, *terminalproto.CommandHistory:
				var data []byte
				var err error
				var token string
				switch request := op.(type) {
				case *terminalproto.History:
					token = request.StreamToken
					data, err = m.History(callCtx, msg.SessionID, request.Lines)
				case *terminalproto.CommandHistory:
					if !terminalproto.SupportsCommandHistory(headers.Get(terminalproto.FeatureHeader)) {
						return respond(terminalhost.ErrTerminalDisabled)
					}
					token = request.StreamToken
					data, err = m.CommandHistory(callCtx, msg.SessionID)
				}
				if !terminalproto.ValidID(token) {
					return terminalproto.ErrInvalid
				}
				if err != nil {
					return respond(err)
				}
				c, err := stream(callCtx, client, headers, msg, token, "history")
				if err != nil {
					return respond(terminalhost.ErrNotRunning)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer c.CloseNow()
					for len(data) > 0 {
						n := min(len(data), terminalproto.MaxBinaryBytes)
						if write(ctx, c, data[:n]) != nil {
							return
						}
						data = data[n:]
					}
					_ = c.Close(websocket.StatusNormalClosure, "")
				}()
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			default:
				return terminalproto.ErrInvalid
			}
		}
		return handler, func() {
			cancel()
			if m != nil {
				m.Close()
			}
			wg.Wait()
		}
	}
}
func code(err error) terminalproto.ResultCode {
	switch {
	case errors.Is(err, terminalhost.ErrTerminalDisabled):
		return terminalproto.Denied
	case errors.Is(err, terminalhost.ErrSessionCapacity):
		return terminalproto.Capacity
	case errors.Is(err, terminalhost.ErrOperationConflict), errors.Is(err, terminalhost.ErrAttachmentBusy), errors.Is(err, terminalhost.ErrAttachmentStale):
		return terminalproto.Conflict
	case errors.Is(err, terminalhost.ErrSessionMissing):
		return terminalproto.NotFound
	default:
		return terminalproto.Unavailable
	}
}
func stream(ctx context.Context, client *http.Client, headers http.Header, m terminalproto.Message, token, operation string) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(dialCtx, privateconn.Origin+"/private/v1/terminal/data", &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers})
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(terminalproto.MaxBinaryBytes)
	h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: token, Operation: operation}
	payload, _ := json.Marshal(h)
	if err = c.Write(dialCtx, websocket.MessageText, payload); err != nil {
		c.CloseNow()
		return nil, err
	}
	return c, nil
}
func write(ctx context.Context, c *websocket.Conn, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Write(ctx, websocket.MessageBinary, data)
}
func bridge(ctx context.Context, c *websocket.Conn, a *terminalhost.Attachment) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buf := make([]byte, terminalproto.MaxBinaryBytes)
		for {
			n, err := a.Read(buf)
			if n > 0 && write(ctx, c, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			c.CloseNow()
			a.Close()
		case <-a.Done():
			c.CloseNow()
			cancel()
		}
	}()
	for {
		kind, data, err := c.Read(ctx)
		if err != nil || kind != websocket.MessageBinary || terminalproto.ValidateBinary(data) != nil {
			break
		}
		if n, err := a.Write(data); err != nil || n != len(data) {
			break
		}
	}
	cancel()
	a.Close()
	c.CloseNow()
	<-done
	<-stopped
}

var _ io.ReadWriteCloser = (*terminalhost.Attachment)(nil)

func newID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}
