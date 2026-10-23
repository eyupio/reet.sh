package privateconn

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/sealed"
)

const (
	defaultSetupTimeout = 20 * time.Second
	defaultRetryMin     = time.Second
	defaultRetryMax     = 15 * time.Second
)

// RuntimeConfig wires the lazy controller listener. LoadIdentity and
// SaveIdentity operate on the sealed record only; this package restores and
// authenticates the identity with Sealer.
type RuntimeConfig struct {
	Sealer       *sealed.Sealer
	RelayMapURL  string
	Handler      http.Handler
	LoadIdentity func() (formatVersion int, ciphertext string, found bool, err error)
	SaveIdentity func(formatVersion int, ciphertext string) error
	// DiscoverIdentity is overridable in tests. Production leaves it nil.
	DiscoverIdentity func(ctx context.Context, relayMapURL string) (*Identity, error)
	SetupTimeout     time.Duration
	RetryMin         time.Duration
	RetryMax         time.Duration
	Logf             func(format string, args ...any)
}

// Runtime starts one controller listener on first use and restores it with the
// same sealed identity after listener loss. It never exposes an unpersisted
// identity: if the store cannot provide or accept the record, no listener runs.
type Runtime struct {
	cfg RuntimeConfig

	mu          sync.Mutex
	identity    *Identity
	listener    *Listener
	server      *http.Server
	serveDone   chan struct{}
	address     string
	attempts    int
	nextAttempt time.Time
	retrying    bool
	closed      bool
	done        chan struct{}
}

// NewRuntime prepares a lazy runtime. It performs no network work, reads no
// identity, and starts no listener until Ensure is called.
func NewRuntime(cfg RuntimeConfig) *Runtime {
	if cfg.DiscoverIdentity == nil {
		cfg.DiscoverIdentity = NewIdentity
	}
	if cfg.SetupTimeout <= 0 {
		cfg.SetupTimeout = defaultSetupTimeout
	}
	if cfg.RetryMin <= 0 {
		cfg.RetryMin = defaultRetryMin
	}
	if cfg.RetryMax <= 0 {
		cfg.RetryMax = defaultRetryMax
	}
	if cfg.RetryMax < cfg.RetryMin {
		cfg.RetryMax = cfg.RetryMin
	}
	return &Runtime{cfg: cfg, done: make(chan struct{})}
}

func (*Runtime) String() string   { return "[private connection runtime]" }
func (*Runtime) GoString() string { return "[private connection runtime]" }

// Ensure starts or returns the current listener address. The address is a
// capability and must never be returned to a browser. Failures are bounded and
// sanitised; a background retry restores the listener with the same identity.
func (r *Runtime) Ensure(ctx context.Context) (string, error) {
	if r == nil {
		return "", ErrUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ensureLocked(ctx)
}

func (r *Runtime) ensureLocked(ctx context.Context) (string, error) {
	if r.closed {
		return "", ErrUnavailable
	}
	if r.listener != nil {
		return r.address, nil
	}
	if !r.nextAttempt.IsZero() && time.Now().Before(r.nextAttempt) {
		return "", ErrUnavailable
	}
	address, err := r.startLocked(ctx)
	if err != nil {
		r.failLocked()
		return "", err
	}
	r.attempts = 0
	r.nextAttempt = time.Time{}
	return address, nil
}

func (r *Runtime) startLocked(ctx context.Context) (string, error) {
	if r.cfg.Sealer == nil || r.cfg.Handler == nil || r.cfg.LoadIdentity == nil || r.cfg.SaveIdentity == nil {
		return "", ErrIdentity
	}
	if r.identity == nil {
		identity, err := r.restoreOrCreateLocked(ctx)
		if err != nil {
			return "", err
		}
		r.identity = identity
	}
	setupCtx, cancel := context.WithTimeout(ctx, r.cfg.SetupTimeout)
	defer cancel()
	listener, err := Listen(setupCtx, r.identity)
	if err != nil {
		return "", err
	}
	server := &http.Server{
		Handler:           markTransport(r.cfg.Handler),
		ReadHeaderTimeout: 5 * time.Second,
	}
	done := make(chan struct{})
	r.listener = listener
	r.server = server
	r.serveDone = done
	r.address = listener.Address()
	go func() {
		_ = server.Serve(listener)
		close(done)
		r.serveEnded(server)
	}()
	return r.address, nil
}

func (r *Runtime) serveEnded(server *http.Server) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != server {
		return
	}
	r.server = nil
	r.listener = nil
	r.serveDone = nil
	r.address = ""
	if r.closed {
		return
	}
	// A listener failure can follow transport loss or local shutdown. The
	// error may contain a capability, so report only the event.
	if r.cfg.Logf != nil {
		r.cfg.Logf("listener stopped; retrying with the saved identity")
	}
	r.failLocked()
}

