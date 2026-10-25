// Package terminalbroker binds owner-authorized browsers to authenticated
// private agent connections. It retains bounded metadata, never terminal logs.
package terminalbroker

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	ErrDenied   = errors.New("terminal access denied")
	ErrOffline  = errors.New("terminal machine is reconnecting")
	ErrConflict = errors.New("terminal attachment conflicts with current writer")
	ErrTicket   = errors.New("terminal ticket expired or already used")
)

type Sender func(context.Context, terminalproto.Message) error
type key struct {
	uid   int64
	agent string
}
type Broker struct {
	mu      sync.Mutex
	wg      sync.WaitGroup
	st      *store.Store
	peers   map[key]*Peer
	tickets map[string]ticket
	bridges map[string]*Bridge
	closed  bool
}
type Peer struct {
	b            *Broker
	key          key
	Generation   int64
	send         Sender
	done         chan struct{}
	once         sync.Once
	mu           sync.Mutex
	ready        bool
	syncMu       sync.Mutex
	enabled      bool
	calls        map[string]chan terminalproto.Message
	streams      map[string]*expected
	next         int64
	observations chan terminalproto.Message
	traffic      func(int64, int64, time.Time)
}

func (p *Peer) SetTrafficObserver(fn func(int64, int64, time.Time)) {
	p.mu.Lock()
	p.traffic = fn
	p.mu.Unlock()
}
func (p *Peer) recordTraffic(input, output int) {
	p.mu.Lock()
	fn := p.traffic
	p.mu.Unlock()
	if fn != nil {
		fn(int64(input), int64(output), time.Now())
	}
}

type expected struct {
	hello   terminalproto.StreamHello
	conn    chan *websocket.Conn
	expires time.Time
	done    chan struct{}
	once    sync.Once
}

func (e *expected) close() {
	e.once.Do(func() {
		close(e.done)
		select {
		case c := <-e.conn:
			c.CloseNow()
		default:
		}
	})
}

type ticket struct {
	uid                                int64
	source, session, agent, attachment string
	generation                         int64
	opts                               Options
	expires                            time.Time
}
type Options struct {
	Cols     int  `json:"cols"`
	Rows     int  `json:"rows"`
	Takeover bool `json:"takeover"`
}

// ResetConnections invalidates transient authorization without ending shells.
func (b *Broker) ResetConnections() {
	b.mu.Lock()
	peers := make([]*Peer, 0, len(b.peers))
	for _, p := range b.peers {
		peers = append(peers, p)
	}
	b.tickets = make(map[string]ticket)
	bridges := make([]*Bridge, 0, len(b.bridges))
	for _, v := range b.bridges {
		bridges = append(bridges, v)
	}
	b.mu.Unlock()
	for _, v := range bridges {
		v.Close()
	}
	for _, p := range peers {
		p.Close()
	}
}

func ID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}
func New(st *store.Store) *Broker {
	return &Broker{st: st, peers: map[key]*Peer{}, tickets: map[string]ticket{}, bridges: map[string]*Bridge{}}
}

func (b *Broker) Revoke(uid int64, agent string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := key{uid, agent}
	if p := b.peers[k]; p != nil {
		p.closeLocked()
		delete(b.peers, k)
	}
	for id, t := range b.tickets {
		if t.uid == uid {
			delete(b.tickets, id)
		}
	}
}
func (b *Broker) RevokeSource(source string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for id, t := range b.tickets {
		if t.source == source {
			delete(b.tickets, id)
		}
	}
	for _, v := range b.bridges {
		if v.source == source {
			v.Close()
		}
	}
}
func (b *Broker) Close() {
	b.mu.Lock()
	b.closed = true
	ps := make([]*Peer, 0, len(b.peers))
	for _, p := range b.peers {
		ps = append(ps, p)
	}
	bs := make([]*Bridge, 0, len(b.bridges))
	for _, v := range b.bridges {
		bs = append(bs, v)
	}
	b.tickets = map[string]ticket{}
	b.mu.Unlock()
	for _, p := range ps {
		p.Close()
	}
	for _, v := range bs {
		v.Close()
	}
	b.wg.Wait()
}

