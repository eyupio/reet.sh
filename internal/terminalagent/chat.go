//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/opencodeacp"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// chatProcess is the part of an OpenCode chat the connector drives. Tests use
// it to exercise the dispatch without a real OpenCode install.
type chatProcess interface {
	Prompt(context.Context, string) error
	Cancel() error
	Allow(string, bool) error
	Working() bool
	Close()
	Done() <-chan struct{}
}

type chatStarter func(context.Context, opencodeacp.Options) (chatProcess, error)

type chatSession struct {
	id, title string
	state     terminalproto.SessionState
	exit      *int
	process   chatProcess
	busy      bool
	ended     sync.Once
}

type chatHost struct {
	ctx        context.Context
	opts       terminalhost.ManagerOptions
	generation int64
	send       func(context.Context, terminalproto.Message) error
	start      chatStarter
	lookPath   func(string) (string, error)

	mu       sync.Mutex
	sessions map[string]*chatSession
	requests map[string]string
	starting map[string]string
	closed   bool
	wg       sync.WaitGroup
}

func newChatHost(ctx context.Context, opts terminalhost.ManagerOptions, generation int64, send func(context.Context, terminalproto.Message) error) *chatHost {
	return &chatHost{ctx: ctx, opts: opts, generation: generation, send: send, start: func(ctx context.Context, opts opencodeacp.Options) (chatProcess, error) {
		return opencodeacp.Start(ctx, opts)
	}, lookPath: exec.LookPath, sessions: map[string]*chatSession{}, requests: map[string]string{}, starting: map[string]string{}}
}

func (h *chatHost) open(requestID, sessionID string, in terminalproto.Create) (terminalproto.SessionChanged, error) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
	}
	if existing, ok := h.requests[requestID]; ok {
		if existing != sessionID {
			h.mu.Unlock()
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		cs := h.sessions[sessionID]
		if cs == nil {
			h.mu.Unlock()
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		state, exit := cs.state, cs.exit
		h.mu.Unlock()
		return terminalproto.SessionChanged{State: state, ExitCode: exit}, nil
	}
	if _, ok := h.sessions[sessionID]; ok {
		h.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	if _, ok := h.starting[requestID]; ok {
		h.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	live := 0
	for _, existing := range h.sessions {
		if existing.state.ReservesQuota() {
			live++
		}
	}
	if live >= terminalproto.MaxSessionsPerAgent {
		h.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrSessionCapacity
	}
	h.starting[requestID] = sessionID
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.starting, requestID)
		h.mu.Unlock()
	}()

	dir, err := terminalhost.ResolveDir(h.opts.Home, in.Dir)
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	binary, err := h.lookPath("opencode")
	if err != nil {
		return terminalproto.SessionChanged{}, opencodeacp.ErrVersion
	}
	cs := &chatSession{id: sessionID, title: in.Title, state: terminalproto.Creating}
	startCtx, cancel := context.WithTimeout(h.ctx, 20*time.Second)
	defer cancel()
	process, err := h.start(startCtx, opencodeacp.Options{
		Binary: binary, Dir: dir, Home: h.opts.Home, Model: in.Model, Trust: in.Trust,
		BaseEnv: os.Environ(),
		OnEvent: func(event opencodeacp.Event) { h.publish(cs, event) },
	})
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	cs.process = process
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		process.Close()
		return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
	}
	cs.state = terminalproto.Running
	h.sessions[sessionID] = cs
	h.requests[requestID] = sessionID
	h.mu.Unlock()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		<-process.Done()
		h.ended(cs, 0)
	}()
	if in.Prompt != "" {
		h.publish(cs, opencodeacp.Event{Kind: "user", Text: in.Prompt})
		h.startPrompt(cs, in.Prompt)
	}
	return terminalproto.SessionChanged{State: terminalproto.Running}, nil
}

func (h *chatHost) snapshot(sessionID string) (terminalproto.SessionSnapshot, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cs := h.sessions[sessionID]
	if cs == nil {
		return terminalproto.SessionSnapshot{}, false
	}
	return terminalproto.SessionSnapshot{ID: cs.id, Title: cs.title, State: cs.state, ExitCode: cs.exit}, true
}

func (h *chatHost) snapshots() []terminalproto.SessionSnapshot {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]terminalproto.SessionSnapshot, 0, len(h.sessions))
	for _, cs := range h.sessions {
		out = append(out, terminalproto.SessionSnapshot{ID: cs.id, Title: cs.title, State: cs.state, ExitCode: cs.exit})
	}
	// The controller only needs live sessions to avoid marking them lost; keep
	// ended receipts too, but never let history push a live one out.
	if len(out) > 32 {
		for i := 0; i < len(out); i++ {
			for j := i + 1; j < len(out); j++ {
				if out[j].State.ReservesQuota() && !out[i].State.ReservesQuota() {
					out[i], out[j] = out[j], out[i]
				}
			}
		}
		out = out[:32]
	}
	return out
}

func (h *chatHost) find(sessionID string) *chatSession {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessions[sessionID]
}

func (h *chatHost) startPrompt(cs *chatSession, text string) bool {
	h.mu.Lock()
	if cs.busy || cs.state != terminalproto.Running {
		h.mu.Unlock()
		return false
	}
	cs.busy = true
	h.mu.Unlock()
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer func() { h.mu.Lock(); cs.busy = false; h.mu.Unlock() }()
		_ = cs.process.Prompt(h.ctx, text)
	}()
	return true
}

func (h *chatHost) prompt(sessionID, text string) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	h.mu.Lock()
	state := cs.state
	h.mu.Unlock()
	if state != terminalproto.Running {
		return terminalhost.ErrOperationConflict
	}
	if !h.startPrompt(cs, text) {
		return terminalhost.ErrOperationConflict
	}
	return nil
}

