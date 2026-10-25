//go:build linux || darwin

package terminalhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

type ManagerOptions struct {
	EnrollmentID string
	Layout       Layout
	Home         string
	// Consent must re-read locally bound enrollment/opt-in. A controller cannot
	// supply it. Later network callers must additionally authenticate the owner.
	Consent func() error
}

type Manager struct {
	enrollment string
	layout     Layout
	home       string
	tmux       Tmux
	consent    func() error
	// lookPath finds agent binaries for launch presets; tests replace it.
	lookPath func(string) (string, error)
	// git runs git for worktrees (ExecGit); tests replace it.
	git         GitRunner
	mu          sync.Mutex
	closed      bool
	attachments map[string]*Attachment
	deadSeen    map[string]time.Time
	changed     chan struct{}
}

func NewManager(ctx context.Context, opts ManagerOptions) (*Manager, error) {
	if !terminalproto.ValidID(opts.EnrollmentID) || opts.Consent == nil {
		return nil, ErrTerminalDisabled
	}
	if err := Check(opts.Layout); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(opts.Home) {
		return nil, ErrUnsafePath
	}
	if info, err := os.Stat(opts.Home); err != nil || !info.IsDir() {
		return nil, ErrUnsafePath
	}
	tmux, err := FindTmux(ctx)
	if err != nil {
		return nil, err
	}
	m := &Manager{enrollment: opts.EnrollmentID, layout: opts.Layout, home: opts.Home, tmux: tmux, consent: opts.Consent, lookPath: exec.LookPath, git: ExecGit, attachments: map[string]*Attachment{}, deadSeen: map[string]time.Time{}, changed: make(chan struct{})}
	// Validate a present registry even when tmux is offline. Never silently
	// repair an unreadable file or adopt sessions from another enrollment.
	lock, err := m.registryLock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	if _, err := m.loadRegistry(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) name(id string) string {
	sum := sha256.Sum256([]byte(m.enrollment + "\x00" + id))
	return "reet-" + hex.EncodeToString(sum[:16])
}
func (m *Manager) allowed() error {
	if m.closed {
		return ErrManagerClosed
	}
	if m.consent() != nil {
		return ErrTerminalDisabled
	}
	return nil
}

func updatedAt(s ManagedSession) int64 {
	now := time.Now().UnixMilli()
	if now < s.UpdatedAt {
		return s.UpdatedAt
	}
	return now
}

// Changed signals durable metadata changes without queuing content or an
// unbounded event history. Obtain the channel BEFORE List, then reconcile again
// when it closes. A later broker must translate snapshots into notifications.
func (m *Manager) Changed() <-chan struct{} { m.mu.Lock(); defer m.mu.Unlock(); return m.changed }

// Create persists intent before issuing tmux new-session, then persists its
// receipt before returning. A retry is lookup/reconciliation, never recreation.
func (m *Manager) Create(ctx context.Context, requestID, sessionID string, in terminalproto.Create) (ManagedSession, error) {
	if !terminalproto.ValidID(requestID) || !terminalproto.ValidID(sessionID) || in.Validate() != nil {
		return ManagedSession{}, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return ManagedSession{}, err
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return ManagedSession{}, err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return ManagedSession{}, err
	}
	key := receiptKey("create", requestID)
	if op, ok := r.Operations[key]; ok {
		if op.SessionID != sessionID || op.Create == nil || *op.Create != in {
			return ManagedSession{}, ErrOperationConflict
		}
		if err := m.reconcileLocked(ctx, &r); err != nil {
			return r.Sessions[sessionID], err
		}
		return r.Sessions[sessionID], nil
	}
	if _, ok := r.Sessions[sessionID]; ok {
		return ManagedSession{}, ErrOperationConflict
	}
	// Resolve the agent and folder before any intent is saved: a refused launch
	// (ErrLaunchUnavailable, reported as launch_failed) leaves no registry entry,
	// no receipt and no tmux session. A plain create keeps today's arguments.
	dir, argv := m.home, []string(nil)
	if in.Tool != "" {
		if dir, err = ResolveDir(m.home, in.Dir); err != nil {
			return ManagedSession{}, err
		}
		if argv, err = LaunchArgv(in, m.lookPath); err != nil {
			return ManagedSession{}, err
		}
	}
	if !responsive(ctx, m.tmux, m.layout) {
		return ManagedSession{}, ErrNotRunning
	}
	live := 0
	for _, s := range r.Sessions {
		if s.State.ReservesQuota() {
			live++
		}
	}
	if live >= terminalproto.MaxSessionsPerAgent {
		return ManagedSession{}, ErrSessionCapacity
	}
	// A server object without our durable intent must never be adopted.
	names, err := m.sessionNames(ctx)
	if err != nil {
		return ManagedSession{}, err
	}
	if names[m.name(sessionID)] {
		return ManagedSession{}, ErrOperationConflict
	}
	if err := m.defaults(ctx); err != nil {
		return ManagedSession{}, err
	}
	// A worktree is made (or, on a retry with a new request id, adopted) last
	// before the intent is saved, and the agent starts in it. Until tmux has
	// shown that no session was started, a worktree this call created is
	// removed again on any failure; an adopted one is never touched.
	var worktree Worktree
	started := false
	if in.Worktree != "" {
		if worktree, err = PrepareWorktree(ctx, m.home, dir, in.Project, in.Worktree, m.git); err != nil {
			return ManagedSession{}, err
		}
		dir = worktree.Path
		defer func() {
			if !started {
				_ = worktree.Undo(context.WithoutCancel(ctx), m.git)
			}
		}()
	}
	at := time.Now().UnixMilli()
	s := ManagedSession{ID: sessionID, Title: in.Title, State: terminalproto.Creating, CreatedAt: at, UpdatedAt: at, CreateRequestID: requestID}
	r.Sessions[sessionID] = s
	r.Operations[key] = operationReceipt{RequestID: requestID, SessionID: sessionID, Kind: "create", Create: &in}
	if err := m.saveRegistry(r); err != nil {
		return s, err
	}
	if err := m.allowed(); err != nil {
		return s, err
	}
	args := append([]string{"new-session", "-d", "-s", m.name(sessionID), "-x", integer(in.Cols), "-y", integer(in.Rows), "-c", tmuxLiteral(dir), "-e", "HOME=" + m.home}, argv...)
	for i := range args {
		args[i] = tmuxArgument(args[i])
	}
	if _, err := m.command(ctx, 0, args...); err != nil {
		// Keep the worktree unless tmux proves the agent was not started.
		if names, nerr := m.sessionNames(context.WithoutCancel(ctx)); nerr != nil || names[m.name(sessionID)] {
			started = true
		}
		return s, err
	}
	started = true
	if err := m.reconcileLocked(ctx, &r); err != nil {
		return s, err
	}
	return r.Sessions[sessionID], nil
}

func (m *Manager) End(ctx context.Context, requestID, sessionID string) (ManagedSession, error) {
	if !terminalproto.ValidID(requestID) || !terminalproto.ValidID(sessionID) {
		return ManagedSession{}, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ManagedSession{}, ErrManagerClosed
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return ManagedSession{}, err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return ManagedSession{}, err
	}
	s, ok := r.Sessions[sessionID]
	if !ok {
		return s, ErrSessionMissing
	}
	key := receiptKey("end", requestID)
	if op, ok := r.Operations[key]; ok {
		if op.SessionID != sessionID {
			return s, ErrOperationConflict
		}
		if op.Complete {
			return s, nil
		}
	}
	if s.State.ReservesQuota() {
		s.State = terminalproto.EndRequested
		s.UpdatedAt = updatedAt(s)
		r.Sessions[sessionID] = s
	}
	r.Operations[key] = operationReceipt{RequestID: requestID, SessionID: sessionID, Kind: "end", Complete: !s.State.ReservesQuota()}
	if err := m.saveRegistry(r); err != nil {
		return s, err
	}
	if a := m.attachments[sessionID]; a != nil {
		_ = a.Close()
	}
	if !s.State.ReservesQuota() {
		return s, nil
	}
	if !responsive(ctx, m.tmux, m.layout) {
		return s, ErrNotRunning
	}
	names, err := m.sessionNames(ctx)
	if err != nil {
		return s, err
	}
	if names[m.name(sessionID)] {
		if _, err := m.command(ctx, 0, "kill-session", "-t", "="+m.name(sessionID)); err != nil {
			return s, err
		}
	}
	if err := m.reconcileLocked(ctx, &r); err != nil {
		return s, err
	}
	return r.Sessions[sessionID], nil
}

func (m *Manager) Rename(ctx context.Context, sessionID, title string) error {
	if !terminalproto.ValidTitle(title) {
		return terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return err
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return err
	}
	s, ok := r.Sessions[sessionID]
	if !ok {
		return ErrSessionMissing
	}
	s.Title = title
	s.UpdatedAt = updatedAt(s)
	r.Sessions[sessionID] = s
	return m.saveRegistry(r)
}

func (m *Manager) List(ctx context.Context) ([]ManagedSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrManagerClosed
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return nil, err
	}
	out := make([]ManagedSession, 0, len(r.Sessions))
	for _, s := range r.Sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt < out[j].CreatedAt || out[i].CreatedAt == out[j].CreatedAt && out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *Manager) Reconcile(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrManagerClosed
	}
	lock, err := m.registryLock(ctx)
	if err != nil {
		return err
	}
	defer lock.Close()
	r, err := m.loadRegistry()
	if err != nil {
		return err
	}
	return m.reconcileLocked(ctx, &r)
}

