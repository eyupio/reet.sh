// Package setuprun applies a reviewed setup batch to one machine at a time.
//
// It follows the software batch driver: the lowest pending member is applied
// with the items the batch holds and the plan fingerprint the person
// reviewed, its outcome is recorded, and the driver continues. A failed or
// refused member stops the batch for a person to retry, skip or abandon. A
// machine that was not connected when its turn came is skipped, because
// nothing reached it. A member left running by a controller stop is tried
// again on resume; the machine finds its own completed journal when the
// first apply went through, so nothing is written twice.
package setuprun

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ApplyTimeout bounds one machine's apply as the controller sees it.
const ApplyTimeout = 2 * time.Minute

// Codes stored on a member when the failure is the controller's or the
// connection's rather than the machine's own closed reason.
const (
	CodeOffline        = "offline"
	CodeUnsupported    = "apply_unsupported"
	CodeBadAnswer      = "bad_answer"
	CodeTimedOut       = "timed_out"
	CodeApplyFailed    = "apply_failed"
	CodeConnectionLost = "connection_lost"
	CodeContentGone    = "content_expired"
)

// Store is what the driver reads and writes: store.Store satisfies it.
type Store interface {
	SetupBatchFor(uid int64, batchID string) (store.SetupBatch, error)
	SetupBatchItems(uid int64, batchID string) ([]byte, error)
	ResumeSetupBatches() ([]store.PendingSoftwareBatch, error)
	SetupBatchMemberStart(uid int64, batchID string, position int) (store.SetupBatchMember, error)
	SetSetupBatchMemberState(uid int64, batchID string, position int, state, code, journal string) error
	RetrySetupBatchMember(uid int64, batchID string, position int) (store.SetupBatchMember, error)
	SkipSetupBatchMember(uid int64, batchID string, position int) error
	AbandonSetupBatch(uid int64, batchID string) (store.SetupBatch, error)
	PurgeSetupBatchContent(at int64) (int, error)
}

// Broker is the machine call: terminalbroker.Broker satisfies it.
type Broker interface {
	SetupFleet(ctx context.Context, uid int64, agentID string, in terminalproto.SetupFleet) (terminalproto.SetupFleetResult, error)
}

// Config wires an Orchestrator.
type Config struct {
	Store  Store
	Broker Broker
	// Settled hears of a batch that reached done or stopped, for auditing.
	Settled func(uid int64, batch store.SetupBatch)
	// Logf logs without content; nil means log.Printf.
	Logf func(format string, args ...any)
	// Paused, when it returns true, stops the driver applying any member.
	Paused func() bool
	// PurgeEvery is how often held setup text past its time is deleted; zero
	// means ten minutes.
	PurgeEvery time.Duration
	// PurgeAlso deletes other held setup text past its time on each purge.
	PurgeAlso func(now time.Time)
	// Now is the clock for purging; nil means time.Now.
	Now func() time.Time
}

// Orchestrator drives setup batches, one goroutine per batch.
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

type driving struct{ again bool }

// New makes an orchestrator and starts its purge loop. Close stops it.
func New(cfg Config) *Orchestrator {
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.PurgeEvery <= 0 {
		cfg.PurgeEvery = 10 * time.Minute
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	o := &Orchestrator{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[batchKey]*driving{}}
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		ticker := time.NewTicker(cfg.PurgeEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				o.Purge()
			}
		}
	}()
	return o
}

// Purge deletes held setup text that is past its time.
func (o *Orchestrator) Purge() {
	now := o.cfg.Now()
	if o.cfg.PurgeAlso != nil {
		o.cfg.PurgeAlso(now)
	}
	if _, err := o.cfg.Store.PurgeSetupBatchContent(now.UnixMilli()); err != nil {
		o.cfg.Logf("setup batches: purge: %v", err)
	}
}

// Start drives the batch's pending members in the background.
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

// Resume deletes expired text and continues every batch with pending work.
func (o *Orchestrator) Resume() error {
	o.Purge()
	pending, err := o.cfg.Store.ResumeSetupBatches()
	if err != nil {
		return err
	}
	for _, p := range pending {
		o.Start(p.UID, p.BatchID)
	}
	return nil
}

// Close stops starting members and waits for calls in flight.
func (o *Orchestrator) Close() {
	o.mu.Lock()
	o.closed = true
	o.mu.Unlock()
	o.cancel()
	o.wg.Wait()
}

// Retry moves one failed member back to pending and starts the batch again.
func (o *Orchestrator) Retry(uid int64, batchID string, position int) error {
	if _, err := o.cfg.Store.RetrySetupBatchMember(uid, batchID, position); err != nil {
		return err
	}
	o.Start(uid, batchID)
	return nil
}

// Skip marks one failed member skipped and continues.
func (o *Orchestrator) Skip(uid int64, batchID string, position int) error {
	if err := o.cfg.Store.SkipSetupBatchMember(uid, batchID, position); err != nil {
		return err
	}
	o.Start(uid, batchID)
	return nil
}