func (b *Broker) start() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	b.wg.Add(1)
	return true
}
func (b *Broker) Connect(uid int64, agent string, generation int64, enabled bool, send Sender) (*Peer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	k := key{uid, agent}
	old := b.peers[k]
	if b.closed || send == nil || !terminalproto.ValidCounter(generation) || old != nil && old.Generation >= generation {
		return nil, ErrOffline
	}
	if old != nil {
		old.closeLocked()
	}
	p := &Peer{b: b, key: k, Generation: generation, send: send, done: make(chan struct{}), enabled: enabled, calls: map[string]chan terminalproto.Message{}, streams: map[string]*expected{}, observations: make(chan terminalproto.Message, 64)}
	b.peers[k] = p
	b.wg.Add(1)
	go func() { defer b.wg.Done(); p.observe() }()
	return p, nil
}

func (p *Peer) observe() {
	for {
		select {
		case <-p.done:
			return
		default:
		}
		select {
		case <-p.done:
			return
		case m := <-p.observations:
			s := m.Body.(*terminalproto.SessionChanged)
			err := p.b.st.ObserveTerminalStateFor(p.key.uid, p.key.agent, m.SessionID, p.Generation, s.State, s.ExitCode)
			if err != nil && !errors.Is(err, store.ErrRemoteNotFound) && !errors.Is(err, store.ErrTerminalConflict) {
				p.Close()
				return
			}
		}
	}
}
func (p *Peer) Done() <-chan struct{} { return p.done }
func (p *Peer) closeLocked() {
	p.once.Do(func() {
		close(p.done)
		for _, v := range p.b.bridges {
			if v.peer == p {
				v.Close()
			}
		}
	})
}
func (p *Peer) Close() {
	p.b.mu.Lock()
	p.closeLocked()
	if p.b.peers[p.key] == p {
		delete(p.b.peers, p.key)
	}
	p.b.mu.Unlock()
}
func (p *Peer) Capabilities(enabled bool) {
	p.mu.Lock()
	p.enabled = enabled
	p.mu.Unlock()
	if !enabled {
		p.b.mu.Lock()
		for _, v := range p.b.bridges {
			if v.peer == p {
				v.Close()
			}
		}
		p.b.mu.Unlock()
	}
}
func (b *Broker) peer(uid int64, agent string, ready bool) (*Peer, error) {
	b.mu.Lock()
	p := b.peers[key{uid, agent}]
	b.mu.Unlock()
	if p == nil {
		return nil, ErrOffline
	}
	p.mu.Lock()
	ok := !ready || p.ready && p.enabled
	p.mu.Unlock()
	select {
	case <-p.done:
		return nil, ErrOffline
	default:
	}
	if !ok {
		return nil, ErrOffline
	}
	return p, nil
}
func (b *Broker) Ready(uid int64, agent string) bool {
	_, err := b.peer(uid, agent, true)
	return err == nil
}

// Receive runs on the sole authenticated control reader. Never wait for an
// RPC caller or database here: a bounded one-element response slot suffices.
func (p *Peer) Receive(m terminalproto.Message) error {
	if m.Generation != p.Generation {
		return ErrConflict
	}
	p.mu.Lock()
	ch := p.calls[m.RequestID]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- m:
			return nil
		default:
			return ErrConflict
		}
	}
	if _, ok := m.Body.(*terminalproto.SessionChanged); ok {
		select {
		case p.observations <- m:
			return nil
		default:
			return ErrOffline
		}
	}
	// A response may arrive after a bounded RPC timeout. It is uncertain work,
	// not permission to recreate it; the next reconciliation resolves it.
	// A late worktree answer is likewise dropped: status is read again before
	// any remove, and a remove's outcome is read again by asking for status.
	switch m.Body.(type) {
	case *terminalproto.Result, *terminalproto.Reconcile, *terminalproto.WorktreeState, *terminalproto.WorktreeResult:
		return nil
	}
	return terminalproto.ErrInvalid
}

