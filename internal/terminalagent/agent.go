//go:build linux || darwin

// Package terminalagent dispatches typed private operations to the locally
// consented host manager. It exposes no general command or network forwarding.
package terminalagent

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/eyupio/reet.sh/internal/agentaccounts"
	"github.com/eyupio/reet.sh/internal/agenthealth"
	"github.com/eyupio/reet.sh/internal/privateconn"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
	"golang.org/x/sync/semaphore"
)

var (
	// browse is terminalhost.Browse; tests replace it to hold a browse.
	browse           = terminalhost.Browse
	inspectWorkspace = terminalhost.InspectSession
	// cloneRepo is terminalhost.CloneRepo; tests replace it to hold a clone.
	cloneRepo = terminalhost.CloneRepo
	// cloneStatus and removeClone are terminalhost.CloneStatus and
	// terminalhost.RemoveClone; tests replace them to hold a remove.
	cloneStatus = terminalhost.CloneStatus
	removeClone = terminalhost.RemoveClone
	// listRepos is terminalhost.ListRepos; tests replace it so no gh runs.
	listRepos = terminalhost.ListRepos
	// repoListAnswerAfter bounds a repository listing that has not returned.
	// Its own deadline is 30 s, which is under this answer, and this answer
	// is under the controller's 40 s (terminalbroker.RepoListCallTimeout).
	repoListAnswerAfter = 35 * time.Second
	// repoCloneStatusAnswerAfter bounds a clone status that has not returned.
	// Its own deadline is 10 s, which is under this answer, and this answer
	// is under the controller's 15 s (terminalbroker.RepoCloneStatusCallTimeout).
	repoCloneStatusAnswerAfter = 12 * time.Second
	// repoCloneFetchAnswerAfter is the same for a clone status that asks to
	// fetch first: its own deadline is 10 s plus terminalhost.CloneFetchTimeout,
	// and the controller's is terminalbroker.RepoCloneStatusFetchCallTimeout.
	repoCloneFetchAnswerAfter = 32 * time.Second
	// repoCloneRemoveAnswerAfter is when a clone remove that has not returned
	// is answered timed_out, under the controller's 60 s
	// (terminalbroker.RepoCloneRemoveCallTimeout). Its checks end at 10 s;
	// only the delete itself, which cannot be stopped, can outlast this, and
	// it keeps the clone slot until it ends.
	repoCloneRemoveAnswerAfter = 55 * time.Second
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
	// branchPruneAnswerAfter bounds a plan or prune that has not returned.
	// Its own deadline is 20 s, which is under this answer.
	branchPruneAnswerAfter = 22 * time.Second
	// branchSyncAnswerAfter bounds a sync that has not returned. Its own
	// deadline is 45 s, which is under this answer.
	branchSyncAnswerAfter = 48 * time.Second
	// baseCheckAnswerAfter bounds a base check that has not returned. Its own
	// deadline is 33 s, which is under this answer.
	baseCheckAnswerAfter = 36 * time.Second
	// repoCloneAnswerAfter bounds a clone that has not returned. Its own
	// deadline is 10 minutes, which is under this answer, and this answer is
	// under the controller's 10 minutes 30 seconds
	// (terminalbroker.RepoCloneCallTimeout), so it arrives before the
	// controller gives up.
	repoCloneAnswerAfter = 10*time.Minute + 15*time.Second
	// modelsAnswerAfter is when a model listing that has not returned is
	// answered timed_out and its slot freed. ListModels kills its process at
	// 10 s; this leaves room for the reply and process reaping.
	modelsAnswerAfter = 12 * time.Second
	// accountsAnswerAfter is when a sign-in read that has not returned is
	// answered timed_out. A read takes agentaccounts.ReadTakesAtMost (11 s:
	// the sign-in files, OpenCode's store among them, then the providers side
	// by side); this leaves a second for the reply and is under the
	// controller's wait (terminalbroker.AccountsCallTimeout).
	accountsAnswerAfter = 12 * time.Second
	// softwareAnswerAfter is when an apply that has not returned is answered
	// timed_out. The install continues under softwareApplyWorkTimeout, and a
	// later reconcile observes the version it left.
	softwareAnswerAfter = 5*time.Minute + 30*time.Second
	// softwareApplyWorkTimeout bounds how long an apply may hold the gate,
	// including its download. It matches the controller's own apply timeout.
	softwareApplyWorkTimeout = 21 * time.Minute
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
	prunesRunning   atomic.Int32
	modelsRunning   atomic.Int32
	softwareRunning atomic.Int32
	inspectsRunning atomic.Int32
	controlsRunning atomic.Int32
	clonesRunning   atomic.Int32
	// cloneStatusesRunning counts clone status work, as browsesRunning does.
	cloneStatusesRunning atomic.Int32
	// repoListsRunning counts repository listings, as browsesRunning does.
	repoListsRunning atomic.Int32
	accountsRunning  atomic.Int32
)

