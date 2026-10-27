// Package projectrun starts a Project's run: one agent per repository, through
// the same create path a single launch uses.
//
// Every create carries a request id derived from the run, the member's
// position and its attempt, so a repeated create is the same request: the
// broker and the machine answer it with the session they already made and
// start nothing new. Just before a create is handed to the broker the member
// is durably marked "sent"; the mark is cleared only when that create is
// known to have started nothing. A retry reuses the attempt (the same request
// id) while the mark is set, whatever the last failure was, and takes the next
// attempt only when it is clear (store.RetryRunMember decides this in one
// transaction). A new request id with the member's worktree token would make
// the machine adopt the worktree and start a second agent in it, so this rule
// is what keeps a member to one agent.
//
// Limits: at most MaxInFlight members of one run, and at most one create per
// machine across all runs, are in flight at once. A member whose machine is
// not ready waits for it (StartWait, or ResumeWait after a restart) without
// holding a slot.
//
// A run interrupted by a controller restart is resumed by Resume: only
// members still pending are started, with their unchanged request ids, and a
// started member is never started again.
//
// A selection run (Start in several projects, store.StartSelectionRunFor)
// follows the same rules. Each member is its own folder project: its create
// names the project (the store checks its machine and folder again), its
// worktree is made under the project's slug from its own base branch, and the
// run's effort and YOLO go to every member. A folder that is not a repository
// starts in its folder.
//
// The shared prompt is sent to the machine and nowhere else: it is never
// logged, audited or put in an error.
package projectrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MaxInFlight is how many members of one run are being started at once.
const MaxInFlight = 2

// Failure codes this package stores on a member. The API words each one.
const (
	// CodeUnknownOutcome: the create was sent (or may have been) and no
	// answer came. The agent may have started.
	CodeUnknownOutcome = "unknown_outcome"
	// CodeSessionLost: the machine reports no such session, so nothing runs.
	CodeSessionLost = "session_lost"
	// CodeMachineRefused: the machine answered the create with a refusal
	// (denied, conflict, failed, not found).
	CodeMachineRefused      = "machine_refused"
	CodeTerminalsDisabled   = "terminals_disabled"
	CodeUnknownMachine      = "unknown_machine"
	CodeMachineUnavailable  = "machine_unavailable"
	CodeSessionLimit        = "session_limit"
	CodeLaunchUnsupported   = "launch_unsupported"
	CodeWorktreeUnsupported = "worktree_unsupported"
	CodeToolNotInstalled    = "tool_not_installed"
	CodeFolderUnavailable   = "folder_unavailable"
	CodePromptUnsupported   = "prompt_unsupported"
	CodeWorktreeRefused     = "worktree_refused"
	CodeWorktreeMismatch    = "worktree_mismatch"
	CodeInvalidLaunch       = "invalid_launch"
	// CodeInternalError: the controller could not record a step (a database
	// error). The member's sent mark is left as it was, so a retry is safe.
	CodeInternalError = "internal_error"
	// Refusals of a selection run's launch options, all before anything is
	// reserved: YOLO, an option the agent's preset lacks, an agent reet does
	// not start, and an agent the machine does not have.
	CodeYoloRefused             = "yolo_refused"
	CodeLaunchOptionUnsupported = "launch_option_unsupported"
	CodeToolUnavailable         = "tool_unavailable"
	// CodeOptionNeedsUpdate: an option (base, effort, YOLO) refused because
	// the machine has not negotiated agents-v2; updating reet there helps.
	CodeOptionNeedsUpdate = "option_needs_update"
	// CodeProjectMoved: the member's folder project was removed, or moved to
	// another machine or folder, since the run started; the session is not
	// put in it.
	CodeProjectMoved = "project_moved"
)

// worktreeCodes are the create refusal reasons and the code stored for each.
var worktreeCodes = map[terminalproto.WorktreeReason]string{
	terminalproto.WorktreeNotRepoRoot: "not_repo_root",
	terminalproto.WorktreeRepoBusy:    "repo_busy",
	terminalproto.WorktreeExists:      "worktree_exists",
	terminalproto.WorktreeGitFailed:   "git_failed",
	terminalproto.WorktreeOutsideHome: "outside_home",
	terminalproto.WorktreeBaseMissing: "base_missing",
}