var (
	// callTimeout bounds an ordinary control call.
	callTimeout = 10 * time.Second
	// worktreeCallTimeout bounds a worktree create, status or remove. The
	// connector gives git one 60-second budget per request (then up to 30
	// seconds to clean up a failed add) and walks the worktree for status, so
	// the ordinary limit would report a slow repository as a reconnect.
	worktreeCallTimeout = 100 * time.Second
)

func (p *Peer) call(ctx context.Context, m terminalproto.Message) (terminalproto.Message, error) {
	return p.callWithin(ctx, m, callTimeout)
}
func (p *Peer) callWithin(ctx context.Context, m terminalproto.Message, limit time.Duration) (terminalproto.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	m.Version = terminalproto.Version
	m.Generation = p.Generation
	if m.RequestID == "" {
		m.RequestID = ID()
	}
	ch := make(chan terminalproto.Message, 1)
	p.mu.Lock()
	if len(p.calls) >= 32 || p.calls[m.RequestID] != nil {
		p.mu.Unlock()
		return m, ErrConflict
	}
	p.calls[m.RequestID] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.calls, m.RequestID); p.mu.Unlock() }()
	if err := p.send(ctx, m); err != nil {
		return m, ErrOffline
	}
	select {
	case result := <-ch:
		if result.SessionID != m.SessionID || result.AttachmentID != m.AttachmentID || result.AttachmentGeneration != m.AttachmentGeneration {
			return m, ErrConflict
		}
		return result, nil
	case <-p.done:
		return m, ErrOffline
	case <-ctx.Done():
		return m, ErrOffline
	}
}
func resultOK(m terminalproto.Message) error {
	if r, ok := m.Body.(*terminalproto.Result); ok && r.Code == terminalproto.OK {
		return nil
	}
	return ErrDenied
}

// Synchronize never resends uncertain creates after reconnect. The host's
// registry and actual tmux state are the only evidence of an existing shell.
// Pending ends are dispatched before the connection becomes attachable.
func (p *Peer) Synchronize(ctx context.Context) error {
	if !p.syncMu.TryLock() {
		return nil
	}
	defer p.syncMu.Unlock()
	p.mu.Lock()
	p.ready = false
	p.mu.Unlock()
	reply, err := p.call(ctx, terminalproto.Message{Body: terminalproto.Reconcile{}})
	if err != nil {
		return err
	}
	snapshot, ok := reply.Body.(*terminalproto.Reconcile)
	if !ok || !snapshot.Complete {
		return ErrOffline
	}
	byID := map[string]terminalproto.SessionSnapshot{}
	for _, s := range snapshot.Sessions {
		byID[s.ID] = s
	}
	sessions, err := p.b.st.TerminalSessionsFor(p.key.uid)
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.AgentID != p.key.agent || !s.State.ReservesQuota() {
			continue
		}
		observed, exists := byID[s.ID]
		state := terminalproto.Lost
		var code *int
		if exists {
			state = observed.State
			code = observed.ExitCode
		}
		if err := p.b.st.ObserveTerminalStateFor(p.key.uid, p.key.agent, s.ID, p.Generation, state, code); err != nil {
			return err
		}
	}
	ops, err := p.b.st.PendingTerminalOperationsFor(p.key.uid, p.key.agent, p.Generation)
	if err != nil {
		return err
	}
	for _, op := range ops {
		if op.Kind != "end" {
			continue
		}
		if err := p.end(ctx, op.SessionID, op.RequestID); err != nil {
			return err
		}
	}
	p.mu.Lock()
	p.ready = true
	p.mu.Unlock()
	return nil
}

var (
	// ErrLaunchUnsupported means the machine did not negotiate launch-v1; the
	// launch fields were not sent and nothing was reserved.
	ErrLaunchUnsupported = errors.New("machine does not support starting agents")
	// ErrLaunchFailed means the connector refused to start the agent
	// (launch_failed): the tool or the folder is unavailable there.
	ErrLaunchFailed = errors.New("machine could not start that agent")
	// ErrPromptUnsupported means the opening prompt is one word for a tool that
	// could read it as a subcommand; it was refused before sending.
	ErrPromptUnsupported = errors.New("opening prompt must be at least two words for that agent")
)