// accountsSlot allows one sign-in read at a time on this connector process,
// across its connections, so a provider is never asked twice at once.
var accountsSlot = make(chan struct{}, 1)

// accountReaders holds one reader per home folder for the life of the
// process, so the figures it remembers outlive a reconnect and a provider is
// not asked again because the connection dropped.
var (
	accountReadersMu sync.Mutex
	accountReaders   = map[string]*agentaccounts.Reader{}
	// newAccountReader makes a reader; tests replace it so no provider is asked.
	newAccountReader = func(home string) *agentaccounts.Reader {
		return agentaccounts.New(agentaccounts.Options{Home: home, Env: os.Environ()})
	}
)

func accountReader(home string) *agentaccounts.Reader {
	accountReadersMu.Lock()
	defer accountReadersMu.Unlock()
	reader := accountReaders[home]
	if reader == nil {
		reader = newAccountReader(home)
		accountReaders[home] = reader
	}
	return reader
}

// cloneSlot allows one repository clone at a time on this connector process,
// across its connections: a clone holds it for as long as it runs on its
// connection. A dropped connection ends its clone and frees the slot, so a
// clone on the next connection may start while the ended one is still
// removing its temporary folder; each clone removes only its own. It is not
// the slot prune, sync and base check share: a clone takes minutes and must
// not hold those up. A clone remove takes this slot too, so a clone and a
// remove never run together; a remove holds it until its delete has ended,
// even after the connector answered for it.
var cloneSlot = make(chan struct{}, 1)

// softwareSlot serialises software applies on this connector process, not just
// one connection, so two controllers cannot install at once.
var softwareSlot = make(chan struct{}, 1)

// softwareGate coordinates a software apply with new OpenCode sessions. An
// apply takes every token from before the download until the installation is
// recorded; the terminal and structured-chat launch paths take one token while
// they create an OpenCode session. That closes the window between the
// controller's live-session check and the installation changing. A launch
// waiting for an apply gives up with its request, and the apply gives up with
// its work deadline, so neither can hold the other forever.
const softwareGateTokens = 1 << 20

var softwareGate = semaphore.NewWeighted(softwareGateTokens)

// lockSoftwareRead waits for an apply to finish, or for ctx to end. The caller
// must call the returned release.
func lockSoftwareRead(ctx context.Context) (func(), error) {
	if err := softwareGate.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	return func() { softwareGate.Release(1) }, nil
}

// lockSoftwareWrite waits for every launch to finish, or for ctx to end. The
// caller must call the returned release.
func lockSoftwareWrite(ctx context.Context) (func(), error) {
	if err := softwareGate.Acquire(ctx, softwareGateTokens); err != nil {
		return nil, err
	}
	return func() { softwareGate.Release(softwareGateTokens) }, nil
}

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
		c := newAgentConn(ctx, opts, supervisor, client, headers, generation, send)
		return c.dispatch, c.close
	}
}