func (m *Manager) reconcileLocked(ctx context.Context, r *hostRegistry) error {
	names, err := m.sessionNames(ctx)
	if err != nil {
		return err
	}
	changed := false
	for id, s := range r.Sessions {
		if !s.State.ReservesQuota() {
			continue
		}
		previous := s
		if !names[m.name(id)] {
			if s.State == terminalproto.EndRequested {
				s.State = terminalproto.Ended
			} else {
				s.State = terminalproto.Lost
			}
		} else if s.State == terminalproto.EndRequested {
			if _, err := m.command(ctx, 0, "kill-session", "-t", "="+m.name(id)); err != nil {
				return err
			}
			s.State = terminalproto.Ended
		} else {
			panes, err := m.panes(ctx, id)
			if err != nil {
				return err
			}
			alive := false
			for _, p := range panes {
				if !p.dead {
					s.PaneID = p.id
					alive = true
					break
				}
			}
			if alive {
				s.State = terminalproto.Running
			} else {
				s.State = terminalproto.Exited
				if len(panes) == 1 {
					s.ExitCode = panes[0].exit
				}
			}
		}
		if s.State != previous.State || s.PaneID != previous.PaneID {
			s.UpdatedAt = updatedAt(s)
			r.Sessions[id] = s
			changed = true
			for key, op := range r.Operations {
				if op.SessionID == id && (op.Kind == "create" || !s.State.ReservesQuota()) {
					op.Complete = true
					r.Operations[key] = op
				}
			}
		}
	}
	if changed {
		if err := m.saveRegistry(*r); err != nil {
			return err
		}
	}
	for id, s := range r.Sessions {
		if !s.State.ReservesQuota() {
			if a := m.attachments[id]; a != nil {
				_ = a.Close()
				delete(m.attachments, id)
			}
		}
	}
	// Persist the exit observation before removing dead panes. A later pass can
	// finish cleanup after a crash; never kill a live, unregistered session.
	for id, s := range r.Sessions {
		if s.State == terminalproto.Exited && names[m.name(id)] {
			panes, err := m.panes(ctx, id)
			if err != nil {
				return err
			}
			dead := len(panes) > 0
			for _, p := range panes {
				if !p.dead {
					dead = false
				}
			}
			if dead {
				// tmux can expose pane_dead before reaping the child's status.
				// Retain that pane briefly and allow a later Exited observation to
				// fill its initially unknown exit code, rather than destroying it.
				if len(panes) == 1 && s.ExitCode == nil {
					if panes[0].exit != nil {
						s.ExitCode = panes[0].exit
						s.UpdatedAt = updatedAt(s)
						r.Sessions[id] = s
						if err := m.saveRegistry(*r); err != nil {
							return err
						}
					} else {
						first, ok := m.deadSeen[id]
						if !ok {
							m.deadSeen[id] = time.Now()
							continue
						}
						if time.Since(first) < time.Second {
							continue
						}
					}
				}
				if _, err := m.command(ctx, 0, "kill-session", "-t", "="+m.name(id)); err != nil {
					return err
				}
				delete(m.deadSeen, id)
			}
		}
	}
	return nil
}

// Run observes exits and local disable. It never creates a replacement shell.
// The caller owns this loop and must wait for it before releasing its resources.
func (m *Manager) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return m.Close()
		}
		m.mu.Lock()
		disabled := m.consent() != nil
		if disabled {
			for _, a := range m.attachments {
				_ = a.Close()
			}
		}
		m.mu.Unlock()
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := m.Reconcile(probeCtx)
		cancel()
		if err != nil && !errors.Is(err, ErrNotRunning) && ctx.Err() == nil {
			_ = m.Close()
			return err
		}
		select {
		case <-ctx.Done():
			return m.Close()
		case <-ticker.C:
		}
	}
}

// Close ends disposable clients only. Shells belong to the independent server.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	close(m.changed)
	for _, a := range m.attachments {
		_ = a.Close()
	}
	return nil
}