// RetryAttempt is the attempt a retry of the failed member m uses: the same
// one (the same request id) while its create may have been sent, the next
// one only when that create is known to have started nothing. The store
// applies the same rule in RetryRunMember.
func RetryAttempt(m store.RunMember) int {
	if m.Sent {
		return m.Attempt
	}
	return m.Attempt + 1
}

// RequestID is the create request id of a member's attempt: the first 48 hex
// characters of SHA-256 of "run|position|attempt".
func RequestID(run string, position, attempt int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d", run, position, attempt))
	return hex.EncodeToString(sum[:])[:48]
}

// WorktreeToken is a member's run token: the first 24 hex characters of
// SHA-256 of "worktree|run|position". It is per member, never per run (two
// members on one machine never share a worktree) and never per attempt (a
// retry adopts the worktree an earlier attempt made).
func WorktreeToken(run string, position int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "worktree|%s|%d", run, position))
	return hex.EncodeToString(sum[:])[:24]
}

// Title is the session title for a member: the Project's name and the
// folder's last part, at most terminalproto.MaxTitleRunes characters.
func Title(project, dir string) string {
	title := project
	if base := path.Base(dir); base != "/" && base != "." && base != "" {
		title = project + " · " + base
	}
	if r := []rune(title); len(r) > terminalproto.MaxTitleRunes {
		title = string(r[:terminalproto.MaxTitleRunes])
	}
	if !terminalproto.ValidTitle(title) {
		title = "Project"
	}
	return title
}

// SelectionTitle is the session title for a member of a start in several
// projects, as a single launch titles one: the prompt's first line with text
// (tabs as spaces, without invisible characters, at most
// terminalproto.MaxTitleRunes characters), else the project's name.
func SelectionTitle(prompt, name string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(strings.Map(func(r rune) rune {
			switch {
			case r == '\t':
				return ' '
			case unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029':
				return -1
			}
			return r
		}, line))
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > terminalproto.MaxTitleRunes {
			line = strings.TrimSpace(string(r[:terminalproto.MaxTitleRunes]))
		}
		if terminalproto.ValidTitle(line) {
			return line
		}
		break
	}
	return Title(name, "/")
}

// Broker is the create path: terminalbroker.Broker satisfies it.
type Broker interface {
	CreateSession(ctx context.Context, uid int64, agent, requestID string, in terminalproto.Create) (store.TerminalSession, bool, error)
	Ready(uid int64, agent string) bool
}

// Store is what the orchestrator reads and writes: store.Store satisfies it.
type Store interface {
	RunFor(uid int64, runID string) (store.Run, error)
	PendingRuns() ([]store.PendingRun, error)
	RunMemberStart(uid int64, runID string, position int) (store.RunMember, error)
	ReleaseRunMember(uid int64, runID string, position int)
	SetRunMemberState(uid int64, runID string, position int, state, code, sessionID, worktreePath, worktreeRepo, worktreeBranch string, attempt int) error
	SetRunMemberWorktreeToken(uid int64, runID string, position int, token string) error
	SettleRunMemberBase(uid int64, runID string, position int, keep bool) error
	MarkRunMemberSent(uid int64, runID string, position, attempt int) error
	ClearRunMemberSent(uid int64, runID string, position, attempt int) error
	RetryRunMember(uid int64, runID string, position int) (store.RunMember, error)
	SyncAgentsFor(uid int64) ([]store.SyncAgent, error)
	RemoteCapabilitiesFor(uid int64, agent string) (store.RemoteAgentCapabilities, error)
	TerminalOperationFor(uid int64, agent, kind, requestID string) (store.TerminalOperation, error)
}

// Config wires an Orchestrator.
type Config struct {
	Store  Store
	Broker Broker
	// Enabled reports whether remote terminals are on; nil means always.
	Enabled func() bool
	// Launched hears of every member that moved to started, exactly once per
	// member, to audit the launch. The create carries the prompt; the callee
	// must not record it.
	Launched func(uid int64, agent string, in terminalproto.Create, session store.TerminalSession)
	// Logf logs without request content; nil means log.Printf.
	Logf func(format string, args ...any)
	// StartWait is how long a member of a fresh start or retry waits for its
	// machine to be ready; zero means 30 seconds.
	StartWait time.Duration
	// ResumeWait is how long a resumed member waits for its machine to
	// reconnect after a controller restart; zero means two minutes.
	ResumeWait time.Duration
	// Poll is how often a waiting member checks its machine; zero means 500ms.
	Poll time.Duration
}

