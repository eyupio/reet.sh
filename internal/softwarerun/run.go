// Package softwarerun applies a reviewed software batch to one machine at a
// time.
//
// A batch is a durable, ordered list of machines whose plans a person reviewed
// together. The driver takes the lowest pending position, sends its frozen plan
// through the same apply path a single machine uses, records the outcome, and
// continues. A failed or refused member stops the batch there; the person then
// retries it, skips it and continues, or abandons the batch.
//
// A member left running when the controller stops is reset to pending by the
// store on resume. Its outcome is unknown, so it is tried again with the same
// frozen evidence; the connector re-checks the live installation and a member
// already at the target version is reconciled as applied.
package softwarerun

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ApplyTimeout bounds one machine's apply as the controller sees it. The
// connector's own budget is shorter, so a timeout is reported rather than
// raced by a late install.
const ApplyTimeout = 22 * time.Minute

// Codes stored on a failed member when the failure comes from the controller
// or the connection rather than the machine's own apply result.
const (
	CodeOffline     = "offline"
	CodeUnsupported = "apply_unsupported"
	CodeBadAnswer   = "bad_answer"
	CodeTimedOut    = "timed_out"
	CodeApplyFailed = "apply_failed"
	// CodeInternal is used only when a member's outcome cannot be stored; the
	// member stays running and a resume tries it again.
	CodeInternal = "internal_error"
)

// Store is what the orchestrator reads and writes: store.Store satisfies it.
type Store interface {
	SoftwareBatchFor(uid int64, batchID string) (store.SoftwareBatch, error)
	ResumeSoftwareBatches() ([]store.PendingSoftwareBatch, error)
	SoftwareBatchMemberStart(uid int64, batchID string, position int) (store.SoftwareBatchMember, error)
	SetSoftwareBatchMemberState(uid int64, batchID string, position int, state, code, version string) error
	RetrySoftwareBatchMember(uid int64, batchID string, position int) (store.SoftwareBatchMember, error)
	SkipSoftwareBatchMember(uid int64, batchID string, position int) error
	AbandonSoftwareBatch(uid int64, batchID string) (store.SoftwareBatch, error)
	RemoteCapabilitiesFor(uid int64, agentID string) (store.RemoteAgentCapabilities, error)
}

// Broker is the apply path: terminalbroker.Broker satisfies it.
type Broker interface {
	SoftwareApply(ctx context.Context, uid int64, agentID string, in terminalproto.SoftwareApply) (terminalproto.SoftwareApplyResult, error)
}

// Config wires an Orchestrator.
type Config struct {
	Store  Store
	Broker Broker
	// Settled hears of a batch that reached done or stopped, for auditing.
	Settled func(uid int64, batch store.SoftwareBatch)
	// Observed hears of a machine's fresh evidence after a successful apply,
	// so the controller's stored capabilities match the machine now.
	Observed func(uid int64, agentID string, entry terminalproto.SoftwareEntry)
	// Logf logs without request content; nil means log.Printf.
	Logf func(format string, args ...any)
	// Poll is how often a stalled driver checks again; zero means 1 second.
	Poll time.Duration
}

// Orchestrator drives batches, one goroutine per batch.
type Orchestrator struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
	runs   map[batchKey]*driving
}

type batchKey struct {
	uid int64
	id  string
}

type driving struct {
	again bool
}

// New makes an orchestrator. Close stops it.
func New(cfg Config) *Orchestrator {
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[batchKey]*driving{}}
}

// Start drives the batch's pending members in the background. Calling it again
// while the batch is being driven makes the driver look for pending members
// once more when it finishes its current pass.
func (o *Orchestrator) Start(uid int64, batchID string) {
	k := batchKey{uid, batchID}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	if d := o.runs[k]; d != nil {
		d.again = true
		return
	}
	d := &driving{}
	o.runs[k] = d
	o.wg.Add(1)
	go o.drive(k, d)
}

// Resume continues every batch that still has pending work, as after a
// controller restart. Members left running are reset to pending by the store.
func (o *Orchestrator) Resume() error {
	pending, err := o.cfg.Store.ResumeSoftwareBatches()
	if err != nil {
		return err
	}
	for _, p := range pending {
		o.Start(p.UID, p.BatchID)
	}
	return nil
}

