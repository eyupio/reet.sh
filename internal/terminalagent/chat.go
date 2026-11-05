//go:build linux || darwin

package terminalagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/acpagent"
	"github.com/eyupio/reet.sh/internal/agentsoftware"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// errAttachmentUnavailable means a prompt named an image the machine cannot
// safely read from its own upload folders.
var errAttachmentUnavailable = errors.New("that image could not be attached to the chat")

// chatProcess is the part of an ACP chat the connector drives. Tests use it to
// exercise the dispatch without a real agent install.
type chatProcess interface {
	Prompt(context.Context, string) error
	PromptWithAttachments(context.Context, string, []acpagent.Attachment) error
	ImagesSupported() bool
	Cancel() error
	Allow(string, bool) error
	SessionID() string
	Working() bool
	Close()
	Done() <-chan struct{}
}

type chatStarter func(context.Context, acpagent.Options) (chatProcess, error)

type chatSession struct {
	id, title string
	tool      string
	dir       string
	model     string
	effort    string
	acpID     string
	state     terminalproto.SessionState
	exit      *int
	process   chatProcess
	cancel    context.CancelFunc
	busy      bool
	changing  bool
	yolo      bool
	// trust is the folder trust the chat was opened with; a swap that has to
	// restore this chat reopens it the same way.
	trust bool
	// quiet is set once a mode swap has taken this chat over: its end and any
	// late events are never told to the controller, whose row stays running.
	quiet bool
	// stray is set on a retired receipt whose process group Close could not
	// confirm gone: it still counts against the quota and liveTool.
	stray bool
	ended sync.Once

	activityAt int64
	bellAt     int64
	lastStatus *terminalproto.ACPEvent
	pending    []terminalproto.ACPEvent
	// questions retains complete cards delivered and not yet resolved. The
	// process still owns their tokens, deadlines and answer validation.
	questions []terminalproto.ACPEvent

	// outbox holds events produced while no control connection is attached,
	// oldest first. It is bounded by the same event and byte ceilings as the
	// controller's ring, shedding old output before current approval state.
	outbox      []terminalproto.ACPEvent
	outboxBytes int
}

const (
	maxDetachedChatEvents = 1000
	maxDetachedChatBytes  = 256 << 10
)

// chatSupervisor owns chat sessions for the life of the agent process. A
// control connection attaches to it; losing that connection detaches the link
// but leaves every ACP process and turn running, so a reconnect can adopt the
// same sessions instead of losing them. This mirrors the detached worker
// model: the transport is replaceable, the agent process is not.
type chatSupervisor struct {
	ctx      context.Context
	cancel   context.CancelFunc
	opts     terminalhost.ManagerOptions
	start    chatStarter
	lookPath func(string) (string, error)

	mu       sync.Mutex
	link     *chatLink
	sessions map[string]*chatSession
	requests map[string]string
	starting map[string]string
	// swapping maps each session a mode swap is moving to the title it is
	// reported under meanwhile; swapEpoch counts every
	// change to it so an observer can tell that a listing raced a swap.
	swapping  map[string]string
	swapEpoch uint64
	// holds counts the creates and ends in flight per session; a swap is
	// refused while any runs, and they are refused while a swap runs.
	holds map[string]int
	// registry remembers live chats across connector restarts. It is nil when
	// this connector has no private layout (tests and uninstalled callers).
	registry *chatRegistry
	previews *chatPreviewStore
	closed   bool
	// stop is closed when Close starts; background waits end on it.
	stop       chan struct{}
	wg         sync.WaitGroup
	close      sync.Once
	resumeOnce sync.Once
}

// chatLink is one control connection's delivery path. history is the
// acp-history-v1 capability of that connection.
type chatLink struct {
	generation int64
	send       func(context.Context, terminalproto.Message) error
	history    bool
	details    bool
	// questions is acp-question-v1 on this connection. Question events are sent
	// only when it is set: an older controller's strict decoder rejects them.
	questions bool
	previews  bool
	live      bool
}

// ChatSupervisor is the process-scoped ACP session owner. The connector creates
// one per agent process and passes it to every control connection.
type ChatSupervisor struct{ s *chatSupervisor }

// NewChatSupervisor creates the process-scoped supervisor. Cancelling ctx
// closes every live chat process.
func NewChatSupervisor(ctx context.Context, opts terminalhost.ManagerOptions) *ChatSupervisor {
	s := newChatSupervisor(ctx, opts)
	s.startResume()
	return &ChatSupervisor{s: s}
}

