//go:build linux || darwin

// Package terminalagent dispatches typed private operations to the locally
// consented host manager. It exposes no general command or network forwarding.
package terminalagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	// browse is terminalhost.Browse; tests replace it to hold a browse.
	browse = terminalhost.Browse
	// browseAnswerAfter is when a browse that has not returned is answered
	// unreadable and its slot freed: after Browse's own 3 s deadline, before
	// the controller's 6 s. Browse cannot cut short every call (resolving the
	// path, opening the root, reading the folder), so on a hung mount it may
	// never return; its result is then dropped when it does.
	browseAnswerAfter = 4 * time.Second
	// branchesAnswerAfter does the same for a branch listing: git is killed
	// after 5 s (and given 2 s to exit), but resolving the folder can hang
	// as a browse can.
	branchesAnswerAfter = 8 * time.Second
	// modelsAnswerAfter is when a model listing that has not returned is
	// answered timed_out and its slot freed. ListModels kills its process at
	// 10 s; this leaves room for the reply and process reaping.
	modelsAnswerAfter = 12 * time.Second
	// softwareAnswerAfter is when an apply that has not returned is answered
	// timed_out. The download itself stops at softwareApplyTimeout, which is
	// shorter, so an answer never races a late install.
	softwareAnswerAfter = 5*time.Minute + 30*time.Second
	// modelListFloor is the shortest interval between two real listings for
	// the same tool. Within it the connector answers its last result.
	modelListFloor = 60 * time.Second
	// listModels is the host listing; tests replace it so no binary runs.
	listModels = (*terminalhost.Manager).ListModels
	// applySoftware is the host install; tests replace it so no download runs.
	applySoftware = terminalhost.ApplySoftware
	// browsesRunning, branchesRunning and modelsRunning count the aside work
	// alive in this process, including work already answered at its deadline
	// and still stuck. They are process-wide so reconnecting cannot multiply
	// stuck work.
	browsesRunning  atomic.Int32
	branchesRunning atomic.Int32
	modelsRunning   atomic.Int32
	softwareRunning atomic.Int32
)

// softwareSlot serialises software applies on this connector process, not just
// one connection, so two controllers cannot install at once.
var softwareSlot = make(chan struct{}, 1)

// softwareGate coordinates a software apply with new OpenCode sessions. An
// apply holds it for writing from before the download until the installation
// is recorded; the terminal and structured-chat launch paths hold it for
// reading while they create an OpenCode session. That closes the window
// between the controller's live-session check and the installation changing.
var softwareGate sync.RWMutex

// modelListCacheEntry is one tool's last listing and when it was made.
type modelListCacheEntry struct {
	result terminalproto.ModelsResult
	at     time.Time
}

// maxAsideRunning bounds the browse (and, separately, the branches) work
// alive in this process. While that many are stuck a new one is refused as
// conflict, even though its connection's slot is free, until one finishes.
const maxAsideRunning = 4

func Factory(opts terminalhost.ManagerOptions) agenthealth.TerminalFactory {
	return FactoryWithChats(opts, nil)
}