// Driving reports whether the batch has a driver.
func (o *Orchestrator) Driving(uid int64, batchID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.runs[batchKey{uid, batchID}]
	return ok
}

// Done is closed when the orchestrator is closed.
func (o *Orchestrator) Done() <-chan struct{} { return o.ctx.Done() }

// Close stops starting members and waits for the calls in flight to return. A
// member whose call was cut short stays running; the next start's Resume resets
// it to pending and reconciles it.
func (o *Orchestrator) Close() {
	o.mu.Lock()
	o.closed = true
	o.mu.Unlock()
	o.cancel()
	o.wg.Wait()
}

// Retry moves one failed member back to pending and starts the batch again.
func (o *Orchestrator) Retry(uid int64, batchID string, position int) (store.SoftwareBatchMember, error) {
	m, err := o.cfg.Store.RetrySoftwareBatchMember(uid, batchID, position)
	if err != nil {
		return m, err
	}
	o.Start(uid, batchID)
	return m, nil
}

// Skip marks one failed member skipped and continues with the batch's pending
// members.
func (o *Orchestrator) Skip(uid int64, batchID string, position int) error {
	if err := o.cfg.Store.SkipSoftwareBatchMember(uid, batchID, position); err != nil {
		return err
	}
	o.Start(uid, batchID)
	return nil
}

// Abandon ends a batch between machines. It is refused while a member is still
// applying.
func (o *Orchestrator) Abandon(uid int64, batchID string) (store.SoftwareBatch, error) {
	return o.cfg.Store.AbandonSoftwareBatch(uid, batchID)
}