func newChatSupervisor(ctx context.Context, opts terminalhost.ManagerOptions) *chatSupervisor {
	ctx, cancel := context.WithCancel(ctx)
	s := &chatSupervisor{
		ctx: ctx, cancel: cancel, opts: opts, previews: newChatPreviewStore(opts.Layout.Dir, opts.Home),
		start: func(ctx context.Context, opts acpagent.Options) (chatProcess, error) {
			return acpagent.Start(ctx, opts)
		},
		lookPath: exec.LookPath,
		sessions: map[string]*chatSession{}, requests: map[string]string{}, starting: map[string]string{},
		swapping: map[string]string{}, holds: map[string]int{}, stop: make(chan struct{}),
	}
	if opts.Layout.Dir != "" {
		s.registry = newChatRegistry(filepath.Join(opts.Layout.Dir, "structured-chats.json"))
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return s
}

// startResume begins adopting the chats this connector was running before it
// restarted. It is separate from construction so a caller can install its
// process hooks first, and runs once.
func (s *chatSupervisor) startResume() {
	if s.registry == nil {
		return
	}
	s.resumeOnce.Do(func() {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		s.wg.Add(1)
		s.mu.Unlock()
		go func() {
			defer s.wg.Done()
			s.resumePersisted()
		}()
	})
}

// Close ends every chat process and refuses new sessions.
func (s *chatSupervisor) Close() {
	s.closeOnce()
}

func (s *chatSupervisor) closeOnce() {
	s.close.Do(func() {
		s.mu.Lock()
		s.closed = true
		sessions := make([]*chatSession, 0, len(s.sessions))
		for _, cs := range s.sessions {
			sessions = append(sessions, cs)
		}
		s.sessions = map[string]*chatSession{}
		s.requests = map[string]string{}
		s.starting = map[string]string{}
		if s.link != nil {
			s.link.live = false
			s.link = nil
		}
		s.mu.Unlock()
		if s.cancel != nil {
			s.cancel()
		}
		close(s.stop)
		for _, cs := range sessions {
			if cs.process != nil {
				cs.process.Close()
			} else if cs.cancel != nil {
				cs.cancel()
			}
		}
		s.wg.Wait()
	})
}

// chatHost is one control connection's view of the supervisor.
type chatHost struct {
	sup  *chatSupervisor
	link *chatLink
}

// attachChats attaches a control connection to a supervisor. A nil supervisor
// creates a connection-scoped one, which keeps unit tests and any caller that
// does not own a process yet isolated.
func attachChats(supervisor *ChatSupervisor, ctx context.Context, opts terminalhost.ManagerOptions, generation int64, send func(context.Context, terminalproto.Message) error, history, details bool, questions ...bool) *chatHost {
	var s *chatSupervisor
	if supervisor != nil {
		s = supervisor.s
	} else {
		s = newChatSupervisor(ctx, opts)
		s.startResume()
	}
	return s.attach(ctx, generation, send, history, details, questions...)
}

func (s *chatSupervisor) attach(ctx context.Context, generation int64, send func(context.Context, terminalproto.Message) error, history, details bool, questions ...bool) *chatHost {
	link := &chatLink{generation: generation, send: send, history: history, details: details, questions: len(questions) > 0 && questions[0], previews: len(questions) > 1 && questions[1]}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return &chatHost{sup: s, link: link}
	}
	s.link = link
	type cancellation struct {
		cs *chatSession
		id string
	}
	var cancelled []cancellation
	for _, cs := range s.sessions {
		if !link.questions {
			for _, question := range cs.questions {
				cancelled = append(cancelled, cancellation{cs, question.QuestionID})
			}
		}
		seedRecoveredChatEvents(cs, link.questions)
	}
	s.mu.Unlock()
	for _, question := range cancelled {
		s.cancelQuestion(question.cs, question.id)
	}

	h := &chatHost{sup: s, link: link}
	s.flush(h)
	return h
}

// flush delivers the buffered events from the previous connection before the
// new link is marked live. Events produced meanwhile keep buffering, so each
// session's order is preserved.
func (s *chatSupervisor) flush(h *chatHost) {
	for {
		s.mu.Lock()
		var (
			cs    *chatSession
			event terminalproto.ACPEvent
		)
		for _, candidate := range s.sessions {
			if len(candidate.outbox) > 0 {
				cs = candidate
				event = candidate.outbox[0]
				break
			}
		}
		if cs == nil {
			if s.link == h.link {
				h.link.live = true
			}
			s.mu.Unlock()
			return
		}
		cs.outbox = cs.outbox[1:]
		cs.outboxBytes -= chatEventBytes(event)
		if event.Kind == "preview" && !h.link.previews {
			s.mu.Unlock()
			continue
		}
		if isQuestionKind(event.Kind) && !h.link.questions {
			// A send on the previous link can fail after attach filtered history
			// and put its frame back. Check negotiation at replay delivery too.
			s.mu.Unlock()
			continue
		}
		s.mu.Unlock()

		if !h.link.details {
			event = withoutToolDetails(event)
		}
		err := h.link.send(s.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
			Generation: h.link.generation, Body: &event})
		if err != nil {
			s.mu.Lock()
			if s.link == h.link {
				h.link.live = false
			}
			cs.outbox = append([]terminalproto.ACPEvent{event}, cs.outbox...)
			cs.outboxBytes += chatEventBytes(event)
			trimChatOutbox(cs)
			s.mu.Unlock()
			return
		}
	}
}

// addWork registers one goroutine whose lifetime the supervisor must wait
// for. It refuses work once Close has started.
func (s *chatSupervisor) addWork() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

func (s *chatSupervisor) detach(h *chatHost) {
	s.mu.Lock()
	if s.link == h.link {
		s.link = nil
		h.link.live = false
	}
	s.mu.Unlock()
}

func (h *chatHost) open(requestID, sessionID string, in terminalproto.Create) (terminalproto.SessionChanged, error) {
	return h.openChat(requestID, sessionID, in, false)
}