// dispatch serves one control message. It is called by one control reader;
// only stream workers use their own immutable attachment. Cleanup happens after
// that reader has stopped.
func (c *agentConn) dispatch(callCtx context.Context, msg terminalproto.Message) error {
	if msg.Generation != c.generation {
		return terminalproto.ErrInvalid
	}
	r := &agentReq{agentConn: c, callCtx: callCtx, msg: msg, reply: msg}
	r.reply.Body = terminalproto.Result{Code: terminalproto.Unavailable}
	if answered, err := r.checkFeatures(); answered {
		return err
	}
	// Each of these is served only on a generation that negotiated its
	// feature (checkFeatures above). Without it the message is a protocol
	// error and nothing is sent back: a peer that did not negotiate a kind may
	// drop the connection on its answer. None needs the manager (tmux), but
	// the worktree and upload ones check it themselves.
	switch op := msg.Body.(type) {
	case *terminalproto.WorktreeStatus, *terminalproto.WorktreeRemove:
		return r.handleWorktree()
	case *terminalproto.Inspect:
		return r.handleInspect(op)
	case *terminalproto.Browse:
		return r.handleBrowse(op)
	case *terminalproto.Branches:
		return r.handleBranches(op)
	case *terminalproto.BranchPrunePlan:
		return r.handleBranchPrunePlan(op)
	case *terminalproto.BranchPrune:
		return r.handleBranchPrune(op)
	case *terminalproto.BranchSync:
		return r.handleBranchSync(op)
	case *terminalproto.BaseCheck:
		return r.handleBaseCheck(op)
	case *terminalproto.RepoClone:
		return r.handleRepoClone(op)
	case *terminalproto.RepoCloneStatus:
		return r.handleRepoCloneStatus(op)
	case *terminalproto.RepoCloneRemove:
		return r.handleRepoCloneRemove(op)
	case *terminalproto.RepoList:
		return r.handleRepoList()
	case *terminalproto.Accounts:
		return r.handleAccounts(op)
	case *terminalproto.Models:
		return r.handleModels(op)
	case *terminalproto.SoftwareApply:
		return r.handleSoftwareApply(op)
	case *terminalproto.Upload:
		return r.handleUpload(op)
	}
	// A create or end of a session a swap is moving is busy: it would
	// start or stop a process the swap does not know about. Held for
	// as long as it runs, so a swap cannot start under it either.
	switch msg.Body.(type) {
	case *terminalproto.Create, *terminalproto.End, *terminalproto.ChatReattach:
		if !c.chats.sup.hold(msg.SessionID) {
			return r.respond(terminalhost.ErrOperationConflict)
		}
		r.held = true
		defer func() {
			if r.held {
				c.chats.sup.unhold(msg.SessionID)
			}
		}()
	}
	switch op := msg.Body.(type) {
	case *terminalproto.Create:
		if op.Structured {
			return r.handleChatCreate(op)
		}
	case *terminalproto.SessionMode:
		return r.handleSessionMode(op)
	case *terminalproto.ChatReattach:
		return r.handleChatReattach(op)
	case *terminalproto.End:
		if _, ok := c.chats.snapshot(msg.SessionID); ok {
			return r.handleChatEnd()
		}
	case *terminalproto.NativeView:
		return r.handleNativeView()
	case *terminalproto.ACPPrompt:
		return r.handleACPPrompt(op)
	case *terminalproto.ACPPreview:
		return r.handleACPPreview(op)
	case *terminalproto.ACPControl:
		return r.handleACPControl(op)
	case *terminalproto.ACPModel:
		return r.handleACPModel(op)
	case *terminalproto.ACPCancel:
		return r.handleACPCancel()
	case *terminalproto.ACPPermission:
		return r.handleACPPermission(op)
	case *terminalproto.ACPQuestionAnswer:
		return r.handleACPQuestionAnswer(op)
	case *terminalproto.ACPAgentMode:
		return r.handleACPAgentMode(op)
	case *terminalproto.ACPMode:
		return r.handleACPMode(op)
	case *terminalproto.Reconcile:
		return r.handleReconcile(op)
	}
	if c.managerErr != nil {
		return c.send(callCtx, r.reply)
	}
	switch op := msg.Body.(type) {
	case *terminalproto.Create:
		return r.handleShellCreate(op)
	case *terminalproto.End:
		return r.handleShellEnd(op)
	case *terminalproto.Attach:
		return r.handleAttach(op)
	case *terminalproto.RenewAttachment:
		return r.handleRenewAttachment(op)
	case *terminalproto.Resize:
		return r.handleResize(op)
	case *terminalproto.Detach:
		return r.handleDetach()
	case *terminalproto.History, *terminalproto.CommandHistory:
		return r.handleHistory(op)
	case *terminalproto.TerminalAnswer:
		// The machine reads the screen again and works out the keys itself.
		r.reply.Body = terminalproto.Result{Code: terminalproto.OK}
		return r.respond(c.m.AnswerQuestion(callCtx, msg.SessionID, op.Fingerprint, op.Option))
	default:
		return terminalproto.ErrInvalid
	}
}

