// Package terminalproto defines the private agent protocol. It grants no
// authorization and carries no terminal bytes in JSON; data uses binary frames.
package terminalproto

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	Version               = 1
	MaxControlBytes       = 16 << 10
	MaxBinaryBytes        = 32 << 10
	MaxQueuedOutputBytes  = 256 << 10
	MaxTitleRunes         = 80
	MaxSessionsPerAgent   = 8
	MaxSessionsPerAccount = 32
	MaxHistoryLines       = 2000
	// Counters also cross JSON/JavaScript boundaries without loss of precision.
	MaxCounter int64 = 1<<53 - 1
)

var ErrInvalid = errors.New("invalid private agent message")

func ValidID(s string) bool {
	if len(s) < 16 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func ValidTitle(s string) bool { return plainText(s, MaxTitleRunes) && strings.TrimSpace(s) != "" }
func plainText(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '\u2028' || c == '\u2029' {
			return false
		}
	}
	return true
}
func ValidCounter(n int64) bool { return n > 0 && n <= MaxCounter }
func ValidDimensions(cols, rows int) bool {
	return cols >= 2 && cols <= 500 && rows >= 1 && rows <= 300
}
func ValidateBinary(data []byte) error {
	if len(data) == 0 || len(data) > MaxBinaryBytes {
		return ErrInvalid
	}
	return nil
}

type SessionState string

const (
	Creating     SessionState = "creating"
	Running      SessionState = "running"
	EndRequested SessionState = "end_requested"
	Ended        SessionState = "ended"
	Exited       SessionState = "exited"
	Lost         SessionState = "lost"
)

func (s SessionState) Valid() bool {
	return s == Creating || s == Running || s == EndRequested || s == Ended || s == Exited || s == Lost
}
func (s SessionState) ReservesQuota() bool { return s == Creating || s == Running || s == EndRequested }

// CanTransition includes idempotent observations. Lost is also the result of a
// failed/uncertain creation reconciled as absent; it must never recreate a shell.
func CanTransition(from, to SessionState) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case Creating:
		return to == Running || to == EndRequested || to == Lost || to == Exited
	case Running:
		return to == EndRequested || to == Exited || to == Lost
	case EndRequested:
		return to == Ended || to == Exited || to == Lost
	}
	return false
}

type Readiness string

const (
	DisabledLocally       Readiness = "disabled_locally"
	Unsupported           Readiness = "unsupported"
	TmuxUnavailable       Readiness = "tmux_unavailable"
	SupervisorUnavailable Readiness = "supervisor_unavailable"
	CapacityReached       Readiness = "capacity_reached"
	Reconnecting          Readiness = "reconnecting"
	Ready                 Readiness = "ready"
)

type Capabilities struct {
	CommandHistory  bool      `json:"commandHistory,omitempty"`
	TerminalControl bool      `json:"terminalControl,omitempty"`
	ProtocolVersion int       `json:"protocolVersion"`
	OS              string    `json:"os"`
	OSUser          string    `json:"osUser"`
	TerminalEnabled bool      `json:"terminalEnabled"`
	HealthEnabled   bool      `json:"healthEnabled"`
	Readiness       Readiness `json:"readiness"`
}

func (c Capabilities) Validate() error {
	if c.CommandHistory && !c.TerminalControl {
		return ErrInvalid
	}
	if c.ProtocolVersion < 1 || c.ProtocolVersion > 65535 || !plainText(c.OSUser, 128) || strings.TrimSpace(c.OSUser) == "" {
		return ErrInvalid
	}
	if c.OS != "linux" && c.OS != "darwin" && c.OS != "windows" && c.OS != "unknown" {
		return ErrInvalid
	}
	switch c.Readiness {
	case DisabledLocally, Unsupported, TmuxUnavailable, SupervisorUnavailable, CapacityReached, Reconnecting, Ready:
	default:
		return ErrInvalid
	}
	if !c.TerminalEnabled && c.Readiness != DisabledLocally && c.Readiness != Unsupported {
		return ErrInvalid
	}
	if c.Readiness == Ready && (c.ProtocolVersion != Version || c.OS != "linux" && c.OS != "darwin") {
		return ErrInvalid
	}
	return nil
}

// Body is a closed set so unknown control operations cannot be marshalled.
type Body interface {
	kind() string
	valid() bool
}
type Message struct {
	Version              int
	RequestID            string
	SessionID            string
	AttachmentID         string
	Generation           int64 // connection generation, not the lifetime of a session
	AttachmentGeneration int64 // fences previous writers after explicit takeover
	Body                 Body
}
type envelope struct {
	Version              int             `json:"version"`
	Type                 string          `json:"type"`
	RequestID            string          `json:"requestId"`
	SessionID            string          `json:"sessionId,omitempty"`
	AttachmentID         string          `json:"attachmentId,omitempty"`
	Generation           int64           `json:"generation"`
	AttachmentGeneration int64           `json:"attachmentGeneration,omitempty"`
	Body                 json.RawMessage `json:"body"`
}

