//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// errAgentStillRunning means a swapped-out chat's process group could not be
// confirmed gone, so nothing else is started on its session store.
var errAgentStillRunning = errors.New("the previous agent process did not stop")

// shellHost is the part of the tmux manager a mode swap drives.
// *terminalhost.Manager implements it; tests use a fake.
type shellHost interface {
	Create(ctx context.Context, requestID, sessionID string, in terminalproto.Create) (terminalhost.ManagedSession, error)
	End(ctx context.Context, requestID, sessionID string) (terminalhost.ManagedSession, error)
	Forget(ctx context.Context, sessionID string) error
	List(ctx context.Context) ([]terminalhost.ManagedSession, error)
}

// shellLister is what the state observer and reconcile read from shellHost.
type shellLister interface {
	List(ctx context.Context) ([]terminalhost.ManagedSession, error)
}

// agentSwitch moves one live session between the structured chat and the
// tool's own terminal under the same session id. The controller row stays
// running throughout: the old side's end is never reported.
type agentSwitch struct {
	chats *chatHost
	// shells is nil when this connection has no terminal manager.
	shells shellHost
}

// swap stops the session's current process completely, forgets its entry so
// the id is unique again, and starts the other mode under the same id. It is
// refused with ErrOperationConflict when the session is already in in.To, when
// a chat has a turn, approval or question in flight, and while another swap of
// the same session runs. A new process that fails to start is followed by one
// attempt to restore the previous mode; the original error is returned either
// way. The old entry is gone before the new one is opened, so the session is
// never counted twice against MaxSessionsPerAgent.
func (a *agentSwitch) swap(ctx context.Context, requestID, sessionID string, in terminalproto.SessionMode) (terminalproto.SessionChanged, error) {
	if !terminalproto.ValidID(requestID) || !terminalproto.ValidID(sessionID) {
		return terminalproto.SessionChanged{}, terminalproto.ErrInvalid
	}
	sup := a.chats.sup
	cs, shell, err := a.claim(ctx, sessionID, in)
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	defer sup.releaseSwap(sessionID)

	if cs != nil {
		// chat to terminal
		sup.mu.Lock()
		title, trust, process := cs.title, cs.trust, cs.process
		sup.mu.Unlock()
		if in.Title != "" {
			title = in.Title
		}
		if _, err := a.chats.end(sessionID); err != nil {
			sup.mu.Lock()
			cs.quiet = false
			sup.mu.Unlock()
			return terminalproto.SessionChanged{}, err
		}
		if retired, ok := process.(interface{ GroupRetired() bool }); ok && !retired.GroupRetired() {
			sup.forget(sessionID)
			return terminalproto.SessionChanged{}, errAgentStillRunning
		}
		if !sup.forget(sessionID) {
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		changed, err := a.startTerminal(ctx, requestID, sessionID, title, in, in.Cols, in.Rows)
		if err == nil {
			return changed, nil
		}
		_, _ = a.startChat(newID(), sessionID, title, in, trust)
		return terminalproto.SessionChanged{}, err
	}

	// terminal to chat
	title := shell.Title
	if in.Title != "" {
		title = in.Title
	}
	ended, err := a.shells.End(ctx, requestID, sessionID)
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	if ended.State.ReservesQuota() {
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	if err := a.shells.Forget(ctx, sessionID); err != nil {
		return terminalproto.SessionChanged{}, err
	}
	changed, err := a.startChat(requestID, sessionID, title, in, in.Trust)
	if err == nil {
		return changed, nil
	}
	cols, rows := in.Cols, in.Rows
	if !terminalproto.ValidDimensions(cols, rows) {
		cols, rows = 80, 24
	}
	_, _ = a.startTerminal(ctx, newID(), sessionID, title, in, cols, rows)
	return terminalproto.SessionChanged{}, err
}

// claim marks the session as swapping and reports its current side: the live
// chat, or the running terminal. The mark hides the session from the state
// observer until releaseSwap, so its old side's end is never reported.
func (a *agentSwitch) claim(ctx context.Context, sessionID string, in terminalproto.SessionMode) (*chatSession, terminalhost.ManagedSession, error) {
	sup := a.chats.sup
	sup.mu.Lock()
	if sup.closed {
		sup.mu.Unlock()
		return nil, terminalhost.ManagedSession{}, terminalhost.ErrManagerClosed
	}
	if _, busy := sup.swapping[sessionID]; busy {
		sup.mu.Unlock()
		return nil, terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
	}
	if cs := sup.sessions[sessionID]; cs != nil && cs.state.ReservesQuota() {
		if in.To == terminalproto.SessionModeChat || !idleLocked(cs) {
			sup.mu.Unlock()
			return nil, terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
		}
		if a.shells == nil {
			sup.mu.Unlock()
			return nil, terminalhost.ManagedSession{}, terminalhost.ErrNotRunning
		}
		cs.quiet = true
		sup.markSwapLocked(sessionID, cs.title)
		sup.mu.Unlock()
		return cs, terminalhost.ManagedSession{}, nil
	}
	sup.mu.Unlock()
	if a.shells == nil {
		return nil, terminalhost.ManagedSession{}, terminalhost.ErrSessionMissing
	}
	sessions, err := a.shells.List(ctx)
	if err != nil {
		return nil, terminalhost.ManagedSession{}, err
	}
	for _, s := range sessions {
		if s.ID != sessionID {
			continue
		}
		if s.State != terminalproto.Running || in.To == terminalproto.SessionModeTerminal {
			return nil, terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
		}
		// Nothing has ended yet, so the listing above may race an observer
		// freely; the mark must only precede the end.
		sup.mu.Lock()
		defer sup.mu.Unlock()
		if _, busy := sup.swapping[sessionID]; busy || sup.closed {
			return nil, terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
		}
		if cs := sup.sessions[sessionID]; cs != nil {
			return nil, terminalhost.ManagedSession{}, terminalhost.ErrOperationConflict
		}
		sup.markSwapLocked(sessionID, s.Title)
		return nil, s, nil
	}
	return nil, terminalhost.ManagedSession{}, terminalhost.ErrSessionMissing
}

func (a *agentSwitch) startChat(requestID, sessionID, title string, in terminalproto.SessionMode, trust bool) (terminalproto.SessionChanged, error) {
	return a.chats.open(requestID, sessionID, terminalproto.Create{
		Title: title, Cols: in.Cols, Rows: in.Rows, Tool: in.Tool, Dir: in.Dir, Model: in.Model, Yolo: in.Yolo,
		Structured: true, Trust: trust, ResumeACP: in.ACPSessionID,
	})
}

func (a *agentSwitch) startTerminal(ctx context.Context, requestID, sessionID, title string, in terminalproto.SessionMode, cols, rows int) (terminalproto.SessionChanged, error) {
	if in.Tool == agentsoftware.OpenCodeTool {
		softwareGate.RLock()
		defer softwareGate.RUnlock()
	}
	s, err := a.shells.Create(ctx, requestID, sessionID, terminalproto.Create{
		Title: title, Cols: cols, Rows: rows, Tool: in.Tool, Dir: in.Dir, Model: in.Model, Yolo: in.Yolo,
		LaunchSession: in.ACPSessionID,
	})
	if err != nil {
		a.discardShell(ctx, sessionID)
		return terminalproto.SessionChanged{}, err
	}
	return terminalproto.SessionChanged{State: s.State, ExitCode: s.ExitCode, ActivityAt: s.ActivityAt, BellAt: s.BellAt, ExitStatus: s.ExitCode}, nil
}

// discardShell removes whatever a failed terminal start left in the registry,
// so a restore can reuse the id.
func (a *agentSwitch) discardShell(ctx context.Context, sessionID string) {
	sessions, err := a.shells.List(ctx)
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.ID != sessionID {
			continue
		}
		if s.State.ReservesQuota() {
			if _, err := a.shells.End(ctx, newID(), sessionID); err != nil {
				return
			}
		}
		_ = a.shells.Forget(ctx, sessionID)
	}
}

// markSwapLocked records a swap of sessionID; the caller holds s.mu. title is
// what a reconcile reports for the session while it moves.
func (s *chatSupervisor) markSwapLocked(sessionID, title string) {
	s.swapping[sessionID] = title
	s.swapEpoch++
}

func (s *chatSupervisor) releaseSwap(sessionID string) {
	s.mu.Lock()
	delete(s.swapping, sessionID)
	s.swapEpoch++
	s.mu.Unlock()
}

// swapView is the current swap epoch and a copy of the sessions being moved,
// with the title each is reported under.
func (s *chatSupervisor) swapView() (uint64, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hidden := make(map[string]string, len(s.swapping))
	for id, title := range s.swapping {
		hidden[id] = title
	}
	return s.swapEpoch, hidden
}

// observedShells lists the terminal sessions without those a swap is moving.
// The listing is retried until no swap started or finished while it ran, so a
// terminal ended by a swap is never reported, and the id a finished swap left
// behind is.
func observedShells(ctx context.Context, shells shellLister, sup *chatSupervisor) ([]terminalhost.ManagedSession, error) {
	sessions, _, err := listShells(ctx, shells, sup)
	return sessions, err
}

func listShells(ctx context.Context, shells shellLister, sup *chatSupervisor) ([]terminalhost.ManagedSession, map[string]string, error) {
	for {
		epoch, hidden := sup.swapView()
		sessions, err := shells.List(ctx)
		if err != nil {
			return nil, nil, err
		}
		if again, _ := sup.swapView(); again != epoch {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			continue
		}
		out := sessions[:0]
		for _, s := range sessions {
			if _, moving := hidden[s.ID]; !moving {
				out = append(out, s)
			}
		}
		return out, hidden, nil
	}
}

// withSwaps replaces whatever the chat snapshots say about a session being
// moved with one running entry, so a reconcile taken mid-swap neither loses the
// row nor lists its id twice.
func withSwaps(chats []terminalproto.SessionSnapshot, hidden map[string]string) []terminalproto.SessionSnapshot {
	if len(hidden) == 0 {
		return chats
	}
	out := make([]terminalproto.SessionSnapshot, 0, len(chats)+len(hidden))
	for _, s := range chats {
		if _, moving := hidden[s.ID]; !moving {
			out = append(out, s)
		}
	}
	for id, title := range hidden {
		if !terminalproto.ValidTitle(title) {
			title = "Session"
		}
		out = append(out, terminalproto.SessionSnapshot{ID: id, Title: title, State: terminalproto.Running})
	}
	return out
}
