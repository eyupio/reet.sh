//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/eyupio/reet.sh/internal/setup"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// setupSlot allows one setup read, plan, apply or undo at a time on this
	// connector process, across its connections.
	setupSlot    = make(chan struct{}, 1)
	setupRunning atomic.Int32
	// setupAnswerAfter bounds one setup operation; each reads or writes a
	// handful of small files.
	setupAnswerAfter = 50 * time.Second
	// setupJournalDir is where applies keep their journals, the directory
	// reet setup journals lists; tests replace it.
	setupJournalDir = func() (string, error) { return setup.SnapshotDir() }
)

// handleSetupFleet serves a fleet setup copy. Every operation needs the
// machine's terminal access; anything that plans or changes files also needs
// its local allow-apply opt-in, read again for each request, and the machine
// unlocked.
func (r *agentReq) handleSetupFleet(op *terminalproto.SetupFleet) error {
	refuse := func(reason string) error {
		r.reply.Body = terminalproto.SetupFleetResult{Op: op.Op, State: terminalproto.SetupRefused, Reason: reason}
		return r.send(r.callCtx, r.reply)
	}
	if !allowed(r.opts.Consent) {
		return refuse(terminalproto.SetupReasonOff)
	}
	if r.opts.Locked != nil && r.opts.Locked() {
		return refuse(terminalproto.SetupReasonLocked)
	}
	if op.Op != terminalproto.SetupOpRead && !allowed(r.opts.ApplyConsent) {
		return refuse(terminalproto.SetupReasonNotAllowed)
	}
	req, home := *op, r.opts.Home
	return r.aside(r.callCtx, r.reply, setupSlot, &setupRunning, setupAnswerAfter,
		terminalproto.SetupFleetResult{Op: req.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonBusy},
		terminalproto.SetupFleetResult{Op: req.Op, State: terminalproto.SetupFailed, Reason: terminalproto.SetupReasonTimedOut},
		func(context.Context) terminalproto.Body { return setupFleet(home, req) })
}

// setupFleet does one operation and turns its outcome into a closed answer.
// Paths and error text stay on the machine.
func setupFleet(home string, req terminalproto.SetupFleet) terminalproto.SetupFleetResult {
	out := terminalproto.SetupFleetResult{Op: req.Op, State: terminalproto.SetupOK}
	fail := func(err error) terminalproto.SetupFleetResult {
		out.State, out.Reason = terminalproto.SetupFailed, terminalproto.SetupReasonWrite
		var refusal *setup.FleetRefusal
		switch {
		case errors.As(err, &refusal):
			out.State, out.Reason, out.Agent, out.Name = terminalproto.SetupRefused, refusal.Reason, refusal.Agent, refusal.Name
		case errors.Is(err, setup.ErrFleetChanged), errors.Is(err, setup.ErrRollbackChanged):
			out.State, out.Reason = terminalproto.SetupRefused, terminalproto.SetupReasonChanged
		case errors.Is(err, setup.ErrNoJournal):
			out.State, out.Reason = terminalproto.SetupRefused, terminalproto.SetupReasonNoJournal
		case errors.Is(err, setup.ErrRollbackDamaged):
			out.Reason = terminalproto.SetupReasonDamaged
		}
		out.Items, out.Files, out.Fingerprint, out.Journal = nil, nil, "", ""
		return out
	}
	journals, err := setupJournalDir()
	if err != nil && req.Op != terminalproto.SetupOpRead {
		return fail(err)
	}
	switch req.Op {
	case terminalproto.SetupOpRead:
		items, err := setup.ReadFleet(home)
		if err != nil {
			out = fail(err)
			if out.Reason == terminalproto.SetupReasonWrite {
				out.Reason = terminalproto.SetupReasonUnreadable
			}
			return out
		}
		out.Items = items
		if data, err := json.Marshal(out); err != nil || len(data) > terminalproto.MaxSetupBytes {
			return terminalproto.SetupFleetResult{Op: req.Op, State: terminalproto.SetupRefused, Reason: terminalproto.SetupReasonTooLarge}
		}
		if len(items) == 0 {
			out.State, out.Reason, out.Items = terminalproto.SetupRefused, terminalproto.SetupReasonNothing, nil
		}
	case terminalproto.SetupOpPlan:
		plan, err := setup.PlanFleet(home, req.Items)
		if err != nil {
			return fail(err)
		}
		out.Files, out.Fingerprint = plan.Files, plan.Fingerprint
		// Many long instruction diffs may not fit one frame: the lines go,
		// the files and fields stay.
		if data, err := json.Marshal(out); err != nil || len(data) > terminalproto.MaxSetupBytes {
			for i := range out.Files {
				if len(out.Files[i].Added)+len(out.Files[i].Removed) > 0 {
					out.Files[i].Added, out.Files[i].Removed, out.Files[i].Truncated = nil, nil, true
				}
			}
		}
	case terminalproto.SetupOpApply:
		journal, err := setup.ApplyFleet(home, journals, req.Label, req.Fingerprint, req.Items)
		if err != nil {
			return fail(err)
		}
		out.Journal, out.Fingerprint = journal, req.Fingerprint
	case terminalproto.SetupOpRollback:
		if _, err := setup.RollbackFleet(journals, req.Journal); err != nil {
			return fail(err)
		}
	}
	return out
}
