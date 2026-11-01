//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// errAgentStillRunning means a swapped-out chat's process group could not be
// confirmed gone, so nothing else is started on its session store.
var errAgentStillRunning = errors.New("the previous agent process did not stop")

// swapShellExitTimeout bounds how long a switch to the chat waits for the
// OpenCode TUI's processes to exit after its tmux session is killed, well
// inside terminalproto.SessionModeTimeout. A variable so tests can shorten it.
var swapShellExitTimeout = 5 * time.Second

// swapShellTermTimeout and swapShellKillTimeout bound the two steps used on a
// terminal that did not exit within swapShellExitTimeout: SIGTERM, then SIGKILL.
var (
	swapShellTermTimeout = 2 * time.Second
	swapShellKillTimeout = 2 * time.Second
)

// terminateShellGroups stops the TUI's process groups and reports whether none
// remains; a variable so a test can simulate a group that survives it.
var terminateShellGroups = func(groups []int) bool {
	for _, g := range groups {
		if !acpagent.TerminateGroup(g, swapShellTermTimeout, swapShellKillTimeout) {
			return false
		}
	}
	return true
}

// shellHost is the part of the tmux manager a mode swap drives.
// *terminalhost.Manager implements it; tests use a fake.
type shellHost interface {
	Create(ctx context.Context, requestID, sessionID string, in terminalproto.Create) (terminalhost.ManagedSession, error)
	End(ctx context.Context, requestID, sessionID string) (terminalhost.ManagedSession, error)
	Forget(ctx context.Context, sessionID string) error
	// PaneProcesses is the process group leader of each of the session's
	// panes, read before End so a swap can wait for them to exit.
	PaneProcesses(ctx context.Context, sessionID string) ([]int, error)
	List(ctx context.Context) ([]terminalhost.ManagedSession, error)
}

// swapShells is the terminal side of a connection's agentSwitch: its manager,
// or nil without one. Tests replace it with a fake.
var swapShells = func(m *terminalhost.Manager) shellHost {
	if m == nil {
		return nil
	}
	return m
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
			// Nothing else may start on its store, and nothing runs the
			// row any more: retire it so the controller is told. Its group
			// may still be alive, so the receipt that replaces it keeps
			// counting against the quota and liveTool until it is gone.
			sup.retireSwapped(sessionID, title, in.Tool, in.ACPSessionID, process)
			return terminalproto.SessionChanged{}, errAgentStillRunning
		}
		if !sup.forget(sessionID) {
			// retireSwapped returns silently when the existing receipt still reserves quota; the swap mark makes
			// that unreachable today, so this branch does not cover that cause.
			// The id cannot be freed for the terminal; nothing runs the row.
			sup.retireSwapped(sessionID, title, in.Tool, in.ACPSessionID, nil)
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		changed, err := a.startTerminal(ctx, requestID, sessionID, title, in, in.Cols, in.Rows)
		if err == nil {
			return changed, nil
		}
		if _, restoreErr := a.startChat(newID(), sessionID, title, in, trust); restoreErr != nil {
			sup.retireSwapped(sessionID, title, in.Tool, in.ACPSessionID, nil)
		}
		return terminalproto.SessionChanged{}, err
	}

	// terminal to chat
	title := shell.Title
	if in.Title != "" {
		title = in.Title
	}
	// End only asks tmux to close the panes; read their process groups
	// first, so the chat waits for the TUI to be gone.
	groups, err := a.shells.PaneProcesses(ctx, sessionID)
	if err != nil {
		return terminalproto.SessionChanged{}, err
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
	if !groupsExited(groups, swapShellExitTimeout) && !terminateShellGroups(groups) {
		// The TUI did not exit on its hangup and survived SIGTERM and SIGKILL.
		// It may still write the OpenCode store: no chat opens on it, and
		// nothing runs the row any more.
		sup.retireSwapped(sessionID, title, in.Tool, "", nil)
		return terminalproto.SessionChanged{}, errAgentStillRunning
	}
	changed, err := a.startChat(requestID, sessionID, title, in, in.Trust)
	if err == nil {
		return changed, nil
	}
	cols, rows := in.Cols, in.Rows
	if !terminalproto.ValidDimensions(cols, rows) {
		cols, rows = 80, 24
	}
	if _, restoreErr := a.startTerminal(ctx, newID(), sessionID, title, in, cols, rows); restoreErr != nil {
		sup.retireSwapped(sessionID, title, in.Tool, "", nil)
	}
	return terminalproto.SessionChanged{}, err
}