// FactoryWithChats builds the per-connection control handler around a
// process-scoped chat supervisor. A nil supervisor keeps the old
// connection-scoped behaviour for callers that do not own one.
func FactoryWithChats(opts terminalhost.ManagerOptions, supervisor *ChatSupervisor) agenthealth.TerminalFactory {
	return func(ctx context.Context, client *http.Client, headers http.Header, generation int64, send agenthealth.Sender) (func(context.Context, terminalproto.Message) error, func()) {
		ctx, cancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		// The chats attach first: the terminal state observer below hides a
		// session while a mode swap moves it between the two.
		chats := attachChats(supervisor, ctx, opts, generation, send, terminalproto.SupportsACPHistory(headers.Get(terminalproto.FeatureHeader)),
			terminalproto.SupportsACPToolDetails(headers.Get(terminalproto.FeatureHeader)),
			terminalproto.SupportsACPQuestion(headers.Get(terminalproto.FeatureHeader)))
		m, managerErr := terminalhost.NewManager(ctx, opts)
		switcher := &agentSwitch{chats: chats}
		if managerErr == nil {
			switcher.shells = m
		}
		if managerErr == nil {
			wg.Add(1)
			go func() { defer wg.Done(); _ = m.Run(ctx) }()
			wg.Add(1)
			go func() {
				defer wg.Done()
				observed := map[string]terminalproto.SessionChanged{}
				initial := true
				for {
					changed := m.Changed()
					// A session a swap is moving is left out: its terminal
					// ending is a hand-off, not the end of the row.
					sessions, err := observedShells(ctx, m, chats.sup)
					if err != nil {
						return
					}
					for _, s := range sessions {
						state := terminalproto.SessionChanged{
							State:      s.State,
							ExitCode:   s.ExitCode,
							ActivityAt: s.ActivityAt,
							BellAt:     s.BellAt,
							ExitStatus: s.ExitCode,
						}.ForFeatures(headers.Get(terminalproto.FeatureHeader))
						old, known := observed[s.ID]
						sameCode := old.ExitCode == nil && state.ExitCode == nil || old.ExitCode != nil && state.ExitCode != nil && *old.ExitCode == *state.ExitCode
						if !initial && (!known || old.State != state.State || !sameCode || old.ActivityAt != state.ActivityAt || old.BellAt != state.BellAt) {
							if send(ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: s.ID, Generation: generation, Body: state}) != nil {
								return
							}
						}
						observed[s.ID] = state
					}
					initial = false
					select {
					case <-ctx.Done():
						return
					case <-changed:
					}
				}
			}()
		}
		attachments := map[string]*terminalhost.Attachment{}
		// worktreeSlot allows one worktree status or remove at a time on this
		// machine. It runs on its own goroutine so the dispatcher, and the
		// controller's other requests, never wait for git.
		worktreeSlot := make(chan struct{}, 1)
		// browseSlot, branchesSlot and modelsSlot allow one browse, branch
		// listing and model listing at a time on this connection, each run
		// like a worktree request.
		browseSlot := make(chan struct{}, 1)
		branchesSlot := make(chan struct{}, 1)
		modelsSlot := make(chan struct{}, 1)
		// modelsMu guards the per-tool last listing used for the floor.
		var modelsMu sync.Mutex
		modelsLast := map[string]modelListCacheEntry{}
		// uploads bounds the image transfers in flight on this connection.
		uploads := &uploadCounter{max: maxUploadsInFlight}
		// aside runs work for reply off the dispatcher. A second request while
		// slot is held, or while maxAsideRunning of running are alive, is
		// answered busy at once. The answer is work's result, or late when
		// work has not returned after the given time; the slot is freed then
		// even if work is stuck, and a result that comes later is dropped. A
		// request whose connection has ended answers nothing.
		aside := func(callCtx context.Context, reply terminalproto.Message, slot chan struct{}, running *atomic.Int32, after time.Duration, busy, late terminalproto.Body, work func(context.Context) terminalproto.Body) error {
			select {
			case slot <- struct{}{}:
			default:
				reply.Body = busy
				return send(callCtx, reply)
			}
			if running.Add(1) > maxAsideRunning {
				running.Add(-1)
				<-slot
				reply.Body = busy
				return send(callCtx, reply)
			}
			done := make(chan terminalproto.Body, 1)
			// Not in wg: cleanup must not wait for work stuck in a syscall.
			go func() { defer running.Add(-1); done <- work(callCtx) }()
			wg.Add(1)
			go func() {
				defer wg.Done()
				timer := time.NewTimer(after)
				defer timer.Stop()
				var body terminalproto.Body
				select {
				case body = <-done:
				case <-timer.C:
					body = late
				case <-callCtx.Done():
				}
				// Free the slot before answering, so a request the controller
				// sends once it has this answer is not busy.
				<-slot
				if callCtx.Err() != nil {
					return
				}
				reply.Body = body
				_ = send(callCtx, reply)
			}()
			return nil
		}
		// Dispatch is called by one control reader; only stream workers use their
		// own immutable attachment. Cleanup happens after that reader has stopped.
		handler := func(callCtx context.Context, msg terminalproto.Message) error {
			if msg.Generation != generation {
				return terminalproto.ErrInvalid
			}
			reply := msg
			reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
			respond := func(err error) error {
				if err != nil {
					reply.Body = result(err)
				}
				return send(callCtx, reply)
			}
			// Launch fields are served only on a generation that negotiated
			// launch-v1; a plain shell is never substituted for them.
			if create, ok := msg.Body.(*terminalproto.Create); ok && create.Tool != "" && !terminalproto.SupportsLaunch(headers.Get(terminalproto.FeatureHeader)) {
				return respond(terminalhost.ErrLaunchUnavailable)
			}
			// A resume is accepted only on a generation that negotiated
			// resume-v1; an older controller never sends it.
			if create, ok := msg.Body.(*terminalproto.Create); ok && create.Resume && !terminalproto.SupportsResume(headers.Get(terminalproto.FeatureHeader)) {
				reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
				return send(callCtx, reply)
			}
			// Likewise a worktree is made only on a generation that negotiated
			// worktree-v1; without it the agent is never started in the folder.
			if create, ok := msg.Body.(*terminalproto.Create); ok && create.Worktree != "" && !terminalproto.SupportsWorktree(headers.Get(terminalproto.FeatureHeader)) {
				return respond(terminalhost.ErrLaunchUnavailable)
			}
			if create, ok := msg.Body.(*terminalproto.Create); ok {
				if (create.Base != "" || create.Effort != "" || create.Yolo || create.Tool != "" && !terminalproto.ValidTool(create.Tool)) && !terminalproto.SupportsAgents(headers.Get(terminalproto.FeatureHeader)) {
					reply.Body = terminalproto.Result{Code: terminalproto.LaunchOptionUnsupported}
					return send(callCtx, reply)
				}
			}
			switch body := msg.Body.(type) {
			case *terminalproto.ACPPrompt:
				if !terminalproto.SupportsACP(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				if len(body.Images) > 0 && !terminalproto.SupportsACPImages(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
			case *terminalproto.ACPModel, *terminalproto.ACPCancel, *terminalproto.ACPPermission:
				if !terminalproto.SupportsACP(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
			case *terminalproto.ACPMode:
				if !terminalproto.SupportsACPApproval(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
			case *terminalproto.SessionMode:
				if !terminalproto.SupportsSessionMode(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
			case *terminalproto.ACPQuestionAnswer:
				if !terminalproto.SupportsACP(headers.Get(terminalproto.FeatureHeader)) || !terminalproto.SupportsACPQuestion(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
			}
			if create, ok := msg.Body.(*terminalproto.Create); ok && create.Tool == "" && create.Dir != "" && !terminalproto.SupportsShellDir(headers.Get(terminalproto.FeatureHeader)) {
				reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
				return send(callCtx, reply)
			}
			if create, ok := msg.Body.(*terminalproto.Create); ok && create.Structured && !terminalproto.SupportsACP(headers.Get(terminalproto.FeatureHeader)) {
				reply.Body = terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "Update reet on this machine to use OpenCode chats."}
				return send(callCtx, reply)
			}
			// Worktree status and remove are served only on a generation that
			// negotiated worktree-v1, and are always answered with a
			// worktree_state or worktree_result.
			switch msg.Body.(type) {
			case *terminalproto.WorktreeStatus, *terminalproto.WorktreeRemove:
				reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Denied}
				if !terminalproto.SupportsWorktree(headers.Get(terminalproto.FeatureHeader)) {
					return send(callCtx, reply)
				}
				if managerErr != nil {
					reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Unavailable}
					return send(callCtx, reply)
				}
				select {
				case worktreeSlot <- struct{}{}:
				default:
					reply.Body = terminalproto.WorktreeResult{Code: terminalproto.Conflict}
					return send(callCtx, reply)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					body := answerWorktree(callCtx, m, msg.Body)
					// Free the slot before answering, so a request the
					// controller sends once it has this answer is not busy.
					<-worktreeSlot
					// A request whose connection has ended answers nothing: the
					// controller has already given up on it and reads status again.
					if callCtx.Err() != nil {
						return
					}
					reply.Body = body
					_ = send(callCtx, reply)
				}()
				return nil
			}
			// Browse and branches are served only on a generation that
			// negotiated browse-v1 or branches-v1. Without it the message is a
			// protocol error and nothing is sent back: a peer that did not
			// negotiate a kind may drop the connection on its answer. Neither
			// needs the manager (tmux).
			switch op := msg.Body.(type) {
			case *terminalproto.Browse:
				if !terminalproto.SupportsBrowse(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				if !allowed(opts.Consent) || !allowed(opts.BrowseConsent) {
					reply.Body = terminalproto.BrowseResult{Reason: terminalproto.BrowseOff}
					return send(callCtx, reply)
				}
				req, home, fn := *op, opts.Home, browse
				return aside(callCtx, reply, browseSlot, &browsesRunning, browseAnswerAfter,
					terminalproto.BrowseResult{Reason: terminalproto.BrowseConflict}, terminalproto.BrowseResult{Reason: terminalproto.BrowseUnreadable},
					func(ctx context.Context) terminalproto.Body {
						return fn(ctx, home, req, terminalhost.OSFS, time.Now)
					})
			case *terminalproto.Branches:
				if !terminalproto.SupportsBranches(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				if !allowed(opts.Consent) {
					reply.Body = terminalproto.BranchesResult{Reason: terminalproto.BranchesDenied}
					return send(callCtx, reply)
				}
				req, home, git := *op, opts.Home, opts.Git
				if git == nil {
					git = terminalhost.ExecGit
				}
				return aside(callCtx, reply, branchesSlot, &branchesRunning, branchesAnswerAfter,
					terminalproto.BranchesResult{Reason: terminalproto.BranchesConflict}, terminalproto.BranchesResult{Reason: terminalproto.BranchesGitFailed},
					func(ctx context.Context) terminalproto.Body {
						return terminalhost.ListBranches(ctx, home, req, git)
					})
			case *terminalproto.Models:
				// Models are served only on a generation that negotiated
				// models-v1, and only for a tool the connector can list. A
				// tool without a listing answers unsupported without exec.
				if !terminalproto.SupportsModels(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				if !terminalproto.ValidToolV2(op.Tool) {
					return terminalproto.ErrInvalid
				}
				if !allowed(opts.Consent) || !allowed(opts.ModelsConsent) {
					reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsOff}
					return send(callCtx, reply)
				}
				if !terminalhost.ListsModels(op.Tool) {
					reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsUnsupported}
					return send(callCtx, reply)
				}
				if managerErr != nil || m == nil {
					// The host manager could not start (for example tmux is
					// missing), so no listing can run; that is a failure, not
					// the local switch being off.
					reply.Body = terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsExited}
					return send(callCtx, reply)
				}
				modelsMu.Lock()
				cached, seen := modelsLast[op.Tool]
				modelsMu.Unlock()
				// The floor answers the previous result instead of running the
				// listing again, whether that result was a list or a failure.
				if seen && time.Since(cached.at) < modelListFloor {
					reply.Body = cached.result
					return send(callCtx, reply)
				}
				tool, fn := op.Tool, listModels
				return aside(callCtx, reply, modelsSlot, &modelsRunning, modelsAnswerAfter,
					terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsConflict},
					terminalproto.ModelsResult{Tool: op.Tool, Reason: terminalproto.ModelsTimedOut},
					func(ctx context.Context) terminalproto.Body {
						r := fn(m, ctx, tool)
						modelsMu.Lock()
						modelsLast[tool] = modelListCacheEntry{result: r, at: time.Now()}
						modelsMu.Unlock()
						return r
					})
			case *terminalproto.SoftwareApply:
				// An apply is served only on software-apply-v1, and only while the
				// local terminal access consent is on. The connector owns the
				// artifact and the command; the request only names them.
				if !terminalproto.SupportsSoftwareApply(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				if op.Tool != "opencode" || !terminalproto.ValidToolV2(op.Tool) {
					return terminalproto.ErrInvalid
				}
				if !allowed(opts.Consent) {
					reply.Body = terminalproto.SoftwareApplyResult{Tool: op.Tool, Action: op.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplyOff}
					return send(callCtx, reply)
				}
				req := *op
				return aside(callCtx, reply, softwareSlot, &softwareRunning, softwareAnswerAfter,
					terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplyBusy},
					terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyFailed, Reason: terminalproto.SoftwareApplyTimedOut},
					func(ctx context.Context) terminalproto.Body {
						softwareGate.Lock()
						defer softwareGate.Unlock()
						// The controller checked before sending, but a session could
						// have started since. Nothing else can now: every OpenCode
						// launch waits on this gate.
						live := chats.sup.liveTool(agentsoftware.OpenCodeTool)
						if m != nil {
							if sessions, err := m.List(ctx); err == nil {
								for _, session := range sessions {
									if session.Tool == agentsoftware.OpenCodeTool && session.State.ReservesQuota() {
										live++
									}
								}
							}
						}
						if live > 0 {
							return terminalproto.SoftwareApplyResult{Tool: req.Tool, Action: req.Action, State: terminalproto.SoftwareApplyRefused, Reason: terminalproto.SoftwareApplySessions}
						}
						return applySoftware(ctx, opts.Home, req, exec.LookPath, terminalhost.SoftwareSource{
							// The controller may need minutes to fill a cold cache
							// before it sends headers; the shared client gives up
							// after 10 seconds.
							Client: privateconn.WithHeaderTimeout(client, mirrorHeaderTimeout),
							Header: mirrorHeaders(headers),
							Path:   "/private/v1/software/artifact/" + agentsoftware.OpenCodeLinuxX64.ArchiveSHA256,
						})
					})
			case *terminalproto.Upload:
				// Uploads are served only on a generation that negotiated
				// upload-v1. Without it the message is a protocol error and
				// nothing is sent back, as for browse and branches.
				if !terminalproto.SupportsUpload(headers.Get(terminalproto.FeatureHeader)) {
					return terminalproto.ErrInvalid
				}
				// The local switches are checked before the manager: a machine
				// that turned uploads or terminal access off answers at once,
				// without opening a stream.
				if !allowed(opts.Consent) || !allowed(opts.UploadConsent) {
					reply.Body = terminalproto.Result{Code: terminalproto.UploadsOff}
					return send(callCtx, reply)
				}
				if managerErr != nil {
					reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
					return send(callCtx, reply)
				}
				if !op.Launch {
					sessions, err := m.List(callCtx)
					if err != nil {
						return respond(err)
					}
					running := false
					for _, s := range sessions {
						if s.ID == msg.SessionID && s.State == terminalproto.Running {
							running = true
							break
						}
					}
					if !running {
						reply.Body = terminalproto.Result{Code: terminalproto.NotFound}
						return send(callCtx, reply)
					}
				}
				if !uploads.acquire() {
					reply.Body = terminalproto.Result{Code: terminalproto.UploadBusy}
					return send(callCtx, reply)
				}
				released := false
				defer func() {
					if !released {
						uploads.release()
					}
				}()
				c, err := dialUploadStream(callCtx, client, headers, msg, op.StreamToken, terminalproto.UploadOperation)
				if err != nil {
					return respond(terminalhost.ErrNotRunning)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				if err := send(callCtx, reply); err != nil {
					c.CloseNow()
					return err
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer uploads.release()
					defer c.CloseNow()
					serveUpload(callCtx, c, m, msg.SessionID, *op)
				}()
				released = true
				return nil
			}
			switch op := msg.Body.(type) {
			case *terminalproto.Create:
				if op.Structured {
					// Starting ACP probes a version and opens a session; keep the
					// control reader answering health probes while it runs. A
					// slow start must never look like a lost machine.
					if !chats.sup.addWork() {
						return respond(terminalhost.ErrManagerClosed)
					}
					go func(in terminalproto.Create) {
						defer chats.sup.wg.Done()
						changed, err := chats.open(msg.RequestID, msg.SessionID, in)
						if callCtx.Err() != nil {
							return
						}
						if err != nil {
							reply.Body = chatResult(err)
						} else {
							reply.Body = changed
						}
						_ = send(callCtx, reply)
					}(*op)
					return nil
				}
			case *terminalproto.SessionMode:
				// A swap stops one process and starts another; answer from its
				// own goroutine like a structured create. It runs to the end
				// (or its rollback) even if this connection is lost meanwhile.
				if !chats.sup.addWork() {
					return respond(terminalhost.ErrManagerClosed)
				}
				go func(in terminalproto.SessionMode) {
					defer chats.sup.wg.Done()
					swapCtx, cancel := context.WithTimeout(context.WithoutCancel(callCtx), sessionModeTimeout)
					defer cancel()
					changed, err := switcher.swap(swapCtx, msg.RequestID, msg.SessionID, in)
					if callCtx.Err() != nil {
						return
					}
					if err != nil {
						reply.Body = chatResult(err)
					} else {
						reply.Body = changed.ForFeatures(headers.Get(terminalproto.FeatureHeader))
					}
					_ = send(callCtx, reply)
				}(*op)
				return nil
			case *terminalproto.End:
				if _, ok := chats.snapshot(msg.SessionID); ok {
					// Closing a chat waits for its process group; answer from
					// its own goroutine so the connection stays responsive.
					if !chats.sup.addWork() {
						return respond(terminalhost.ErrManagerClosed)
					}
					go func() {
						defer chats.sup.wg.Done()
						changed, err := chats.end(msg.SessionID)
						if callCtx.Err() != nil {
							return
						}
						if err != nil {
							reply.Body = result(err)
						} else {
							reply.Body = changed
						}
						_ = send(callCtx, reply)
					}()
					return nil
				}
			case *terminalproto.ACPPrompt:
				if err := chats.prompt(msg.SessionID, op.Text, op.Images); err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return send(callCtx, reply)
			case *terminalproto.ACPModel:
				if !terminalproto.SupportsACPModel(headers.Get(terminalproto.FeatureHeader)) {
					return respond(terminalhost.ErrOperationConflict)
				}
				if err := chats.setModel(callCtx, msg.SessionID, op.Model); err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return send(callCtx, reply)
			case *terminalproto.ACPCancel:
				if err := chats.cancel(msg.SessionID); err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return send(callCtx, reply)
			case *terminalproto.ACPPermission:
				if err := chats.permission(msg.SessionID, op.PermissionID, op.Allow); err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return send(callCtx, reply)
			case *terminalproto.ACPQuestionAnswer:
				answer, err := chats.question(msg.SessionID, op)
				if err != nil {
					return respond(err)
				}
				reply.Body = answer
				return send(callCtx, reply)
			case *terminalproto.ACPMode:
				if err := chats.mode(msg.SessionID, op.Yolo); err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return send(callCtx, reply)
			case *terminalproto.Reconcile:
				if op.Complete || len(op.Sessions) != 0 {
					return terminalproto.ErrInvalid
				}
				snapshot := terminalproto.Reconcile{Complete: true, Sessions: []terminalproto.SessionSnapshot{}}
				var moving map[string]string
				if m != nil {
					if err := m.Reconcile(callCtx); err != nil {
						return respond(err)
					}
					sessions, hidden, err := listShells(callCtx, m, chats.sup)
					if err != nil {
						return respond(err)
					}
					moving = hidden
					sort.Slice(sessions, func(i, j int) bool {
						if sessions[i].State.ReservesQuota() != sessions[j].State.ReservesQuota() {
							return sessions[i].State.ReservesQuota()
						}
						return sessions[i].UpdatedAt > sessions[j].UpdatedAt
					})
					for _, s := range sessions {
						snapshot.Sessions = append(snapshot.Sessions, terminalproto.SessionSnapshot{
							ID:         s.ID,
							Title:      s.Title,
							State:      s.State,
							ExitCode:   s.ExitCode,
							ActivityAt: s.ActivityAt,
							BellAt:     s.BellAt,
							ExitStatus: s.ExitCode,
						}.ForFeatures(headers.Get(terminalproto.FeatureHeader)))
					}
				}
				if moving == nil {
					_, moving = chats.sup.swapView()
				}
				chatSnapshots := withSwaps(chats.snapshots(), moving)
				features := headers.Get(terminalproto.FeatureHeader)
				for i := range chatSnapshots {
					chatSnapshots[i] = chatSnapshots[i].ForFeatures(features)
				}
				reply.Body = reconcileSnapshot(snapshot.Sessions, chatSnapshots)
				return respond(nil)
			}
			if managerErr != nil {
				return send(callCtx, reply)
			}
			switch op := msg.Body.(type) {
			case *terminalproto.Create:
				createCtx := callCtx
				if terminalproto.SupportsProgress(headers.Get(terminalproto.FeatureHeader)) {
					// Each stage is told to the controller as it begins, before this create's one answer.
					createCtx = terminalhost.WithProgress(callCtx, func(stage terminalproto.LaunchStage) {
						note := msg
						note.Body = terminalproto.LaunchProgress{Stage: stage}
						noteCtx, cancel := context.WithTimeout(callCtx, time.Second)
						defer cancel()
						_ = send(noteCtx, note)
					})
				}
				if op.Tool == agentsoftware.OpenCodeTool {
					softwareGate.RLock()
					defer softwareGate.RUnlock()
				}
				s, err := m.Create(createCtx, msg.RequestID, msg.SessionID, *op)
				if err != nil {
					return respond(err)
				}
				// Only a worktree create's session has a worktree to report.
				reply.Body = terminalproto.SessionChanged{
					State:        s.State,
					ExitCode:     s.ExitCode,
					ActivityAt:   s.ActivityAt,
					BellAt:       s.BellAt,
					ExitStatus:   s.ExitCode,
					WorktreePath: s.WorktreePath,
					WorktreeRepo: s.WorktreeRepo,
				}.ForFeatures(headers.Get(terminalproto.FeatureHeader))
				return respond(nil)
			case *terminalproto.End:
				s, err := m.End(callCtx, msg.RequestID, msg.SessionID)
				if err != nil {
					return respond(err)
				}
				reply.Body = terminalproto.SessionChanged{
					State:      s.State,
					ExitCode:   s.ExitCode,
					ActivityAt: s.ActivityAt,
					BellAt:     s.BellAt,
					ExitStatus: s.ExitCode,
				}.ForFeatures(headers.Get(terminalproto.FeatureHeader))
				return respond(nil)
			case *terminalproto.Attach:
				if !terminalproto.ValidID(op.StreamToken) {
					return terminalproto.ErrInvalid
				}
				a, err := m.Attach(ctx, msg.SessionID, terminalhost.AttachOptions{ID: msg.AttachmentID, Generation: msg.AttachmentGeneration, Cols: op.Cols, Rows: op.Rows, Takeover: op.Takeover, Lease: time.Duration(op.LeaseSeconds) * time.Second})
				if err != nil {
					return respond(err)
				}
				attachments[msg.SessionID] = a
				c, err := stream(callCtx, client, headers, msg, op.StreamToken, "attach")
				if err != nil {
					a.Close()
					return respond(terminalhost.ErrNotRunning)
				}
				wg.Add(1)
				go func() { defer wg.Done(); defer a.Close(); defer c.CloseNow(); bridge(ctx, c, a) }()
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			case *terminalproto.RenewAttachment:
				err := m.RenewAttachment(msg.SessionID, msg.AttachmentID, msg.AttachmentGeneration, time.Duration(op.LeaseSeconds)*time.Second)
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(err)
			case *terminalproto.Resize:
				a := attachments[msg.SessionID]
				if a == nil || a.ID != msg.AttachmentID || a.Generation != msg.AttachmentGeneration {
					return respond(terminalhost.ErrAttachmentStale)
				}
				err := a.Resize(op.Cols, op.Rows)
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(err)
			case *terminalproto.Detach:
				a := attachments[msg.SessionID]
				if a != nil && a.ID == msg.AttachmentID && a.Generation == msg.AttachmentGeneration {
					a.Close()
					delete(attachments, msg.SessionID)
				}
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			case *terminalproto.History, *terminalproto.CommandHistory:
				var data []byte
				var err error
				var token string
				switch request := op.(type) {
				case *terminalproto.History:
					token = request.StreamToken
					data, err = m.History(callCtx, msg.SessionID, request.Lines)
				case *terminalproto.CommandHistory:
					if !terminalproto.SupportsCommandHistory(headers.Get(terminalproto.FeatureHeader)) {
						return respond(terminalhost.ErrTerminalDisabled)
					}
					token = request.StreamToken
					data, err = m.CommandHistory(callCtx, msg.SessionID)
				}
				if !terminalproto.ValidID(token) {
					return terminalproto.ErrInvalid
				}
				if err != nil {
					return respond(err)
				}
				c, err := stream(callCtx, client, headers, msg, token, "history")
				if err != nil {
					return respond(terminalhost.ErrNotRunning)
				}
				wg.Add(1)
				go func() {
					defer wg.Done()
					defer c.CloseNow()
					for len(data) > 0 {
						n := min(len(data), terminalproto.MaxBinaryBytes)
						if write(ctx, c, data[:n]) != nil {
							return
						}
						data = data[n:]
					}
					_ = c.Close(websocket.StatusNormalClosure, "")
				}()
				reply.Body = terminalproto.Result{Code: terminalproto.OK}
				return respond(nil)
			default:
				return terminalproto.ErrInvalid
			}
		}
		return handler, func() {
			cancel()
			chats.close()
			if m != nil {
				m.Close()
			}
			wg.Wait()
		}
	}
}

// dialUploadStream is the production reverse-stream dialer; tests replace it
// to check the operation and lifecycle without a live controller.
var dialUploadStream = func(ctx context.Context, client *http.Client, headers http.Header, msg terminalproto.Message, token, operation string) (uploadConn, error) {
	return stream(ctx, client, headers, msg, token, operation)
}

// maxUploadsInFlight is the most image transfers one connection serves at
// once. A further request is refused before any stream opens.
const maxUploadsInFlight = 2

// uploadCounter counts the uploads running on one connection.
type uploadCounter struct {
	max int32
	n   atomic.Int32
}

func (c *uploadCounter) acquire() bool {
	for {
		n := c.n.Load()
		if n >= c.max {
			return false
		}
		if c.n.CompareAndSwap(n, n+1) {
			return true
		}
	}
}

func (c *uploadCounter) release() { c.n.Add(-1) }

// uploadConn is the reverse stream an upload reads. *websocket.Conn satisfies
// it; tests use an in-memory fake.
type uploadConn interface {
	Read(context.Context) (websocket.MessageType, []byte, error)
	Write(context.Context, websocket.MessageType, []byte) error
	Close(websocket.StatusCode, string) error
	CloseNow() error
	SetReadLimit(int64)
}

// uploadSink stores the bytes an upload streams in. *terminalhost.Manager
// satisfies it.
type uploadSink interface {
	WriteUpload(context.Context, string, terminalproto.Upload, io.Reader) (string, error)
}

var (
	errUploadShort             = errors.New("upload stream ended before its declared size")
	errUploadInvalidFrame      = errors.New("upload stream carried an invalid binary frame")
	errUploadFrameTooLarge     = errors.New("upload stream carried an oversize frame")
	errUploadNonBinaryFrame    = errors.New("upload stream carried a non-binary frame")
	errUploadFrameOverDeclared = errors.New("upload stream carried more bytes than declared")
)

// serveUpload reads the reverse stream's binary frames into the sink and sends
// exactly one final UploadResult text frame before closing normally. It never
// answers with a path or hash on an error.
func serveUpload(ctx context.Context, c uploadConn, sink uploadSink, sessionID string, req terminalproto.Upload) terminalproto.UploadResult {
	c.SetReadLimit(terminalproto.MaxBinaryBytes)
	pr, pw := io.Pipe()
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	readErr := make(chan error, 1)
	go func() {
		err := readUploadFrames(readCtx, c, pw, req.Size)
		_ = pw.CloseWithError(err)
		readErr <- err
	}()
	path, writeErr := sink.WriteUpload(ctx, sessionID, req, pr)
	cancelRead()
	_ = pr.Close()
	frameErr := <-readErr
	result := terminalproto.UploadResult{Code: uploadResultCode(writeErr, frameErr)}
	if result.Code == terminalproto.UploadOK {
		result.Path = path
	}
	if payload, err := json.Marshal(result); err == nil {
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_ = c.Write(writeCtx, websocket.MessageText, payload)
		cancel()
	}
	_ = c.Close(websocket.StatusNormalClosure, "")
	return result
}

// readUploadFrames copies exactly the declared number of bytes from binary
// frames into w, then returns without waiting for a close: the controller
// reads the result and closes after it. A short stream, a non-binary or empty
// frame, a frame over the binary limit and a frame that would exceed the
// declared size are errors so the result can name them.
func readUploadFrames(ctx context.Context, c uploadConn, w io.Writer, size int64) error {
	if size < 1 || size > terminalproto.MaxUploadBytes {
		return errUploadFrameTooLarge
	}
	var total int64
	for total < size {
		kind, data, err := c.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusMessageTooBig {
				return errUploadFrameTooLarge
			}
			return errUploadShort
		}
		if kind != websocket.MessageBinary {
			return errUploadNonBinaryFrame
		}
		if len(data) == 0 {
			return errUploadInvalidFrame
		}
		if len(data) > terminalproto.MaxBinaryBytes {
			return errUploadFrameTooLarge
		}
		if total+int64(len(data)) > size {
			return errUploadFrameOverDeclared
		}
		total += int64(len(data))
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}

func uploadResultCode(writeErr, frameErr error) terminalproto.UploadCode {
	switch {
	case errors.Is(writeErr, terminalhost.ErrUploadDisabled):
		return terminalproto.UploadDisabled
	case errors.Is(writeErr, terminalhost.ErrUploadTooLarge):
		return terminalproto.UploadTooLarge
	case errors.Is(writeErr, terminalhost.ErrUploadType):
		return terminalproto.UploadType
	case errors.Is(writeErr, terminalhost.ErrUploadQuota):
		return terminalproto.UploadQuota
	case errors.Is(writeErr, terminalhost.ErrUploadMismatch):
		return terminalproto.UploadMismatch
	}
	switch {
	case errors.Is(frameErr, errUploadFrameTooLarge):
		return terminalproto.UploadTooLarge
	case errors.Is(frameErr, errUploadNonBinaryFrame), errors.Is(frameErr, errUploadInvalidFrame):
		return terminalproto.UploadType
	case errors.Is(frameErr, errUploadShort), errors.Is(frameErr, errUploadFrameOverDeclared):
		return terminalproto.UploadMismatch
	}
	if writeErr == nil && frameErr == nil {
		return terminalproto.UploadOK
	}
	if errors.Is(writeErr, terminalproto.ErrInvalid) {
		return terminalproto.UploadType
	}
	return terminalproto.UploadFailed
}

// allowed reports whether a local switch is present and on.
func allowed(consent func() error) bool { return consent != nil && consent() == nil }

// result answers a failed request; a refused worktree keeps its reason and
// detail for the controller.
func result(err error) terminalproto.Result {
	if r, ok := terminalhost.WorktreeResult(err); ok {
		return r
	}
	return terminalproto.Result{Code: code(err)}
}

// answerWorktree runs a worktree status or remove: a status is answered with
// the worktree_state, everything else with a worktree_result.
func answerWorktree(ctx context.Context, m *terminalhost.Manager, body terminalproto.Body) terminalproto.Body {
	switch op := body.(type) {
	case *terminalproto.WorktreeStatus:
		s, err := m.WorktreeStatus(ctx, *op)
		if err != nil {
			return worktreeResult(err)
		}
		return s
	case *terminalproto.WorktreeRemove:
		removed, err := m.WorktreeRemove(ctx, *op)
		if err != nil {
			return worktreeResult(err)
		}
		return terminalproto.WorktreeResult{Code: terminalproto.OK, Path: removed.Path, Branch: removed.Branch}
	}
	return terminalproto.WorktreeResult{Code: terminalproto.Failed}
}

// worktreeResult maps a worktree status or remove outcome: nil is ok, a
// worktree that is not there not_found, counts that moved worktree_changed, a
// refusal worktree_refused with its reason and detail, and anything else the
// usual code.
func worktreeResult(err error) terminalproto.WorktreeResult {
	if err == nil {
		return terminalproto.WorktreeResult{Code: terminalproto.OK}
	}
	r := result(err)
	return terminalproto.WorktreeResult{Code: r.Code, Reason: r.Reason, Detail: r.Detail}
}

func code(err error) terminalproto.ResultCode {
	if errors.Is(err, terminalhost.ErrWorktreeRefused) {
		return terminalproto.WorktreeRefused
	}
	if errors.Is(err, terminalhost.ErrWorktreeChanged) {
		return terminalproto.WorktreeChanged
	}
	if errors.Is(err, terminalhost.ErrWorktreeNotFound) {
		return terminalproto.NotFound
	}
	if c := terminalhost.LaunchResultCode(err); c != "" {
		return c
	}
	switch {
	case errors.Is(err, terminalhost.ErrTerminalDisabled):
		return terminalproto.Denied
	case errors.Is(err, terminalhost.ErrSessionCapacity):
		return terminalproto.Capacity
	case errors.Is(err, terminalhost.ErrOperationConflict), errors.Is(err, terminalhost.ErrAttachmentBusy), errors.Is(err, terminalhost.ErrAttachmentStale):
		return terminalproto.Conflict
	case errors.Is(err, terminalhost.ErrSessionMissing):
		return terminalproto.NotFound
	case errors.Is(err, errAttachmentUnavailable):
		return terminalproto.Failed
	default:
		return terminalproto.Unavailable
	}
}
func stream(ctx context.Context, client *http.Client, headers http.Header, m terminalproto.Message, token, operation string) (*websocket.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(dialCtx, privateconn.Origin+"/private/v1/terminal/data", &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers})
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(terminalproto.MaxBinaryBytes)
	h := terminalproto.StreamHello{Version: terminalproto.Version, RequestID: m.RequestID, SessionID: m.SessionID, AttachmentID: m.AttachmentID, Generation: m.Generation, AttachmentGeneration: m.AttachmentGeneration, Token: token, Operation: operation}
	payload, _ := json.Marshal(h)
	if err = c.Write(dialCtx, websocket.MessageText, payload); err != nil {
		c.CloseNow()
		return nil, err
	}
	return c, nil
}
func write(ctx context.Context, c *websocket.Conn, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return c.Write(ctx, websocket.MessageBinary, data)
}
func bridge(ctx context.Context, c *websocket.Conn, a *terminalhost.Attachment) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		buf := make([]byte, terminalproto.MaxBinaryBytes)
		for {
			n, err := a.Read(buf)
			if n > 0 && write(ctx, c, buf[:n]) != nil {
				return
			}
			if err != nil {
				return
			}
		}
	}()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			c.CloseNow()
			a.Close()
		case <-a.Done():
			c.CloseNow()
			cancel()
		}
	}()
	for {
		kind, data, err := c.Read(ctx)
		if err != nil || kind != websocket.MessageBinary || terminalproto.ValidateBinary(data) != nil {
			break
		}
		if n, err := a.Write(data); err != nil || n != len(data) {
			break
		}
	}
	cancel()
	a.Close()
	c.CloseNow()
	<-done
	<-stopped
}