// openChat starts a structured chat under sessionID. With dropReplay set, the
// events the agent emits before startup finishes (the conversation that
// session/load replays) are not forwarded: the controller already holds that
// transcript, so only what happens after the start reaches it.
func (h *chatHost) openChat(requestID, sessionID string, in terminalproto.Create, dropReplay bool) (terminalproto.SessionChanged, error) {
	if in.Tool == agentsoftware.OpenCodeTool {
		release, err := lockSoftwareRead(h.sup.ctx)
		if err != nil {
			return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
		}
		defer release()
	}
	h.sup.mu.Lock()
	if h.sup.closed {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
	}
	if existing, ok := h.sup.requests[requestID]; ok {
		if existing != sessionID {
			h.sup.mu.Unlock()
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		cs := h.sup.sessions[sessionID]
		if cs == nil {
			h.sup.mu.Unlock()
			return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
		}
		state, exit, acpID := cs.state, cs.exit, cs.acpID
		history := h.link.history
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{State: state, ExitCode: exit, ACPSessionID: reportedACP(in.Tool, acpID, history)}, nil
	}
	// For a reattach (dropReplay), a session that appeared under the id since
	// reattach checked is another start in flight for it.
	busy := terminalhost.ErrOperationConflict
	if dropReplay {
		busy = errReattachStarting
	}
	if _, ok := h.sup.sessions[sessionID]; ok {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, busy
	}
	if _, ok := h.sup.starting[requestID]; ok {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, busy
	}
	if h.sup.conversationInUseLocked(in.Tool, in.ResumeACP) {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	if h.sup.quotaLocked() >= terminalproto.MaxSessionsPerAgent {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrSessionCapacity
	}
	consent := h.sup.opts.YoloConsent == nil || h.sup.opts.YoloConsent() != nil
	if in.Yolo && consent {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrYoloRefused
	}
	cs := &chatSession{id: sessionID, title: in.Title, tool: in.Tool, acpID: in.ResumeACP, state: terminalproto.Creating, yolo: in.Yolo, trust: in.Trust}
	h.sup.sessions[sessionID] = cs
	h.sup.requests[requestID] = sessionID
	h.sup.starting[requestID] = sessionID
	h.sup.mu.Unlock()
	succeeded := false
	defer func() {
		h.sup.mu.Lock()
		delete(h.sup.starting, requestID)
		if !succeeded {
			delete(h.sup.sessions, sessionID)
			delete(h.sup.requests, requestID)
		}
		h.sup.mu.Unlock()
	}()

	dir, err := terminalhost.ResolveDir(h.sup.opts.Home, in.Dir)
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	h.sup.mu.Lock()
	cs.dir, cs.model = dir, in.Model
	h.sup.mu.Unlock()
	spec, binary, err := h.sup.chatBinary(in.Tool)
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	// The start runs on the supervisor's process-lifetime context: a control
	// connection lost during startup must not kill a chat the controller can
	// adopt after it reconnects.
	startCtx, cancel := context.WithTimeout(h.sup.ctx, 60*time.Second)
	defer cancel()
	h.sup.mu.Lock()
	cs.cancel = cancel
	h.sup.mu.Unlock()
	var started atomic.Bool
	if !dropReplay {
		started.Store(true)
	}
	process, err := h.sup.start(startCtx, acpagent.Options{
		Agent: spec, Binary: binary, Dir: dir, Home: h.sup.opts.Home, Model: in.Model, Effort: in.Effort, Trust: in.Trust, Resume: in.ResumeACP,
		BaseEnv: os.Environ(),
		// Start calls OnReady after the replay and before its own closes of
		// interrupted tool calls and its first idle status, which the
		// controller needs even when the replay is dropped.
		OnReady: func() { started.Store(true) },
		OnEvent: func(event acpagent.Event) {
			if started.Load() {
				h.sup.publish(cs, event)
			}
		},
	})
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
	started.Store(true)
	acpID := process.SessionID()
	h.sup.mu.Lock()
	if h.sup.closed {
		delete(h.sup.sessions, sessionID)
		delete(h.sup.requests, requestID)
		h.sup.mu.Unlock()
		process.Close()
		return terminalproto.SessionChanged{}, terminalhost.ErrManagerClosed
	}
	cs.process = process
	cs.acpID = acpID
	cs.state = terminalproto.Running
	cs.cancel = nil
	succeeded = true
	h.sup.wg.Add(1)
	h.sup.mu.Unlock()
	if controls, ok := process.(liveChatControls); ok {
		state := controls.Controls()
		h.sup.mu.Lock()
		for _, o := range state.Options {
			if o.Category == "thought_level" {
				cs.effort = o.Current
			}
		}
		h.sup.mu.Unlock()
	}
	h.sup.remember(cs)
	// The create reply belongs to the connection that asked, which may already
	// have been replaced while startup ran. Publish the running state to
	// whichever link is attached now so a reconcile done during startup does
	// not leave the controller stuck in Creating.
	h.sup.announceRunning(cs)
	go func() {
		defer h.sup.wg.Done()
		<-process.Done()
		h.sup.ended(cs, 0)
	}()
	if in.Prompt != "" {
		h.sup.publish(cs, acpagent.Event{Kind: "user", Text: in.Prompt})
		h.startPrompt(cs, in.Prompt, nil)
	}
	return terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: reportedACP(in.Tool, acpID, h.link.history)}, nil
}

// reportedACP is the native session id sent to the controller. It is sent only
// on a connection that negotiated history and only for a tool whose descriptor
// offers it: Claude's UUID does not fit the wire's id format and its replay is
// unverified, so reporting it would break the notices that carry it.
func reportedACP(tool, acpID string, history bool) string {
	if !history || !acpagent.Describe(tool, true, "").History {
		return ""
	}
	return acpID
}

func (h *chatHost) snapshot(sessionID string) (terminalproto.SessionSnapshot, bool) {
	h.sup.mu.Lock()
	defer h.sup.mu.Unlock()
	cs := h.sup.sessions[sessionID]
	if cs == nil {
		return terminalproto.SessionSnapshot{}, false
	}
	return terminalproto.SessionSnapshot{
		ID: cs.id, Title: cs.title, State: cs.state, ExitCode: cs.exit,
		ACPSessionID: reportedACP(cs.tool, cs.acpID, h.link.history),
		ActivityAt:   cs.activityAt, BellAt: cs.bellAt,
	}, true
}

func (h *chatHost) snapshots() []terminalproto.SessionSnapshot {
	h.sup.mu.Lock()
	defer h.sup.mu.Unlock()
	out := make([]terminalproto.SessionSnapshot, 0, len(h.sup.sessions))
	for _, cs := range h.sup.sessions {
		out = append(out, terminalproto.SessionSnapshot{
			ID: cs.id, Title: cs.title, State: cs.state, ExitCode: cs.exit,
			ACPSessionID: reportedACP(cs.tool, cs.acpID, h.link.history),
			ActivityAt:   cs.activityAt, BellAt: cs.bellAt,
		})
	}
	// The controller only needs live sessions to avoid marking them lost; keep
	// ended receipts too, but never let history push a live one out.
	if len(out) > 32 {
		for i := range out {
			for j := i + 1; j < len(out); j++ {
				if out[j].State.ReservesQuota() && !out[i].State.ReservesQuota() {
					out[i], out[j] = out[j], out[i]
				}
			}
		}
		out = out[:32]
	}
	return out
}

// A pending or unconfirmed process still owns its conversation. Both manual
// continuation and restart recovery take this gate before starting a load;
// the caller holds s.mu.
func (s *chatSupervisor) conversationInUseLocked(tool, acpID string) bool {
	if acpID == "" {
		return false
	}
	for _, cs := range s.sessions {
		matches := cs.tool == tool && cs.acpID == acpID
		reserves := cs.state.ReservesQuota() || cs.stray
		if matches && reserves {
			return true
		}
	}
	return false
}

// quotaLocked counts the chats that hold a slot of MaxSessionsPerAgent; the
// caller holds s.mu.
func (s *chatSupervisor) quotaLocked() int {
	n := 0
	for _, cs := range s.sessions {
		if cs.state.ReservesQuota() || cs.stray {
			n++
		}
	}
	return n
}

// liveTool counts this process's live sessions for one tool, across every
// connection. A software apply uses it to refuse while an OpenCode session
// still needs the installation it is replacing.
func (s *chatSupervisor) liveTool(tool string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, cs := range s.sessions {
		reserves := cs.state.ReservesQuota() || cs.stray
		if cs.tool == tool && reserves {
			n++
		}
	}
	return n
}

func (h *chatHost) find(sessionID string) *chatSession {
	h.sup.mu.Lock()
	defer h.sup.mu.Unlock()
	return h.sup.sessions[sessionID]
}

func chatUnavailable(cs *chatSession) bool {
	return cs.busy || cs.changing || cs.quiet || cs.state != terminalproto.Running
}

func (h *chatHost) startPrompt(cs *chatSession, text string, images []acpagent.Attachment) bool {
	h.sup.mu.Lock()
	if h.sup.closed || chatUnavailable(cs) {
		h.sup.mu.Unlock()
		return false
	}
	cs.busy = true
	h.sup.wg.Add(1)
	h.sup.mu.Unlock()
	go func() {
		defer h.sup.wg.Done()
		defer func() { h.sup.mu.Lock(); cs.busy = false; h.sup.mu.Unlock() }()
		if len(images) > 0 {
			_ = cs.process.PromptWithAttachments(h.sup.ctx, text, images)
		} else {
			_ = cs.process.Prompt(h.sup.ctx, text)
		}
	}()
	return true
}

func (h *chatHost) prompt(sessionID, text string, paths []string) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	h.sup.mu.Lock()
	state := cs.state
	h.sup.mu.Unlock()
	if state != terminalproto.Running {
		return terminalhost.ErrOperationConflict
	}
	var images []acpagent.Attachment
	if len(paths) > 0 {
		if cs.process == nil || !cs.process.ImagesSupported() {
			return errAttachmentUnavailable
		}
		var err error
		images, err = h.sup.readAttachments(paths)
		if err != nil {
			return err
		}
	}
	if !h.startPrompt(cs, text, images) {
		return terminalhost.ErrOperationConflict
	}
	return nil
}

