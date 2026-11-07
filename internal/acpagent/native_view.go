//go:build linux || darwin

package acpagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// NativeTerminalSpec is connector-local. A terminal UI is a disposable client
// of this agent's authenticated loopback service, never a second agent worker.
// None of these fields is serialised or accepted from a controller.
type NativeTerminalSpec struct {
	Binary, Dir string
	Args, Env   []string
	Done        <-chan struct{}
}

type nativeOwnPrompt struct {
	text string
	at   int64
}
type nativeView struct {
	address, password, binary, dir string
	env                            []string
	client                         *http.Client
	ctx                            context.Context
	cancel                         context.CancelFunc
	ready                          atomic.Bool
	mu                             sync.Mutex
	next                           uint64
	own                            map[uint64]nativeOwnPrompt
	// Only the observer goroutine touches the bounded native-message ledger.
	seen              map[string]bool
	order             []string
	permissions       map[string]string
	permissionReplies map[string]string
}

// The first adapter is deliberately pinned to the native service/TUI contract
// exercised by the runtime probe. A newer ACP adapter can still work as chat
// without being advertised as a verified native terminal.
const nativeOpenCodeVersion = "1.19.5"

func prepareNativeView(opts Options, version, binary, password string) (*nativeView, []string) {
	if !opts.NativeTerminal || opts.Agent.Key != "opencode" || version != nativeOpenCodeVersion {
		return nil, nil
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, nil
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	v := &nativeView{address: "http://127.0.0.1:" + strconv.Itoa(port), password: password, binary: binary, dir: opts.Dir,
		env: agentEnvironment(opts.Agent, opts.Home, opts.BaseEnv, LaunchEnv{Model: opts.Model, Password: password}), ctx: ctx, cancel: cancel,
		own: map[uint64]nativeOwnPrompt{}, seen: map[string]bool{}, permissions: map[string]string{}, permissionReplies: map[string]string{},
		client: &http.Client{Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	// Global user configuration must not turn this private service into a LAN listener.
	return v, []string{"--hostname", "127.0.0.1", "--port", strconv.Itoa(port), "--mdns=false"}
}

func (v *nativeView) request(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.address+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth("opencode", v.password)
	return v.client.Do(req)
}
func (v *nativeView) json(path string, out any) error {
	ctx, cancel := context.WithTimeout(v.ctx, 3*time.Second)
	defer cancel()
	response, err := v.request(ctx, path)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("native session unavailable")
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(out)
}

func (s *Session) startNativeView() {
	v := s.native
	if v == nil {
		return
	}
	// A startup refusal degrades only the optional view, never the existing chat.
	var health struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if v.json("/global/health", &health) != nil || !health.Healthy || health.Version != nativeOpenCodeVersion {
		v.cancel()
		return
	}
	if err := v.seed(s.SessionID()); err != nil {
		v.cancel()
		return
	}
	response, err := v.request(v.ctx, "/event")
	if err != nil {
		v.cancel()
		return
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		v.cancel()
		return
	}
	v.ready.Store(true)
	go func() { <-s.Done(); v.ready.Store(false); v.cancel(); v.client.CloseIdleConnections() }()
	go v.observe(s, response)
}

func (s *Session) NativeTerminal() (NativeTerminalSpec, bool) {
	v := s.native
	if v == nil || !v.ready.Load() {
		return NativeTerminalSpec{}, false
	}
	select {
	case <-s.Done():
		return NativeTerminalSpec{}, false
	default:
	}
	return NativeTerminalSpec{Binary: v.binary, Dir: v.dir, Args: []string{"attach", v.address, "--session", s.SessionID(), "--dir", v.dir},
		Env: append(append([]string{}, v.env...), "TERM=xterm-256color", "COLORTERM=truecolor"), Done: s.Done()}, true
}

func (v *nativeView) remember(id string) bool {
	if id == "" || v.seen[id] {
		return false
	}
	v.seen[id] = true
	v.order = append(v.order, id)
	if len(v.order) > 512 {
		delete(v.seen, v.order[0])
		v.order = v.order[1:]
	}
	return true
}

type nativeMessage struct {
	Info struct {
		ID, Role, SessionID string
		Time                struct{ Created int64 }
	} `json:"info"`
	Parts []struct {
		Type, Text         string
		Synthetic, Ignored bool
	} `json:"parts"`
}

func (v *nativeView) seed(session string) error {
	var messages []nativeMessage
	if err := v.json("/session/"+url.PathEscape(session)+"/message?limit=128", &messages); err != nil {
		return err
	}
	for _, message := range messages {
		if message.Info.Role == "user" {
			v.remember(message.Info.ID)
		}
	}
	return nil
}
func (v *nativeView) expectUser(text string) func(error) {
	v.mu.Lock()
	v.next++
	id := v.next
	v.own[id] = nativeOwnPrompt{text: text, at: time.Now().UnixMilli()}
	v.mu.Unlock()
	return func(err error) {
		if err != nil {
			v.mu.Lock()
			delete(v.own, id)
			v.mu.Unlock()
		}
	}
}
func (v *nativeView) user(s *Session, id string) {
	if id == "" || v.seen[id] {
		return
	}
	var message nativeMessage
	if v.json("/session/"+url.PathEscape(s.SessionID())+"/message/"+url.PathEscape(id), &message) != nil || message.Info.Role != "user" || message.Info.SessionID != s.SessionID() {
		return
	}
	var texts []string
	for _, part := range message.Parts {
		if part.Type == "text" && !part.Synthetic && !part.Ignored {
			texts = append(texts, part.Text)
		}
	}
	text := strings.Join(texts, "\n")
	if text == "" || !v.remember(id) {
		return
	}
	v.mu.Lock()
	own := false
	for key, prompt := range v.own {
		if message.Info.Time.Created >= prompt.at-1000 && prompt.text == text {
			own = true
			delete(v.own, key)
			break
		}
	}
	// A failed/disconnected native event stream must not grow this ledger without bound.
	if len(v.own) > 128 {
		for key := range v.own {
			delete(v.own, key)
			if len(v.own) <= 64 {
				break
			}
		}
	}
	v.mu.Unlock()
	if !own {
		s.emit(Event{Kind: "user", Text: cutUTF8(text, 8<<10)})
	}
}

func (v *nativeView) observe(s *Session, initial *http.Response) {
	for v.ctx.Err() == nil {
		v.consume(s, initial)
		initial = nil
		if v.ctx.Err() != nil {
			return
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-v.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		// The event service is volatile. Re-read bounded native records after a gap.
		var messages []nativeMessage
		if v.json("/session/"+url.PathEscape(s.SessionID())+"/message?limit=128", &messages) == nil {
			for _, message := range messages {
				if message.Info.Role == "user" {
					v.user(s, message.Info.ID)
				}
			}
		}
	}
}
func (v *nativeView) consume(s *Session, response *http.Response) {
	if response == nil {
		var err error
		response, err = v.request(v.ctx, "/event")
		if err != nil {
			return
		}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), 256<<10)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event struct {
			Type       string          `json:"type"`
			Properties json.RawMessage `json:"properties"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) != nil {
			continue
		}
		var p struct {
			SessionID string `json:"sessionID"`
			Status    struct {
				Type string `json:"type"`
			} `json:"status"`
			Info                 struct{ ID, Role, SessionID string }  `json:"info"`
			Part                 struct{ MessageID, SessionID string } `json:"part"`
			ID, RequestID, Reply string
			Tool                 struct {
				CallID string `json:"callID"`
			} `json:"tool"`
		}
		if json.Unmarshal(event.Properties, &p) != nil {
			continue
		}
		switch event.Type {
		case "session.status":
			if p.SessionID == s.SessionID() {
				s.nativeStatus(p.Status.Type != "idle")
			}
		case "message.updated":
			if p.Info.SessionID == s.SessionID() && p.Info.Role == "user" {
				v.user(s, p.Info.ID)
			}
		case "message.part.updated":
			if p.Part.SessionID == s.SessionID() {
				v.user(s, p.Part.MessageID)
			}
		case "permission.asked":
			if p.SessionID == s.SessionID() && p.ID != "" && p.Tool.CallID != "" {
				v.mu.Lock()
				if len(v.permissions) < 128 {
					v.permissions[p.ID] = p.Tool.CallID
				}
				v.mu.Unlock()
			}
		case "permission.replied":
			if p.SessionID == s.SessionID() {
				v.mu.Lock()
				tool := v.permissions[p.RequestID]
				if tool == "" {
					tool = p.RequestID
				}
				delete(v.permissions, p.RequestID)
				if tool != "" && len(v.permissionReplies) < 128 {
					v.permissionReplies[tool] = p.Reply
				}
				v.mu.Unlock()
				s.nativePermissionReply(tool, p.Reply)
			}
		}
	}
}
func (s *Session) nativeStatus(busy bool) {
	s.turnMu.Lock()
	changed := s.nativeBusy != busy
	own := s.working
	if changed {
		s.nativeCancelled = false
		if busy {
			s.nativeEpoch++
		}
	}
	s.nativeBusy = busy
	s.turnMu.Unlock()
	if !changed || own {
		return
	}
	state := "idle"
	if busy {
		state = "working"
	}
	s.emit(Event{Kind: "status", State: state})
}

// A terminal can answer the same permission as its chat card. Retire the card
// when native evidence says the request was answered; never infer consent from output.
func (s *Session) nativePermissionReply(request, decision string) {
	s.mu.Lock()
	var ids []string
	for id, pending := range s.pending {
		if pending.nativeID == request && request != "" {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		choice := "reject"
		if decision == "once" || decision == "always" {
			choice = "once"
		}
		_ = s.resolve(id, choice, "")
	}
}

// Stop has the same bounded recovery when a turn was started in the native UI.
// The epoch prevents a delayed stop from terminating a later turn.
func (s *Session) armNativeStop(epoch uint64) {
	go func() {
		timer := time.NewTimer(s.opts.forceStopGrace())
		defer timer.Stop()
		select {
		case <-s.Done():
			return
		case <-timer.C:
		}
		s.turnMu.Lock()
		stuck := s.nativeBusy && s.nativeEpoch == epoch && s.nativeCancelled
		s.turnMu.Unlock()
		if stuck {
			s.emit(Event{Kind: "notice", Bad: true, Text: "The agent did not confirm stopping, so this chat was ended."})
			s.Close()
		}
	}()
}