// Orchestrator drives runs. One goroutine per run and machine starts the
// run's pending members on that machine in position order.
type Orchestrator struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
	runs   map[runKey]*driving
	hosts  map[hostKey]*hostEntry
}

type runKey struct {
	uid int64
	run string
}

type hostKey struct {
	uid   int64
	agent string
}

// hostEntry is a machine's one create slot and how many members are using
// it; the entry is dropped when none are.
type hostEntry struct {
	slot  chan struct{}
	users int
}

type driving struct {
	again bool
	wait  time.Duration
}

// New makes an orchestrator. Close stops it.
func New(cfg Config) *Orchestrator {
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	if cfg.StartWait <= 0 {
		cfg.StartWait = 30 * time.Second
	}
	if cfg.ResumeWait <= 0 {
		cfg.ResumeWait = 2 * time.Minute
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[runKey]*driving{}, hosts: map[hostKey]*hostEntry{}}
}

// Start starts the run's pending members in the background. It returns at
// once; calling it again while the run is being driven makes the driver look
// for pending members once more when it has finished its current pass.
func (o *Orchestrator) Start(uid int64, runID string) { o.start(uid, runID, o.cfg.StartWait) }

// Resume continues every run that still has pending members, as after a
// controller restart. Members whose machine has not reconnected yet wait for
// it for ResumeWait before they are marked machine_unavailable.
func (o *Orchestrator) Resume() error {
	pending, err := o.cfg.Store.PendingRuns()
	if err != nil {
		return err
	}
	for _, p := range pending {
		o.start(p.UID, p.RunID, o.cfg.ResumeWait)
	}
	return nil
}

// Driving reports whether the run has a driver: started and not yet through
// its last pass. Start registers the driver before it returns, so once Driving
// is false every create that Start led to has returned.
func (o *Orchestrator) Driving(uid int64, runID string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	_, ok := o.runs[runKey{uid, runID}]
	return ok
}

// Done is closed when the orchestrator is closed.
func (o *Orchestrator) Done() <-chan struct{} { return o.ctx.Done() }

// Close stops starting members and waits for the calls in flight to return.
// A member whose call was cut short stays pending (still marked sent), to be
// resumed with the same request id. Close may be called more than once.
func (o *Orchestrator) Close() {
	o.mu.Lock()
	o.closed = true
	o.mu.Unlock()
	o.cancel()
	o.wg.Wait()
}

// Errors from Retry, besides the store's.
var (
	ErrNoSuchMember = errors.New("no repository at that position in the run")
	ErrNotFailed    = store.ErrRunMemberNotFailed
)

// Retry moves a failed member back to pending and starts it. The store picks
// the attempt (see RetryAttempt) in the same transaction, so of two racing
// retries exactly one proceeds and the other gets ErrNotFailed. During
// shutdown the member is moved to pending but not started; Resume on the next
// start picks it up with that attempt.
func (o *Orchestrator) Retry(uid int64, runID string, position int) (store.RunMember, error) {
	run, err := o.cfg.Store.RunFor(uid, runID)
	if err != nil {
		return store.RunMember{}, err
	}
	if !slices.ContainsFunc(run.Members, func(m store.RunMember) bool { return m.Position == position }) {
		return store.RunMember{}, ErrNoSuchMember
	}
	m, err := o.cfg.Store.RetryRunMember(uid, runID, position)
	if err != nil {
		return m, err
	}
	o.Start(uid, runID)
	return m, nil
}

func (o *Orchestrator) start(uid int64, runID string, wait time.Duration) {
	k := runKey{uid, runID}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return
	}
	if d := o.runs[k]; d != nil {
		d.again = true
		d.wait = max(d.wait, wait)
		return
	}
	d := &driving{wait: wait}
	o.runs[k] = d
	o.wg.Add(1)
	go o.drive(k, d)
}

func (o *Orchestrator) drive(k runKey, d *driving) {
	defer o.wg.Done()
	for {
		o.mu.Lock()
		// Each pass takes the wait it was asked for; a later Retry pass does
		// not inherit a Resume's longer wait.
		wait := d.wait
		d.again, d.wait = false, 0
		o.mu.Unlock()
		o.pass(k.uid, k.run, wait)
		o.mu.Lock()
		if !d.again || o.closed {
			delete(o.runs, k)
			o.mu.Unlock()
			return
		}
		o.mu.Unlock()
	}
}

