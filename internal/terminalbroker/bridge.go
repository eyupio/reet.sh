package terminalbroker

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const leaseSeconds = 15

type Bridge struct {
	source        string
	peer          *Peer
	hello         terminalproto.StreamHello
	browser, host *websocket.Conn
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
}

// Close fences input immediately. It never ends the durable shell.
func (v *Bridge) Close() {
	v.cancel()
	v.mu.Lock()
	if v.browser != nil {
		v.browser.CloseNow()
	}
	if v.host != nil {
		v.host.CloseNow()
	}
	v.mu.Unlock()
}
func (v *Bridge) setHost(c *websocket.Conn) {
	v.mu.Lock()
	v.host = c
	if v.ctx.Err() != nil {
		c.CloseNow()
	}
	v.mu.Unlock()
}

// Serve consumes a ticket after the API reauthenticates its browser principal.
// authorize must check the original login, enrollment and current session state
// again at every renewal. It must never just trust the initial HTTP context.
func (b *Broker) Serve(ctx context.Context, uid int64, source, id string, c *websocket.Conn, authorize func() bool) error {
	if !b.start() {
		return ErrOffline
	}
	defer b.wg.Done()
	t, err := b.consume(id, uid, source)
	if err != nil {
		return err
	}
	if authorize == nil || !authorize() {
		return ErrDenied
	}
	s, err := b.st.TerminalSessionFor(uid, t.session)
	if err != nil || s.State != terminalproto.Running {
		return ErrDenied
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return err
	}
	if t.agent != s.AgentID || t.generation != p.Generation {
		return ErrTicket
	}
	validate := func() bool {
		if !authorize() {
			return false
		}
		current, err := b.st.TerminalSessionFor(uid, s.ID)
		if err != nil || current.State != terminalproto.Running {
			return false
		}
		caps, err := b.st.RemoteCapabilitiesFor(uid, s.AgentID)
		return err == nil && caps.Generation == p.Generation && caps.Capabilities.TerminalEnabled && caps.Capabilities.Readiness == terminalproto.Ready
	}
	vctx, cancel := context.WithCancel(ctx)
	v := &Bridge{peer: p, browser: c, ctx: vctx, cancel: cancel, source: source}
	// Reserve/fence under one lock. No input from a displaced writer is queued
	// or retried. Its in-flight write has an intentionally uncertain outcome.
	b.mu.Lock()
	old := b.bridges[s.ID]
	if old != nil && !t.opts.Takeover {
		b.mu.Unlock()
		cancel()
		return ErrConflict
	}
	if old != nil {
		old.Close()
	}
	b.bridges[s.ID] = v
	b.mu.Unlock()
	defer func() {
		v.Close()
		b.mu.Lock()
		if b.bridges[s.ID] == v {
			delete(b.bridges, s.ID)
		}
		b.mu.Unlock()
		if v.hello.RequestID != "" {
			detachCtx, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			_ = p.send(detachCtx, message(v.hello, terminalproto.Detach{}))
		}
	}()
	h, e, err := p.expect(s.ID, t.attachment, "attach")
	if err != nil {
		return err
	}
	v.hello = h
	defer p.forget(h)
	defer e.close()
	reply, err := p.call(vctx, message(h, terminalproto.Attach{Cols: t.opts.Cols, Rows: t.opts.Rows, Takeover: t.opts.Takeover, LeaseSeconds: leaseSeconds, StreamToken: h.Token}))
	if err != nil {
		return err
	}
	if err = resultOK(reply); err != nil {
		return err
	}
	select {
	case host := <-e.conn:
		v.setHost(host)
		defer host.CloseNow()
	case <-vctx.Done():
		return ErrOffline
	case <-p.done:
		return ErrOffline
	case <-time.After(10 * time.Second):
		return ErrOffline
	}
	if !validate() {
		return ErrDenied
	}
	if err := write(vctx, c, websocket.MessageText, []byte(`{"type":"ready"}`)); err != nil {
		return err
	}
	// Exactly 8 * 32 KiB pending output. Overflow closes the attachment; no
	// dropped bytes are disguised as a complete live terminal.
	output := make(chan []byte, terminalproto.MaxQueuedOutputBytes/terminalproto.MaxBinaryBytes)
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	start := func(fn func() error) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := fn()
			select {
			case errs <- err:
			default:
			}
		}()
	}
	start(func() error {
		for {
			kind, data, err := v.host.Read(vctx)
			if err != nil {
				return err
			}
			if kind != websocket.MessageBinary || terminalproto.ValidateBinary(data) != nil {
				return terminalproto.ErrInvalid
			}
			p.recordTraffic(0, len(data))
			select {
			case output <- data:
			default:
				return ErrOffline
			}
		}
	})
	start(func() error {
		for {
			select {
			case data := <-output:
				if err := write(vctx, c, websocket.MessageBinary, data); err != nil {
					return err
				}
			case <-vctx.Done():
				return vctx.Err()
			}
		}
	})
	start(func() error {
		for {
			kind, data, err := c.Read(vctx)
			if err != nil {
				return err
			}
			if kind == websocket.MessageBinary {
				if terminalproto.ValidateBinary(data) != nil {
					return terminalproto.ErrInvalid
				}
				if err := write(vctx, v.host, kind, data); err != nil {
					return err
				}
				p.recordTraffic(len(data), 0)
				continue
			}
			var resize struct {
				Type string `json:"type"`
				Cols int    `json:"cols"`
				Rows int    `json:"rows"`
			}
			if kind != websocket.MessageText || len(data) > 1024 || json.Unmarshal(data, &resize) != nil || resize.Type != "resize" || !terminalproto.ValidDimensions(resize.Cols, resize.Rows) {
				return terminalproto.ErrInvalid
			}
			m := message(h, terminalproto.Resize{Cols: resize.Cols, Rows: resize.Rows})
			m.RequestID = ID()
			reply, err := p.call(vctx, m)
			if err != nil {
				return err
			}
			if err = resultOK(reply); err != nil {
				return err
			}
		}
	})
	start(func() error {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if !validate() {
					return ErrDenied
				}
				current, err := b.peer(uid, s.AgentID, true)
				if err != nil || current != p {
					return ErrOffline
				}
				m := message(h, terminalproto.RenewAttachment{LeaseSeconds: leaseSeconds})
				m.RequestID = ID()
				reply, err := p.call(vctx, m)
				if err != nil {
					return err
				}
				if err = resultOK(reply); err != nil {
					return err
				}
			case <-vctx.Done():
				return vctx.Err()
			}
		}
	})
	select {
	case err = <-errs:
	case <-vctx.Done():
		err = vctx.Err()
	case <-p.done:
		err = ErrOffline
	}
	v.Close()
	wg.Wait()
	return err
}
func message(h terminalproto.StreamHello, body terminalproto.Body) terminalproto.Message {
	return terminalproto.Message{Version: terminalproto.Version, Generation: h.Generation, RequestID: h.RequestID, SessionID: h.SessionID, AttachmentID: h.AttachmentID, AttachmentGeneration: h.AttachmentGeneration, Body: body}
}
func write(ctx context.Context, c *websocket.Conn, kind websocket.MessageType, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Write(ctx, kind, data)
}

