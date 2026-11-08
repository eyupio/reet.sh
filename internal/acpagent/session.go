//go:build linux || darwin

package acpagent

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

var (
	ErrBusy         = errors.New("the chat is already working")
	ErrClosed       = errors.New("the chat is closed")
	ErrNoPermission = errors.New("that approval is no longer waiting")
	ErrExited       = errors.New("the agent exited")
	ErrTimeout      = errors.New("the chat turn timed out")
	// ErrImages means the agent did not advertise image prompts, so an image
	// cannot be attached to this chat.
	ErrImages = errors.New("the agent does not accept images")
)

// A bounded 5 MB preview needs room for base64 and the enclosing ACP update.
const maxLineBytes = 8 << 20

// An exited agent's final frames get a bounded chance to reach the sender.
// A stuck callback must not keep the session alive indefinitely.
const stdoutDrainTimeout = 2 * time.Second

// Attachment is one image embedded in an ACP prompt. The bytes come from the
// connector's private upload folder; no path is sent to the agent.
type Attachment struct {
	MIME string
	Data []byte
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return safeRPCText(e.Message) }

// authRequired reports the refusal an adapter sends when nobody is signed in on
// that machine. Codex says it at session/new; Claude creates a session without a
// login and says it at the first prompt, so both places ask this.
func authRequired(err error) bool {
	var rpcErr *rpcError
	// Claude and Codex answer exactly "Authentication required"; Cline appends
	// its own advice to the same bounded prefix.
	prefix := "Authentication required"
	return errors.As(err, &rpcErr) && rpcErr.Code == -32000 &&
		(rpcErr.Message == prefix || strings.HasPrefix(rpcErr.Message, prefix+":"))
}

// loginError names the sign-in step for the agent whose request was refused, or
// nil when this is not a login refusal or the agent has no reviewed chat login.
func loginError(spec Spec, err error) error {
	if !authRequired(err) {
		return nil
	}
	switch spec.Key {
	case CodexCandidate.Key:
		return ErrCodexLogin
	case ClaudeCandidate.Key:
		return ErrClaudeLogin
	case Cline.Key:
		return ErrClineLogin
	case Antigravity.Key:
		return ErrAntigravityLogin
	}
	return nil
}

// noStoredConversation reports an adapter's refusal to load a conversation it
// does not have. Cline answers -32002 "Resource not found: <id>", because it
// stores a chat only when its first prompt arrives.
func noStoredConversation(err error) bool {
	var rpcErr *rpcError
	return errors.As(err, &rpcErr) && rpcErr.Code == -32002 && strings.HasPrefix(rpcErr.Message, "Resource not found")
}

type rpcResult struct {
	result json.RawMessage
	err    *rpcError
}

type rpcCall struct {
	reply chan rpcResult
	apply func(json.RawMessage)
}

type pendingPermission struct {
	nativeID string
	rpcID    json.RawMessage
	allowID  string
	rejectID string
	timer    *time.Timer
}

// Session is one ACP agent process. Methods are safe for concurrent use;
// Prompt runs one turn at a time and Cancel may be called from any goroutine.
type Session struct {
	opts   Options
	binary string
	native *nativeView
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *cappedBuffer

	configMu     sync.Mutex
	steerMu      sync.Mutex
	settings     []terminalproto.ACPSetting
	previewText  map[string]string
	previewTools map[string]previewTarget
	steering     bool
	compacting   bool
	configuring  bool
	mu           sync.Mutex
	writeMu      sync.Mutex
	nextID       int64
	calls        map[int64]rpcCall
	pending      map[string]*pendingPermission
	questions    map[string]*pendingQuestion
	closed       bool
	acpID        string
	model        string
	promptImages bool
	mode         string
	tools        map[string]string
	toolStart    map[string]time.Time
	done         chan struct{}
	doneOnce     sync.Once
	retired      bool

	turnMu          sync.Mutex
	working         bool
	nativeBusy      bool
	nativeCancelled bool
	nativeEpoch     uint64
	turnProgress    bool
	turnError       bool
	turnSeq         uint64
	forceArmed      uint64
	cancelledSeq    uint64
	forceClosing    bool
	turnStart       time.Time
	lastEvent       time.Time
	warned          bool
}

