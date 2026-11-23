//go:build linux || darwin

package terminalagent

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// agentConn is the state one control connection's handlers share. It is built
// once per connection by newAgentConn and lives until close. The work that
// outlives a connection (the slots and counters shared by every connection)
// stays in package variables in agent.go.
type agentConn struct {
	// ctx ends with the connection: close cancels it.
	ctx           context.Context
	cancel        context.CancelFunc
	opts          terminalhost.ManagerOptions
	client        *http.Client
	headers       http.Header
	generation    int64
	send          agenthealth.Sender
	featureHeader string
	features      terminalproto.FeatureSet
	// wg counts this connection's goroutines; close waits for them.
	wg         sync.WaitGroup
	chats      *chatHost
	m          *terminalhost.Manager
	managerErr error
	switcher   *agentSwitch
	// swaps tracks the mode swaps in flight. A swap outlives a lost
	// connection, so the manager does too: its loop runs on its own
	// context, which close ends only after waiting for the swaps.
	swaps       *swapGroup
	stopManager context.CancelFunc
	// attachments is read and written only by the dispatcher.
	attachments map[string]*terminalhost.Attachment
	// worktreeSlot allows one worktree status or remove at a time on this
	// machine. It runs on its own goroutine so the dispatcher, and the
	// controller's other requests, never wait for git.
	worktreeSlot chan struct{}
	// inspectSlot, browseSlot, branchesSlot and modelsSlot allow one inspect,
	// browse, branch listing and model listing at a time on this connection,
	// each run like a worktree request.
	inspectSlot       chan struct{}
	inspectGitHubSlot chan struct{}
	browseSlot        chan struct{}
	branchesSlot      chan struct{}
	pruneSlot         chan struct{}
	// cloneStatusSlot allows one clone status at a time on this
	// connection. It only reads, so it has a slot of its own: neither a
	// clone nor a sync holds it up.
	cloneStatusSlot chan struct{}
	// repoListSlot allows one repository listing at a time on this
	// connection. It only reads GitHub, so a clone does not hold it up.
	repoListSlot chan struct{}
	modelsSlot   chan struct{}
	// modelsMu guards the per-tool last listing used for the floor.
	modelsMu   sync.Mutex
	modelsLast map[string]modelListCacheEntry
	// uploads bounds the image transfers in flight on this connection.
	uploads *uploadCounter
}

// agentReq is one dispatched message. It is built per call, so nothing on it
// is shared between messages; its goroutines own it once the dispatcher has
// returned.
type agentReq struct {
	*agentConn
	callCtx context.Context
	msg     terminalproto.Message
	reply   terminalproto.Message
	// held is set while the dispatcher holds msg.SessionID against a mode swap
	// and releases it on return; a handler that hands the hold to a goroutine
	// clears it first.
	held bool
}

// respond answers err as a result, or the reply as it stands when err is nil.
func (r *agentReq) respond(err error) error {
	if err != nil {
		r.reply.Body = result(err)
	}
	return r.send(r.callCtx, r.reply)
}

// newAgentConn builds one connection's state and starts its background work:
// the terminal manager's loop and the observer that reports session changes.
func newAgentConn(ctx context.Context, opts terminalhost.ManagerOptions, supervisor *ChatSupervisor, client *http.Client, headers http.Header, generation int64, send agenthealth.Sender) *agentConn {
	ctx, cancel := context.WithCancel(ctx)
	c := &agentConn{ctx: ctx, cancel: cancel, opts: opts, client: client, headers: headers, generation: generation, send: send}
	c.featureHeader = headers.Get(terminalproto.FeatureHeader)
	c.features = terminalproto.ParseFeatures(c.featureHeader)
	features := c.features
	// The chats attach first: the terminal state observer below hides a
	// session while a mode swap moves it between the two.
	c.chats = attachChats(supervisor, ctx, opts, generation, send, chatFeatures{
		history:    features.Has(terminalproto.ACPHistoryFeature),
		details:    features.Has(terminalproto.ACPToolDetailsFeature),
		questions:  features.Has(terminalproto.ACPQuestionFeature),
		previews:   features.Has(terminalproto.ACPPreviewsFeature),
		catalogue:  features.Has(terminalproto.ACPModelCatalogueFeature),
		provider:   features.Has(terminalproto.ACPProviderFeature),
		usage:      features.Has(terminalproto.ACPUsageFeature),
		background: features.Has(terminalproto.ACPBackgroundFeature),
	})
	c.m, c.managerErr = terminalhost.NewManager(ctx, opts)
	c.switcher = &agentSwitch{chats: c.chats, shells: swapShells(c.m)}
	c.swaps = newSwapGroup()
	managerCtx, stopManager := context.WithCancel(context.WithoutCancel(ctx))
	c.stopManager = stopManager
	if c.managerErr == nil {
		c.wg.Add(1)
		go func() { defer c.wg.Done(); _ = c.m.Run(managerCtx) }()
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.observeShells()
		}()
	}
	c.attachments = map[string]*terminalhost.Attachment{}
	c.worktreeSlot = make(chan struct{}, 1)
	c.inspectSlot = make(chan struct{}, 1)
	c.inspectGitHubSlot = make(chan struct{}, 1)
	c.browseSlot = make(chan struct{}, 1)
	c.branchesSlot = make(chan struct{}, 1)
	c.pruneSlot = make(chan struct{}, 1)
	c.cloneStatusSlot = make(chan struct{}, 1)
	c.repoListSlot = make(chan struct{}, 1)
	c.modelsSlot = make(chan struct{}, 1)
	c.modelsLast = map[string]modelListCacheEntry{}
	c.uploads = &uploadCounter{max: maxUploadsInFlight}
	return c
}