func launchRequested(in terminalproto.Create) bool {
	return in.Tool != "" || in.Model != "" || in.Dir != "" || in.Prompt != "" || worktreeRequested(in)
}
func worktreeRequested(in terminalproto.Create) bool { return in.Worktree != "" || in.Project != "" }

func (b *Broker) Create(ctx context.Context, uid int64, agent, request string, in terminalproto.Create) (store.TerminalSession, error) {
	s, _, err := b.CreateSession(ctx, uid, agent, request, in)
	return s, err
}

// CreateSession also reports whether this call is the one that learned the
// session started (false for a replay of a known outcome), so callers can
// record a launch exactly once.
func (b *Broker) CreateSession(ctx context.Context, uid int64, agent, request string, in terminalproto.Create) (store.TerminalSession, bool, error) {
	// A launch is checked in full before any peer state, so a malformed or
	// refused launch never reserves a session or reaches the connector.
	if launchRequested(in) && in.Validate() != nil {
		return store.TerminalSession{}, false, terminalproto.ErrInvalid
	}
	if terminalproto.PromptNeedsWords(in.Tool, in.Prompt) {
		return store.TerminalSession{}, false, ErrPromptUnsupported
	}
	p, err := b.peer(uid, agent, true)
	if err != nil {
		return store.TerminalSession{}, false, err
	}
	if launchRequested(in) {
		caps, err := b.st.RemoteCapabilitiesFor(uid, agent)
		if err != nil || caps.Generation != p.Generation {
			return store.TerminalSession{}, false, ErrOffline
		}
		// A peer without worktree-v1 is never sent the worktree fields, and the
		// launch is not started without its worktree.
		if worktreeRequested(in) && !caps.Capabilities.Worktree {
			return store.TerminalSession{}, false, ErrWorktreeUnsupported
		}
		if !caps.Capabilities.Launch {
			return store.TerminalSession{}, false, ErrLaunchUnsupported
		}
	}
	s, created, err := b.st.ReserveTerminalCreateFor(uid, agent, request, p.Generation, in)
	if err != nil {
		return s, false, err
	}
	if !created {
		if s.State == terminalproto.Creating {
			if err := p.Synchronize(ctx); err != nil {
				return s, false, err
			}
			s, err = b.st.TerminalSessionFor(uid, s.ID)
			return s, err == nil && s.State != terminalproto.Creating && s.State != terminalproto.Lost, err
		}
		if s.State == terminalproto.Lost {
			if op, err := b.st.TerminalOperationFor(uid, agent, "create", request); err == nil && op.State == "failed" {
				switch op.Code {
				case terminalproto.LaunchFailed:
					return s, false, ErrLaunchFailed
				case terminalproto.WorktreeRefused:
					// Only the code is stored; the reason was reported to the
					// call that learned it.
					return s, false, ErrWorktreeRefused
				}
			}
		}
		return s, false, nil
	}
	limit := callTimeout
	if worktreeRequested(in) {
		limit = worktreeCallTimeout
	}
	reply, err := p.callWithin(ctx, terminalproto.Message{RequestID: request, SessionID: s.ID, Body: in}, limit)
	if err != nil {
		return s, false, err
	}
	var refusal error
	if state, ok := reply.Body.(*terminalproto.SessionChanged); ok {
		err = b.st.ObserveTerminalStateFor(uid, agent, s.ID, p.Generation, state.State, state.ExitCode)
	} else if r, ok := reply.Body.(*terminalproto.Result); ok && r.Code != terminalproto.OK && r.Code != terminalproto.Unavailable {
		err = b.st.FailTerminalCreateFor(uid, agent, request, p.Generation, r.Code)
		switch r.Code {
		case terminalproto.LaunchFailed:
			refusal = ErrLaunchFailed
		case terminalproto.WorktreeRefused:
			refusal = &WorktreeError{Reason: r.Reason, Detail: r.Detail}
		}
	} else {
		err = ErrOffline
	}
	if err != nil {
		return s, false, err
	}
	s, err = b.st.TerminalSessionFor(uid, s.ID)
	if err == nil && refusal != nil {
		err = refusal
	}
	return s, err == nil && s.State != terminalproto.Creating && s.State != terminalproto.Lost, err
}
func (p *Peer) end(ctx context.Context, session, request string) error {
	reply, err := p.call(ctx, terminalproto.Message{RequestID: request, SessionID: session, Body: terminalproto.End{}})
	if err != nil {
		return err
	}
	state, ok := reply.Body.(*terminalproto.SessionChanged)
	if !ok {
		return ErrOffline
	}
	return p.b.st.ObserveTerminalStateFor(p.key.uid, p.key.agent, session, p.Generation, state.State, state.ExitCode)
}
func (b *Broker) End(ctx context.Context, uid int64, session, request string, removeHistory ...bool) (store.TerminalSession, error) {
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil {
		return s, err
	}
	s, err = b.st.RequestTerminalEndFor(uid, s.AgentID, session, request, removeHistory...)
	if err != nil {
		return s, err
	}
	b.mu.Lock()
	if v := b.bridges[session]; v != nil {
		v.Close()
	}
	b.mu.Unlock()
	if p, e := b.peer(uid, s.AgentID, false); e == nil {
		_ = p.end(ctx, session, request)
	}
	return b.st.TerminalSessionFor(uid, session)
}

