//go:build linux || darwin

package terminalagent

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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
	acpID     string
	state     terminalproto.SessionState
	exit      *int
	process   chatProcess
	cancel    context.CancelFunc
	busy      bool
	yolo      bool
	ended     sync.Once

	activityAt int64
	bellAt     int64
	lastStatus *terminalproto.ACPEvent
	pending    []terminalproto.ACPEvent

	// outbox holds events produced while no control connection is attached,
	// oldest first. It is bounded by the same event and byte ceilings as the
	// controller's ring, and sheds the oldest first.
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
	opts     terminalhost.ManagerOptions
	start    chatStarter
	lookPath func(string) (string, error)

	mu       sync.Mutex
	link     *chatLink
	sessions map[string]*chatSession
	requests map[string]string
	starting map[string]string
	closed   bool
	wg       sync.WaitGroup
	close    sync.Once
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
	live      bool
}

// ChatSupervisor is the process-scoped ACP session owner. The connector creates
// one per agent process and passes it to every control connection.
type ChatSupervisor struct{ s *chatSupervisor }

// NewChatSupervisor creates the process-scoped supervisor. Cancelling ctx
// closes every live chat process.
func NewChatSupervisor(ctx context.Context, opts terminalhost.ManagerOptions) *ChatSupervisor {
	return &ChatSupervisor{s: newChatSupervisor(ctx, opts)}
}

func newChatSupervisor(ctx context.Context, opts terminalhost.ManagerOptions) *chatSupervisor {
	s := &chatSupervisor{
		ctx: ctx, opts: opts,
		start: func(ctx context.Context, opts acpagent.Options) (chatProcess, error) {
			return acpagent.Start(ctx, opts)
		},
		lookPath: exec.LookPath,
		sessions: map[string]*chatSession{}, requests: map[string]string{}, starting: map[string]string{},
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return s
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
	}
	return s.attach(ctx, generation, send, history, details, questions...)
}