// groupsExited polls until no live process remains in any of groups, for at
// most timeout. It is false when one is still alive.
func groupsExited(groups []int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		alive := false
		for _, g := range groups {
			if acpagent.GroupAlive(g) {
				alive = true
				break
			}
		}
		if !alive {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
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
	if _, busy := sup.swapping[sessionID]; busy || sup.holds[sessionID] > 0 {
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
		// A chat the folder's trust gate would refuse is refused here, before
		// the terminal is stopped: a refusal leaves the OpenCode TUI running.
		if err := a.chatTrusted(in); err != nil {
			return nil, terminalhost.ManagedSession{}, err
		}
		// Nothing has ended yet, so the listing above may race an observer
		// freely; the mark must only precede the end.
		sup.mu.Lock()
		defer sup.mu.Unlock()
		if _, busy := sup.swapping[sessionID]; busy || sup.holds[sessionID] > 0 || sup.closed {
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

// chatTrusted is the chat start's trust gate, asked before anything is
// stopped: the same *acpagent.TrustError (or ErrPluginsUnreadable) the start
// would return. A folder that does not resolve is refused here too, with the
// start's own ResolveDir error, so the terminal keeps running.
func (a *agentSwitch) chatTrusted(in terminalproto.SessionMode) error {
	spec, ok := acpagent.Lookup(in.Tool)
	if !ok {
		return nil
	}
	dir, err := terminalhost.ResolveDir(a.chats.sup.opts.Home, in.Dir)
	if err != nil {
		return err
	}
	return spec.CheckTrust(dir, in.Trust)
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

// hold registers a create or end of sessionID. It is false while a swap is
// moving the session; the caller then answers busy.
func (s *chatSupervisor) hold(sessionID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, moving := s.swapping[sessionID]; moving {
		return false
	}
	s.holds[sessionID]++
	return true
}

func (s *chatSupervisor) unhold(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.holds[sessionID]--; s.holds[sessionID] <= 0 {
		delete(s.holds, sessionID)
	}
}

// retireSwapped reports a session whose swap could not leave a process
// running it. It leaves an ended chat receipt, replacing an ended one the
// swap left behind, so a reconcile reports the session ended, and tells the
// controller through the supervisor's normal ended notice once the swap no
// longer hides the id. The receipt is created ended: no reconcile taken after
// the swap mark is released ever sees the id live.
//
// stray, when set, is the swapped-out chat process whose group Close could not
// confirm gone. The receipt keeps it and counts against the chat quota and
// liveTool until a later Close confirms the group gone.
func (s *chatSupervisor) retireSwapped(sessionID, title, tool, acpID string, stray chatProcess) {
	s.mu.Lock()
	if existing := s.sessions[sessionID]; s.closed || existing != nil && existing.state.ReservesQuota() {
		s.mu.Unlock()
		return
	}
	s.dropLocked(sessionID)
	cs := &chatSession{id: sessionID, title: title, tool: tool, acpID: acpID, state: terminalproto.Exited, process: stray, stray: stray != nil}
	s.sessions[sessionID] = cs
	if stray != nil {
		s.wg.Add(1)
	}
	s.mu.Unlock()
	if stray != nil {
		go s.awaitStray(cs, stray)
	}
	s.releaseSwap(sessionID)
	beforeRetireNotice()
	s.ended(cs, -1)
}

// awaitStray closes a retired chat's process again every strayRecheckInterval
// until Close confirms its group gone, then stops counting the receipt. It
// gives up when the supervisor closes.
func (s *chatSupervisor) awaitStray(cs *chatSession, process chatProcess) {
	defer s.wg.Done()
	retired, ok := process.(interface{ GroupRetired() bool })
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.stop:
			return
		case <-time.After(strayRecheckInterval):
		}
		process.Close()
		if !ok || retired.GroupRetired() {
			s.mu.Lock()
			cs.stray = false
			s.mu.Unlock()
			return
		}
	}
}

// strayRecheckInterval is how often a retired chat whose process group Close
// could not confirm gone is closed again; a variable so tests can shorten it.
var strayRecheckInterval = 5 * time.Second

// beforeRetireNotice runs once a retired session is visible again and before
// its ended notice is sent. It does nothing; tests land a reconcile there.
var beforeRetireNotice = func() {}

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
			// Yield before listing again, as reconcile does: back-to-back
			// swaps must not spin this loop.
			time.Sleep(time.Millisecond)
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

// reconcile is the complete snapshot of this connection's sessions: the
// terminals, then the chats, with each session a swap is moving reported once
// as running.
func (a *agentSwitch) reconcile(ctx context.Context, features string) (terminalproto.Reconcile, error) {
	sup := a.chats.sup
	for {
		// Both reads must see the same swaps: a swap landing between them
		// could leave its id out of both, or in both.
		epoch, moving := sup.swapView()
		var sessions []terminalhost.ManagedSession
		if a.shells != nil {
			var err error
			if sessions, err = a.shells.List(ctx); err != nil {
				return terminalproto.Reconcile{}, err
			}
		}
		betweenReconcileReads()
		chats := a.chats.snapshots()
		if again, _ := sup.swapView(); again != epoch {
			if err := ctx.Err(); err != nil {
				return terminalproto.Reconcile{}, err
			}
			time.Sleep(time.Millisecond)
			continue
		}
		visible := sessions[:0]
		for _, s := range sessions {
			if _, hidden := moving[s.ID]; !hidden {
				visible = append(visible, s)
			}
		}
		chats = withSwaps(chats, moving)
		for i := range chats {
			chats[i] = chats[i].ForFeatures(features)
		}
		return reconcileSnapshot(shellSnapshots(visible, features), chats), nil
	}
}

// betweenReconcileReads runs between the terminal and chat reads of a
// reconcile. It does nothing; tests use it to land a swap in that gap.
var betweenReconcileReads = func() {}

// shellSnapshots orders terminals live first, then most recently updated.
func shellSnapshots(sessions []terminalhost.ManagedSession, features string) []terminalproto.SessionSnapshot {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].State.ReservesQuota() != sessions[j].State.ReservesQuota() {
			return sessions[i].State.ReservesQuota()
		}
		return sessions[i].UpdatedAt > sessions[j].UpdatedAt
	})
	out := make([]terminalproto.SessionSnapshot, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, terminalproto.SessionSnapshot{
			ID: s.ID, Title: s.Title, State: s.State, ExitCode: s.ExitCode,
			ActivityAt: s.ActivityAt, BellAt: s.BellAt, ExitStatus: s.ExitCode,
		}.ForFeatures(features))
	}
	return out
}