func (o *Orchestrator) drive(k batchKey, d *driving) {
	defer o.wg.Done()
	for {
		o.mu.Lock()
		d.again = false
		o.mu.Unlock()
		terminal := o.pass(k)
		o.mu.Lock()
		if terminal || !d.again || o.closed {
			delete(o.runs, k)
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
	}
}

// pass applies the batch's pending members in position order until one fails,
// the batch ends, or the controller stops.
func (o *Orchestrator) pass(k batchKey) bool {
	for {
		if o.ctx.Err() != nil {
			return true
		}
		batch, err := o.cfg.Store.SoftwareBatchFor(k.uid, k.id)
		if err != nil {
			o.cfg.Logf("software batch %s: %v", k.id, err)
			return true
		}
		if batch.State == store.SoftwareBatchDone || batch.State == store.SoftwareBatchAbandoned {
			return true
		}
		// A stopped batch waits for a person's retry, skip or abandon. Driving
		// it again would continue past the failure it stopped on.
		if batch.State == store.SoftwareBatchStopped {
			return true
		}
		next := -1
		for _, m := range batch.Members {
			if m.State == store.SoftwareMemberPending {
				next = m.Position
				break
			}
		}
		if next < 0 {
			running := false
			for _, m := range batch.Members {
				running = running || m.State == store.SoftwareMemberRunning
			}
			if !running {
				o.settled(k.uid, batch)
			}
			return true
		}
		member, err := o.cfg.Store.SoftwareBatchMemberStart(k.uid, k.id, next)
		if errors.Is(err, store.ErrSoftwareBatchMemberNotPending) {
			// Skip or abandon took the member first; read the batch again.
			continue
		}
		if err != nil {
			o.cfg.Logf("software batch %s member %d: %v", k.id, next, err)
			return true
		}
		o.step(k.uid, batch, member)
		current, err := o.cfg.Store.SoftwareBatchFor(k.uid, k.id)
		if err != nil {
			o.cfg.Logf("software batch %s: %v", k.id, err)
			return true
		}
		if current.State == store.SoftwareBatchDone || current.State == store.SoftwareBatchStopped {
			o.settled(k.uid, current)
			return true
		}
		if current.State == store.SoftwareBatchAbandoned {
			return true
		}
	}
}

// step applies one member's frozen plan and records its outcome.
func (o *Orchestrator) step(uid int64, batch store.SoftwareBatch, member store.SoftwareBatchMember) {
	request := terminalproto.SoftwareApply{
		Tool: batch.Tool, Action: agentsoftware.Action(member.Action),
		ExpectedState:    terminalproto.SoftwareState(member.ExpectedState),
		ExpectedVersion:  member.ExpectedVersion,
		ExpectedPrevious: member.ExpectedPrevious,
		ExpectedManaged:  member.ExpectedManaged,
	}
	ctx, cancel := context.WithTimeout(o.ctx, ApplyTimeout)
	defer cancel()
	result, err := o.cfg.Broker.SoftwareApply(ctx, uid, member.AgentID, request)
	if o.ctx.Err() != nil {
		return // shutting down: the member stays running for Resume to reset
	}
	if err != nil {
		o.record(uid, batch.ID, member.Position, store.SoftwareMemberFailed, failureCode(err), "")
		return
	}
	switch result.State {
	case terminalproto.SoftwareApplyApplied:
		if result.Entry != nil && o.cfg.Observed != nil {
			o.cfg.Observed(uid, member.AgentID, *result.Entry)
		}
		o.record(uid, batch.ID, member.Position, store.SoftwareMemberApplied, "", result.Version)
	case terminalproto.SoftwareApplyRefused:
		// A plan can be refused because the machine already reached the target
		// while the reply was lost, or another person applied it first.
		if o.reconciled(uid, batch, member) {
			o.record(uid, batch.ID, member.Position, store.SoftwareMemberApplied, "", targetVersion(member))
			return
		}
		o.record(uid, batch.ID, member.Position, store.SoftwareMemberFailed, reasonCode(result.Reason), "")
	case terminalproto.SoftwareApplyFailed:
		o.record(uid, batch.ID, member.Position, store.SoftwareMemberFailed, reasonCode(result.Reason), "")
	default:
		o.record(uid, batch.ID, member.Position, store.SoftwareMemberFailed, CodeBadAnswer, "")
	}
}

func (o *Orchestrator) record(uid int64, batchID string, position int, state, code, version string) {
	if err := o.cfg.Store.SetSoftwareBatchMemberState(uid, batchID, position, state, code, version); err != nil {
		o.cfg.Logf("software batch %s member %d: record %s: %v", batchID, position, state, err)
	}
}

// reconciled reports whether the machine now has the target version, which is
// what a refused plan means when the earlier attempt actually succeeded.
func (o *Orchestrator) reconciled(uid int64, batch store.SoftwareBatch, member store.SoftwareBatchMember) bool {
	caps, err := o.cfg.Store.RemoteCapabilitiesFor(uid, member.AgentID)
	if err != nil {
		return false
	}
	target := targetVersion(member)
	for _, entry := range caps.Capabilities.Software {
		if entry.Tool != batch.Tool {
			continue
		}
		if entry.State == terminalproto.SoftwareInstalled && entry.Managed && entry.Version == target {
			return true
		}
	}
	return false
}

func (o *Orchestrator) settled(uid int64, batch store.SoftwareBatch) {
	if o.cfg.Settled != nil {
		o.cfg.Settled(uid, batch)
	}
}

func targetVersion(member store.SoftwareBatchMember) string {
	if member.Target != "" {
		return member.Target
	}
	return agentsoftware.OpenCodeReviewedVersion
}

func failureCode(err error) string {
	switch {
	case errors.Is(err, terminalbroker.ErrOffline):
		return CodeOffline
	case errors.Is(err, terminalbroker.ErrSoftwareApplyUnsupported):
		return CodeUnsupported
	case errors.Is(err, terminalbroker.ErrSoftwareApplyAnswer):
		return CodeBadAnswer
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return CodeTimedOut
	default:
		return CodeApplyFailed
	}
}

// reasonCode keeps the machine's own reason when it is a safe stored code.
func reasonCode(reason terminalproto.SoftwareApplyReason) string {
	s := string(reason)
	if s == "" || len(s) > 40 {
		return CodeApplyFailed
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		if !(b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '_') {
			return CodeApplyFailed
		}
	}
	return s
}