type Hello struct {
	AgentInstanceID string       `json:"agentInstanceId"`
	Capabilities    Capabilities `json:"capabilities"`
}

func (Hello) kind() string  { return "hello" }
func (h Hello) valid() bool { return ValidID(h.AgentInstanceID) && h.Capabilities.Validate() == nil }

type SessionSnapshot struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	State    SessionState `json:"state"`
	ExitCode *int         `json:"exitCode"`
}

func validExit(state SessionState, code *int) bool {
	return code == nil || state == Exited && *code >= 0 && *code <= 255
}

type Reconcile struct {
	Complete bool              `json:"complete"`
	Sessions []SessionSnapshot `json:"sessions"`
}

func (Reconcile) kind() string { return "reconcile" }
func (r Reconcile) valid() bool {
	if len(r.Sessions) > 32 || !r.Complete && len(r.Sessions) != 0 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range r.Sessions {
		if !ValidID(s.ID) || !ValidTitle(s.Title) || !s.State.Valid() || !validExit(s.State, s.ExitCode) || seen[s.ID] {
			return false
		}
		seen[s.ID] = true
	}
	return true
}

type Create struct {
	Title string `json:"title"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

func (Create) kind() string  { return "create" }
func (c Create) valid() bool { return c.Validate() == nil }
func (c Create) Validate() error {
	if !ValidTitle(c.Title) || !ValidDimensions(c.Cols, c.Rows) {
		return ErrInvalid
	}
	return nil
}

type Attach struct {
	StreamToken  string `json:"streamToken,omitempty"`
	Cols         int    `json:"cols"`
	Rows         int    `json:"rows"`
	Takeover     bool   `json:"takeover"`
	LeaseSeconds int    `json:"leaseSeconds"`
}

func (Attach) kind() string { return "attach" }
func (a Attach) valid() bool {
	return (a.StreamToken == "" || ValidID(a.StreamToken)) && ValidDimensions(a.Cols, a.Rows) && a.LeaseSeconds >= 1 && a.LeaseSeconds <= 60
}

type Resize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

func (Resize) kind() string  { return "resize" }
func (r Resize) valid() bool { return ValidDimensions(r.Cols, r.Rows) }

// RenewAttachment is sent only after the controller revalidates authorization.
// It renews an existing attachment; it must never create or take over a writer.
type RenewAttachment struct {
	LeaseSeconds int `json:"leaseSeconds"`
}

func (RenewAttachment) kind() string  { return "renew_attachment" }
func (r RenewAttachment) valid() bool { return r.LeaseSeconds >= 1 && r.LeaseSeconds <= 60 }

type Detach struct{}

func (Detach) kind() string { return "detach" }
func (Detach) valid() bool  { return true }

type End struct{}

func (End) kind() string { return "end" }
func (End) valid() bool  { return true }

type History struct {
	StreamToken string `json:"streamToken,omitempty"`
	Lines       int    `json:"lines"`
}

func (History) kind() string { return "history" }
func (h History) valid() bool {
	return (h.StreamToken == "" || ValidID(h.StreamToken)) && h.Lines >= 1 && h.Lines <= MaxHistoryLines
}

type Probe struct {
	Sequence int64  `json:"sequence"`
	Nonce    string `json:"nonce"`
}

func (Probe) kind() string  { return "probe" }
func (p Probe) valid() bool { return ValidCounter(p.Sequence) && ValidID(p.Nonce) }

type ProbeResult Probe

func (ProbeResult) kind() string  { return "probe_result" }
func (p ProbeResult) valid() bool { return Probe(p).valid() }

type ResultCode string

const (
	OK          ResultCode = "ok"
	Unavailable ResultCode = "unavailable"
	Denied      ResultCode = "denied"
	Capacity    ResultCode = "capacity"
	NotFound    ResultCode = "not_found"
	Failed      ResultCode = "failed"
	Conflict    ResultCode = "conflict"
)

func (c ResultCode) Valid() bool {
	return c == OK || c == Unavailable || c == Denied || c == Capacity || c == NotFound || c == Failed || c == Conflict
}

type Result struct {
	Code ResultCode `json:"code"`
}

func (Result) kind() string  { return "result" }
func (r Result) valid() bool { return r.Code.Valid() }

type SessionChanged struct {
	State    SessionState `json:"state"`
	ExitCode *int         `json:"exitCode"`
}

func (SessionChanged) kind() string  { return "session_changed" }
func (s SessionChanged) valid() bool { return s.State.Valid() && validExit(s.State, s.ExitCode) }

func (m Message) Validate() error {
	if m.Body == nil || reflect.ValueOf(m.Body).Kind() == reflect.Pointer && reflect.ValueOf(m.Body).IsNil() {
		return ErrInvalid
	}
	if m.Version != Version || !ValidID(m.RequestID) || !ValidCounter(m.Generation) || !m.Body.valid() {
		return ErrInvalid
	}
	kind := m.Body.kind()
	session := kind == "create" || kind == "attach" || kind == "resize" || kind == "renew_attachment" || kind == "detach" || kind == "end" || kind == "history" || kind == "command_history" || kind == "session_changed"
	attachment := kind == "attach" || kind == "resize" || kind == "renew_attachment" || kind == "detach" || kind == "history" || kind == "command_history" || kind == "result" && m.AttachmentID != ""
	if session && !ValidID(m.SessionID) || !session && m.SessionID != "" && (kind != "result" || !ValidID(m.SessionID)) {
		return ErrInvalid
	}
	if attachment {
		if !ValidID(m.SessionID) || !ValidID(m.AttachmentID) || !ValidCounter(m.AttachmentGeneration) {
			return ErrInvalid
		}
	} else if m.AttachmentID != "" || m.AttachmentGeneration != 0 {
		return ErrInvalid
	}
	return nil
}

func Encode(m Message) ([]byte, error) {
	if m.Validate() != nil {
		return nil, ErrInvalid
	}
	body, err := json.Marshal(m.Body)
	if err != nil {
		return nil, ErrInvalid
	}
	b, err := json.Marshal(envelope{m.Version, m.Body.kind(), m.RequestID, m.SessionID, m.AttachmentID, m.Generation, m.AttachmentGeneration, body})
	if err != nil || len(b) > MaxControlBytes {
		return nil, ErrInvalid
	}
	return b, nil
}
func Decode(data []byte) (Message, error) {
	if len(data) == 0 || len(data) > MaxControlBytes || !utf8.Valid(data) || !uniqueKeys(data) {
		return Message{}, ErrInvalid
	}
	var e envelope
	if strictJSON(data, &e) != nil || len(e.Body) == 0 || bytes.Equal(bytes.TrimSpace(e.Body), []byte("null")) {
		return Message{}, ErrInvalid
	}
	var b Body
	switch e.Type {
	case "hello":
		b = &Hello{}
	case "reconcile":
		b = &Reconcile{}
	case "create":
		b = &Create{}
	case "attach":
		b = &Attach{}
	case "resize":
		b = &Resize{}
	case "renew_attachment":
		b = &RenewAttachment{}
	case "detach":
		b = &Detach{}
	case "end":
		b = &End{}
	case "history":
		b = &History{}
	case "command_history":
		b = &CommandHistory{}
	case "probe":
		b = &Probe{}
	case "probe_result":
		b = &ProbeResult{}
	case "health_report":
		b = &HealthReport{}
	case "result":
		b = &Result{}
	case "session_changed":
		b = &SessionChanged{}
	default:
		return Message{}, ErrInvalid
	}
	if strictJSON(e.Body, b) != nil {
		return Message{}, ErrInvalid
	}
	m := Message{e.Version, e.RequestID, e.SessionID, e.AttachmentID, e.Generation, e.AttachmentGeneration, b}
	if m.Validate() != nil {
		return Message{}, ErrInvalid
	}
	return m, nil
}
func strictJSON(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return ErrInvalid
	}
	if d.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

// encoding/json otherwise accepts duplicate fields (including case aliases).
// Bound nesting as well as bytes before interpreting any control operation.
func uniqueKeys(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 32 {
			return false
		}
		t, err := d.Token()
		if err != nil {
			return false
		}
		switch t {
		case json.Delim('{'):
			keys := map[string]bool{}
			for d.More() {
				k, err := d.Token()
				if err != nil {
					return false
				}
				s, ok := k.(string)
				if !ok {
					return false
				}
				s = strings.ToLower(s)
				if keys[s] {
					return false
				}
				keys[s] = true
				if !value(depth + 1) {
					return false
				}
			}
			t, err = d.Token()
			return err == nil && t == json.Delim('}')
		case json.Delim('['):
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			t, err = d.Token()
			return err == nil && t == json.Delim(']')
		default:
			_, delim := t.(json.Delim)
			return !delim
		}
	}
	if !value(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
