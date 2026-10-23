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
	if !terminalproto.ValidID(opts.ID) || !terminalproto.ValidCounter(opts.Generation) || !terminalproto.ValidDimensions(opts.Cols, opts.Rows) || opts.Lease <= 0 || opts.Lease > 60*time.Second {
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
	cmd := exec.CommandContext(attachCtx, m.tmux.Path, "-S", m.layout.Socket, "attach-session", "-t", "="+m.name(sessionID))
	cmd.Env = m.environment()
	cmd.Dir = m.home
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
	if a == nil || a.ID != attachmentID || a.Generation != generation {
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