// hostSlot is the one create slot of a machine, shared by every run. Every
// call must be paired with releaseHost.
func (o *Orchestrator) hostSlot(uid int64, agent string) chan struct{} {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := hostKey{uid, agent}
	e := o.hosts[k]
	if e == nil {
		e = &hostEntry{slot: make(chan struct{}, 1)}
		o.hosts[k] = e
	}
	e.users++
	return e.slot
}

func (o *Orchestrator) releaseHost(uid int64, agent string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := hostKey{uid, agent}
	if e := o.hosts[k]; e != nil {
		if e.users--; e.users <= 0 {
			delete(o.hosts, k)
		}
	}
}

// pass starts every member that is pending now. Members on one machine go
// in position order, one after another; machines proceed in parallel, with at
// most MaxInFlight creates of this run in flight. One member's failure never
// stops the others.
func (o *Orchestrator) pass(uid int64, runID string, wait time.Duration) {
	run, err := o.cfg.Store.RunFor(uid, runID)
	if err != nil {
		o.cfg.Logf("project run %s: %v", runID, err)
		return
	}
	byHost := map[string][]int{}
	var hosts []string
	for _, m := range run.Members {
		if m.State != store.RunMemberPending {
			continue
		}
		if byHost[m.Host] == nil {
			hosts = append(hosts, m.Host)
		}
		byHost[m.Host] = append(byHost[m.Host], m.Position)
	}
	slots := make(chan struct{}, MaxInFlight)
	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Add(1)
		go func(host string, positions []int) {
			defer wg.Done()
			for _, position := range positions {
				if o.ctx.Err() != nil {
					return
				}
				o.member(uid, run, host, position, wait, slots)
			}
		}(host, byHost[host])
	}
	wg.Wait()
}

// member waits, without holding a slot, until the member's machine can take
// a create (or the wait runs out), then takes the machine's slot and a run
// slot and starts it. A machine that stops being ready while the member
// queued sends it back to waiting.
func (o *Orchestrator) member(uid int64, run store.Run, host string, position int, wait time.Duration, slots chan struct{}) {
	deadline := time.Now().Add(wait)
	hostSlot := o.hostSlot(uid, host)
	defer o.releaseHost(uid, host)
	for {
		if code := o.precheck(uid, host, deadline); code != "" {
			if o.ctx.Err() == nil {
				o.startMember(uid, run, position, code)
			}
			return
		}
		select {
		case hostSlot <- struct{}{}:
		case <-o.ctx.Done():
			return
		}
		select {
		case slots <- struct{}{}:
		case <-o.ctx.Done():
			<-hostSlot
			return
		}
		code := o.precheck(uid, host, time.Time{})
		done := code == "" || !time.Now().Before(deadline)
		if done {
			o.startMember(uid, run, position, code)
		}
		<-slots
		<-hostSlot
		if done {
			return
		}
	}
}

// precheck refuses a member when its machine is not one of the account's
// enrolled machines (a run never reaches another account's machine), when
// terminals are off, or when the machine is not connected. Until the deadline
// it waits for terminals and the machine instead of refusing.
func (o *Orchestrator) precheck(uid int64, agent string, deadline time.Time) string {
	agents, err := o.cfg.Store.SyncAgentsFor(uid)
	if err != nil || !slices.ContainsFunc(agents, func(a store.SyncAgent) bool { return a.ID == agent }) {
		return CodeUnknownMachine
	}
	for {
		code := ""
		switch {
		case o.cfg.Enabled != nil && !o.cfg.Enabled():
			code = CodeTerminalsDisabled
		case !o.cfg.Broker.Ready(uid, agent):
			code = CodeMachineUnavailable
		}
		if code == "" || !time.Now().Before(deadline) {
			return code
		}
		select {
		case <-o.ctx.Done():
			return code
		case <-time.After(o.cfg.Poll):
		}
	}
}