// attachmentRoots are the only folders a prompt may read an image from. Both
// are private to this connector; a controller can never use an image path to
// read another file on the machine.
func (s *chatSupervisor) attachmentRoots() []string {
	if s.opts.Layout.Dir == "" || !filepath.IsAbs(s.opts.Layout.Dir) {
		return nil
	}
	return []string{
		filepath.Join(s.opts.Layout.Dir, "launch-uploads"),
		filepath.Join(s.opts.Layout.Dir, "uploads"),
	}
}

// readAttachments turns upload paths into the bytes embedded in an ACP prompt.
// It refuses a path outside the upload roots, a symlink escape, a non-image,
// or a file over the upload limit before the agent sees any of it.
func (s *chatSupervisor) readAttachments(paths []string) ([]acpagent.Attachment, error) {
	roots := s.attachmentRoots()
	if len(roots) == 0 {
		return nil, errAttachmentUnavailable
	}
	images := make([]acpagent.Attachment, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if !filepath.IsAbs(clean) {
			return nil, errAttachmentUnavailable
		}
		resolved, err := filepath.EvalSymlinks(clean)
		if err != nil {
			return nil, errAttachmentUnavailable
		}
		if !underAnyRoot(resolved, roots) {
			return nil, errAttachmentUnavailable
		}
		info, err := os.Stat(resolved)
		invalid := err != nil || !info.Mode().IsRegular() || info.Size() < 1 ||
			info.Size() > terminalproto.MaxUploadBytes
		if invalid {
			return nil, errAttachmentUnavailable
		}
		data, err := os.ReadFile(resolved)
		if err != nil || int64(len(data)) != info.Size() {
			return nil, errAttachmentUnavailable
		}
		mime, _, ok := terminalproto.SniffUploadMIME(data)
		if !ok {
			return nil, errAttachmentUnavailable
		}
		images = append(images, acpagent.Attachment{MIME: mime, Data: data})
	}
	return images, nil
}