func (b *Broker) History(ctx context.Context, uid int64, session string, lines int) ([]byte, error) {
	if !b.start() {
		return nil, ErrOffline
	}
	defer b.wg.Done()
	if lines < 1 || lines > terminalproto.MaxHistoryLines {
		return nil, terminalproto.ErrInvalid
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil || s.State != terminalproto.Running {
		return nil, ErrDenied
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return nil, err
	}
	h, e, err := p.expect(session, ID(), "history")
	if err != nil {
		return nil, err
	}
	defer p.forget(h)
	defer e.close()
	reply, err := p.call(ctx, message(h, terminalproto.History{Lines: lines, StreamToken: h.Token}))
	if err != nil {
		return nil, err
	}
	if err = resultOK(reply); err != nil {
		return nil, err
	}
	var c *websocket.Conn
	select {
	case c = <-e.conn:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.done:
		return nil, ErrOffline
	case <-time.After(10 * time.Second):
		return nil, ErrOffline
	}
	defer c.CloseNow()
	historyCtx, stopHistory := context.WithCancel(ctx)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-p.done:
			c.CloseNow()
		case <-historyCtx.Done():
		}
	}()
	defer func() { stopHistory(); <-stopped }()
	var data []byte
	for {
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		kind, frame, err := c.Read(readCtx)
		cancel()
		if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
			return data, nil
		}
		if err != nil {
			return nil, io.ErrUnexpectedEOF
		}
		if kind != websocket.MessageBinary || terminalproto.ValidateBinary(frame) != nil || len(data)+len(frame) > 256<<10 {
			return nil, terminalproto.ErrInvalid
		}
		data = append(data, frame...)
		p.recordTraffic(0, len(frame))
	}
}
