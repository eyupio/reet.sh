// Package projectrun starts a Project's run: one agent per repository, through
// the same create path a single launch uses, at most two at a time per run.
//
// Every create carries a request id derived from the run, the member's
// position and its attempt, so a repeated create is the same request: the
// broker and the machine answer it with the session they already made and
// start nothing new. A retry keeps the request id when the earlier outcome is
// unknown and takes the next attempt only after a definite refusal, when no
// agent was started (see RetryAttempt). A run interrupted by a controller
// restart is resumed by Resume: only members still pending are started, with
// their unchanged request ids, and a started member is never started again.
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
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// MaxInFlight is how many members of one run are being started at once.
const MaxInFlight = 2

// Failure codes this package stores on a member. The API words each one.
const (
	// CodeUnknownOutcome: the create was sent (or may have been) and no
	// answer came. The agent may have started, so a retry reuses the request id.
	CodeUnknownOutcome = "unknown_outcome"
	// CodeSessionLost: the machine reports no such session, so nothing runs.
	CodeSessionLost         = "session_lost"
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
)

// worktreeCodes are the create refusal reasons and the code stored for each.
var worktreeCodes = map[terminalproto.WorktreeReason]string{
	terminalproto.WorktreeNotRepoRoot: "not_repo_root",
	terminalproto.WorktreeRepoBusy:    "repo_busy",
	terminalproto.WorktreeExists:      "worktree_exists",
	terminalproto.WorktreeGitFailed:   "git_failed",
	terminalproto.WorktreeOutsideHome: "outside_home",
}

// sameRequestCodes are the failures after which an agent may be running: a
// retry must send the same request id, which the broker and the machine
// answer with the session they already have instead of starting another.
// worktree_mismatch is one: the agent started, only the machine's report of
// the folder did not match.
var sameRequestCodes = map[string]bool{CodeUnknownOutcome: true, CodeWorktreeMismatch: true}

