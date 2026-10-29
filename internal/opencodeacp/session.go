//go:build linux || darwin

package opencodeacp

import (
	"bufio"
	"context"
	"crypto/rand"
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
)

var (
	ErrBusy         = errors.New("the chat is already working")
	ErrClosed       = errors.New("the chat is closed")
	ErrNoPermission = errors.New("that approval is no longer waiting")
	ErrExited       = errors.New("OpenCode exited")
	ErrTimeout      = errors.New("the chat turn timed out")
	ErrModel        = errors.New("OpenCode did not use the chosen model")
)

const maxLineBytes = 1 << 20

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

type rpcResult struct {
	result json.RawMessage
	err    *rpcError
}

type pendingPermission struct {
	rpcID    json.RawMessage
	allowID  string
	rejectID string
	timer    *time.Timer
}

// Session is one OpenCode ACP process. Methods are safe for concurrent use;
// Prompt runs one turn at a time and Cancel may be called from any goroutine.
type Session struct {
	opts   Options
	binary string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr *cappedBuffer

	mu       sync.Mutex
	writeMu  sync.Mutex
	nextID   int64
	calls    map[int64]chan rpcResult
	pending  map[string]*pendingPermission
	closed   bool
	acpID    string
	model    string
	tools    map[string]string
	done     chan struct{}
	doneOnce sync.Once

	turnMu    sync.Mutex
	working   bool
	turnStart time.Time
	lastEvent time.Time
	warned    bool
}