var _ io.ReadWriteCloser = (*terminalhost.Attachment)(nil)

func newID() string {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("secure randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}

// reconcileSnapshot merges terminal and chat receipts without letting history
// push a live session out of the complete 32-entry snapshot. The controller
// treats an omitted live session as lost.
func reconcileSnapshot(shells, chats []terminalproto.SessionSnapshot) terminalproto.Reconcile {
	sessions := append(append([]terminalproto.SessionSnapshot{}, shells...), chats...)
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].State.ReservesQuota() && !sessions[j].State.ReservesQuota()
	})
	if len(sessions) > 32 {
		sessions = sessions[:32]
	}
	return terminalproto.Reconcile{Complete: true, Sessions: sessions}
}

// mirrorHeaderTimeout covers the controller's cache download (15 minutes) and
// a short wait for a serving slot.
const mirrorHeaderTimeout = 16 * time.Minute

// sessionModeTimeout bounds one swap: stopping a chat waits up to a few
// seconds for its process group, and a chat start may take up to a minute.
const sessionModeTimeout = 3 * time.Minute

// mirrorHeaders is the machine enrollment the mirror route authenticates. The
// session token and feature flags are not part of it.
func mirrorHeaders(from http.Header) http.Header {
	out := http.Header{}
	for _, name := range []string{"X-Reet-Agent", "X-Reet-Agent-Key", "X-Reet-Version"} {
		if v := from.Get(name); v != "" {
			out.Set(name, v)
		}
	}
	return out
}