// Abandon ends a batch between machines.
func (o *Orchestrator) Abandon(uid int64, batchID string) (store.SetupBatch, error) {
	return o.cfg.Store.AbandonSetupBatch(uid, batchID)
}

func (o *Orchestrator) drive(k batchKey, d *driving) {
	defer o.wg.Done()
	for {
		o.mu.Lock()
		d.again = false
		o.mu.Unlock()
		terminal := o.pass(k)
		o.mu.Lock()
		if terminal && !d.again || o.closed {
			delete(o.runs, k)
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
	}
}

// pass applies pending members in position order until one fails, the batch
// ends, or the controller stops.
func (o *Orchestrator) pass(k batchKey) bool {
	for {
		if o.ctx.Err() != nil || o.cfg.Paused != nil && o.cfg.Paused() {
			return true
		}
		batch, err := o.cfg.Store.SetupBatchFor(k.uid, k.id)
		if err != nil {
			o.cfg.Logf("setup batch %s: %v", k.id, err)
			return true
		}
		if batch.State != store.SoftwareBatchPending && batch.State != store.SoftwareBatchRunning {
			return true // done, abandoned, or stopped for a person
		}
		next := -1
		running := false
		for _, m := range batch.Members {
			running = running || m.State == store.SoftwareMemberRunning
			if next < 0 && m.State == store.SoftwareMemberPending {
				next = m.Position
			}
		}
		if next < 0 {
			if !running {
				o.settled(k.uid, batch)
			}
			return true
		}
		member, err := o.cfg.Store.SetupBatchMemberStart(k.uid, k.id, next)
		if errors.Is(err, store.ErrSetupBatchMemberNotPending) {
			continue
		}
		if err != nil {
			o.cfg.Logf("setup batch %s member %d: %v", k.id, next, err)
			return true
		}
		o.step(k.uid, batch.ID, member)
		current, err := o.cfg.Store.SetupBatchFor(k.uid, k.id)
		if err != nil {
			o.cfg.Logf("setup batch %s: %v", k.id, err)
			return true
		}
		switch current.State {
		case store.SoftwareBatchDone, store.SoftwareBatchStopped:
			o.settled(k.uid, current)
			return true
		case store.SoftwareBatchAbandoned:
			return true
		}
	}
}

// step applies one member and records its outcome.
func (o *Orchestrator) step(uid int64, batchID string, member store.SetupBatchMember) {
	record := func(state, code, journal string) {
		if err := o.cfg.Store.SetSetupBatchMemberState(uid, batchID, member.Position, state, code, journal); err != nil {
			o.cfg.Logf("setup batch %s member %d: record %s: %v", batchID, member.Position, state, err)
		}
	}
	raw, err := o.cfg.Store.SetupBatchItems(uid, batchID)
	var items []terminalproto.SetupItem
	if err == nil {
		err = json.Unmarshal(raw, &items)
	}
	if err != nil {
		record(store.SoftwareMemberFailed, CodeContentGone, "")
		return
	}
	request := terminalproto.SetupFleet{Op: terminalproto.SetupOpApply, Items: items, Fingerprint: member.Fingerprint, Label: "fleet-" + batchID}
	ctx, cancel := context.WithTimeout(o.ctx, ApplyTimeout)
	defer cancel()
	result, err := o.cfg.Broker.SetupFleet(ctx, uid, member.AgentID, request)
	if o.ctx.Err() != nil {
		return // shutting down: the member stays running for Resume
	}
	switch {
	case errors.Is(err, terminalbroker.ErrNotSent):
		// Nothing reached the machine, so nothing changed there.
		record(store.SoftwareMemberSkipped, CodeOffline, "")
	case err != nil:
		record(store.SoftwareMemberFailed, failureCode(err), "")
	case result.State == terminalproto.SetupOK && terminalproto.ValidSetupJournal(result.Journal):
		record(store.SoftwareMemberApplied, "", result.Journal)
	case result.State == terminalproto.SetupRefused || result.State == terminalproto.SetupFailed:
		record(store.SoftwareMemberFailed, result.Reason, "")
	default:
		record(store.SoftwareMemberFailed, CodeBadAnswer, "")
	}
}

func (o *Orchestrator) settled(uid int64, batch store.SetupBatch) {
	if o.cfg.Settled != nil {
		o.cfg.Settled(uid, batch)
	}
}

func failureCode(err error) string {
	switch {
	case errors.Is(err, terminalbroker.ErrOffline):
		// The connection dropped after the apply went out: the machine may
		// have done it, and a retry finds its journal if it did.
		return CodeConnectionLost
	case errors.Is(err, terminalbroker.ErrSetupUnsupported):
		return CodeUnsupported
	case errors.Is(err, terminalbroker.ErrSetupAnswer):
		return CodeBadAnswer
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return CodeTimedOut
	default:
		return CodeApplyFailed
	}
}