func underAnyRoot(path string, roots []string) bool {
	for _, root := range roots {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		if strings.HasPrefix(path, resolved+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func (h *chatHost) cancel(sessionID string) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	if cs.process == nil {
		return terminalhost.ErrOperationConflict
	}
	return cs.process.Cancel()
}

func (h *chatHost) mode(sessionID string, yolo bool) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	h.sup.mu.Lock()
	if cs.process == nil || cs.state != terminalproto.Running {
		h.sup.mu.Unlock()
		return terminalhost.ErrOperationConflict
	}
	consent := h.sup.opts.YoloConsent == nil || h.sup.opts.YoloConsent() != nil
	if yolo && consent {
		h.sup.mu.Unlock()
		return terminalhost.ErrYoloRefused
	}
	cs.yolo = yolo
	process := cs.process
	h.sup.mu.Unlock()
	h.sup.touchRecord(sessionID, yolo)
	if yolo {
		if allower, ok := process.(interface{ AllowAllPending() error }); ok {
			_ = allower.AllowAllPending()
		}
	}
	return nil
}

func (h *chatHost) permission(sessionID, permissionID string, allow bool) error {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	if cs.process == nil {
		return terminalhost.ErrOperationConflict
	}
	err := cs.process.Allow(permissionID, allow)
	if errors.Is(err, acpagent.ErrNoPermission) {
		return terminalhost.ErrOperationConflict
	}
	return err
}

// question delivers the person's answer to a waiting typed question. An answer
// that fails validation is reported as question_invalid and leaves the question
// waiting; an answer to a question that is no longer waiting is a conflict.
func (h *chatHost) question(sessionID string, a *terminalproto.ACPQuestionAnswer) (terminalproto.Result, error) {
	cs := h.find(sessionID)
	if cs == nil {
		return terminalproto.Result{}, terminalhost.ErrSessionMissing
	}
	answerer, ok := cs.process.(questionAnswerer)
	if !ok {
		return terminalproto.Result{}, terminalhost.ErrOperationConflict
	}
	resolution := acpagent.QuestionResolution{Action: acpagent.QuestionAction(a.Action)}
	if a.Action == "accept" {
		answers, err := acpagent.DecodeAnswers(a.Answers)
		if err != nil {
			return terminalproto.Result{Code: terminalproto.QuestionInvalid, Detail: err.Error()}, nil
		}
		resolution.Answers = answers
	}
	err := answerer.AnswerQuestion(a.QuestionID, resolution)
	var invalid *acpagent.AnswerError
	switch {
	case err == nil:
		return terminalproto.Result{Code: terminalproto.OK}, nil
	case errors.As(err, &invalid):
		return terminalproto.Result{Code: terminalproto.QuestionInvalid, Detail: cutText(invalid.Error(), 200)}, nil
	case errors.Is(err, acpagent.ErrNoQuestion):
		return terminalproto.Result{}, terminalhost.ErrOperationConflict
	}
	return terminalproto.Result{}, err
}

func (h *chatHost) end(sessionID string) (terminalproto.SessionChanged, error) {
	h.sup.mu.Lock()
	cs := h.sup.sessions[sessionID]
	if cs == nil {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrSessionMissing
	}
	if cs.process == nil {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	// Keep the ended record and its request receipt: a lost end reply must be
	// answerable with the same exited state, not looked up in the tmux manager
	// and reported missing.
	h.sup.mu.Unlock()
	cs.process.Close()
	h.sup.forgetRecord(sessionID)
	h.sup.ended(cs, 0)
	return terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: cs.exit, ACPSessionID: reportedACP(cs.tool, cs.acpID, h.link.history)}, nil
}

func (h *chatHost) ended(cs *chatSession, code int) { h.sup.ended(cs, code) }

// announceRunning tells the currently attached connection that startup
// finished. It deliberately ignores link.live: a link being flushed is still
// writable, and this state must not wait behind the chat-event queue.
func (s *chatSupervisor) announceRunning(cs *chatSession) {
	s.mu.Lock()
	link := s.link
	acpID := cs.acpID
	s.mu.Unlock()
	if link == nil {
		return
	}
	_ = link.send(s.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
		Generation: link.generation, Body: &terminalproto.SessionChanged{State: terminalproto.Running, ACPSessionID: reportedACP(cs.tool, acpID, link.history)}})
}

func (s *chatSupervisor) ended(cs *chatSession, code int) {
	cs.ended.Do(func() {
		s.mu.Lock()
		if cs.state != terminalproto.Exited {
			cs.state = terminalproto.Exited
			if code >= 0 && code <= 255 {
				cs.exit = &code
			}
		}
		link := s.link
		acpID, exit, quiet := cs.acpID, cs.exit, cs.quiet
		s.mu.Unlock()
		live := link != nil && link.live
		if !live || quiet {
			return
		}
		_ = link.send(s.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
			Generation: link.generation, Body: &terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: exit, ACPSessionID: reportedACP(cs.tool, acpID, link.history)}})
	})
}

func (h *chatHost) close() { h.sup.detach(h) }

// idle reports whether a running chat has nothing in flight: no turn, no
// approval and no question waiting on the person. Only an idle chat may be
// swapped to a terminal.
func (h *chatHost) idle(sessionID string) bool {
	h.sup.mu.Lock()
	defer h.sup.mu.Unlock()
	cs := h.sup.sessions[sessionID]
	return cs != nil && idleLocked(cs)
}

// idleLocked is idle for a session the caller holds the supervisor lock for.
func idleLocked(cs *chatSession) bool {
	return cs.state == terminalproto.Running && cs.process != nil && !cs.busy && !cs.changing && !cs.quiet &&
		len(cs.pending) == 0 && len(cs.questions) == 0 && !cs.process.Working()
}

// forget drops an ended chat's receipt, and the request ids that name it, so
// the id is unique again. It is false for a chat that is still live.
func (s *chatSupervisor) forget(sessionID string) bool {
	s.mu.Lock()
	cs := s.sessions[sessionID]
	if cs == nil || cs.state.ReservesQuota() {
		s.mu.Unlock()
		return false
	}
	s.dropLocked(sessionID)
	s.mu.Unlock()
	s.forgetRecord(sessionID)
	return true
}

// dropLocked removes the id's receipt and the request ids that name it; the
// caller holds s.mu.
func (s *chatSupervisor) dropLocked(sessionID string) {
	delete(s.sessions, sessionID)
	for request, id := range s.requests {
		if id == sessionID {
			delete(s.requests, request)
		}
	}
}

// questionAnswerer is the part of a chat process that can resolve a typed
// question. It is optional: only a process that can ask questions has it.
type questionAnswerer interface {
	AnswerQuestion(string, acpagent.QuestionResolution) error
}

func isQuestionKind(kind string) bool {
	return kind == "question" || kind == "question_resolved" || kind == "question_cancelled"
}