func (r *Runtime) failLocked() {
	r.attempts++
	delay := r.cfg.RetryMin
	for i := 1; i < r.attempts && delay < r.cfg.RetryMax; i++ {
		delay *= 2
		if delay > r.cfg.RetryMax {
			delay = r.cfg.RetryMax
		}
	}
	if delay < r.cfg.RetryMax {
		delay = time.Duration(float64(delay) * (0.8 + 0.4*rand.Float64()))
	}
	if delay > r.cfg.RetryMax {
		delay = r.cfg.RetryMax
	}
	r.nextAttempt = time.Now().Add(delay)
	r.scheduleRetryLocked()
}

func (r *Runtime) scheduleRetryLocked() {
	if r.retrying || r.closed {
		return
	}
	r.retrying = true
	wait := time.Until(r.nextAttempt)
	if wait < 0 {
		wait = 0
	}
	go func() {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-r.done:
			return
		case <-timer.C:
		}
		r.mu.Lock()
		defer r.mu.Unlock()
		r.retrying = false
		if r.closed || r.listener != nil {
			return
		}
		_, _ = r.ensureLocked(context.Background())
	}()
}

func (r *Runtime) restoreOrCreateLocked(ctx context.Context) (*Identity, error) {
	version, ciphertext, found, err := r.cfg.LoadIdentity()
	if err != nil {
		return nil, ErrIdentity
	}
	if found {
		if version != 1 || ciphertext == "" {
			return nil, ErrIdentity
		}
		identity, err := OpenIdentity(r.cfg.Sealer, ciphertext)
		if err != nil {
			return nil, ErrIdentity
		}
		return identity, nil
	}
	identity, err := r.cfg.DiscoverIdentity(ctx, r.cfg.RelayMapURL)
	if err != nil {
		return nil, err
	}
	ciphertext, err = identity.Seal(r.cfg.Sealer)
	if err != nil {
		return nil, ErrIdentity
	}
	if err := r.cfg.SaveIdentity(1, ciphertext); err != nil {
		// Another controller process may have stored its identity first. Use
		// only that persisted identity; never serve under an unpersisted one.
		version, stored, ok, loadErr := r.cfg.LoadIdentity()
		if loadErr != nil || !ok || version != 1 || stored == "" {
			return nil, ErrIdentity
		}
		identity, err = OpenIdentity(r.cfg.Sealer, stored)
		if err != nil {
			return nil, ErrIdentity
		}
	}
	return identity, nil
}

// Close stops the listener, active streams, and any pending retry. It is safe
// to call more than once.
func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.done)
	server, listener, done := r.server, r.listener, r.serveDone
	r.server, r.listener, r.serveDone = nil, nil, nil
	r.address = ""
	r.mu.Unlock()
	var err error
	if server != nil {
		_ = server.Close()
	}
	if listener != nil {
		err = listener.Close()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			err = errors.Join(err, ErrUnavailable)
		}
	}
	return err
}
