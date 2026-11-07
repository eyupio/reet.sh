//go:build linux || darwin

package terminalhost

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"

	"github.com/eyupio/reet.sh/internal/filelock"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var ErrAttachmentBusy = errors.New("terminal session already has a controlling attachment")
var ErrAttachmentStale = errors.New("terminal attachment generation is stale")

// Attachment is one disposable tmux client. It owns no durable shell or output
// queue; later brokers must bound their queues and authorize/fence every input.
type Attachment struct {
	ID         string
	Generation int64
	ptmx       *os.File
	cancel     context.CancelFunc
	done       chan struct{}
	closed     atomic.Bool
	once       sync.Once
	resize     sync.Mutex
	leaseMu    sync.Mutex
	deadline   time.Time
	lease      *time.Timer
	lock       *os.File
}
type AttachOptions struct {
	ID         string
	Generation int64
	Cols       int
	Rows       int
	Takeover   bool
	Lease      time.Duration
}

func (m *Manager) Attach(ctx context.Context, sessionID string, opts AttachOptions) (*Attachment, error) {
	validIDs := terminalproto.ValidID(opts.ID) && terminalproto.ValidCounter(opts.Generation)
	validDims := terminalproto.ValidDimensions(opts.Cols, opts.Rows)
	validLease := opts.Lease > 0 && opts.Lease <= 60*time.Second
	valid := validIDs && validDims && validLease
	if !valid {
		return nil, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return nil, err
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
	if err := m.reconcileLocked(ctx, &r); err != nil {
		return nil, err
	}
	s, ok := r.Sessions[sessionID]
	if !ok || s.State != terminalproto.Running {
		return nil, ErrSessionMissing
	}
	// Mouse reporting belongs to tmux attachments, not native UI clients.
	if _, err := m.command(ctx, 0, "set-option", "-g", "mouse", "on"); err != nil {
		return nil, err
	}
	return m.attachProcessLocked(ctx, sessionID, opts, func(attachCtx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(attachCtx, m.tmux.Path, "-S", m.layout.Socket, "attach-session", "-t", "="+m.name(sessionID))
		cmd.Env, cmd.Dir = m.environment(), m.home
		return cmd
	})
}

// NativeCommand is constructed only by a locally verified chat adapter. No
// controller-supplied executable, endpoint, arguments or environment reach it.
// The password stays in Env on this machine, never in argv or a protocol body.
type NativeCommand struct {
	Binary, Dir string
	Args, Env   []string
}

func (m *Manager) AttachNative(ctx context.Context, sessionID string, opts AttachOptions, native NativeCommand) (*Attachment, error) {
	if !terminalproto.ValidID(sessionID) || !terminalproto.ValidID(opts.ID) || !terminalproto.ValidCounter(opts.Generation) ||
		!terminalproto.ValidDimensions(opts.Cols, opts.Rows) || opts.Lease <= 0 || opts.Lease > time.Minute ||
		!filepath.IsAbs(native.Binary) || !filepath.IsAbs(native.Dir) {
		return nil, terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return nil, err
	}
	return m.attachProcessLocked(ctx, sessionID, opts, func(attachCtx context.Context) *exec.Cmd {
		cmd := exec.CommandContext(attachCtx, native.Binary, native.Args...)
		cmd.Dir, cmd.Env = native.Dir, native.Env
		// pty.StartWithSize gives this disposable client its own session/group.
		// Editors or other client children must leave with it; the ACP worker
		// owns a different process group and is never signalled here.
		cmd.Cancel = func() error {
			if cmd.Process == nil {
				return os.ErrProcessDone
			}
			err := unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
			if errors.Is(err, unix.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return cmd
	})
}

// The caller holds m.mu and has proved which live session may create this UI
// attachment. Closing it kills only the client; the agent process is owned elsewhere.
func (m *Manager) attachProcessLocked(ctx context.Context, sessionID string, opts AttachOptions, command func(context.Context) *exec.Cmd) (*Attachment, error) {
	if old := m.attachments[sessionID]; old != nil {
		if opts.Generation <= old.Generation || opts.ID == old.ID {
			return nil, ErrAttachmentStale
		}
		if !old.closed.Load() && !opts.Takeover {
			return nil, ErrAttachmentBusy
		}
		_ = old.Close()
	}
	if err := m.allowed(); err != nil {
		return nil, err
	}
	attachmentLock, err := filelock.Acquire(filepath.Join(m.layout.Dir, m.name(sessionID)+".attachment.lock"))
	if err != nil {
		if errors.Is(err, filelock.ErrLocked) {
			return nil, ErrAttachmentBusy
		}
		return nil, ErrRegistry
	}
	started := false
	defer func() {
		if !started {
			attachmentLock.Close()
		}
	}()
	attachCtx, cancel := context.WithCancel(ctx)
	cmd := command(attachCtx)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(opts.Cols), Rows: uint16(opts.Rows)})
	if err != nil {
		cancel()
		return nil, ErrNotRunning
	}
	// Follow creack/pty's cancellation guidance: nonblocking descriptors let
	// Close interrupt blocked reads/writes without touching the tmux server.
	if err := unix.SetNonblock(int(ptmx.Fd()), true); err != nil {
		cancel()
		ptmx.Close()
		_ = cmd.Wait()
		return nil, ErrNotRunning
	}
	// Register the already-nonblocking descriptor with Go's poller. Setting
	// O_NONBLOCK after os.NewFile (as pty.Open does) leaves Darwin reads returning
	// EAGAIN instead of waiting for readiness.
	fd, err := unix.FcntlInt(ptmx.Fd(), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		cancel()
		ptmx.Close()
		_ = cmd.Wait()
		return nil, ErrNotRunning
	}
	_ = ptmx.Close()
	ptmx = os.NewFile(uintptr(fd), "reet-terminal-pty")
	a := &Attachment{ID: opts.ID, Generation: opts.Generation, ptmx: ptmx, cancel: cancel, done: make(chan struct{}), lock: attachmentLock}
	started = true
	m.attachments[sessionID] = a
	a.deadline = time.Now().Add(opts.Lease)
	a.lease = time.AfterFunc(opts.Lease, func() {
		a.leaseMu.Lock()
		expired := !time.Now().Before(a.deadline)
		if expired {
			a.closed.Store(true)
		} else {
			a.lease.Reset(time.Until(a.deadline))
		}
		a.leaseMu.Unlock()
		if expired {
			_ = a.Close()
		}
	})
	go func() {
		_ = cmd.Wait()
		a.closed.Store(true)
		a.leaseMu.Lock()
		a.lease.Stop()
		a.leaseMu.Unlock()
		a.resize.Lock()
		_ = ptmx.Close()
		a.resize.Unlock()
		_ = a.lock.Close()
		cancel()
		close(a.done)
	}()
	return a, nil
}

func (a *Attachment) Done() <-chan struct{} { return a.done }
func (a *Attachment) Read(p []byte) (int, error) {
	if !a.live() {
		return 0, io.ErrClosedPipe
	}
	if len(p) > terminalproto.MaxBinaryBytes {
		p = p[:terminalproto.MaxBinaryBytes]
	}
	n, err := a.ptmx.Read(p)
	if !a.live() {
		return 0, io.ErrClosedPipe
	}
	return n, err
}
func (a *Attachment) Write(p []byte) (int, error) {
	if terminalproto.ValidateBinary(p) != nil {
		return 0, terminalproto.ErrInvalid
	}
	if !a.live() {
		return 0, io.ErrClosedPipe
	}
	return a.ptmx.Write(p)
}
func (a *Attachment) Resize(cols, rows int) error {
	if !terminalproto.ValidDimensions(cols, rows) {
		return terminalproto.ErrInvalid
	}
	a.resize.Lock()
	defer a.resize.Unlock()
	if !a.live() {
		return io.ErrClosedPipe
	}
	return pty.Setsize(a.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}
func (a *Attachment) Close() error {
	a.once.Do(func() {
		a.closed.Store(true)
		a.leaseMu.Lock()
		a.lease.Stop()
		a.leaseMu.Unlock()
		a.cancel()
		a.resize.Lock()
		_ = a.ptmx.Close()
		a.resize.Unlock()
		<-a.done
	})
	return nil
}

func (a *Attachment) live() bool {
	a.leaseMu.Lock()
	defer a.leaseMu.Unlock()
	return !a.closed.Load() && time.Now().Before(a.deadline)
}

// RenewAttachment is called only after the later broker revalidates its owner
// principal. A stale ID/generation or already expired lease cannot be revived.
func (m *Manager) RenewAttachment(sessionID, attachmentID string, generation int64, lease time.Duration) error {
	if lease <= 0 || lease > 60*time.Second {
		return terminalproto.ErrInvalid
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.allowed(); err != nil {
		return err
	}
	a := m.attachments[sessionID]
	valid := a != nil && a.ID == attachmentID && a.Generation == generation
	if !valid {
		return ErrAttachmentStale
	}
	a.leaseMu.Lock()
	defer a.leaseMu.Unlock()
	if a.closed.Load() || !time.Now().Before(a.deadline) {
		return ErrAttachmentStale
	}
	a.deadline = time.Now().Add(lease)
	a.lease.Reset(lease)
	return nil
}