// startMember claims one pending member and either records the precheck's
// refusal (code) or sends its create and records the outcome. A member
// another caller holds, or one no longer pending, is left alone.
func (o *Orchestrator) startMember(uid int64, run store.Run, position int, code string) {
	m, err := o.cfg.Store.RunMemberStart(uid, run.ID, position)
	if err != nil {
		return
	}
	defer o.cfg.Store.ReleaseRunMember(uid, run.ID, position)
	if code != "" {
		// Nothing is sent now; a mark left by an earlier send of this attempt
		// stays, so a retry still reuses its request id.
		o.fail(uid, run.ID, m, code)
		return
	}
	in := terminalproto.Create{Title: Title(run.ProjectName, m.Dir), Cols: 100, Rows: 30, Tool: m.Tool, Model: m.Model, Dir: m.Dir, Prompt: run.Prompt}
	project := run.ProjectName
	if run.Origin == store.RunOriginSelection {
		// Each member is its own folder project: the session belongs to it
		// (the store checks its machine and folder again) and is titled as a
		// single launch is, from the prompt.
		project = m.Name
		in.Title = SelectionTitle(run.Prompt, m.Name)
		in.ProjectID, in.Effort, in.Yolo = m.ProjectID, run.Effort, run.Yolo
	}
	if m.InWorktree(run) {
		token := WorktreeToken(run.ID, position)
		if err := o.cfg.Store.SetRunMemberWorktreeToken(uid, run.ID, position, token); err != nil {
			o.cfg.Logf("project run %s member %d: worktree token: %v", run.ID, position, err)
			o.fail(uid, run.ID, m, CodeInternalError) // nothing was sent now
			return
		}
		in.Worktree = token
		in.Project = terminalproto.Slug(project, "project")
		if m.BaseInherited {
			// Only the account's default: a machine without agents-v2 would
			// refuse the start for it, so it starts from the checkout's HEAD
			// there instead. Decided once and stored before anything is sent,
			// so a replay of this request id carries the same base.
			keep := true
			if caps, err := o.cfg.Store.RemoteCapabilitiesFor(uid, m.Host); err == nil && !caps.Capabilities.AgentsV2 {
				keep = false
			}
			if err := o.cfg.Store.SettleRunMemberBase(uid, run.ID, position, keep); err != nil {
				o.cfg.Logf("project run %s member %d: settle base: %v", run.ID, position, err)
				o.fail(uid, run.ID, m, CodeInternalError) // nothing was sent now
				return
			}
			if !keep {
				m.Base = ""
			}
			m.BaseInherited = false
		}
		in.Base = m.Base
	}
	request := RequestID(run.ID, position, m.Attempt)
	// Durable before the call: from here on the create may reach the machine.
	if err := o.cfg.Store.MarkRunMemberSent(uid, run.ID, position, m.Attempt); err != nil {
		o.cfg.Logf("project run %s member %d: mark sent: %v", run.ID, position, err)
		o.fail(uid, run.ID, m, CodeInternalError) // nothing was sent now
		return
	}
	session, _, err := o.cfg.Broker.CreateSession(o.ctx, uid, m.Host, request, in)
	if err != nil && o.ctx.Err() != nil {
		return // shutting down: stays pending and is resumed with the same request id
	}
	if err == nil && session.State != terminalproto.Creating && session.State != terminalproto.Lost {
		if err := o.cfg.Store.SetRunMemberState(uid, run.ID, position, store.RunMemberStarted, "", session.ID,
			session.WorktreePath, session.WorktreeRepo, session.WorktreeBranch, m.Attempt); err != nil {
			o.cfg.Logf("project run %s member %d: record started: %v", run.ID, position, err)
			// The agent runs and the member stays marked sent, so a retry
			// sends the same request id and records the session it gets back.
			o.fail(uid, run.ID, m, CodeInternalError)
			return
		}
		// Audited when the member becomes started, whichever call learned the
		// session started (a replay after a lost answer included): once.
		if o.cfg.Launched != nil {
			o.cfg.Launched(uid, m.Host, in, session)
		}
		return
	}
	code, nothingStarted := o.classify(uid, m, request, session, err)
	if nothingStarted {
		if err := o.cfg.Store.ClearRunMemberSent(uid, run.ID, position, m.Attempt); err != nil {
			o.cfg.Logf("project run %s member %d: clear sent: %v", run.ID, position, err)
		}
	}
	o.fail(uid, run.ID, m, code)
}