func Start(ctx context.Context, opts Options) (*Session, error) {
	if opts.Agent.Key == "" {
		return nil, errors.New("the chat agent is not configured")
	}
	if opts.Dir == "" || !filepath.IsAbs(opts.Dir) {
		return nil, errors.New("the chat folder must be absolute")
	}
	if info, err := os.Stat(opts.Dir); err != nil || !info.IsDir() {
		return nil, errors.New("the chat folder is unavailable")
	}
	if opts.Home == "" || !filepath.IsAbs(opts.Home) {
		return nil, errors.New("the chat home is unavailable")
	}
	spec := opts.Agent
	binary := opts.Binary
	if binary == "" {
		found, err := exec.LookPath(spec.Command)
		if err != nil {
			return nil, &VersionError{Agent: spec.displayName(), Required: spec.MinVersion, Reason: "the agent is not installed"}
		}
		binary = found
	}
	if err := spec.CheckTrust(opts.Dir, opts.Trust); err != nil {
		return nil, err
	}
	version, err := ProbeVersion(ctx, binary, spec, opts.Dir, opts.Home, opts.BaseEnv)
	if err != nil {
		return nil, err
	}
	password, err := serverPassword(opts.Random)
	if err != nil {
		return nil, err
	}
	native, nativeArgs := prepareNativeView(opts, version, binary, password)
	if native != nil {
		defer func() {
			if !native.ready.Load() {
				native.cancel()
			}
		}()
	}
	args := append(append([]string(nil), spec.Args...), nativeArgs...)
	cmd := exec.Command(binary, args...)
	// A descendant can retain stderr after the leader exits. Bound exec's
	// copy goroutine so it cannot hide that exit from the supervisor forever.
	cmd.WaitDelay = time.Second
	cmd.Dir = opts.Dir
	cmd.Env = agentEnvironment(spec, opts.Home, opts.BaseEnv, LaunchEnv{Model: opts.Model, Password: password})
	cmd.SysProcAttr = procAttr()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	// Own stdout ourselves: StdoutPipe would let cmd.Wait close the reader
	// before the final buffered frames have been delivered.
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	cmd.Stdout = stdoutWriter
	stderr := &cappedBuffer{limit: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		return nil, ErrExited
	}
	_ = stdoutWriter.Close()
	s := &Session{opts: opts, binary: binary, native: native, cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr,
		calls: map[int64]rpcCall{}, pending: map[string]*pendingPermission{}, tools: map[string]string{},
		toolStart: map[string]time.Time{}, done: make(chan struct{})}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		defer stdout.Close()
		s.readLoop()
	}()
	go func() {
		// Observe leader exit independently: a child may still hold stdout open.
		_ = cmd.Wait()
		s.killGroup()
		// Killing descendants closes their inherited writers. Let the reader
		// drain before ended marks the session closed and discards later emits.
		timer := time.NewTimer(stdoutDrainTimeout)
		defer timer.Stop()
		select {
		case <-readDone:
		case <-timer.C:
		}
		_ = stdout.Close()
		s.ended()
	}()
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var initResult struct {
		Meta struct {
			Steering struct {
				Supported bool `json:"supported"`
			} `json:"steering"`
		} `json:"_meta"`
		ProtocolVersion int `json:"protocolVersion"`
		AgentInfo       struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
		AgentCapabilities struct {
			PromptCapabilities struct {
				Image bool `json:"image"`
			} `json:"promptCapabilities"`
		} `json:"agentCapabilities"`
	}
	raw, err := s.request(initCtx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"clientCapabilities": map[string]any{
			"fs":       map[string]bool{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
		"clientInfo": map[string]string{"name": "reet", "version": "1"},
	})
	if err != nil {
		s.Close()
		return nil, ErrExited
	}
	if json.Unmarshal(raw, &initResult) != nil {
		s.Close()
		return nil, ErrExited
	}
	s.promptImages = initResult.AgentCapabilities.PromptCapabilities.Image
	s.steering = initResult.Meta.Steering.Supported
	if initResult.ProtocolVersion != ProtocolVersion {
		s.Close()
		return nil, &VersionError{Agent: spec.displayName(), Found: fmt.Sprintf("ACP protocol %d", initResult.ProtocolVersion), Required: fmt.Sprintf("ACP protocol %d", ProtocolVersion)}
	}
	if spec.ExpectedName != "" && initResult.AgentInfo.Name != spec.ExpectedName {
		s.Close()
		return nil, &VersionError{Agent: spec.displayName(), Found: "agent name " + safeName(initResult.AgentInfo.Name), Reason: "a different agent answered"}
	}
	if err := checkVersion(spec, initResult.AgentInfo.Version); err != nil {
		s.Close()
		return nil, err
	}
	newCtx, newCancel := context.WithTimeout(ctx, 45*time.Second)
	defer newCancel()
	opened := opts.Resume == ""
	method, params := sessionRequest(spec, opts.Dir, opts.Resume)
	raw, err = s.request(newCtx, method, params)
	if err != nil {
		s.Close()
		if login := loginError(spec, err); login != nil {
			return nil, login
		}
		if !opened && noStoredConversation(err) {
			return nil, ErrNoStoredConversation
		}
		return nil, ErrExited
	}
	var created struct {
		SessionID     string         `json:"sessionId"`
		ConfigOptions []configOption `json:"configOptions"`
	}
	valid := json.Unmarshal(raw, &created) == nil && (!opened || created.SessionID != "")
	if !valid {
		s.Close()
		return nil, ErrExited
	}
	// A profile may pin an option that must be in force before any turn, such as
	// Cline's auto-approve switch being off. The launch flag is a request; only
	// the session's own report is the acknowledgement.
	if err := spec.checkRequiredOptions(created.ConfigOptions); err != nil {
		s.Close()
		return nil, err
	}
	mode, model := "", ""
	for _, option := range created.ConfigOptions {
		switch option.ID {
		case "mode":
			mode = string(option.CurrentValue)
		case "model":
			model = string(option.CurrentValue)
		}
	}
	// A profile may name one mode to start in (StartMode). It is switched to only
	// when the adapter's reply confirms it; otherwise the chat is refused below.
	specified := spec.StartMode != "" && mode != ""
	if specified && mode != spec.StartMode {
		session := sessionID0(created.SessionID, opts.Resume)
		switched, err := s.switchMode(newCtx, session, spec.StartMode)
		if err != nil || switched != spec.StartMode {
			s.Close()
			return nil, fmt.Errorf("%s did not start in %s mode; the chat was not started", spec.displayName(), spec.StartMode)
		}
		mode = switched
	}
	// No other mode switching is offered: a custom primary agent must never
	// become the default behind the approval config.
	if spec.RequireMode != "" && mode != spec.RequireMode {
		s.Close()
		return nil, fmt.Errorf("%s is not in %s mode; the chat was not started", spec.displayName(), spec.RequireMode)
	}
	sessionID := created.SessionID
	if !opened {
		sessionID = opts.Resume
	}
	s.mu.Lock()
	s.acpID = sessionID
	s.mode = mode
	s.model = model
	s.mu.Unlock()
	s.updateSettings(raw)
	// The launch environment carries a request, not an acknowledgement. A
	// chosen model needs one unambiguous read-back on both new and load before
	// this session can be announced or receive its opening prompt. An adapter
	// that ignores a model at session/new (Cline) applies it by a confirmed
	// switch instead; an unconfirmed switch still refuses the chat.
	if opts.Provider != "" && opts.Resume == "" {
		providerID, providerNow := "", ""
		for _, o := range s.Controls().Options {
			if o.Category == "provider" {
				providerID, providerNow = o.ID, o.Current
			}
		}
		if providerID == "" {
			s.Close()
			return nil, ErrProvider
		}
		// Already on the chosen source: nothing to switch, nothing to confirm.
		if providerNow != opts.Provider {
			if err := s.SetOption(newCtx, providerID, opts.Provider); err != nil {
				s.Close()
				if errors.Is(err, ErrControl) {
					return nil, ErrProviderUnconfirmed
				}
				return nil, ErrProvider
			}
		}
	}
	if opts.Model != "" {
		// Startup has no omitted-list exception, including for OpenCode.
		confirmed, ok := acknowledgedModel(raw, opts.Model, false)
		switch {
		case ok && confirmed == opts.Model:
		case spec.ConfirmModelBySwitch:
			if err := s.setModel(newCtx, opts.Model, true); err != nil {
				s.Close()
				return nil, ErrModel
			}
		default:
			s.Close()
			return nil, ErrModel
		}
	}
	if opts.Model != "" && s.Model() != opts.Model {
		s.Close()
		return nil, ErrModel
	}
	if opts.Effort != "" {
		effortID := ""
		for _, o := range s.Controls().Options {
			if o.Category == "thought_level" {
				effortID = o.ID
			}
		}
		if effortID == "" {
			s.Close()
			return nil, ErrControl
		}
		if err := s.SetOption(newCtx, effortID, opts.Effort); err != nil {
			s.Close()
			return nil, err
		}
	}
	// Every replayed update was handled by the reader before the load reply
	// it precedes, so the replay has been delivered by now.
	if opts.OnReady != nil {
		opts.OnReady()
	}
	if !opened {
		s.markInterrupted()
	}
	s.emit(Event{Kind: "status", State: "idle", Detail: s.Model()})
	s.startNativeView()
	return s, nil
}

// SessionID is the agent's own id, for diagnostics only; the controller uses
// its own session id.
func (s *Session) SessionID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acpID
}

// Done closes when the process has exited and all calls and cards have ended.
func (s *Session) Done() <-chan struct{} { return s.done }

// Working reports whether a prompt turn is in progress.
func (s *Session) Working() bool {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	return s.working || s.nativeBusy
}

// Model is the last confirmed model; empty means it is not known.
func (s *Session) Model() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

// ImagesSupported reports whether the agent advertised image prompts at
// initialize. The connector uses it to refuse an image before a turn starts.
func (s *Session) ImagesSupported() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.promptImages
}

// cancelGrace is how long an adapter gets to answer session/cancel after the
// watchdog stops a silent turn before the prompt request is abandoned.
var cancelGrace = 30 * time.Second

func (s *Session) emit(event Event) {
	s.mu.Lock()
	now := time.Now()
	s.lastEvent = now
	closed := s.closed
	s.mu.Unlock()
	if closed || s.opts.OnEvent == nil {
		return
	}
	s.opts.OnEvent(event)
}

func (s *Session) nextRequestID() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return s.nextID
}

func (s *Session) write(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrClosed
	}
	_, err = s.stdin.Write(data)
	return err
}

func (s *Session) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return s.requestWithReply(ctx, method, params, nil)
}

func (s *Session) writeRequest(method string, message any) error {
	if method == "session/prompt" {
		// Working is visible before the prompt is written. Order that write
		// with Cancel: never send a turn Stop already cancelled, and never
		// let session/cancel overtake a prompt being committed to the pipe.
		s.turnMu.Lock()
		defer s.turnMu.Unlock()
		if s.cancelledSeq == s.turnSeq {
			return context.Canceled
		}
	}
	return s.write(message)
}