func (h *chatHost) cancel(sessionID string) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	return cs.process.Cancel()
}

func (h *chatHost) permission(sessionID, permissionID string, allow bool) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	err := cs.process.Allow(permissionID, allow)
	if errors.Is(err, opencodeacp.ErrNoPermission) {
		return terminalhost.ErrOperationConflict
	}
	return err
}

func (h *chatHost) end(sessionID string) (terminalproto.SessionChanged, error) {
	h.mu.Lock()
	cs := h.sessions[sessionID]
	if cs == nil {
		h.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrSessionMissing
	}
	// Keep the ended record and its request receipt: a lost end reply must be
	// answerable with the same exited state, not looked up in the tmux manager
	// and reported missing.
	h.mu.Unlock()
	cs.process.Close()
	h.ended(cs, 0)
	return terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: cs.exit}, nil
}

func (h *chatHost) ended(cs *chatSession, code int) {
	cs.ended.Do(func() {
		h.mu.Lock()
		if cs.state != terminalproto.Exited {
			cs.state = terminalproto.Exited
			if code >= 0 && code <= 255 {
				cs.exit = &code
			}
		}
		h.mu.Unlock()
		_ = h.send(h.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
			Generation: h.generation, Body: &terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: cs.exit}})
	})
}

func (h *chatHost) close() {
	h.mu.Lock()
	h.closed = true
	sessions := make([]*chatSession, 0, len(h.sessions))
	for _, cs := range h.sessions {
		sessions = append(sessions, cs)
	}
	h.sessions = map[string]*chatSession{}
	h.requests = map[string]string{}
	h.starting = map[string]string{}
	h.mu.Unlock()
	for _, cs := range sessions {
		cs.process.Close()
	}
	h.wg.Wait()
}

func (h *chatHost) publish(cs *chatSession, event opencodeacp.Event) {
	wire, ok := wireEvent(event)
	if !ok {
		return
	}
	_ = h.send(h.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
		Generation: h.generation, Body: &wire})
	if event.Kind == "status" && event.State == "ended" {
		h.ended(cs, 0)
	}
}

func wireEvent(event opencodeacp.Event) (terminalproto.ACPEvent, bool) {
	wire := terminalproto.ACPEvent{Kind: event.Kind, MessageID: event.MessageID, ToolCallID: event.ToolCallID,
		PermissionID: event.PermissionID, Title: event.Title, ToolKind: event.ToolKind, Status: event.Status,
		State: event.State, Text: event.Text, Command: event.Command, Path: event.Path, Diff: event.Diff,
		Locations: event.Locations, Allow: event.Allow, Bad: event.Bad, Truncated: event.Truncated,
		Reason: event.Reason, Detail: event.Detail}
	if len(wire.Diff) > 9<<10 {
		wire.Diff = cutText(wire.Diff, 9<<10)
		wire.Truncated = true
	}
	if len(wire.Command) > 6<<10 {
		wire.Command = cutText(wire.Command, 6<<10)
		wire.Truncated = true
	}
	if len(wire.Text) > 8<<10 {
		wire.Text = cutText(wire.Text, 8<<10)
		wire.Bad = true
	}
	if len(wire.Path) > 1<<10 {
		wire.Path = cutText(wire.Path, 1<<10)
	}
	if len(wire.Locations) > 4 {
		wire.Locations = wire.Locations[:4]
	}
	for i, location := range wire.Locations {
		if len(location) > 512 {
			wire.Locations[i] = cutText(location, 512)
		}
	}
	if !terminalproto.ACPEventSize(wire) {
		if wire.Kind != "permission" {
			return terminalproto.ACPEvent{}, false
		}
		// A card that will not fit must still reach the person: keep only
		// what identifies it and disable Allow once with Truncated.
		minimal := terminalproto.ACPEvent{Kind: "permission", PermissionID: wire.PermissionID, ToolCallID: wire.ToolCallID,
			Title: wire.Title, ToolKind: wire.ToolKind, Path: wire.Path, Truncated: true}
		if len(minimal.Title) > 200 {
			minimal.Title = cutText(minimal.Title, 200)
		}
		if len(minimal.Path) > 512 {
			minimal.Path = cutText(minimal.Path, 512)
		}
		if minimal.Path == "" {
			minimal.Path = "the workspace"
		}
		if !terminalproto.ACPEventSize(minimal) {
			return terminalproto.ACPEvent{}, false
		}
		return minimal, true
	}
	return wire, true
}

func chatResult(err error) terminalproto.Result {
	var trust *opencodeacp.TrustError
	switch {
	case errors.As(err, &trust):
		detail := "This folder has OpenCode plugins: " + strings.Join(trust.Plugins, ", ") + ". Start a chat only in a folder you trust."
		if len(detail) > 480 {
			detail = cutText(detail, 480)
		}
		return terminalproto.Result{Code: terminalproto.ACPTrustRequired, Detail: detail}
	case errors.Is(err, opencodeacp.ErrVersion):
		return terminalproto.Result{Code: terminalproto.ACPVersion, Detail: "Chat needs OpenCode 1.19.5 on this machine."}
	case errors.Is(err, opencodeacp.ErrPluginsUnreadable):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The folder's OpenCode plugins could not be read, so the chat was not started."}
	case errors.Is(err, opencodeacp.ErrModel):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "OpenCode did not use the chosen model, so the chat was not started."}
	default:
		r := result(err)
		if r.Code == terminalproto.Unavailable {
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "OpenCode could not start the chat in that folder."}
		}
		return r
	}
}

func cutText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}