func (s *chatSupervisor) attach(ctx context.Context, generation int64, send func(context.Context, terminalproto.Message) error, history, details bool, questions ...bool) *chatHost {
	link := &chatLink{generation: generation, send: send, history: history, details: details, questions: len(questions) > 0 && questions[0]}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return &chatHost{sup: s, link: link}
	}
	s.link = link
	for _, cs := range s.sessions {
		seedRecoveredChatEvents(cs)
	}
	s.mu.Unlock()

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
	if in.Tool == agentsoftware.OpenCodeTool {
		softwareGate.RLock()
		defer softwareGate.RUnlock()
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
	if _, ok := h.sup.sessions[sessionID]; ok {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	if _, ok := h.sup.starting[requestID]; ok {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrOperationConflict
	}
	live := 0
	for _, existing := range h.sup.sessions {
		if existing.state.ReservesQuota() {
			live++
		}
	}
	if live >= terminalproto.MaxSessionsPerAgent {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrSessionCapacity
	}
	if in.Yolo && (h.sup.opts.YoloConsent == nil || h.sup.opts.YoloConsent() != nil) {
		h.sup.mu.Unlock()
		return terminalproto.SessionChanged{}, terminalhost.ErrYoloRefused
	}
	cs := &chatSession{id: sessionID, title: in.Title, tool: in.Tool, state: terminalproto.Creating, yolo: in.Yolo}
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
	spec, ok := acpagent.Lookup(in.Tool)
	if !ok {
		return terminalproto.SessionChanged{}, acpagent.ErrUnsupported
	}
	// A managed installation is preferred over PATH, the same as a terminal
	// launch, so an install reviewed for this tool is the one a chat runs.
	binary, err := h.sup.lookPath(spec.Command)
	if managed, found, managedErr := agentsoftware.ManagedBinary(h.sup.opts.Home, in.Tool); managedErr != nil {
		return terminalproto.SessionChanged{}, &acpagent.VersionError{Agent: spec.DisplayName, Required: spec.MinVersion, Reason: "reet's managed installation is broken"}
	} else if found {
		binary, err = managed, nil
	}
	if err != nil {
		return terminalproto.SessionChanged{}, &acpagent.VersionError{Agent: spec.DisplayName, Required: spec.MinVersion, Reason: "the agent is not installed"}
	}
	// The start runs on the supervisor's process-lifetime context: a control
	// connection lost during startup must not kill a chat the controller can
	// adopt after it reconnects.
	startCtx, cancel := context.WithTimeout(h.sup.ctx, 60*time.Second)
	defer cancel()
	h.sup.mu.Lock()
	cs.cancel = cancel
	h.sup.mu.Unlock()
	process, err := h.sup.start(startCtx, acpagent.Options{
		Agent: spec, Binary: binary, Dir: dir, Home: h.sup.opts.Home, Model: in.Model, Trust: in.Trust, Resume: in.ResumeACP,
		BaseEnv: os.Environ(),
		OnEvent: func(event acpagent.Event) { h.sup.publish(cs, event) },
	})
	if err != nil {
		return terminalproto.SessionChanged{}, err
	}
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
		for i := 0; i < len(out); i++ {
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

// liveTool counts this process's live sessions for one tool, across every
// connection. A software apply uses it to refuse while an OpenCode session
// still needs the installation it is replacing.
func (s *chatSupervisor) liveTool(tool string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, cs := range s.sessions {
		if cs.tool == tool && cs.state.ReservesQuota() {
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

func (h *chatHost) startPrompt(cs *chatSession, text string, images []acpagent.Attachment) bool {
	h.sup.mu.Lock()
	if cs.busy || cs.state != terminalproto.Running {
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
		if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > terminalproto.MaxUploadBytes {
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
	if yolo && (h.sup.opts.YoloConsent == nil || h.sup.opts.YoloConsent() != nil) {
		h.sup.mu.Unlock()
		return terminalhost.ErrYoloRefused
	}
	cs.yolo = yolo
	process := cs.process
	h.sup.mu.Unlock()
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
		acpID, exit := cs.acpID, cs.exit
		s.mu.Unlock()
		if link == nil || !link.live {
			return
		}
		_ = link.send(s.ctx, terminalproto.Message{Version: terminalproto.Version, RequestID: newID(), SessionID: cs.id,
			Generation: link.generation, Body: &terminalproto.SessionChanged{State: terminalproto.Exited, ExitCode: exit, ACPSessionID: reportedACP(cs.tool, acpID, link.history)}})
	})
}

func (h *chatHost) close() { h.sup.detach(h) }

// questionAnswerer is the part of a chat process that can resolve a typed
// question. It is optional: only a process that can ask questions has it.
type questionAnswerer interface {
	AnswerQuestion(string, acpagent.QuestionResolution) error
}

func isQuestionKind(kind string) bool {
	return kind == "question" || kind == "question_resolved" || kind == "question_cancelled"
}

func (s *chatSupervisor) publish(cs *chatSession, event acpagent.Event) {
	wire, ok := wireEvent(event)
	if !ok {
		if event.Kind == "question" && event.QuestionID != "" {
			s.cancelQuestion(cs, event.QuestionID)
		}
		return
	}
	if isQuestionKind(wire.Kind) {
		s.mu.Lock()
		link := s.link
		usable := link != nil && link.live && link.questions
		s.mu.Unlock()
		if !usable {
			// Nobody connected can answer, and an older controller would reject the
			// event. A question is never buffered for later: cancel it now.
			if wire.Kind == "question" {
				s.cancelQuestion(cs, wire.QuestionID)
			}
			return
		}
	}
	s.deliver(cs, wire)
	s.mu.Lock()
	yolo, process := cs.yolo, cs.process
	s.mu.Unlock()
	if yolo && event.Kind == "permission" && event.PermissionID != "" && process != nil {
		// YOLO answers this once, for the person. The process emits the real
		// permission_resolved (which may be a rejection when no allow-once
		// option exists), and that is what the controller audits.
		_ = process.Allow(event.PermissionID, true)
	}
	if event.Kind == "status" && event.State == "ended" {
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
	recordChatSessionEvent(cs, wire)
	link := s.link
	if link == nil || !link.live {
		bufferChatEvent(cs, wire)
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
			cs.bellAt = 0
		}
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
			if len(cs.pending) == 0 {
				cs.bellAt = 0
				if cs.lastStatus != nil && cs.lastStatus.State == "waiting" {
					next := "idle"
					if cs.busy || (cs.process != nil && cs.process.Working()) {
						next = "working"
					}
					cs.lastStatus = &terminalproto.ACPEvent{Kind: "status", State: next}
				}
			}
		}
	case "agent", "tool", "user":
		cs.activityAt = nowMs
	}
}

func seedRecoveredChatEvents(cs *chatSession) {
	if cs.state != terminalproto.Running {
		return
	}
	var prefix []terminalproto.ACPEvent
	if cs.lastStatus != nil {
		hasStatus := false
		for _, e := range cs.outbox {
			if e.Kind == "status" {
				hasStatus = true
				break
			}
		}
		if !hasStatus {
			prefix = append(prefix, *cs.lastStatus)
		}
	}
	for _, p := range cs.pending {
		inOutbox := false
		for _, e := range cs.outbox {
			if (e.Kind == "permission" || e.Kind == "permission_resolved" || e.Kind == "permission_cancelled") && e.PermissionID == p.PermissionID {
				inOutbox = true
				break
			}
		}
		if !inOutbox {
			prefix = append(prefix, p)
		}
	}
	if len(prefix) == 0 {
		return
	}
	saved := cs.outbox
	cs.outbox = nil
	cs.outboxBytes = 0
	for _, e := range prefix {
		bufferChatEvent(cs, e)
	}
	for _, e := range saved {
		bufferChatEvent(cs, e)
	}
}

func bufferChatEvent(cs *chatSession, event terminalproto.ACPEvent) {
	size := chatEventBytes(event)
	if size > maxDetachedChatBytes {
		return
	}
	cs.outbox = append(cs.outbox, event)
	cs.outboxBytes += size
	for len(cs.outbox) > maxDetachedChatEvents || cs.outboxBytes > maxDetachedChatBytes {
		cs.outboxBytes -= chatEventBytes(cs.outbox[0])
		cs.outbox = cs.outbox[1:]
	}
}

func chatEventBytes(event terminalproto.ACPEvent) int {
	return len(event.Kind) + len(event.MessageID) + len(event.ToolCallID) + len(event.PermissionID) +
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
		Reason: event.Reason, Detail: event.Detail, QuestionID: event.QuestionID}
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
			if version.Required == "" {
				// A different agent answered: update/install guidance would
				// point at the wrong thing, so report it as unavailable.
				code = terminalproto.ACPUnavailable
			}
		}
		if len(detail) > 480 {
			detail = cutText(detail, 480)
		}
		return terminalproto.Result{Code: code, Detail: detail}
	case errors.Is(err, acpagent.ErrUnsupported):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "That agent does not support chats on this machine."}
	case errors.Is(err, acpagent.ErrPluginsUnreadable):
		return terminalproto.Result{Code: terminalproto.ACPUnavailable, Detail: "The folder's agent extensions could not be read, so the chat was not started."}
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
	cs := h.find(sessionID)
	if cs == nil {
		return terminalhost.ErrSessionMissing
	}
	h.sup.mu.Lock()
	if cs.busy || cs.state != terminalproto.Running {
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
	return process.SetModel(changeCtx, model)
}