// RetryAttempt is the attempt a retry of the failed member m uses: the same
// one (the same request id) when the outcome may have been a started agent,
// the next one (a new request id) after a definite refusal.
func RetryAttempt(m store.RunMember) int {
	if sameRequestCodes[m.Code] {
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
	SyncAgentsFor(uid int64) ([]store.SyncAgent, error)
	RemoteCapabilitiesFor(uid int64, agent string) (store.RemoteAgentCapabilities, error)
}

// Config wires an Orchestrator.
type Config struct {
	Store  Store
	Broker Broker
	// Enabled reports whether remote terminals are on; nil means always.
	Enabled func() bool
	// Launched hears of every agent this orchestrator learned had started,
	// exactly once per session, to audit it. The create carries the prompt;
	// the callee must not record it.
	Launched func(uid int64, agent string, in terminalproto.Create, session store.TerminalSession)
	// Logf logs without request content; nil means log.Printf.
	Logf func(format string, args ...any)
	// ResumeWait is how long a resumed member waits for its machine to
	// reconnect after a controller restart; zero means two minutes.
	ResumeWait time.Duration
	// Poll is how often a waiting member checks its machine; zero means 500ms.
	Poll time.Duration
}

// Orchestrator drives runs. One goroutine per run starts its pending members.
type Orchestrator struct {
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	closed bool
	runs   map[runKey]*driving
}

type runKey struct {
	uid int64
	run string
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
	if cfg.ResumeWait <= 0 {
		cfg.ResumeWait = 2 * time.Minute
	}
	if cfg.Poll <= 0 {
		cfg.Poll = 500 * time.Millisecond
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{cfg: cfg, ctx: ctx, cancel: cancel, runs: map[runKey]*driving{}}
}

// Start starts the run's pending members in the background. It returns at
// once; calling it again while the run is being driven makes the driver look
// for pending members once more when it has finished its current pass.
func (o *Orchestrator) Start(uid int64, runID string) { o.start(uid, runID, 0) }

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

// Close stops starting members and waits for the calls in flight to return.
// A member whose call was cut short stays pending, to be resumed with the
// same request id. Close may be called more than once.
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
	ErrNotFailed    = errors.New("only a failed repository can be tried again")
)

// Retry moves a failed member back to pending with RetryAttempt and starts it.
func (o *Orchestrator) Retry(uid int64, runID string, position int) (store.RunMember, error) {
	run, err := o.cfg.Store.RunFor(uid, runID)
	if err != nil {
		return store.RunMember{}, err
	}
	i := slices.IndexFunc(run.Members, func(m store.RunMember) bool { return m.Position == position })
	if i < 0 {
		return store.RunMember{}, ErrNoSuchMember
	}
	m := run.Members[i]
	if m.State != store.RunMemberFailed {
		return m, ErrNotFailed
	}
	attempt := RetryAttempt(m)
	if err := o.cfg.Store.SetRunMemberState(uid, runID, position, store.RunMemberPending, "", "", "", "", "", attempt); err != nil {
		return m, err
	}
	o.Start(uid, runID)
	m.State, m.Code, m.Attempt = store.RunMemberPending, "", attempt
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
		wait := d.wait
		d.again = false
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

// pass starts every member that is pending now, in position order, with at
// most MaxInFlight at a time. One member's failure never stops the others.
func (o *Orchestrator) pass(uid int64, runID string, wait time.Duration) {
	run, err := o.cfg.Store.RunFor(uid, runID)
	if err != nil {
		o.cfg.Logf("project run %s: %v", runID, err)
		return
	}
	slots := make(chan struct{}, MaxInFlight)
	var wg sync.WaitGroup
	defer wg.Wait()
	for _, m := range run.Members {
		if m.State != store.RunMemberPending {
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-o.ctx.Done():
			return
		}
		wg.Add(1)
		go func(position int) {
			defer wg.Done()
			defer func() { <-slots }()
			o.startMember(uid, run, position, wait)
		}(m.Position)
	}
}

// startMember claims one pending member, sends its create and records the
// outcome. A member another caller holds, or one no longer pending, is left
// alone.
func (o *Orchestrator) startMember(uid int64, run store.Run, position int, wait time.Duration) {
	m, err := o.cfg.Store.RunMemberStart(uid, run.ID, position)
	if err != nil {
		return
	}
	defer o.cfg.Store.ReleaseRunMember(uid, run.ID, position)
	in := terminalproto.Create{Title: Title(run.ProjectName, m.Dir), Cols: 100, Rows: 30, Tool: m.Tool, Model: m.Model, Dir: m.Dir, Prompt: run.Prompt}
	if run.UseWorktrees {
		token := WorktreeToken(run.ID, position)
		if err := o.cfg.Store.SetRunMemberWorktreeToken(uid, run.ID, position, token); err != nil {
			o.cfg.Logf("project run %s member %d: worktree token: %v", run.ID, position, err)
			return // stays pending; nothing was sent
		}
		in.Worktree = token
		in.Project = terminalproto.Slug(run.ProjectName, "project")
	}
	if code := o.precheck(uid, m.Host, wait); code != "" {
		if o.ctx.Err() == nil {
			o.fail(uid, run.ID, m, code)
		}
		return
	}
	session, started, err := o.cfg.Broker.CreateSession(o.ctx, uid, m.Host, RequestID(run.ID, position, m.Attempt), in)
	if err != nil && o.ctx.Err() != nil {
		return // shutting down: stays pending and is resumed with the same request id
	}
	if err == nil && session.State != terminalproto.Creating && session.State != terminalproto.Lost {
		if started && o.cfg.Launched != nil {
			o.cfg.Launched(uid, m.Host, in, session)
		}
		if err := o.cfg.Store.SetRunMemberState(uid, run.ID, position, store.RunMemberStarted, "", session.ID,
			session.WorktreePath, session.WorktreeRepo, session.WorktreeBranch, m.Attempt); err != nil {
			o.cfg.Logf("project run %s member %d: record started: %v", run.ID, position, err)
		}
		return
	}
	o.fail(uid, run.ID, m, o.classify(uid, m, session, err))
}

// precheck refuses, before anything is sent, a member when its machine is
// not one of the account's enrolled machines (a run never reaches another
// account's machine), when terminals are off, or when the machine is not
// connected. A resumed member (wait > 0) waits that long for terminals to be
// on and its machine to reconnect.
func (o *Orchestrator) precheck(uid int64, agent string, wait time.Duration) string {
	agents, err := o.cfg.Store.SyncAgentsFor(uid)
	if err != nil || !slices.ContainsFunc(agents, func(a store.SyncAgent) bool { return a.ID == agent }) {
		return CodeUnknownMachine
	}
	deadline := time.Now().Add(wait)
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

// classify maps a create that did not start an agent to the code stored on
// the member. Only refusals that are known to have started nothing get their
// own code; everything else, including a lost reply and the machine going
// away mid-call, is an unknown outcome, retried with the same request id.
func (o *Orchestrator) classify(uid int64, m store.RunMember, session store.TerminalSession, err error) string {
	var refusal *terminalbroker.WorktreeError
	switch {
	case err == nil && session.State == terminalproto.Lost:
		return CodeSessionLost
	case err == nil:
		return CodeUnknownOutcome
	case errors.Is(err, terminalbroker.ErrPromptUnsupported):
		return CodePromptUnsupported
	case errors.Is(err, terminalbroker.ErrWorktreeUnsupported):
		return CodeWorktreeUnsupported
	case errors.As(err, &refusal) && worktreeCodes[refusal.Reason] != "":
		return worktreeCodes[refusal.Reason]
	case errors.Is(err, terminalbroker.ErrWorktreeRefused):
		return CodeWorktreeRefused
	case errors.Is(err, terminalbroker.ErrWorktreeMismatch):
		return CodeWorktreeMismatch
	case errors.Is(err, terminalbroker.ErrLaunchUnsupported):
		return CodeLaunchUnsupported
	case errors.Is(err, terminalbroker.ErrLaunchFailed):
		if caps, e := o.cfg.Store.RemoteCapabilitiesFor(uid, m.Host); e == nil && !slices.Contains(caps.Capabilities.Tools, m.Tool) {
			return CodeToolNotInstalled
		}
		return CodeFolderUnavailable
	case errors.Is(err, store.ErrTerminalCapacity):
		return CodeSessionLimit
	case errors.Is(err, store.ErrTerminalNotReady):
		return CodeMachineUnavailable
	case errors.Is(err, store.ErrRemoteNotFound):
		return CodeUnknownMachine
	case errors.Is(err, terminalproto.ErrInvalid):
		return CodeInvalidLaunch
	}
	return CodeUnknownOutcome
}

func (o *Orchestrator) fail(uid int64, runID string, m store.RunMember, code string) {
	if err := o.cfg.Store.SetRunMemberState(uid, runID, m.Position, store.RunMemberFailed, code, "", "", "", "", m.Attempt); err != nil {
		o.cfg.Logf("project run %s member %d: record %s: %v", runID, m.Position, code, err)
	}
}