// apply runs in protocol read order before a later notification can supersede
// this reply. It must be bounded and must never wait for another RPC.
func (s *Session) requestWithReply(ctx context.Context, method string, params any, apply func(json.RawMessage)) (json.RawMessage, error) {
	id := s.nextRequestID()
	ch := make(chan rpcResult, 1)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	s.calls[id] = rpcCall{reply: ch, apply: apply}
	s.mu.Unlock()
	if err := s.writeRequest(method, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		s.mu.Lock()
		delete(s.calls, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("%s: %w", s.opts.Agent.displayName(), result.err)
		}
		return result.result, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.calls, id)
		s.mu.Unlock()
		return nil, ErrTimeout
	case <-s.done:
		return nil, ErrExited
	}
}

func (s *Session) respond(id json.RawMessage, result any, rpcErr *rpcError) {
	if len(id) == 0 {
		return
	}
	message := map[string]any{"jsonrpc": "2.0", "id": id}
	if rpcErr != nil {
		message["error"] = rpcErr
	} else {
		message["result"] = result
	}
	_ = s.write(message)
}

func (s *Session) readLoop() {
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 64<<10), maxLineBytes)
	for scanner.Scan() {
		// Scanner reuses its buffer, but json.Unmarshal copies every field it
		// keeps (strings and json.RawMessage), and handle is synchronous, so
		// the line does not need another copy.
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var message rpcMessage
		if json.Unmarshal(line, &message) != nil {
			continue
		}
		s.handle(message)
	}
	// EOF or a framing error makes a live adapter unusable. The independent
	// waiter observes its exit and releases requests and cards after retirement.
	s.killGroup()
}

func (s *Session) handle(message rpcMessage) {
	if message.Method != "" {
		switch message.Method {
		case "session/update":
			s.handleUpdate(message.Params)
		case "_auth/status_update":
			if len(message.ID) == 0 {
				s.handleAuthStatus(message.Params)
			} else {
				s.respond(message.ID, nil, &rpcError{Code: -32601, Message: "not available"})
			}
		case "session/request_permission":
			s.handlePermission(message.ID, message.Params)
		case "elicitation/create":
			s.handleElicitation(message.ID, message.Params)
		case "fs/write_text_file":
			// The agent has already written the file itself. The client fs call is
			// a mirror, never a gate; answering without writing avoids a double
			// write and keeps the protocol moving.
			s.respond(message.ID, nil, nil)
		case "fs/read_text_file", "terminal/create", "terminal/output", "terminal/wait_for_exit", "terminal/kill", "terminal/release":
			s.respond(message.ID, nil, &rpcError{Code: -32601, Message: "not available"})
		default:
			s.respond(message.ID, nil, &rpcError{Code: -32601, Message: "not available"})
		}
		return
	}
	var id int64
	if len(message.ID) == 0 || json.Unmarshal(message.ID, &id) != nil {
		return
	}
	s.mu.Lock()
	call := s.calls[id]
	delete(s.calls, id)
	s.mu.Unlock()
	if call.reply != nil {
		if message.Error == nil && call.apply != nil {
			call.apply(message.Result)
		}
		call.reply <- rpcResult{result: message.Result, err: message.Error}
	}
}

type updateEnvelope struct {
	SessionID string `json:"sessionId"`
	Update    struct {
		SessionUpdate string `json:"sessionUpdate"`
		MessageID     string `json:"messageId"`
		ToolCallID    string `json:"toolCallId"`
		Title         string `json:"title"`
		Kind          string `json:"kind"`
		Status        string `json:"status"`
		Locations     []struct {
			Path string `json:"path"`
		} `json:"locations"`
		Content   json.RawMessage `json:"content"`
		RawInput  json.RawMessage `json:"rawInput"`
		RawOutput json.RawMessage `json:"rawOutput"`
	} `json:"update"`
}

func (s *Session) handleUpdate(raw json.RawMessage) {
	var update updateEnvelope
	if json.Unmarshal(raw, &update) != nil {
		return
	}
	u := update.Update
	switch u.SessionUpdate {
	case "config_option_update", "config_options_update":
		var envelope struct {
			Update json.RawMessage `json:"update"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			s.updateSettings(envelope.Update)
		}
	case "compaction_update":
		var envelope struct {
			Update struct {
				Status string `json:"status"`
			} `json:"update"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			s.mu.Lock()
			s.compacting = envelope.Update.Status == "in_progress"
			s.mu.Unlock()
		}
	case "agent_message_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.markProgress()
			s.emit(Event{Kind: "agent", MessageID: u.MessageID, Text: text})
			s.emitPreviews(u.MessageID, "", s.messagePreviews(u.MessageID, text))
		}
		s.emitPreviews(u.MessageID, "", contentPreviews(u.Content))
	case "user_message_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.emit(Event{Kind: "user", Text: text})
		}
	case "tool_call", "tool_call_update":
		s.markProgress()
		durationMs := int64(0)
		if u.ToolCallID != "" {
			now := time.Now()
			s.mu.Lock()
			start, started := s.toolStart[u.ToolCallID]
			status := u.Status
			if status == "" {
				// Output-only updates must not reopen an already finished tool.
				status = s.tools[u.ToolCallID]
			}
			final := status == "completed" || status == "failed" || status == "cancelled"
			if !started && !final {
				s.toolStart[u.ToolCallID] = now
			}
			if final {
				if started {
					durationMs = now.Sub(start).Milliseconds()
				}
				delete(s.toolStart, u.ToolCallID)
			}
			s.tools[u.ToolCallID] = status
			s.mu.Unlock()
		}
		text := toolText(u.Content)
		locations := make([]string, 0, len(u.Locations))
		for _, location := range u.Locations {
			if location.Path != "" {
				locations = append(locations, location.Path)
			}
		}
		details := toolDetails(u.RawInput, u.RawOutput)
		if len(locations) > 0 && details.path == "" {
			details.path = locations[0]
		}
		s.emit(Event{Kind: "tool", ToolCallID: u.ToolCallID, Title: u.Title, ToolKind: u.Kind, Status: u.Status, Text: text,
			Command: details.command, Path: details.path, Diff: details.diff, Additions: details.additions, Deletions: details.deletions,
			DurationMs: durationMs, Locations: locations})
		pictures := contentPreviews(u.Content)
		if len(pictures) == 0 {
			pictures = contentPreviews(u.RawOutput)
		}
		target := s.previewTarget(u.ToolCallID, u.Kind, details.path, u.Status)
		if len(pictures) == 0 && target != "" {
			pictures = append(pictures, Preview{Path: target})
		}
		s.emitPreviews("", u.ToolCallID, pictures)
	case "agent_thought_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.markProgress()
			s.emit(Event{Kind: "tool", ToolCallID: "thought-" + u.MessageID, Title: "Thinking", ToolKind: "think", Status: "in_progress", Text: text})
		}
	}
}

func (s *Session) markProgress() {
	s.mu.Lock()
	s.turnProgress = true
	s.mu.Unlock()
}

// toolDetails pulls the parts a tool row shows (its command, target path and diff summary) out of the adapter's raw
// input and output. Adapters differ: anything absent is left empty and the browser falls back to the title.
type toolDetailsValue struct {
	command, path, diff  string
	additions, deletions int
}