// close ends the connection: its context, its chats, the swaps in flight, the
// manager, and then every goroutine it started.
func (c *agentConn) close() {
	c.cancel()
	c.chats.close()
	c.swaps.Wait(swapDrainTimeout)
	c.stopManager()
	if c.m != nil {
		c.m.Close()
	}
	c.wg.Wait()
}

// observeShells reports each terminal session's changes until the connection
// ends or a send fails.
func (c *agentConn) observeShells() {
	ctx, m, send := c.ctx, c.m, c.send
	observed := map[string]terminalproto.SessionChanged{}
	sentDirs := map[string]string{}
	initial := true
	for {
		changed := m.Changed()
		// A session a swap is moving is left out: its terminal
		// ending is a hand-off, not the end of the row. The hidden
		// set also keeps a swapping id from being pruned, so it is
		// reported again only when it really returns.
		sessions, hidden, err := listShells(ctx, m, c.chats.sup)
		if err != nil {
			return
		}
		seen := make(map[string]struct{}, len(sessions))
		for _, s := range sessions {
			seen[s.ID] = struct{}{}
			state, modified := observeShell(observed, sentDirs, s, c.featureHeader)
			if !initial && modified {
				if send(ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: s.ID, Generation: c.generation, Body: state}) != nil {
					return
				}
			}
		}
		// Forget sessions that left the listing, so a reused id is
		// reported as new rather than compared with a stale state.
		for id := range observed {
			if _, still := seen[id]; still {
				continue
			}
			if _, moving := hidden[id]; moving {
				continue
			}
			delete(observed, id)
			delete(sentDirs, id)
		}
		initial = false
		select {
		case <-ctx.Done():
			return
		case <-changed:
		}
	}
}

// aside runs work for reply off the dispatcher. A second request while
// slot is held, or while maxAsideRunning of running are alive, is
// answered busy at once. The answer is work's result, or late when
// work has not returned after the given time; the slot is freed then
// even if work is stuck, and a result that comes later is dropped. A
// request whose connection has ended answers nothing.
func (c *agentConn) aside(callCtx context.Context, reply terminalproto.Message, slot chan struct{}, running *atomic.Int32, after time.Duration, busy, late terminalproto.Body, work func(context.Context) terminalproto.Body) error {
	return c.asideHolding(callCtx, reply, slot, running, after, false, busy, late, work)
}

// asideHolding is aside with hold: for work that cannot be stopped (a delete),
// the slot is freed only when work itself returns, never at the deadline:
// nothing that shares the slot starts beside work that is still running.
func (c *agentConn) asideHolding(callCtx context.Context, reply terminalproto.Message, slot chan struct{}, running *atomic.Int32, after time.Duration, hold bool, busy, late terminalproto.Body, work func(context.Context) terminalproto.Body) error {
	select {
	case slot <- struct{}{}:
	default:
		reply.Body = busy
		return c.send(callCtx, reply)
	}
	if running.Add(1) > maxAsideRunning {
		running.Add(-1)
		<-slot
		reply.Body = busy
		return c.send(callCtx, reply)
	}
	done := make(chan terminalproto.Body, 1)
	// Not in wg: cleanup must not wait for work stuck in a syscall.
	go func() {
		defer running.Add(-1)
		body := work(callCtx)
		if hold {
			<-slot
		}
		done <- body
	}()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		// Prefer a result that was ready before the timer: a select with
		// both ready picks at random, which would report a completed
		// request as timed out.
		var body terminalproto.Body
		select {
		case body = <-done:
		default:
			timer := time.NewTimer(after)
			defer timer.Stop()
			select {
			case body = <-done:
			case <-timer.C:
				select {
				case body = <-done:
				default:
					body = late
				}
			case <-callCtx.Done():
			}
		}
		// Free the slot before answering, so a request the controller
		// sends once it has this answer is not busy.
		if !hold {
			<-slot
		}
		if callCtx.Err() != nil {
			return
		}
		reply.Body = body
		_ = c.send(callCtx, reply)
	}()
	return nil
}