func Start(ctx context.Context, opts Options) (*Session, error) {
	if opts.Dir == "" || !filepath.IsAbs(opts.Dir) {
		return nil, errors.New("the chat folder must be absolute")
	}
	if info, err := os.Stat(opts.Dir); err != nil || !info.IsDir() {
		return nil, errors.New("the chat folder is unavailable")
	}
	if opts.Home == "" || !filepath.IsAbs(opts.Home) {
		return nil, errors.New("the chat home is unavailable")
	}
	binary := opts.Binary
	if binary == "" {
		found, err := exec.LookPath("opencode")
		if err != nil {
			return nil, fmt.Errorf("%w: OpenCode is not installed", ErrVersion)
		}
		binary = found
	}
	plugins, err := Plugins(opts.Dir)
	if err != nil {
		return nil, err
	}
	if len(plugins) > 0 && !opts.Trust {
		return nil, &TrustError{Plugins: plugins}
	}
	if _, err := ProbeVersion(ctx, binary, opts.Dir, opts.Home, opts.BaseEnv); err != nil {
		return nil, err
	}
	password, err := serverPassword(opts.Random)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(binary, "acp")
	cmd.Dir = opts.Dir
	cmd.Env = chatEnvironment(opts.Home, opts.BaseEnv, password, opts.Model)
	cmd.SysProcAttr = procAttr()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr := &cappedBuffer{limit: 4 << 10}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, ErrExited
	}
	s := &Session{opts: opts, binary: binary, cmd: cmd, stdin: stdin, stdout: stdout, stderr: stderr,
		calls: map[int64]chan rpcResult{}, pending: map[string]*pendingPermission{}, tools: map[string]string{}, done: make(chan struct{})}
	go s.readLoop()
	go func() {
		_ = cmd.Wait()
		s.ended()
	}()
	initCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var initResult struct {
		AgentInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"agentInfo"`
	}
	raw, err := s.request(initCtx, "initialize", map[string]any{
		"protocolVersion": 1,
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
	if json.Unmarshal(raw, &initResult) != nil || initResult.AgentInfo.Version != Version {
		s.Close()
		return nil, fmt.Errorf("%w (found %s)", ErrVersion, safeVersion(initResult.AgentInfo.Version))
	}
	newCtx, newCancel := context.WithTimeout(ctx, 45*time.Second)
	defer newCancel()
	opened := opts.Resume == ""
	method, params := "session/new", map[string]any{"cwd": opts.Dir, "mcpServers": []any{}}
	if !opened {
		method = "session/load"
		params = map[string]any{"sessionId": opts.Resume, "cwd": opts.Dir, "mcpServers": []any{}}
	}
	raw, err = s.request(newCtx, method, params)
	if err != nil {
		s.Close()
		return nil, ErrExited
	}
	var created struct {
		SessionID     string `json:"sessionId"`
		ConfigOptions []struct {
			ID           string `json:"id"`
			Category     string `json:"category"`
			CurrentValue string `json:"currentValue"`
		} `json:"configOptions"`
	}
	if json.Unmarshal(raw, &created) != nil || opened && created.SessionID == "" {
		s.Close()
		return nil, ErrExited
	}
	mode, model := "", ""
	for _, option := range created.ConfigOptions {
		switch option.ID {
		case "mode":
			mode = option.CurrentValue
		case "model":
			model = option.CurrentValue
		}
	}
	// No mode switching is offered: a custom primary agent must never become
	// the default behind the approval config.
	if mode != "build" {
		s.Close()
		return nil, errors.New("OpenCode is not in build mode; the chat was not started")
	}
	if opts.Model != "" && model != "" && model != opts.Model {
		s.Close()
		return nil, ErrModel
	}
	sessionID := created.SessionID
	if !opened {
		sessionID = opts.Resume
	}
	s.mu.Lock()
	s.acpID = sessionID
	s.model = model
	s.mu.Unlock()
	if !opened {
		s.markInterrupted()
	}
	s.emit(Event{Kind: "status", State: "idle", Detail: model})
	return s, nil
}

// SessionID is OpenCode's own id, for diagnostics only; the controller uses
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
	return s.working
}

func (s *Session) Model() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.model
}

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
	id := s.nextRequestID()
	ch := make(chan rpcResult, 1)
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	s.calls[id] = ch
	s.mu.Unlock()
	if err := s.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		s.mu.Lock()
		delete(s.calls, id)
		s.mu.Unlock()
		return nil, err
	}
	select {
	case result := <-ch:
		if result.err != nil {
			return nil, fmt.Errorf("OpenCode: %s", safeRPCText(result.err.Message))
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
		line := append([]byte(nil), scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var message rpcMessage
		if json.Unmarshal(line, &message) != nil {
			continue
		}
		s.handle(message)
	}
}

func (s *Session) handle(message rpcMessage) {
	if message.Method != "" {
		switch message.Method {
		case "session/update":
			s.handleUpdate(message.Params)
		case "session/request_permission":
			s.handlePermission(message.ID, message.Params)
		case "fs/write_text_file":
			// OpenCode has already written the file itself. The client fs call is
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
	ch := s.calls[id]
	delete(s.calls, id)
	s.mu.Unlock()
	if ch != nil {
		ch <- rpcResult{result: message.Result, err: message.Error}
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
	case "agent_message_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.emit(Event{Kind: "agent", MessageID: u.MessageID, Text: text})
		}
	case "user_message_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.emit(Event{Kind: "user", Text: text})
		}
	case "tool_call", "tool_call_update":
		if u.ToolCallID != "" {
			s.mu.Lock()
			s.tools[u.ToolCallID] = u.Status
			s.mu.Unlock()
		}
		text := toolText(u.Content)
		locations := make([]string, 0, len(u.Locations))
		for _, location := range u.Locations {
			if location.Path != "" {
				locations = append(locations, location.Path)
			}
		}
		s.emit(Event{Kind: "tool", ToolCallID: u.ToolCallID, Title: u.Title, ToolKind: u.Kind, Status: u.Status, Text: text, Locations: locations})
	case "agent_thought_chunk":
		text := contentText(u.Content)
		if text != "" {
			s.emit(Event{Kind: "tool", ToolCallID: "thought-" + u.MessageID, Title: "Thinking", ToolKind: "think", Status: "in_progress", Text: text})
		}
	}
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
		RawInput struct {
			Command     string `json:"command"`
			Description string `json:"description"`
			Diff        string `json:"diff"`
			FilePath    string `json:"filepath"`
			ParentDir   string `json:"parentDir"`
		} `json:"rawInput"`
	} `json:"toolCall"`
	Options []struct {
		OptionID string `json:"optionId"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	} `json:"options"`
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
	diff := params.ToolCall.RawInput.Diff
	truncated := false
	if len(diff) > 10<<10 {
		diff = cutUTF8(diff, 10<<10)
		truncated = true
	}
	// A card the person cannot review must not offer Allow once. The probe
	// always sends the diff for an edit and the command for an execute; a
	// missing one is a protocol surprise, not a reason to guess.
	if params.ToolCall.Kind == "edit" && diff == "" {
		truncated = true
	}
	if params.ToolCall.Kind == "execute" && params.ToolCall.RawInput.Command == "" {
		truncated = true
	}
	path := params.ToolCall.RawInput.FilePath
	if path == "" && len(params.ToolCall.Locations) > 0 {
		path = params.ToolCall.Locations[0].Path
	}
	locations := make([]string, 0, len(params.ToolCall.Locations))
	for _, location := range params.ToolCall.Locations {
		if location.Path != "" {
			locations = append(locations, location.Path)
		}
	}
	pending := &pendingPermission{rpcID: rpcID, allowID: allowID, rejectID: rejectID}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.respond(rpcID, map[string]any{"outcome": map[string]string{"outcome": "cancelled"}}, nil)
		return
	}
	s.pending[permissionID] = pending
	s.mu.Unlock()
	pending.timer = time.AfterFunc(s.opts.permissionTimeout(), func() {
		s.resolve(permissionID, "cancelled", "timeout")
		s.emit(Event{Kind: "notice", Text: "The approval request timed out, so OpenCode was stopped.", Bad: true})
		_ = s.sendCancel()
	})
	title := params.ToolCall.Title
	if title == "" {
		title = "OpenCode tool call"
	}
	s.emit(Event{Kind: "status", State: "waiting", Detail: "Waiting for your approval."})
	s.emit(Event{Kind: "permission", PermissionID: permissionID, ToolCallID: params.ToolCall.ToolCallID,
		Title: title, ToolKind: params.ToolCall.Kind, Command: params.ToolCall.RawInput.Command,
		Path: path, Diff: diff, Locations: locations, Truncated: truncated,
		Text: params.ToolCall.RawInput.Description})
}