func (s *chatSupervisor) publish(cs *chatSession, event acpagent.Event) {
	if event.Kind == "preview" {
		s.mu.Lock()
		skip := cs.quiet || s.link != nil && !s.link.previews
		s.mu.Unlock()
		if skip {
			return
		}
	}
	var wire terminalproto.ACPEvent
	var ok bool
	if event.Kind == "preview" && event.Preview != nil {
		preview := s.previews.capture(cs.id, cs.dir, event.Preview)
		wire = terminalproto.ACPEvent{Kind: "preview", MessageID: event.MessageID, ToolCallID: event.ToolCallID, Preview: &preview}
		ok = terminalproto.ACPEventSize(wire)
	} else {
		wire, ok = wireEvent(event)
	}
	if !ok {
		if event.Kind == "question" && event.QuestionID != "" {
			s.cancelQuestion(cs, event.QuestionID)
		}
		return
	}
	s.deliver(cs, wire)
	if event.Kind == "user" && s.registry != nil {
		s.registry.seen(cs.id)
	}
	s.mu.Lock()
	yolo, process := cs.yolo, cs.process
	s.mu.Unlock()
	permission := yolo && event.Kind == "permission"
	hasPermission := permission && event.PermissionID != ""
	if hasPermission && process != nil {
		// YOLO answers this once, for the person. The process emits the real
		// permission_resolved (which may be a rejection when no allow-once
		// option exists), and that is what the controller audits.
		_ = process.Allow(event.PermissionID, true)
	}
	// A rejected Start closes its child before returning the refusal. That
	// callback must not turn the controller's pending create into an exited
	// session before the refusal is recorded. Once admitted, Done also observes
	// an exit that raced startup and publishes its final lifecycle state.
	ended := event.Kind == "status" && event.State == "ended"
	if ended && process != nil {
		s.ended(cs, 0)
	}
}

// cancelQuestion answers a question with a cancellation. It runs off the event
// path because resolving it emits another event.
func (s *chatSupervisor) cancelQuestion(cs *chatSession, questionID string) {
	s.mu.Lock()
	process := cs.process
	s.mu.Unlock()
	answerer, ok := process.(questionAnswerer)
	if !ok || !s.addWork() {
		return
	}
	go func() {
		defer s.wg.Done()
		_ = answerer.AnswerQuestion(questionID, acpagent.QuestionResolution{Action: acpagent.QuestionCancel})
	}()
}