// Ticket values are random, short lived, single use, and bound to the exact
// browser login (source is its hash), account, logical session and operation.
func (b *Broker) Ticket(uid int64, source, session string, opts Options) (string, error) {
	if source == "" || !terminalproto.ValidDimensions(opts.Cols, opts.Rows) {
		return "", ErrDenied
	}
	s, err := b.st.TerminalSessionFor(uid, session)
	if err != nil || s.State != terminalproto.Running {
		return "", ErrDenied
	}
	p, err := b.peer(uid, s.AgentID, true)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if v := b.bridges[session]; v != nil && !opts.Takeover {
		select {
		case <-v.ctx.Done():
		default:
			return "", ErrConflict
		}
	}
	for id, t := range b.tickets {
		if !now.Before(t.expires) {
			delete(b.tickets, id)
		}
	}
	if b.closed || len(b.tickets) >= 1024 {
		return "", ErrOffline
	}
	id := ID()
	b.tickets[id] = ticket{uid: uid, source: source, session: session, agent: s.AgentID, attachment: ID(), generation: p.Generation, opts: opts, expires: now.Add(30 * time.Second)}
	return id, nil
}
func (b *Broker) consume(id string, uid int64, source string) (ticket, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	t, ok := b.tickets[id]
	delete(b.tickets, id)
	if !ok || t.uid != uid || t.source != source || !time.Now().Before(t.expires) {
		return ticket{}, ErrTicket
	}
	return t, nil
}

func (p *Peer) expect(session, attachment, operation string) (terminalproto.StreamHello, *expected, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.streams) >= 16 || p.next >= terminalproto.MaxCounter {
		return terminalproto.StreamHello{}, nil, ErrConflict
	}
	p.next++
	h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: ID(), SessionID: session, AttachmentID: attachment, Generation: p.Generation, AttachmentGeneration: p.next, Token: ID(), Operation: operation}
	e := &expected{hello: h, conn: make(chan *websocket.Conn, 1), expires: time.Now().Add(10 * time.Second), done: make(chan struct{})}
	p.streams[h.RequestID] = e
	return h, e, nil
}
func (p *Peer) forget(h terminalproto.StreamHello) {
	p.mu.Lock()
	delete(p.streams, h.RequestID)
	p.mu.Unlock()
}
func (b *Broker) Bind(ctx context.Context, uid int64, agent string, h terminalproto.StreamHello, c *websocket.Conn) error {
	if h.Validate() != nil {
		return ErrDenied
	}
	p, err := b.peer(uid, agent, false)
	if err != nil {
		return err
	}
	p.mu.Lock()
	e := p.streams[h.RequestID]
	if e == nil || e.hello != h || !time.Now().Before(e.expires) {
		p.mu.Unlock()
		return ErrDenied
	}
	delete(p.streams, h.RequestID)
	e.conn <- c
	p.mu.Unlock()
	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.done:
		return ErrOffline
	}
}