func permissionID() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
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

// Cancel stops the current turn and closes every card this client still holds.
// OpenCode does not withdraw an outstanding request_permission, so reet must.
func (s *Session) Cancel() error {
	s.mu.Lock()
	pending := make([]string, 0, len(s.pending))
	for id := range s.pending {
		pending = append(pending, id)
	}
	s.mu.Unlock()
	for _, id := range pending {
		_ = s.resolve(id, "cancelled", "cancelled")
	}
	s.emit(Event{Kind: "status", State: "cancelled"})
	return s.sendCancel()
}

// markInterrupted closes tools that session/load replayed as still running.
// OpenCode does not re-ask their permission, so leaving them "working" would
// be a lie; a new prompt starts cleanly after them.
func (s *Session) markInterrupted() {
	s.mu.Lock()
	ids := make([]string, 0)
	for id, status := range s.tools {
		if status == "pending" || status == "in_progress" {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.emit(Event{Kind: "tool", ToolCallID: id, Status: "failed", ToolKind: "other", Title: "Interrupted tool call",
			Text: "Interrupted before this chat was reopened."})
	}
}

// Prompt runs one turn. It blocks until the turn ends; the connector calls it
// in its own goroutine so control messages (cancel, approval) stay responsive.
func (s *Session) Prompt(ctx context.Context, text string) error {
	s.turnMu.Lock()
	if s.working {
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
	s.turnStart = time.Now()
	s.warned = false
	s.mu.Lock()
	s.lastEvent = s.turnStart
	s.mu.Unlock()
	s.turnMu.Unlock()
	defer func() {
		s.turnMu.Lock()
		s.working = false
		s.turnMu.Unlock()
	}()
	s.emit(Event{Kind: "status", State: "working"})
	watchDone := make(chan struct{})
	go func() {
		interval := time.Second
		if s.opts.stillWorkingAfter() < interval/2 {
			interval = s.opts.stillWorkingAfter() / 2
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ticker.C:
				s.turnMu.Lock()
				if !s.working {
					s.turnMu.Unlock()
					return
				}
				elapsed := time.Since(s.turnStart)
				warnNow := false
				if !s.warned {
					s.mu.Lock()
					quiet := time.Since(s.lastEvent)
					pending := len(s.pending) > 0
					s.mu.Unlock()
					if !pending && quiet >= s.opts.stillWorkingAfter() {
						s.warned = true
						warnNow = true
					}
				}
				timedOut := elapsed >= s.opts.turnTimeout()
				s.turnMu.Unlock()
				if warnNow {
					s.emit(Event{Kind: "notice", Text: "Still working: the provider may be retrying."})
				}
				if timedOut {
					s.emit(Event{Kind: "notice", Text: "The turn took too long, so OpenCode was stopped.", Bad: true})
					_ = s.Cancel()
					return
				}
			}
		}
	}()
	defer close(watchDone)
	requestCtx, cancel := context.WithTimeout(ctx, s.opts.turnTimeout()+30*time.Second)
	defer cancel()
	raw, err := s.request(requestCtx, "session/prompt", map[string]any{
		"sessionId": acpID,
		"prompt":    []map[string]string{{"type": "text", "text": text}},
	})
	if err != nil {
		if errors.Is(err, ErrTimeout) {
			_ = s.Cancel()
			s.emit(Event{Kind: "status", State: "cancelled", Detail: "The turn timed out."})
			return ErrTimeout
		}
		s.emit(Event{Kind: "status", State: "error", Detail: safeRPCText(err.Error())})
		return err
	}
	var result struct {
		StopReason string `json:"stopReason"`
	}
	_ = json.Unmarshal(raw, &result)
	switch result.StopReason {
	case "cancelled":
		s.emit(Event{Kind: "status", State: "cancelled"})
	case "end_turn", "":
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
		return "OpenCode reported an error."
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

func (s *Session) ended() {
	s.doneOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		for _, ch := range s.calls {
			ch <- rpcResult{err: &rpcError{Message: "OpenCode exited"}}
		}
		s.calls = map[int64]chan rpcResult{}
		pending := make([]string, 0, len(s.pending))
		for id, p := range s.pending {
			if p.timer != nil {
				p.timer.Stop()
			}
			pending = append(pending, id)
		}
		s.pending = map[string]*pendingPermission{}
		s.mu.Unlock()
		// This last state is delivered after closed is set: the connector still
		// needs to close the person's cards and say the chat ended.
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
		s.killGroup()
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
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
	// Wait may return when the leader exits even if a child it started is
	// still alive; the group kill is the backstop either way.
	s.killGroup()
}