func toolDetails(rawInput, rawOutput json.RawMessage) toolDetailsValue {
	var detail toolDetailsValue
	if len(rawInput) > 0 {
		var input struct {
			Command     string `json:"command"`
			CommandLine string `json:"CommandLine"`
			FilePath    string `json:"filePath"`
			Filepath    string `json:"filepath"`
			Path        string `json:"path"`
		}
		if json.Unmarshal(rawInput, &input) == nil {
			detail.command = input.Command
			if detail.command == "" {
				detail.command = input.CommandLine
			}
			detail.path = firstNonEmpty(input.FilePath, input.Filepath, input.Path)
		}
	}
	if len(rawOutput) > 0 {
		var output struct {
			Metadata struct {
				FileDiff struct {
					Patch     string `json:"patch"`
					File      string `json:"file"`
					Additions int    `json:"additions"`
					Deletions int    `json:"deletions"`
				} `json:"filediff"`
			} `json:"metadata"`
		}
		if json.Unmarshal(rawOutput, &output) == nil {
			detail.diff = output.Metadata.FileDiff.Patch
			detail.additions = max(0, output.Metadata.FileDiff.Additions)
			detail.deletions = max(0, output.Metadata.FileDiff.Deletions)
			if detail.path == "" {
				detail.path = output.Metadata.FileDiff.File
			}
		}
	}
	return detail
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func contentText(raw json.RawMessage) string {
	var content struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &content) == nil && content.Type == "text" {
		return content.Text
	}
	return ""
}

func toolText(raw json.RawMessage) string {
	var parts []struct {
		Type    string `json:"type"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		for _, part := range parts {
			if part.Type == "content" && part.Content.Text != "" {
				return part.Content.Text
			}
		}
	}
	return ""
}

// permissionContent is one typed content block of a tool call. Claude's adapter
// describes an edit as {type:"diff", path, oldText, newText} instead of
// OpenCode's rawInput.diff string.
type permissionContent struct {
	Type    string  `json:"type"`
	Path    string  `json:"path"`
	OldText *string `json:"oldText"`
	NewText *string `json:"newText"`
}

// subagentSummary renders a subagent delegation for review: its type and the
// bounded prompt. It returns "" when there is no prompt to review.
func subagentSummary(kind, prompt string) (string, bool) {
	if strings.TrimSpace(prompt) == "" {
		return "", false
	}
	text := prompt
	if kind != "" {
		text = "[" + cutUTF8(kind, 64) + "] " + prompt
	}
	if len(text) > maxDiffBytes {
		return cutUTF8(text, maxDiffBytes), true
	}
	return text, false
}

// maxDiffBytes bounds a rendered permission diff.
const maxDiffBytes = 10 << 10

// typedDiff renders the first typed diff block as a bounded, line-prefixed
// patch (old lines "-", new lines "+"). It makes no attempt to align lines, so
// it never hides a change; ok is false when no usable block exists, which keeps
// the card unapprovable. truncated reports that the bound cut the text.
func typedDiff(blocks []permissionContent) (diff, path string, truncated, ok bool) {
	for _, b := range blocks {
		valid := b.Type == "diff" && b.NewText != nil && b.Path != ""
		if !valid {
			continue
		}
		diff, truncated = renderTextDiff(b.Path, b.OldText, *b.NewText)
		return diff, b.Path, truncated, true
	}
	return "", "", false, false
}

// renderTextDiff renders one file change as a bounded, line-prefixed patch. A
// nil oldText means the change only adds text. It makes no attempt to align
// lines, so it never hides a change.
func renderTextDiff(path string, oldText *string, newText string) (string, bool) {
	var sb strings.Builder
	sb.WriteString("--- " + path + "\n+++ " + path + "\n")
	write := func(prefix string, text string) {
		for _, line := range strings.SplitAfter(text, "\n") {
			if line == "" || sb.Len() > maxDiffBytes {
				continue
			}
			// Bound each line's copy so a huge single line cannot allocate more.
			if len(line) > maxDiffBytes {
				line = line[:maxDiffBytes]
			}
			sb.WriteString(prefix + line)
			if !strings.HasSuffix(line, "\n") {
				sb.WriteString("\n")
			}
		}
	}
	if oldText != nil {
		write("-", *oldText)
	}
	write("+", newText)
	out := sb.String()
	if len(out) > maxDiffBytes {
		return cutUTF8(out, maxDiffBytes), true
	}
	return out, false
}

type permissionParams struct {
	SessionID string `json:"sessionId"`
	ToolCall  struct {
		ToolCallID string `json:"toolCallId"`
		Title      string `json:"title"`
		Kind       string `json:"kind"`
		Status     string `json:"status"`
		Locations  []struct {
			Path string `json:"path"`
		} `json:"locations"`
		Meta struct {
			ClaudeCode struct {
				ParentToolUseID string `json:"parentToolUseId"`
			} `json:"claudeCode"`
			// Permission carries the Codex adapter's justification: the model's
			// own words for wanting to leave its sandbox. Claude sends no such
			// field; its rawInput description is used instead.
			Permission struct {
				Description string `json:"description"`
			} `json:"permission"`
		} `json:"_meta"`
		Content  []permissionContent `json:"content"`
		RawInput struct {
			Command     string   `json:"command"`
			CommandLine string   `json:"CommandLine"`
			Commands    []string `json:"commands"`
			URL         string   `json:"url"`
			Queries     []string `json:"queries"`
			Files       []struct {
				Path string `json:"path"`
			} `json:"files"`
			Description  string `json:"description"`
			Diff         string `json:"diff"`
			FilePath     string `json:"filepath"`
			SnakePath    string `json:"file_path"`
			Path         string `json:"path"`
			OldText      string `json:"old_text"`
			NewText      string `json:"new_text"`
			Prompt       string `json:"prompt"`
			SubagentType string `json:"subagent_type"`
			ParentDir    string `json:"parentDir"`
		} `json:"rawInput"`
	} `json:"toolCall"`
	Options []struct {
		OptionID string `json:"optionId"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	} `json:"options"`
}

// turnAcceptsRequests reports whether a permission or question request can
// belong to a turn the person is still waiting on. A request that arrives with no
// turn running, or after the running turn was cancelled, has no owner: the agent
// does not withdraw it, so it is answered as cancelled instead of raising a card.
func (s *Session) turnAcceptsRequests() bool {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	return s.nativeBusy && !s.nativeCancelled || s.working && s.cancelledSeq != s.turnSeq
}