// classify maps a create that did not start an agent to the code stored on
// the member, and says whether the create is known to have started nothing
// under its request id. Those are the refusals the broker gives only for a
// request id it never reserved (the capability, capacity and readiness checks
// all come after the reservation lookup), validation of the identical create,
// and the machine's own refusals and reports. Everything else, including a
// lost reply, the machine going away mid-call and a revoked machine, keeps
// the member marked sent.
func (o *Orchestrator) classify(uid int64, m store.RunMember, request string, session store.TerminalSession, err error) (string, bool) {
	var refusal *terminalbroker.WorktreeError
	switch {
	case err == nil && session.State == terminalproto.Lost:
		if op, e := o.cfg.Store.TerminalOperationFor(uid, m.Host, "create", request); e == nil && op.State == "failed" {
			switch op.Code {
			case terminalproto.Capacity:
				return CodeSessionLimit, true
			case terminalproto.Denied, terminalproto.Failed, terminalproto.NotFound:
				return CodeMachineRefused, true
			case terminalproto.Conflict:
				// The machine already has something under this request or
				// session: it may be the agent, so the request id is kept.
				return CodeMachineRefused, false
			}
		}
		return CodeSessionLost, true
	case err == nil:
		return CodeUnknownOutcome, false
	case errors.Is(err, terminalbroker.ErrPromptUnsupported):
		return CodePromptUnsupported, true
	case errors.Is(err, terminalbroker.ErrWorktreeUnsupported):
		return CodeWorktreeUnsupported, true
	case errors.As(err, &refusal) && worktreeCodes[refusal.Reason] != "":
		return worktreeCodes[refusal.Reason], true
	case errors.Is(err, terminalbroker.ErrWorktreeRefused):
		return CodeWorktreeRefused, true
	case errors.Is(err, terminalbroker.ErrWorktreeMismatch):
		return CodeWorktreeMismatch, false // the agent started
	case errors.Is(err, terminalbroker.ErrLaunchUnsupported):
		return CodeLaunchUnsupported, true
	case errors.Is(err, terminalbroker.ErrLaunchFailed):
		if caps, e := o.cfg.Store.RemoteCapabilitiesFor(uid, m.Host); e == nil && !slices.Contains(caps.Capabilities.Tools, m.Tool) {
			return CodeToolNotInstalled, true
		}
		return CodeFolderUnavailable, true
	case errors.Is(err, store.ErrTerminalCapacity):
		return CodeSessionLimit, true
	case errors.Is(err, store.ErrTerminalNotReady):
		return CodeMachineUnavailable, true
	case errors.Is(err, store.ErrRemoteNotFound):
		return CodeUnknownMachine, false // checked before the reservation lookup
	case errors.Is(err, terminalproto.ErrInvalid):
		return CodeInvalidLaunch, true
	// A selection run's options and its project, all checked before anything
	// is reserved.
	case errors.Is(err, terminalbroker.ErrYoloRefused):
		return CodeYoloRefused, true
	case errors.Is(err, terminalbroker.ErrLaunchOptionUnsupported):
		// As a single launch words it: the agent's own limit on a machine
		// with agents-v2, else the machine's version.
		if caps, e := o.cfg.Store.RemoteCapabilitiesFor(uid, m.Host); e == nil && caps.Capabilities.AgentsV2 {
			return CodeLaunchOptionUnsupported, true
		}
		return CodeOptionNeedsUpdate, true
	case errors.Is(err, terminalbroker.ErrToolUnavailable):
		return CodeToolUnavailable, true
	case errors.Is(err, terminalbroker.ErrToolNotInstalled):
		return CodeToolNotInstalled, true
	case errors.Is(err, store.ErrFolderProjectNotFound), errors.Is(err, store.ErrProjectMachineMismatch), errors.Is(err, store.ErrProjectFolderMismatch):
		return CodeProjectMoved, true
	}
	return CodeUnknownOutcome, false
}

// fail records a failure. If even that cannot be written the member stays
// pending, and the next Start, Retry pass or Resume picks it up.
func (o *Orchestrator) fail(uid int64, runID string, m store.RunMember, code string) {
	if err := o.cfg.Store.SetRunMemberState(uid, runID, m.Position, store.RunMemberFailed, code, "", "", "", "", m.Attempt); err != nil {
		o.cfg.Logf("project run %s member %d: record %s: %v", runID, m.Position, code, err)
	}
}