func (s *chatSupervisor) deliver(cs *chatSession, wire terminalproto.ACPEvent) {
	s.mu.Lock()
	if cs.quiet {
		// A swapped-out process's last words belong to no live row.
		s.mu.Unlock()
		return
	}
	link := s.link
	preview := wire.Kind == "preview" && link != nil
	if preview && !link.previews {
		s.mu.Unlock()
		return
	}
	live := link != nil && link.live && link.questions
	if wire.Kind == "question" && !live {
		// Only an already delivered question survives a disconnect. A newly
		// asked question still needs a live, negotiated recipient.
		s.mu.Unlock()
		s.cancelQuestion(cs, wire.QuestionID)
		return
	}
	previousStatus := cs.lastStatus
	recordChatSessionEvent(cs, wire)
	question := isQuestionKind(wire.Kind) && link != nil
	if question && !link.questions {
		// Retire local state even for an older peer, but never send fields its
		// strict decoder cannot read. A settled status uses the old protocol.
		if cs.lastStatus == nil || cs.lastStatus == previousStatus {
			s.mu.Unlock()
			return
		}
		wire = *cs.lastStatus
	}
	if link == nil || !link.live {
		bufferChatEvent(cs, wire)
		// A resolution can settle the retained waiting status without a native
		// status event. Keep that new state available during a stalled replay.
		changed := cs.lastStatus != nil && cs.lastStatus != previousStatus
		if wire.Kind != "status" && changed {
			bufferChatEvent(cs, *cs.lastStatus)
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if !link.details {
		wire = withoutToolDetails(wire)
	}
	if err := link.send(s.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
		Generation: link.generation, Body: &wire}); err != nil {
		s.mu.Lock()
		if s.link == link {
			link.live = false
		}
		bufferChatEvent(cs, wire)
		s.mu.Unlock()
	}
}

func recordChatSessionEvent(cs *chatSession, wire terminalproto.ACPEvent) {
	nowMs := time.Now().UnixMilli()
	switch wire.Kind {
	case "status":
		cp := wire
		cs.lastStatus = &cp
		switch wire.State {
		case "working":
			cs.activityAt = nowMs
		case "waiting":
			if cs.bellAt == 0 {
				cs.bellAt = nowMs
			}
		case "idle", "cancelled", "error", "ended":
			cs.pending = nil
			cs.questions = nil
			cs.bellAt = 0
		}
	case "question":
		if wire.QuestionID != "" {
			for i := range cs.questions {
				if cs.questions[i].QuestionID == wire.QuestionID {
					cs.questions[i] = wire
					return
				}
			}
			cs.questions = append(cs.questions, wire)
		}
	case "question_resolved", "question_cancelled":
		out := cs.questions[:0]
		for _, question := range cs.questions {
			if question.QuestionID != wire.QuestionID {
				out = append(out, question)
			}
		}
		clear(cs.questions[len(out):])
		cs.questions = out
		settleChatWaitingStatus(cs)
	case "permission":
		if wire.PermissionID != "" {
			replaced := false
			for i := range cs.pending {
				if cs.pending[i].PermissionID == wire.PermissionID {
					cs.pending[i] = wire
					replaced = true
					break
				}
			}
			if !replaced {
				cs.pending = append(cs.pending, wire)
			}
			if cs.bellAt == 0 {
				cs.bellAt = nowMs
			}
		}
	case "permission_resolved", "permission_cancelled":
		if wire.PermissionID != "" {
			out := cs.pending[:0]
			for _, p := range cs.pending {
				if p.PermissionID != wire.PermissionID {
					out = append(out, p)
				}
			}
			cs.pending = out
			settleChatWaitingStatus(cs)
		}
	case "agent", "tool", "user":
		cs.activityAt = nowMs
	}
}

func settleChatWaitingStatus(cs *chatSession) {
	if len(cs.pending) != 0 || len(cs.questions) != 0 {
		return
	}
	cs.bellAt = 0
	if cs.lastStatus != nil && cs.lastStatus.State == "waiting" {
		next := "idle"
		working := cs.process != nil && cs.process.Working()
		if cs.busy || working {
			next = "working"
		}
		cs.lastStatus = &terminalproto.ACPEvent{Kind: "status", State: next}
	}
}

// seedRecoveredChatEvents ends the replay with the current pending cards
// and status. Putting them before a full backlog would evict them again as the
// bounded outbox is rebuilt. An approval already buffered is moved to its
// current snapshot rather than replayed twice; resolved approvals keep their
// original history and are never recreated from the pending set.
func seedRecoveredChatEvents(cs *chatSession, questions bool) {
	pending := make(map[string]bool, len(cs.pending))
	for _, approval := range cs.pending {
		pending[approval.PermissionID] = true
	}
	pendingQuestions := make(map[string]bool, len(cs.questions))
	for _, question := range cs.questions {
		pendingQuestions[question.QuestionID] = true
	}
	saved := cs.outbox
	cs.outbox = nil
	cs.outboxBytes = 0
	for _, event := range saved {
		skip := !questions || (event.Kind == "question" && pendingQuestions[event.QuestionID])
		if isQuestionKind(event.Kind) && skip {
			continue
		}
		if event.Kind == "permission" && pending[event.PermissionID] {
			continue
		}
		bufferChatEvent(cs, event)
	}
	if cs.state != terminalproto.Running {
		return
	}
	for _, approval := range cs.pending {
		bufferChatEvent(cs, approval)
	}
	if questions {
		for _, question := range cs.questions {
			bufferChatEvent(cs, question)
		}
	}
	if cs.lastStatus != nil {
		bufferChatEvent(cs, *cs.lastStatus)
	}
}

func bufferChatEvent(cs *chatSession, event terminalproto.ACPEvent) {
	size := chatEventBytes(event)
	if size > maxDetachedChatBytes {
		return
	}
	cs.outbox = append(cs.outbox, event)
	cs.outboxBytes += size
	trimChatOutbox(cs)
}

func trimChatOutbox(cs *chatSession) {
	for len(cs.outbox) > maxDetachedChatEvents || cs.outboxBytes > maxDetachedChatBytes {
		drop := oldestChatHistoryEvent(cs)
		cs.outboxBytes -= chatEventBytes(cs.outbox[drop])
		if drop == 0 {
			cs.outbox[0] = terminalproto.ACPEvent{}
			cs.outbox = cs.outbox[1:]
		} else {
			copy(cs.outbox[drop:], cs.outbox[drop+1:])
			cs.outbox[len(cs.outbox)-1] = terminalproto.ACPEvent{}
			cs.outbox = cs.outbox[:len(cs.outbox)-1]
		}
	}
}

// Evict old history before pending cards and the latest buffered status.
// These records still need to survive if native output
// continues while a replacement transport is blocked. The hard queue ceilings
// also apply when the entire buffer consists of approval state.
func oldestChatHistoryEvent(cs *chatSession) int {
	latestStatus := -1
	for i := len(cs.outbox) - 1; i >= 0; i-- {
		if cs.outbox[i].Kind == "status" {
			latestStatus = i
			break
		}
	}
	for i, event := range cs.outbox {
		if i == latestStatus {
			continue
		}
		if event.Kind == "permission" {
			pending := false
			for _, approval := range cs.pending {
				if approval.PermissionID == event.PermissionID {
					pending = true
					break
				}
			}
			if pending {
				continue
			}
		}
		if event.Kind == "question" {
			pending := false
			for _, question := range cs.questions {
				if question.QuestionID == event.QuestionID {
					pending = true
					break
				}
			}
			if pending {
				continue
			}
		}
		return i
	}
	return 0
}

func chatEventBytes(event terminalproto.ACPEvent) int {
	questionBytes := 0
	if event.Question != nil {
		data, _ := json.Marshal(event.Question)
		questionBytes = len(data)
	}
	return questionBytes + len(event.QuestionID) + len(event.Kind) + len(event.MessageID) + len(event.ToolCallID) + len(event.PermissionID) +
		len(event.Title) + len(event.ToolKind) + len(event.Status) + len(event.State) + len(event.Text) +
		len(event.Command) + len(event.Path) + len(event.Diff) + len(event.Reason) + len(event.Detail) +
		len(strings.Join(event.Locations, "")) + 48 // the numeric detail fields and their JSON separators
}

// withoutToolDetails is the plain tool event an older controller can decode: its strict decoder rejects unknown fields.
func withoutToolDetails(event terminalproto.ACPEvent) terminalproto.ACPEvent {
	if event.Kind == "tool" {
		event.Command, event.Path, event.Diff = "", "", ""
		event.Additions, event.Deletions, event.DurationMs = 0, 0, 0
	}
	return event
}

func wireEvent(event acpagent.Event) (terminalproto.ACPEvent, bool) {
	wire := terminalproto.ACPEvent{Kind: event.Kind, MessageID: event.MessageID, ToolCallID: event.ToolCallID,
		PermissionID: event.PermissionID, Title: event.Title, ToolKind: event.ToolKind, Status: event.Status,
		State: event.State, Text: event.Text, Command: event.Command, Path: event.Path, Diff: event.Diff,
		Additions: event.Additions, Deletions: event.Deletions, DurationMs: event.DurationMs,
		Locations: event.Locations, Allow: event.Allow, Bad: event.Bad, Truncated: event.Truncated,
		Reason: event.Reason, Detail: event.Detail, QuestionID: event.QuestionID, Code: event.Code}
	if event.Question != nil {
		question := event.Question.Wire()
		wire.Question = &question
	}
	if len(wire.Diff) > 9<<10 {
		wire.Diff = cutText(wire.Diff, 9<<10)
		wire.Truncated = true
	}
	if len(wire.Command) > 6<<10 {
		wire.Command = cutText(wire.Command, 6<<10)
		wire.Truncated = true
	}
	if len(wire.Text) > 8<<10 {
		wire.Text = cutText(wire.Text, 8<<10)
		wire.Bad = true
	}
	if len(wire.Path) > 1<<10 {
		wire.Path = cutText(wire.Path, 1<<10)
	}
	if len(wire.Title) > 200 {
		// The title is only a label. Some agents use the whole shell command as
		// it, and the full text still travels in Command, so a cut title must not
		// make the card unreviewable and disable Allow once.
		wire.Title = cutText(wire.Title, 200)
	}
	if len(wire.Locations) > 4 {
		wire.Locations = wire.Locations[:4]
	}
	for i, location := range wire.Locations {
		if len(location) > 512 {
			wire.Locations[i] = cutText(location, 512)
		}
	}
	if !terminalproto.ACPEventSize(wire) {
		if wire.Kind != "permission" {
			return terminalproto.ACPEvent{}, false
		}
		// A card that will not fit must still reach the person: keep only
		// what identifies it and disable Allow once with Truncated.
		minimal := terminalproto.ACPEvent{Kind: "permission", PermissionID: wire.PermissionID, ToolCallID: wire.ToolCallID,
			Title: wire.Title, ToolKind: wire.ToolKind, Path: wire.Path, Truncated: true}
		if len(minimal.Title) > 200 {
			minimal.Title = cutText(minimal.Title, 200)
		}
		if len(minimal.Path) > 512 {
			minimal.Path = cutText(minimal.Path, 512)
		}
		if minimal.Path == "" {
			minimal.Path = "the workspace"
		}
		if !terminalproto.ACPEventSize(minimal) {
			return terminalproto.ACPEvent{}, false
		}
		return minimal, true
	}
	return wire, true
}

func chatResult(err error) terminalproto.Result {
	var trust *acpagent.TrustError
	switch {
	case errors.As(err, &trust):
		// The web's reattach card (web/src/lib/terminal/reattachState.ts
		// reattachDetail) trims the trailing "Start a chat only…" sentence.
		detail := "This folder has agent extensions: " + strings.Join(trust.Files, ", ") + ". Start a chat only in a folder you trust."
		if len(detail) > 480 {
			detail = cutText(detail, 480)
		}
		return terminalproto.Result{Code: terminalproto.ACPTrustRequired, Detail: detail}
	case errors.Is(err, acpagent.ErrVersion):
		detail := "That agent needs a newer version on this machine."
		code := terminalproto.ACPVersion
		var version *acpagent.VersionError
		if errors.As(err, &version) {
			detail = version.Error()
			if version.Required == "" || version.Found == "" && version.Reason != "" && version.Reason != "the agent is not installed" {
				// Also when the probe itself failed or timed out: no version was
				// read, so "needs 1.19.5 or newer" would be untrue.
				// A different agent answered: update/install guidance would
				// point at the wrong thing, so report it as unavailable.
				code = terminalproto.ACPUnavailable
			}
		}
		if len(detail) > 480 {
			detail = cutText(detail, 480)
		}
		return terminalproto.Result{Code: code, Detail: detail}
	case errors.Is(err, acpagent.ErrCodexLogin):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: acpagent.ErrCodexLogin.Error()}
	case errors.Is(err, acpagent.ErrClaudeLogin):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: acpagent.ErrClaudeLogin.Error()}
	case errors.Is(err, acpagent.ErrClineLogin):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: acpagent.ErrClineLogin.Error()}
	case errors.Is(err, acpagent.ErrAntigravityLogin):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: acpagent.ErrAntigravityLogin.Error()}
	case errors.Is(err, acpagent.ErrUnsupported):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "That agent does not support chats on this machine."}
	case errors.Is(err, acpagent.ErrPluginsUnreadable):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The folder's agent extensions could not be read, so the chat was not started."}
	case errors.Is(err, acpagent.ErrControl):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The agent did not confirm the chosen effort for this model. Choose an offered effort or Agent default. No opening prompt was sent."}
	case errors.Is(err, acpagent.ErrSafety):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The agent did not confirm that it asks before each command or edit, so the chat was not started."}

	case errors.Is(err, acpagent.ErrModel):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The agent did not use the chosen model, so the chat was not started."}
	default:
		r := result(err)
		if r.Code == terminalproto.Unavailable {
			return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "That agent could not start the chat in that folder."}
		}
		return r
	}
}