func (s *Session) handlePermission(rpcID json.RawMessage, raw json.RawMessage) {
	if len(rpcID) == 0 {
		return
	}
	var params permissionParams
	if json.Unmarshal(raw, &params) != nil {
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	if s.native != nil && s.native.ready.Load() && !s.turnAcceptsRequests() {
		s.native.confirmPermissionTurn(s, params.ToolCall.ToolCallID)
	}
	if !s.turnAcceptsRequests() {
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	allowID, rejectID := "", ""
	for _, option := range params.Options {
		switch option.Kind {
		case "allow_once":
			allowID = option.OptionID
		case "reject_once":
			rejectID = option.OptionID
		}
	}
	if rejectID == "" {
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	permissionID, err := permissionID()
	if err != nil {
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	input := params.ToolCall.RawInput
	diff := input.Diff
	truncated := false
	typedPath := ""
	if diff == "" && params.ToolCall.Kind == "edit" {
		if typed, fromContent, cut, ok := typedDiff(params.ToolCall.Content); ok {
			diff, typedPath, truncated = typed, fromContent, cut
		} else if input.Path != "" && input.NewText != "" {
			// Cline's editor reports the file and its new text instead of a
			// diff. old_text is the replaced text when the call is a
			// replacement; without it the call creates or inserts, so the
			// addition is the whole change.
			var oldText *string
			if input.OldText != "" {
				oldText = &input.OldText
			}
			diff, truncated = renderTextDiff(input.Path, oldText, input.NewText)
			typedPath = input.Path
		}
	}
	if len(diff) > maxDiffBytes {
		diff = cutUTF8(diff, maxDiffBytes)
		truncated = true
	}
	command := input.Command
	if command == "" {
		command = input.CommandLine
	}
	if command == "" && len(input.Commands) > 0 {
		command = strings.Join(input.Commands, "\n")
	}
	if command == "" && input.URL != "" {
		command = input.URL
	}
	if command == "" && len(input.Queries) > 0 {
		command = strings.Join(input.Queries, "\n")
	}
	if len(command) > maxDiffBytes {
		command = cutUTF8(command, maxDiffBytes)
		truncated = true
	}
	path := input.FilePath
	if path == "" {
		path = input.SnakePath
	}
	if path == "" {
		path = input.Path
	}
	if path == "" {
		path = typedPath
	}
	if path == "" && len(input.Files) > 0 {
		path = input.Files[0].Path
	}
	if path == "" && len(params.ToolCall.Locations) > 0 {
		path = params.ToolCall.Locations[0].Path
	}
	// A card the person cannot review must not offer Allow once. The probe
	// always sends the diff for an edit and the command for an execute; a
	// missing one is a protocol surprise, not a reason to guess.
	if params.ToolCall.Kind == "edit" && diff == "" {
		truncated = true
	}
	if params.ToolCall.Kind == "execute" && command == "" {
		truncated = true
	}
	// Some tools (a read of several files, a search, a fetch) carry nothing the
	// card's command, path or diff would show. The adapter's own title names the
	// tool and its input, so it fills the command rather than dropping a request
	// the person is still expected to answer.
	if command == "" && path == "" && diff == "" {
		command = strings.TrimSpace(params.ToolCall.Title)
		if command == "" {
			// Nothing names the request, so it cannot be reviewed.
			command = s.opts.Agent.displayName() + " tool call"
			truncated = true
		}
		if len(command) > maxDiffBytes {
			command = cutUTF8(command, maxDiffBytes)
			truncated = true
		}
	}
	if params.ToolCall.Kind == "think" {
		// Claude reports a subagent delegation as kind "think". The person must
		// see what the subagent is told before it may run tools on their behalf.
		var cut bool
		command, cut = subagentSummary(params.ToolCall.RawInput.SubagentType, params.ToolCall.RawInput.Prompt)
		truncated = truncated || cut || command == ""
	}
	locations := make([]string, 0, len(params.ToolCall.Locations))
	for _, location := range params.ToolCall.Locations {
		if location.Path != "" {
			locations = append(locations, location.Path)
		}
	}
	pending := &pendingPermission{rpcID: rpcID, allowID: allowID, rejectID: rejectID}
	if s.native != nil {
		pending.nativeID = params.ToolCall.ToolCallID
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	s.pending[permissionID] = pending
	pending.timer = time.AfterFunc(s.opts.permissionTimeout(), func() {
		s.resolve(permissionID, "cancelled", "timeout")
		s.emit(Event{Kind: "notice", Text: "The approval request timed out, so " + s.opts.Agent.displayName() + " was stopped.", Bad: true})
		_ = s.sendCancel()
	})
	s.mu.Unlock()
	title := permissionTitle(params.ToolCall.Title, params.ToolCall.Meta.ClaudeCode.ParentToolUseID, s.opts.Agent.displayName())
	// The agent's own words for the request, shown on the card so a person can
	// judge it: Codex sends the model's justification in _meta.permission
	// .description, Claude a short description of the call in rawInput. Neither
	// is required, and neither is a reason to refuse the card.
	rationale := params.ToolCall.Meta.Permission.Description
	if rationale == "" {
		rationale = params.ToolCall.RawInput.Description
	}
	s.emit(Event{Kind: "status", State: "waiting", Detail: "Waiting for your approval."})
	s.emit(Event{Kind: "permission", PermissionID: permissionID, ToolCallID: params.ToolCall.ToolCallID,
		Title: title, ToolKind: params.ToolCall.Kind, Command: command,
		Path: path, Diff: diff, Locations: locations, Truncated: truncated,
		Text: rationale})
	if s.native != nil {
		reply := s.native.takePermissionReply(pending.nativeID)
		if reply != "" {
			s.nativePermissionReply(pending.nativeID, reply)
		}
	}
}

// permissionTitle names the card. A subagent's own tool call is a separate
// approval from the delegation that started it (SC2 probe); say so, so it is not
// mistaken for the parent's.
func permissionTitle(title, parent, agent string) string {
	if title == "" {
		title = agent + " tool call"
	}
	if parent != "" {
		title = "Subagent: " + title
	}
	return title
}

func permissionID() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// AllowAllPending answers every card still waiting with Allow once. It is used
// when a live chat is switched to YOLO: the person has asked reet to stop
// interrupting, including for the card already on screen.
func (s *Session) AllowAllPending() error {
	s.mu.Lock()
	ids := make([]string, 0, len(s.pending))
	for id := range s.pending {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		if err := s.Allow(id, true); err != nil && !errors.Is(err, ErrNoPermission) {
			return err
		}
	}
	return nil
}

// Allow answers one card. allow true is Allow once; false is Deny. There is no
// Always allow, and no answer is remembered for the next tool call.
func (s *Session) Allow(id string, allow bool) error {
	decision := "reject"
	if allow {
		decision = "once"
	}
	return s.resolve(id, decision, "")
}

func (s *Session) resolve(id, decision, reason string) error {
	s.mu.Lock()
	pending := s.pending[id]
	if pending != nil {
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if pending == nil {
		return ErrNoPermission
	}
	if s.native != nil {
		s.native.retirePermission(pending.nativeID)
	}
	if pending.timer != nil {
		pending.timer.Stop()
	}
	var result map[string]any
	if decision == "cancelled" {
		result = map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}
	} else {
		option := pending.rejectID
		if decision == "once" {
			if pending.allowID == "" {
				option = pending.rejectID
				decision = "reject"
			} else {
				option = pending.allowID
			}
		}
		result = map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": option}}
	}
	message := map[string]any{"jsonrpc": "2.0", "id": pending.rpcID, "result": result}
	if err := s.write(message); err != nil {
		if decision != "cancelled" {
			s.emit(Event{Kind: "permission_cancelled", PermissionID: id, Reason: "not_sent"})
			return err
		}
	}
	if decision == "cancelled" {
		s.emit(Event{Kind: "permission_cancelled", PermissionID: id, Reason: reason})
	} else {
		s.emit(Event{Kind: "permission_resolved", PermissionID: id, Allow: boolPtr(decision == "once")})
	}
	return nil
}

func (s *Session) sendCancel() error {
	s.mu.Lock()
	acpID := s.acpID
	s.mu.Unlock()
	if acpID == "" {
		return ErrClosed
	}
	return s.write(map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]string{"sessionId": acpID}})
}

// commitForceStop decides, under the turn lock, to end the chat because turn seq
// is still running. From that point no new prompt may start, so a turn that
// finishes just afterwards cannot be followed by one that the close then kills.
func (s *Session) commitForceStop(seq uint64) bool {
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if !s.working || s.turnSeq != seq {
		return false
	}
	s.forceClosing = true
	return true
}

// Cancel stops the current turn and closes every card this client still holds.
// The agent does not withdraw an outstanding request_permission, so reet must.
func (s *Session) Cancel() error {
	s.turnMu.Lock()
	working := s.working || s.nativeBusy
	external, epoch := s.nativeBusy && !s.working, s.nativeEpoch
	if s.nativeBusy {
		s.nativeCancelled = true
	}
	if working {
		s.cancelledSeq = s.turnSeq
	}
	s.turnMu.Unlock()
	// Read before the cards are cancelled and the agent winds the turn down.
	s.mu.Lock()
	toolRunning := len(s.toolStart) > 0
	s.mu.Unlock()
	endChat := working && toolRunning && s.opts.Agent.EndChatOnStopWhileToolRuns
	s.cancelCards()
	s.emit(Event{Kind: "status", State: "cancelled"})
	err := s.sendCancel()
	if endChat {
		// The adapter ends the turn but leaves the command alive, so end the chat
		// and retire the process group rather than say it stopped while it runs on.
		s.emit(Event{Kind: "notice", Text: s.opts.Agent.displayName() + " was running a command when you pressed Stop, so this chat was ended to be sure nothing keeps running.", Bad: true})
		s.Close()
		return err
	}
	s.armForceStop()
	if external {
		s.armNativeStop(epoch)
	}
	return err
}

// armForceStop gives the agent ForceStopGrace to end the turn it was asked to
// cancel. An agent that ignores session/cancel would otherwise leave the chat
// busy until the turn timeout while the person has already been told it
// stopped. If the same turn is still running after the grace period, the chat
// is ended explicitly and the process group is retired, never left half alive.
// It is bound to the turn that was cancelled, so it cannot end a later turn.
func (s *Session) armForceStop() {
	s.turnMu.Lock()
	if !s.working || s.forceArmed == s.turnSeq {
		s.turnMu.Unlock()
		return
	}
	seq := s.turnSeq
	s.forceArmed = seq
	s.turnMu.Unlock()
	go func() {
		timer := time.NewTimer(s.opts.forceStopGrace())
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-s.done:
			return
		}
		if !s.commitForceStop(seq) {
			return
		}
		s.emit(Event{Kind: "notice", Text: s.opts.Agent.displayName() + " did not stop when asked, so this chat was ended.", Bad: true})
		s.Close()
	}()
}

// stopTurn cancels the agent without reporting the turn as cancelled. The
// caller has already reported a terminal provider error, which must remain the
// last state the person sees.
func (s *Session) stopTurn() error {
	s.turnMu.Lock()
	if s.working {
		s.cancelledSeq = s.turnSeq
	}
	s.turnMu.Unlock()
	s.cancelCards()
	err := s.sendCancel()
	s.armForceStop()
	return err
}

func (s *Session) cancelCards() {
	s.cancelQuestions("cancelled")
	s.mu.Lock()
	pending := make([]string, 0, len(s.pending))
	for id := range s.pending {
		pending = append(pending, id)
	}
	s.mu.Unlock()
	for _, id := range pending {
		_ = s.resolve(id, "cancelled", "cancelled")
	}
}

// recoverRoot is where this adapter keeps its own logs and store: the person's
// login folder when the profile routes one (Cline's ~/.cline), otherwise the
// private chat home.
func (s *Session) recoverRoot() string {
	if s.opts.Agent.LoginHomeEnv == "" {
		return s.opts.Home
	}
	if root, err := loginHome(s.opts.Agent, s.opts.BaseEnv); err == nil {
		return root
	}
	return ""
}

// recoverLive asks the adapter for an error it logged while the turn has been
// silent. It returns "" when the adapter has no live source or the log is quiet.
func (s *Session) recoverLive(acpID string, afterMS int64) string {
	if s.opts.Agent.RecoverLive == nil {
		return ""
	}
	return s.opts.Agent.RecoverLive(s.recoverRoot(), acpID, afterMS)
}

// reportTurnError ends a turn whose provider failed without ACP reporting it.
// The error stays the last observable state; the cancelled reply that follows
// session/cancel is suppressed by Prompt. The turn sequence keeps a watchdog
// from reporting against a later turn.
func (s *Session) reportTurnError(seq uint64, message string) bool {
	s.turnMu.Lock()
	active := s.working && s.turnSeq == seq
	s.turnMu.Unlock()
	if !active {
		return false
	}
	s.mu.Lock()
	if s.turnProgress {
		s.mu.Unlock()
		return false
	}
	s.turnError = true
	s.mu.Unlock()
	s.emit(Event{Kind: "status", State: "error", Detail: safeRPCText(message)})
	_ = s.stopTurn()
	return true
}

// markInterrupted closes tools that session/load replayed as still running.
// The agent does not re-ask their permission, so leaving them "working" would
// be a lie; a new prompt starts cleanly after them.
func (s *Session) markInterrupted() {
	s.mu.Lock()
	ids := make([]string, 0)
	for id, status := range s.tools {
		if status == "pending" || status == "in_progress" {
			ids = append(ids, id)
		}
	}
	// Interrupted calls are over: they must not count as running tools.
	for _, id := range ids {
		delete(s.toolStart, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.emit(Event{Kind: "tool", ToolCallID: id, Status: "failed", ToolKind: "other", Title: "Interrupted tool call",
			Text: "Interrupted before this chat was reopened."})
	}
}

// Prompt runs one turn with text only. It blocks until the turn ends.
func (s *Session) Prompt(ctx context.Context, text string) error {
	return s.PromptWithAttachments(ctx, text, nil)
}

// PromptWithAttachments runs one turn with text and optional images. It
// blocks until the turn ends; the connector calls it in its own goroutine so
// control messages (cancel, approval) stay responsive. Images are embedded as
// ACP image content blocks, so the agent sees the image itself instead of a
// path it has to open with a tool.
func (s *Session) PromptWithAttachments(ctx context.Context, text string, images []Attachment) error {
	if len(images) > 0 {
		s.mu.Lock()
		allowed := s.promptImages
		s.mu.Unlock()
		if !allowed {
			return ErrImages
		}
	}
	s.turnMu.Lock()
	if s.configuring {
		s.turnMu.Unlock()
		return ErrBusy
	}
	if s.forceClosing {
		s.turnMu.Unlock()
		return ErrClosed
	}
	if s.working || s.nativeBusy {
		s.turnMu.Unlock()
		return ErrBusy
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.turnMu.Unlock()
		return ErrClosed
	}
	acpID := s.acpID
	s.mu.Unlock()
	s.working = true
	s.turnSeq++
	seq := s.turnSeq
	s.turnStart = time.Now()
	s.warned = false
	s.mu.Lock()
	s.lastEvent = s.turnStart
	s.compacting = false
	s.turnProgress = false
	s.turnError = false
	// No tool can be running when a turn begins; an entry left by an earlier turn
	// or by a replayed history must not make Stop think a command is running.
	s.toolStart = map[string]time.Time{}
	s.mu.Unlock()
	startedAt := s.turnStart
	s.turnMu.Unlock()
	watchDone := make(chan struct{})
	watchStopped := make(chan struct{})
	// The request may outlive an idle turn only while the agent keeps talking:
	// once the watchdog gives up, an adapter that ignores session/cancel gets a
	// bounded grace period instead of the whole MaxTurn.
	requestCtx, cancel := context.WithTimeout(ctx, s.opts.maxTurn()+30*time.Second)
	defer cancel()
	// Join the watchdog before the next turn can start: a recovery already
	// reading the log must not report against, or cancel, that next turn.
	defer func() {
		close(watchDone)
		<-watchStopped
		s.turnMu.Lock()
		s.working = false
		s.mu.Lock()
		s.compacting = false
		s.mu.Unlock()
		s.turnMu.Unlock()
	}()
	var nativePromptDone func(error)
	if s.native != nil && s.native.ready.Load() {
		nativePromptDone = s.native.expectUser(text)
	}
	s.emit(Event{Kind: "status", State: "working"})
	go func() {
		defer close(watchStopped)
		interval := time.Second
		if s.opts.stillWorkingAfter() < interval/2 {
			interval = s.opts.stillWorkingAfter() / 2
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		nextRecover := time.Now().Add(s.opts.stillWorkingAfter())
		for {
			select {
			case <-watchDone:
				return
			case <-ticker.C:
				select {
				case <-watchDone:
					return
				default:
				}
				s.turnMu.Lock()
				if !s.working {
					s.turnMu.Unlock()
					return
				}
				elapsed := time.Since(s.turnStart)
				quietNow := time.Now()
				warnNow, recoverNow := false, false
				s.mu.Lock()
				quiet := quietNow.Sub(s.lastEvent)
				// ACP tools need not stream output while they run. Their silence
				// is not a stalled provider; the hard deadline still bounds them.
				waiting := len(s.pending) > 0 || len(s.toolStart) > 0
				progress := s.turnProgress
				s.mu.Unlock()
				if !waiting && quiet >= s.opts.stillWorkingAfter() {
					if !s.warned {
						s.warned = true
						warnNow = true
					}
					if !progress && !quietNow.Before(nextRecover) {
						nextRecover = quietNow.Add(s.opts.stillWorkingAfter())
						recoverNow = true
					}
				}
				timedOut := elapsed >= s.opts.maxTurn() || (!waiting && quiet >= s.opts.turnTimeout())
				s.turnMu.Unlock()
				if recoverNow {
					if message := s.recoverLive(acpID, startedAt.UnixMilli()); message != "" {
						select {
						case <-watchDone:
							return
						default:
						}
						if s.reportTurnError(seq, message) {
							return
						}
					}
				}
				if warnNow {
					s.emit(Event{Kind: "notice", Text: "Still working: the provider may be retrying."})
				}
				if timedOut {
					s.mu.Lock()
					progress := s.turnProgress
					s.mu.Unlock()
					if !progress {
						if message := s.recoverLive(acpID, startedAt.UnixMilli()); message != "" {
							select {
							case <-watchDone:
								return
							default:
							}
							if s.reportTurnError(seq, message) {
								return
							}
						}
					}
					s.emit(Event{Kind: "notice", Text: "The turn took too long, so " + s.opts.Agent.displayName() + " was stopped.", Bad: true})
					_ = s.Cancel()
					time.AfterFunc(cancelGrace, cancel)
					return
				}
			}
		}
	}()
	prompt := make([]map[string]string, 0, 1+len(images))
	if text != "" {
		prompt = append(prompt, map[string]string{"type": "text", "text": text})
	}
	for _, image := range images {
		prompt = append(prompt, map[string]string{
			"type": "image", "mimeType": image.MIME,
			"data": base64.StdEncoding.EncodeToString(image.Data),
		})
	}
	raw, err := s.request(requestCtx, "session/prompt", map[string]any{
		"sessionId": acpID,
		"prompt":    prompt,
	})
	if nativePromptDone != nil {
		nativePromptDone(err)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Cancel already reported the stopped turn; no request was sent.
			return nil
		}
		s.mu.Lock()
		reported := s.turnError
		s.mu.Unlock()
		if reported {
			return nil
		}
		if errors.Is(err, ErrTimeout) {
			_ = s.Cancel()
			s.emit(Event{Kind: "status", State: "cancelled", Detail: "The turn timed out."})
			return ErrTimeout
		}
		if login := loginError(s.opts.Agent, err); login != nil {
			s.emit(Event{Kind: "status", State: "error", Detail: login.Error(), Code: "login"})
			return login
		}
		s.emit(Event{Kind: "status", State: "error", Detail: safeRPCText(err.Error())})
		return err
	}
	var result struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(raw, &result)
	s.mu.Lock()
	reported := s.turnError
	s.mu.Unlock()
	if reported {
		return nil
	}
	switch result.StopReason {
	case "cancelled":
		s.emit(Event{Kind: "status", State: "cancelled"})
	case "end_turn", "":
		s.mu.Lock()
		progress := s.turnProgress
		s.mu.Unlock()
		if !progress && s.opts.Agent.RecoverError != nil {
			if message := s.opts.Agent.RecoverError(s.recoverRoot(), acpID, startedAt.UnixMilli()); message != "" {
				s.emit(Event{Kind: "status", State: "error", Detail: safeRPCText(message)})
				return nil
			}
		}
		s.emit(Event{Kind: "status", State: "idle"})
	default:
		s.emit(Event{Kind: "status", State: "idle", Detail: result.StopReason})
	}
	return nil
}

func safeRPCText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = cutUTF8(s, 200)
	}
	if strings.ContainsRune(s, '\x00') {
		return "The agent reported an error."
	}
	return s
}

func cutUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	for max > 0 && !utf8.RuneStart(s[max]) {
		max--
	}
	return s[:max]
}