// bodyFeatureError is the protocol gate every message kind passes before it is
// served. It answers ErrInvalid for a kind the connection did not negotiate,
// so a peer that never agreed to a message never receives its typed answer.
// Chat sub-features require acp-v1 as well: a connection with an inconsistent
// header must not reach a handler that assumes a live chat.
func bodyFeatureError(body terminalproto.Body, features terminalproto.FeatureSet) error {
	has := features.Has
	acp := func(feature string) bool { return has(terminalproto.ACPFeature) && has(feature) }
	switch b := body.(type) {
	case *terminalproto.ACPPreview:
		if !acp(terminalproto.ACPPreviewsFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPControl:
		if !acp(terminalproto.ACPLiveControlsFeature) {
			return terminalproto.ErrInvalid
		}
		if b.Action == "commands" && !has(terminalproto.ACPCommandsFeature) {
			return terminalproto.ErrInvalid
		}
		if len(b.Images) > 0 && !has(terminalproto.ACPImageFeature) {
			return terminalproto.ErrInvalid
		}
		if len(b.Text) > terminalproto.MaxChatPromptBytes && !has(terminalproto.ACPLongPromptFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPPrompt:
		if !has(terminalproto.ACPFeature) {
			return terminalproto.ErrInvalid
		}
		if len(b.Images) > 0 && !has(terminalproto.ACPImageFeature) {
			return terminalproto.ErrInvalid
		}
		if len(b.Text) > terminalproto.MaxChatPromptBytes && !has(terminalproto.ACPLongPromptFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPModel, *terminalproto.ACPCancel, *terminalproto.ACPPermission:
		if !has(terminalproto.ACPFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPMode:
		if !acp(terminalproto.ACPApprovalFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPAgentMode:
		if !acp(terminalproto.ACPAgentModeFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ACPQuestionAnswer:
		if !acp(terminalproto.ACPQuestionFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.NativeView:
		if !has(terminalproto.NativeViewFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Attach:
		if b.NativeView && !has(terminalproto.NativeViewFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Create:
		// A desk pane reference (and the folder outside home it may carry) is
		// sent only by a controller that negotiated desk-shell-v1.
		if b.Desk != (terminalproto.DeskRef{}) && !has(terminalproto.DeskShellFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.SessionMode:
		if !has(terminalproto.SessionModeFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.TerminalAnswer:
		if !has(terminalproto.TerminalQuestionFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.ChatReattach:
		if !acp(terminalproto.ChatReattachFeature) {
			return terminalproto.ErrInvalid
		}
		if b.Effort != "" && !has(terminalproto.ACPLiveControlsFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.End:
		// Only a controller that negotiated desk-shell-v1 asks for a desk
		// pane's history to be removed with its shell.
		if b.ForgetDesk && !has(terminalproto.DeskShellFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.CommandHistory:
		if !has(terminalproto.CommandHistoryFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Inspect:
		if !has(terminalproto.InspectFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Browse:
		if !has(terminalproto.BrowseFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Branches:
		if !has(terminalproto.BranchesFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.BranchPrunePlan, *terminalproto.BranchPrune:
		if !has(terminalproto.BranchPruneFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.BranchSync:
		if !has(terminalproto.BranchSyncFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.BaseCheck:
		if !has(terminalproto.BaseFreshFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.RepoClone:
		if !has(terminalproto.RepoCloneFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.RepoCloneStatus, *terminalproto.RepoCloneRemove:
		if !has(terminalproto.RepoCloneRemoveFeature) {
			return terminalproto.ErrInvalid
		}
		if status, ok := body.(*terminalproto.RepoCloneStatus); ok && status.Fetch && !has(terminalproto.RepoCloneFetchFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.RepoList:
		if !has(terminalproto.RepoListFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Models:
		if !has(terminalproto.ModelsFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Accounts:
		if !has(terminalproto.AgentAccountsFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.SoftwareApply:
		if !has(terminalproto.SoftwareApplyFeature) {
			return terminalproto.ErrInvalid
		}
	case *terminalproto.Upload:
		if !has(terminalproto.UploadFeature) {
			return terminalproto.ErrInvalid
		}
	}
	return nil
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
	// Only a create with a desk folder, which only a desk-shell-v1 controller
	// sends, can fail this way, so an older controller never sees the detail.
	if errors.Is(err, terminalhost.ErrFolderGone) {
		return terminalproto.Result{Code: terminalproto.LaunchFailed, Detail: terminalproto.FolderGone}
	}
	return terminalproto.Result{Code: code(err)}
}

// resultFor is result for a controller whose negotiated features are given: a
// reattach of a chat that is still starting carries its detail marker only
// when the controller advertised chat-reattach-detail-v1; an older controller
// rejects Conflict with a detail, so it gets the plain Conflict.
func resultFor(err error, features string) terminalproto.Result {
	if errors.Is(err, errReattachStarting) && terminalproto.SupportsChatReattachDetail(features) {
		return terminalproto.Result{Code: terminalproto.Conflict, Detail: terminalproto.ConflictStarting}
	}
	return result(err)
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
	case errors.Is(err, terminalhost.ErrQuestionChanged):
		return terminalproto.QuestionChanged
	case errors.Is(err, terminalhost.ErrAlreadyAnswered):
		return terminalproto.AlreadyAnswered
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
		valid := err == nil && kind == websocket.MessageBinary && terminalproto.ValidateBinary(data) == nil
		if !valid {
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
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	// A request id is not a secret, so a failed entropy read must not take the
	// connector and every session down with it. Time, pid and a counter keep
	// ids distinct within and across processes.
	binary.BigEndian.PutUint64(b[0:8], uint64(time.Now().UnixNano()))
	binary.BigEndian.PutUint64(b[8:16], idFallback.Add(1))
	binary.BigEndian.PutUint64(b[16:24], uint64(os.Getpid()))
	return hex.EncodeToString(b[:])
}

// idFallback disambiguates ids made after crypto/rand failed.
var idFallback atomic.Uint64

// observeShell compares one listed terminal with what this connection last
// observed of it, records the new observation, and returns the change to send
// and whether there is one. The live folder rides along only when it differs
// from the last one sent for that session on this connection, and only when
// desk-shell-v1 was negotiated: a folder change alone is then news too.
func observeShell(observed map[string]terminalproto.SessionChanged, sentDirs map[string]string, s terminalhost.ManagedSession, features string) (terminalproto.SessionChanged, bool) {
	state := terminalproto.SessionChanged{
		State:      s.State,
		ExitCode:   s.ExitCode,
		ActivityAt: s.ActivityAt,
		BellAt:     s.BellAt,
		ExitStatus: s.ExitCode,
		Dir:        s.LiveDir,
		// The question is the whole of it each time: absent means none, so a
		// question going is news as well.
		TerminalQuestion: s.Question,
	}.ForFeatures(features)
	old, known := observed[s.ID]
	sameCode := old.ExitCode == nil && state.ExitCode == nil || old.ExitCode != nil && state.ExitCode != nil && *old.ExitCode == *state.ExitCode
	changed := !known || old.State != state.State || !sameCode
	timesChanged := old.ActivityAt != state.ActivityAt || old.BellAt != state.BellAt
	dirChanged := state.Dir != "" && state.Dir != sentDirs[s.ID]
	questionChanged := questionFingerprint(old.TerminalQuestion) != questionFingerprint(state.TerminalQuestion)
	if dirChanged {
		sentDirs[s.ID] = state.Dir
	} else {
		state.Dir = ""
	}
	observed[s.ID] = state
	return state, changed || timesChanged || dirChanged || questionChanged
}

func questionFingerprint(q *terminalproto.TerminalQuestion) string {
	if q == nil {
		return ""
	}
	return q.Fingerprint
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
// It is shared with the controller, which waits longer than it.
const sessionModeTimeout = terminalproto.SessionModeTimeout

// swapDrainTimeout is how long a closing connection keeps its terminal manager
// for a swap still in flight: as long as a chat start may take.
const swapDrainTimeout = 60 * time.Second

// swapGroup counts the mode swaps in flight and lets a closing connection wait
// a bounded time for them without leaving a waiter goroutine behind.
type swapGroup struct {
	mu   sync.Mutex
	n    int
	idle chan struct{}
}

func newSwapGroup() *swapGroup {
	g := &swapGroup{idle: make(chan struct{})}
	close(g.idle)
	return g
}

func (g *swapGroup) Add(delta int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.n == 0 && delta > 0 {
		g.idle = make(chan struct{})
	}
	g.n += delta
	if g.n == 0 {
		close(g.idle)
	}
}

func (g *swapGroup) Done() { g.Add(-1) }

// Wait waits for every swap to finish, or for d to pass. It reports whether
// the group drained.
func (g *swapGroup) Wait(d time.Duration) bool {
	g.mu.Lock()
	idle := g.idle
	g.mu.Unlock()
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-idle:
		return true
	case <-timer.C:
		return false
	}
}

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
