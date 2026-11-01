//go:build linux || darwin

package acpagent

import (
	"io"
	"time"
)

// Options is one chat session's connector-owned launch data. No controller
// argument beyond these fields reaches the process.
type Options struct {
	// Agent is the adapter profile from the registry.
	Agent Spec
	// Binary overrides the spec command (tests and explicit installs).
	Binary string
	Dir    string
	Home   string
	Model  string
	Trust  bool
	// Questions lets the session accept typed agent questions (elicitation/create).
	// It is off in every launch today: the form capability is not advertised to
	// the adapter, and a question that arrives anyway is cancelled.
	Questions bool
	// Resume is the agent's own session id to load instead of creating a new
	// session. The load replay is delivered through OnEvent.
	Resume string
	// BaseEnv is the parent environment the allowlist is selected from. The
	// connector passes its own environment; tests pass a controlled slice.
	BaseEnv []string
	// Random supplies the server password; nil uses crypto/rand.
	Random io.Reader
	// PermissionTimeout closes an unanswered card and cancels the turn.
	PermissionTimeout time.Duration
	// StillWorkingAfter emits one notice when the model has been silent.
	StillWorkingAfter time.Duration
	// TurnTimeout cancels a prompt that has not finished.
	TurnTimeout time.Duration
	// CancelGrace is how long an agent has to end a turn after it is asked to
	// cancel. After that the chat is ended explicitly instead of staying busy.
	CancelGrace time.Duration
	// OnEvent receives bounded structured events in order. It must not block
	// indefinitely; the connector's sender has its own deadline.
	OnEvent func(Event)
}

func (o Options) permissionTimeout() time.Duration {
	if o.PermissionTimeout > 0 {
		return o.PermissionTimeout
	}
	return 5 * time.Minute
}
func (o Options) stillWorkingAfter() time.Duration {
	if o.StillWorkingAfter > 0 {
		return o.StillWorkingAfter
	}
	return 20 * time.Second
}
func (o Options) cancelGrace() time.Duration {
	if o.CancelGrace > 0 {
		return o.CancelGrace
	}
	return 15 * time.Second
}
func (o Options) turnTimeout() time.Duration {
	if o.TurnTimeout > 0 {
		return o.TurnTimeout
	}
	return 10 * time.Minute
}

// Event is the package's event shape, kept independent of the wire protocol so
// the client can be tested without a connector. The connector maps it to a
// terminalproto.ACPEvent.
type Event struct {
	Kind         string
	MessageID    string
	ToolCallID   string
	PermissionID string
	// QuestionID and Question carry a typed question (kind "question"). The
	// connector does not forward these kinds yet.
	QuestionID string
	Question   *Question
	Title      string
	ToolKind   string
	Status     string
	State      string
	Text       string
	Command    string
	Path       string
	Diff       string
	Additions  int
	Deletions  int
	DurationMs int64
	Locations  []string
	Allow      *bool
	Bad        bool
	Truncated  bool
	Reason     string
	Detail     string
}

func boolPtr(v bool) *bool { return &v }