func (s *Session) killGroup() {
	if s.cmd != nil && s.cmd.Process != nil {
		// The leader may already be gone while an approved command's child
		// survives; kill the whole process group, not just the leader.
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// retireGroup kills the process group and waits, up to timeout, until no live
// member remains. Leader exit alone is not retirement (SC2 probe: a group member
// outlived its leader after SIGTERM). It returns false if the group is still
// alive, so a caller must not start a replacement as if the old agent were gone.
func retireGroup(pgid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		// Check before signalling: a group seen gone may have had its id
		// reused by an unrelated group since, so a retry must not kill it.
		if !groupAlive(pgid) {
			return true
		}
		killGroup(pgid)
		if !groupAlive(pgid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// GroupAlive reports whether any live (non-zombie) process remains in the
// process group pgid.
func GroupAlive(pgid int) bool { return groupAlive(pgid) }

// killGroup sends SIGKILL to the whole process group; a variable so a test can
// see whether the escalation was needed.
var killGroup = func(pgid int) { _ = syscall.Kill(-pgid, syscall.SIGKILL) }

// TerminateGroup stops a process group this package did not start (a terminal
// TUI): SIGTERM first, so the program can flush, waiting up to termTimeout; then
// SIGKILL, waiting up to killTimeout. It returns true once no live member
// remains, and false if the group is still alive, so a caller must not start a
// replacement on the same OpenCode session. Call it only for a group that was
// seen alive moments ago, so the group id cannot have been reused.
func TerminateGroup(pgid int, termTimeout, killTimeout time.Duration) bool {
	if !groupAlive(pgid) {
		return true
	}
	_ = syscall.Kill(-pgid, syscall.SIGTERM)
	if waitGroupGone(pgid, termTimeout) {
		return true
	}
	killGroup(pgid)
	return waitGroupGone(pgid, killTimeout)
}

func waitGroupGone(pgid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !groupAlive(pgid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// GroupRetired reports whether Close confirmed that every process the agent
// started is gone. It is false before Close and when retirement timed out.
func (s *Session) GroupRetired() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retired
}

func (s *Session) ended() {
	s.doneOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		for _, call := range s.calls {
			call.reply <- rpcResult{err: &rpcError{Message: "the agent exited"}}
		}
		s.calls = map[int64]rpcCall{}
		pending := make([]string, 0, len(s.pending))
		for id, p := range s.pending {
			if p.timer != nil {
				p.timer.Stop()
			}
			pending = append(pending, id)
		}
		s.pending = map[string]*pendingPermission{}
		s.mu.Unlock()
		questionIDs := s.dropQuestionsAfterExit()
		// This last state is delivered after closed is set: the connector still
		// needs to close the person's cards and say the chat ended.
		for _, id := range questionIDs {
			s.emitFinal(Event{Kind: "question_cancelled", QuestionID: id, Reason: "ended"})
		}
		for _, id := range pending {
			s.emitFinal(Event{Kind: "permission_cancelled", PermissionID: id, Reason: "ended"})
		}
		s.emitFinal(Event{Kind: "status", State: "ended"})
		s.killGroup()
		close(s.done)
		_ = s.stdin.Close()
	})
}

func (s *Session) emitFinal(event Event) {
	if s.opts.OnEvent != nil {
		s.opts.OnEvent(event)
	}
}

// Close ends the process group. SIGTERM is enough for 1.19.5; SIGKILL is the
// backstop so a stuck provider or shell cannot outlive the chat.
func (s *Session) Close() {
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		retired := true
		if s.cmd != nil && s.cmd.Process != nil {
			retired = retireGroup(s.cmd.Process.Pid, 2*time.Second)
		}
		s.mu.Lock()
		s.retired = retired
		s.mu.Unlock()
		return
	}
	_ = s.sendCancel()
	s.mu.Lock()
	pending := make([]string, 0, len(s.pending))
	for id := range s.pending {
		pending = append(pending, id)
	}
	s.mu.Unlock()
	for _, id := range pending {
		_ = s.resolve(id, "cancelled", "closed")
	}
	s.cancelQuestions("closed")
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
	// Wait may return when the leader exits even if a child it started is
	// still alive; the group kill is the backstop either way, and Close
	// records whether the group was confirmed gone.
	retired := true
	if s.cmd != nil && s.cmd.Process != nil {
		retired = retireGroup(s.cmd.Process.Pid, 2*time.Second)
	}
	s.mu.Lock()
	s.retired = retired
	s.mu.Unlock()
}

// SetModel changes the model without replacing the ACP session or its history.
func (s *Session) SetModel(ctx context.Context, model string) error {
	return s.setModel(ctx, model, false)
}

// setModel is SetModel. At startup a switch that is not confirmed fails the whole start, so nothing is announced: the
// person must not see an idle chat, or a "model unknown" notice, for a chat that never opened.
func (s *Session) setModel(ctx context.Context, model string, startup bool) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.working || s.nativeBusy {
		return ErrBusy
	}
	s.mu.Lock()
	id := s.acpID
	s.mu.Unlock()
	if strings.TrimSpace(model) == "" {
		return ErrModel
	}
	raw, err := s.request(ctx, "session/set_config_option", map[string]string{"sessionId": id, "configId": "model", "value": model})
	if err != nil {
		var refusal *rpcError
		// Invalid parameters and an unsupported method are explicit protocol
		// refusals. An internal error may come after applying the change.
		unsupported := errors.As(err, &refusal) && (refusal.Code == -32602 || refusal.Code == -32601)
		if unsupported {
			return err
		}
		s.unconfirmedModel(startup)
		return fmt.Errorf("%w: %w", ErrModelUnconfirmed, err)
	}
	s.updateSettings(raw)
	applied, confirmed := acknowledgedModel(raw, model, s.opts.Agent.AllowOmittedModelAck)
	if !confirmed {
		s.unconfirmedModel(startup)
		return ErrModelUnconfirmed
	}
	s.mu.Lock()
	s.model = applied
	s.mu.Unlock()
	// A live chat shows the model it is now on even when that is not the one asked for.
	// Startup announces readiness only after every launch option is confirmed: a later
	// thinking-level refusal must not leave an idle status for a chat that never opened.
	if !startup {
		s.emit(Event{Kind: "status", State: "idle", Detail: applied})
	}
	if applied != model {
		return ErrModel
	}
	return nil
}

func (s *Session) unconfirmedModel(startup bool) {
	s.mu.Lock()
	s.model = ""
	s.mu.Unlock()
	if startup {
		return
	}
	// The connector retains status for reconnects. Retire the old model detail
	// as well as the process value, so replay cannot present it as current.
	s.emit(Event{Kind: "status", State: "idle"})
	s.emit(Event{Kind: "notice", Text: s.opts.Agent.displayName() + " did not confirm the model change. The current model is unknown."})
}

// An omitted list is distinct from a replacement with no model. Only the
// OpenCode profile permits omission; Claude's observed currentValue is required.
func acknowledgedModel(raw json.RawMessage, requested string, allowOmitted bool) (string, bool) {
	var result map[string]json.RawMessage
	if json.Unmarshal(raw, &result) != nil || result == nil {
		return "", false
	}
	options, present := result["configOptions"]
	if !present {
		return requested, allowOmitted
	}
	var entries []struct {
		ID           string          `json:"id"`
		CurrentValue json.RawMessage `json:"currentValue"`
	}
	if json.Unmarshal(options, &entries) != nil {
		return "", false
	}
	model, found := "", false
	for _, option := range entries {
		if option.ID != "model" {
			continue
		}
		invalid := found || json.Unmarshal(option.CurrentValue, &model) != nil ||
			!terminalproto.ValidModel(model)
		if invalid {
			return "", false
		}
		found = true
	}
	return model, found
}

// sessionID0 is the id session requests use during start: the new session's own,
// or the one being resumed.
func sessionID0(created, resume string) string {
	if created != "" {
		return created
	}
	return resume
}

// switchMode asks the adapter to change the session's mode and returns the mode
// its reply reports. An empty result means the reply did not say, which callers
// treat as not switched.
func (s *Session) switchMode(ctx context.Context, id, mode string) (string, error) {
	raw, err := s.request(ctx, "session/set_config_option", map[string]string{"sessionId": id, "configId": "mode", "value": mode})
	if err != nil {
		return "", err
	}
	var result struct {
		ConfigOptions []configOption `json:"configOptions"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return "", nil
	}
	for _, option := range result.ConfigOptions {
		if option.ID == "mode" {
			return string(option.CurrentValue), nil
		}
	}
	return "", nil
}

// Mode is the mode the chat is in, as last confirmed by the agent.
func (s *Session) Mode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// modeRestoreTimeout bounds the one attempt to put a chat back in its previous
// mode after a switch went unanswered.
var modeRestoreTimeout = 3 * time.Second

// SetMode moves a live chat to one of the profile's ChoosableModes. It is
// refused while a turn is working, and reported as applied only when the
// agent's reply names the requested mode, the same rule as the model switch.
//
// A switch the agent may have applied but did not confirm (no answer in time, or
// an answer that does not name the mode) leaves the mode unknown: the session
// then tries once to put the chat back in the mode it was in, and reports that
// only if the agent confirms it. Otherwise it returns ErrModeUnconfirmed and
// Mode is empty, so nothing claims the old mode survived.
func (s *Session) SetMode(ctx context.Context, mode string) error {
	allowed := false
	for _, m := range s.opts.Agent.ChoosableModes {
		if m == mode {
			allowed = true
		}
	}
	if !allowed {
		return ErrMode
	}
	s.turnMu.Lock()
	defer s.turnMu.Unlock()
	if s.working || s.nativeBusy {
		return ErrBusy
	}
	s.mu.Lock()
	id, previous := s.acpID, s.mode
	s.mu.Unlock()
	applied, err := s.switchMode(ctx, id, mode)
	if err != nil && !errors.Is(err, ErrTimeout) {
		// The agent answered with a refusal, or the request never left: unchanged.
		return err
	}
	if err == nil && applied != "" {
		s.mu.Lock()
		s.mode = applied
		s.mu.Unlock()
		if applied != mode {
			return ErrMode
		}
		return nil
	}
	// Unanswered, or answered without naming a mode: the agent may have switched.
	s.mu.Lock()
	s.mode = ""
	s.mu.Unlock()
	if previous != "" && previous != mode {
		restoreCtx, cancel := context.WithTimeout(context.Background(), modeRestoreTimeout)
		defer cancel()
		if back, rerr := s.switchMode(restoreCtx, id, previous); rerr == nil && back == previous {
			s.mu.Lock()
			s.mode = previous
			s.mu.Unlock()
			return ErrMode
		}
	}
	return ErrModeUnconfirmed
}