func cutText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

func (h *chatHost) setModel(ctx context.Context, sessionID, model string) error {
	h.sup.mu.Lock()
	if h.sup.closed {
		h.sup.mu.Unlock()
		return terminalhost.ErrManagerClosed
	}
	cs := h.sup.sessions[sessionID]
	if cs == nil {
		h.sup.mu.Unlock()
		return terminalhost.ErrSessionMissing
	}
	if chatUnavailable(cs) {
		h.sup.mu.Unlock()
		return terminalhost.ErrOperationConflict
	}
	cs.busy = true
	h.sup.mu.Unlock()
	defer func() { h.sup.mu.Lock(); cs.busy = false; h.sup.mu.Unlock() }()
	process, ok := cs.process.(interface {
		SetModel(context.Context, string) error
	})
	if !ok {
		return terminalhost.ErrOperationConflict
	}
	changeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	err := process.SetModel(changeCtx, model)
	// A switch may apply a different model, or lose its acknowledgement. Use
	// the process read-back even on error, so a restart cannot restore a stale
	// launch model. A plain refusal leaves the process's old read-back intact.
	if reader, ok := cs.process.(interface{ Model() string }); ok {
		confirmed := reader.Model()
		h.sup.mu.Lock()
		cs.model = confirmed
		h.sup.mu.Unlock()
		if h.sup.registry != nil {
			h.sup.registry.setModel(sessionID, confirmed)
		}
	}
	return err
}

// setAgentMode moves a live chat to one of its agent's choosable modes. The
// session refuses any other mode and reports success only when the agent
// confirms the new one.
func (h *chatHost) setAgentMode(ctx context.Context, sessionID, mode string) error {
	h.sup.mu.Lock()
	if h.sup.closed {
		h.sup.mu.Unlock()
		return terminalhost.ErrManagerClosed
	}
	cs := h.sup.sessions[sessionID]
	if cs == nil {
		h.sup.mu.Unlock()
		return terminalhost.ErrSessionMissing
	}
	if chatUnavailable(cs) {
		h.sup.mu.Unlock()
		return terminalhost.ErrOperationConflict
	}
	cs.busy = true
	h.sup.mu.Unlock()
	defer func() { h.sup.mu.Lock(); cs.busy = false; h.sup.mu.Unlock() }()
	process, ok := cs.process.(interface {
		SetMode(context.Context, string) error
	})
	if !ok {
		return terminalhost.ErrOperationConflict
	}
	changeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return process.SetMode(changeCtx, mode)
}
