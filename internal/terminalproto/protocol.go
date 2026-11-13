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
	Version              = 1
	MaxControlBytes      = 64 << 10
	MaxBinaryBytes       = 32 << 10
	MaxQueuedOutputBytes = 256 << 10
	MaxTitleRunes        = 80
	MaxHistoryLines      = 2000
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

// ValidPlainText applies the protocol's plain-text rule (valid UTF-8, bounded
// rune count, no control, format or line/paragraph separator characters).
func ValidPlainText(s string, max int) bool { return plainText(s, max) }
func plainText(s string, max int) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, c := range s {
		isControl := unicode.IsControl(c) || unicode.Is(unicode.Cf, c)
		isSeparator := c == '\u2028' || c == '\u2029'
		if isControl || isSeparator {
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

// Closed reports whether the session has stopped: it runs no more work and a
// worktree it used can be released.
func (s SessionState) Closed() bool { return s == Ended || s == Exited || s == Lost }

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

// Capabilities is what a connector reports. BrowseOff is reported, instead
// of Browse, by a connector that negotiated browse-v1 but has folder browse
// switched off locally, so a controller can tell that from an older
// connector. It is sent only on a connection that negotiated browse-v1: an
// older controller's strict decoder would refuse the field. Clipboard is sent
// only on a connection that negotiated clipboard-v1, for the same reason.
// Uploads and UploadsOff follow the same rule for upload-v1.
type Capabilities struct {
	Launch bool `json:"launch,omitempty"`
	// Resume is sent only on a connection that negotiated resume-v1: an older
	// controller's strict decoder would refuse the field.
	Resume bool `json:"resume,omitempty"`
	// Yolo is sent only on a connection that negotiated agents-v2, and only
	// when the machine accepts YOLO sessions locally.
	Yolo           bool     `json:"yolo,omitempty"`
	AgentsV2       bool     `json:"agentsV2,omitempty"`
	OSVersion      string   `json:"osVersion,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	CommandHistory bool     `json:"commandHistory,omitempty"`
	Worktree       bool     `json:"worktree,omitempty"`
	Browse         bool     `json:"browse,omitempty"`
	Branches       bool     `json:"branches,omitempty"`
	// BranchPrune is sent only on a connection that negotiated
	// branch-prune-v1: the machine can plan and delete merged branches.
	BranchPrune bool `json:"branchPrune,omitempty"`
	// BranchSync is sent only on a connection that negotiated
	// branch-sync-v1: the machine can fast-forward a repository's base branch.
	BranchSync bool `json:"branchSync,omitempty"`
	BrowseOff  bool `json:"browseOff,omitempty"`
	Clipboard  bool `json:"clipboard,omitempty"`
	Uploads    bool `json:"uploads,omitempty"`
	UploadsOff bool `json:"uploadsOff,omitempty"`
	Models     bool `json:"models,omitempty"`
	ModelsOff  bool `json:"modelsOff,omitempty"`
	// ModelsRefresh is sent only on a connection that negotiated
	// models-refresh-v1: a refresh may then ask for a listing that ignores the
	// connector's short cache.
	ModelsRefresh bool `json:"modelsRefresh,omitempty"`
	// ACP is sent only on a connection that negotiated acp-v1.
	ACP bool `json:"acp,omitempty"`
	// ACPAgents is present only after acp-agents-v1 negotiation.
	ACPAgents *[]ACPAgent `json:"acpAgents,omitempty"`
	// ACPWorktree requires acp-worktree-v1 in addition to ACP and worktrees.
	ACPWorktree bool `json:"acpWorktree,omitempty"`
	// ACPHistory is sent only when acp-history-v1 was negotiated as well.
	ACPHistory      bool `json:"acpHistory,omitempty"`
	ACPModel        bool `json:"acpModel,omitempty"`
	ACPLiveControls bool `json:"acpLiveControls,omitempty"`
	ACPPreviews     bool `json:"acpPreviews,omitempty"`
	ACPStartOptions bool `json:"acpStartOptions,omitempty"`
	// ACPProvider is set on a connection that negotiated acp-provider-v1 and
	// live controls: the connector may report a provider setting and take a
	// provider on a structured Cline create.
	ACPProvider bool `json:"acpProvider,omitempty"`
	// ACPAgentMode is set on a connection that negotiated acp-agent-mode-v1.
	ACPAgentMode bool `json:"acpAgentMode,omitempty"`
	// ACPAgentOptions is set on a connection that negotiated
	// acp-agent-options-v1 and an available descriptor has options to describe.
	ACPAgentOptions bool `json:"acpAgentOptions,omitempty"`
	// ACPAgentReattach is set on a connection that negotiated
	// acp-agent-reattach-v1 and an available descriptor claims in-place
	// reattach. ChatReattach stays the machine-wide chat-reattach-v1 flag.
	ACPAgentReattach bool `json:"acpAgentReattach,omitempty"`
	// ACPImages is sent only when acp-image-v1 was negotiated as well.
	ACPImages bool `json:"acpImages,omitempty"`
	// ACPApproval is sent only when acp-approval-v1 was negotiated; a live
	// chat can then switch between asking and YOLO.
	ACPApproval bool `json:"acpApproval,omitempty"`
	// SessionMode is sent only when session-mode-v1 was negotiated; a session
	// can then switch between the OpenCode terminal and the chat.
	SessionMode bool `json:"sessionMode,omitempty"`
	// NativeView is negotiated separately from the process-replacing session mode operation.
	NativeView bool `json:"nativeView,omitempty"`
	// ChatReattach is sent only when chat-reattach-v1 was negotiated; a lost
	// chat can then be reconnected in place. OpenCode is allowed by the
	// original rule; another agent also needs acp-agent-reattach-v1 and its
	// descriptor's Reattach.
	ChatReattach bool `json:"chatReattach,omitempty"`
	// ACPQuestions is sent only when acp-question-v1 was negotiated; typed agent
	// questions can then be delivered and answered. No agent offers them yet.
	ACPQuestions bool `json:"acpQuestions,omitempty"`
	// ShellDir is sent only when shell-dir-v1 was negotiated; a plain shell
	// may then start in a chosen folder.
	ShellDir bool `json:"shellDir,omitempty"`
	Inspect  bool `json:"inspect,omitempty"`
	// Attention is sent only when attention-v1 was negotiated.
	Attention       bool `json:"attention,omitempty"`
	TerminalControl bool `json:"terminalControl,omitempty"`
	// Software is sent only when software-v1 was negotiated; it observes
	// installed agent software and never asks for a change.
	Software []SoftwareEntry `json:"software,omitempty"`
	// SoftwareApply is sent only when software-apply-v1 was negotiated and
	// terminal access is on; the machine may then run one reviewed artifact.
	SoftwareApply   bool      `json:"softwareApply,omitempty"`
	ProtocolVersion int       `json:"protocolVersion"`
	OS              string    `json:"os"`
	OSUser          string    `json:"osUser"`
	TerminalEnabled bool      `json:"terminalEnabled"`
	HealthEnabled   bool      `json:"healthEnabled"`
	Readiness       Readiness `json:"readiness"`
}

func (c Capabilities) Validate() error {
	var features []string
	if c.ACPPreviews {
		features = append(features, ACPPreviewsFeature)
	}
	if c.ACPStartOptions {
		features = append(features, ACPStartOptionsFeature)
	}
	if c.ACPProvider {
		features = append(features, ACPProviderFeature)
	}
	if c.ACPLiveControls {
		features = append(features, ACPLiveControlsFeature)
	}
	if c.AgentsV2 {
		features = append(features, AgentsFeature)
	}
	if c.ModelsRefresh {
		features = append(features, ModelsRefreshFeature)
	}
	for _, tool := range c.Tools {
		if tool == "antigravity" {
			features = append(features, AntigravityFeature)
			break
		}
	}
	if c.ACPAgents != nil {
		features = append(features, ACPAgentsFeature)
		for _, entry := range *c.ACPAgents {
			if entry.Tool == "antigravity" {
				features = append(features, AntigravityACPFeature)
				break
			}
		}
	}
	if c.ACPAgentReattach {
		features = append(features, ACPAgentReattachFeature)
	}
	if c.ACPWorktree {
		features = append(features, ACPWorktreeFeature)
	}
	return c.ValidateFor(strings.Join(features, ","))
}

// ValidateFor validates capabilities against the features negotiated by the peer.
func (c Capabilities) ValidateFor(features string) error {
	if c.ACPWorktree && !(c.ACP && c.Worktree && c.Launch && SupportsACPWorktree(features)) {
		return ErrInvalid
	}
	if c.ACPAgents != nil {
		for _, entry := range *c.ACPAgents {
			if entry.Tool == "antigravity" && !SupportsAntigravityACP(features) {
				return ErrInvalid
			}
		}
	}
	validPreviews := c.ACP && SupportsACPPreviews(features)
	if c.ACPPreviews && !validPreviews {
		return ErrInvalid
	}
	validStartOptions := SupportsACPStartOptions(features) && c.ACPLiveControls
	if c.ACPStartOptions && !validStartOptions {
		return ErrInvalid
	}
	validProvider := SupportsACPProvider(features) && c.ACPLiveControls
	if c.ACPProvider && !validProvider {
		return ErrInvalid
	}
	if c.ACPLiveControls && !SupportsACPLiveControls(features) {
		return ErrInvalid
	}
	unsupported := c.ACPAgents != nil && !SupportsACPAgents(features)
	if unsupported || !c.validACPAgents() {
		return ErrInvalid
	}
	if c.ACPAgentReattach && !(SupportsACPAgentReattach(features) && c.ACPAgents != nil) {
		return ErrInvalid
	}
	needsAgents := c.AgentsV2 || c.OSVersion != ""
	validOS := plainText(c.OSVersion, 128)
	agentSupport := !needsAgents || SupportsAgents(features)
	if !agentSupport || !validOS {
		return ErrInvalid
	}
	validLaunch := !c.Launch || c.TerminalControl
	toolsNeedLaunch := len(c.Tools) == 0 || c.Launch
	validToolList := validTools(c.Tools, SupportsAgents(features))
	for _, tool := range c.Tools {
		if tool == "antigravity" && !SupportsAntigravity(features) {
			validToolList = false
		}
	}
	toolsValid := validLaunch && toolsNeedLaunch && validToolList
	if !toolsValid {
		return ErrInvalid
	}
	yoloValid := !c.Yolo || (c.Launch && c.AgentsV2 && SupportsAgents(features))
	if !yoloValid {
		return ErrInvalid
	}
	if c.Resume && !c.Launch {
		return ErrInvalid
	}
	terminalRule := c.CommandHistory && !c.TerminalControl
	worktreeRule := c.Worktree && !c.Launch
	if terminalRule || worktreeRule {
		return ErrInvalid
	}
	needsTerminal := c.Browse || c.BrowseOff || c.Clipboard
	browseConflict := c.BrowseOff && c.Browse
	browseTerminalRule := needsTerminal && !c.TerminalControl
	branchWorktreeRule := c.Branches && !c.Worktree
	browseValid := !browseTerminalRule && !branchWorktreeRule && !browseConflict
	if !browseValid {
		return ErrInvalid
	}
	uploadRule := c.Uploads && !c.TerminalControl
	uploadOffRule := c.UploadsOff && (c.Uploads || !c.TerminalControl)
	if uploadRule || uploadOffRule {
		return ErrInvalid
	}
	modelRule := c.Models && !c.TerminalControl
	modelOffRule := c.ModelsOff && (c.Models || !c.TerminalControl)
	modelRefreshRule := c.ModelsRefresh && (!c.Models || !SupportsModelsRefresh(features))
	if modelRule || modelOffRule || modelRefreshRule {
		return ErrInvalid
	}
	acpExtras := c.ACPHistory || c.ACPImages || c.ACPApproval || c.ACPQuestions ||
		c.ACPModel || c.ACPLiveControls || c.ACPAgentMode || c.ACPAgentOptions || c.ACPAgentReattach
	needsTerminalForACP := c.ACP || c.Inspect || c.ShellDir || c.Attention
	acpRule := acpExtras && !c.ACP
	acpTerminalRule := needsTerminalForACP && !c.TerminalControl
	if acpRule || acpTerminalRule {
		return ErrInvalid
	}
	// A switch launches the tool in a terminal (launch-v1, which needs
	// terminal control). Every connector that reports session mode has
	// launch-v1 on: its chat tools are among its launch tools.
	if c.NativeView && !(c.ACP && c.TerminalControl && c.Launch) {
		return ErrInvalid
	}
	if c.SessionMode && !(c.ACP && c.ACPHistory && c.Launch) {
		return ErrInvalid
	}
	if c.ChatReattach && !(c.ACP && c.ACPHistory) {
		return ErrInvalid
	}
	validSoftware := c.TerminalControl && ValidSoftware(c.Software)
	if len(c.Software) > 0 && !validSoftware {
		return ErrInvalid
	}
	if c.SoftwareApply && !c.TerminalControl {
		return ErrInvalid
	}
	validVersion := c.ProtocolVersion >= 1 && c.ProtocolVersion <= 65535
	validUser := plainText(c.OSUser, 128) && strings.TrimSpace(c.OSUser) != ""
	if !validVersion || !validUser {
		return ErrInvalid
	}
	knownOS := c.OS == "linux" || c.OS == "darwin" || c.OS == "windows" || c.OS == "unknown"
	if !knownOS {
		return ErrInvalid
	}
	switch c.Readiness {
	case DisabledLocally, Unsupported, TmuxUnavailable, SupervisorUnavailable, CapacityReached, Reconnecting, Ready:
	default:
		return ErrInvalid
	}
	notExplained := c.Readiness != DisabledLocally && c.Readiness != Unsupported
	if !c.TerminalEnabled && notExplained {
		return ErrInvalid
	}
	ready := c.Readiness == Ready
	readyVersion := c.ProtocolVersion == Version
	readyOS := c.OS == "linux" || c.OS == "darwin"
	readyValid := readyVersion && readyOS
	if ready && !readyValid {
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

func (Hello) kind() string { return "hello" }
func (h Hello) valid() bool {
	return ValidID(h.AgentInstanceID) && h.Capabilities.Validate() == nil
}

type SessionSnapshot struct {
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	State      SessionState `json:"state"`
	ExitCode   *int         `json:"exitCode"`
	ActivityAt int64        `json:"activityAt,omitempty"`
	BellAt     int64        `json:"bellAt,omitempty"`
	ExitStatus *int         `json:"exitStatus,omitempty"`
	// ACPSessionID is set only for a structured chat, so the controller can
	// reopen it after a connection loss.
	ACPSessionID string `json:"acpSessionId,omitempty"`
}

func validExit(state SessionState, code *int) bool {
	return code == nil || state == Exited && *code >= 0 && *code <= 255
}

func validSnapshotAttention(s SessionSnapshot) bool {
	validTimes := validAttentionTime(s.ActivityAt) && validAttentionTime(s.BellAt)
	if !validTimes || !validExit(s.State, s.ExitStatus) {
		return false
	}
	return s.ExitCode == nil || s.ExitStatus == nil || *s.ExitCode == *s.ExitStatus
}

type Reconcile struct {
	Complete bool              `json:"complete"`
	Sessions []SessionSnapshot `json:"sessions"`
}

func (Reconcile) kind() string { return "reconcile" }
func (r Reconcile) valid() bool {
	partial := !r.Complete && len(r.Sessions) != 0
	if len(r.Sessions) > 32 || partial {
		return false
	}
	seen := map[string]bool{}
	for _, s := range r.Sessions {
		validSession := ValidID(s.ID) && ValidTitle(s.Title) && s.State.Valid() &&
			validExit(s.State, s.ExitCode) && validSnapshotAttention(s)
		validACP := s.ACPSessionID == "" || ValidACPHandle(s.ACPSessionID)
		valid := validSession && validACP && !seen[s.ID]
		if !valid {
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
	// Launch fields stay plain strings so Create remains comparable.
	Tool   string `json:"tool,omitempty"`
	Model  string `json:"model,omitempty"`
	Dir    string `json:"dir,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	// Worktree is a run token; the connector makes the agent's own git worktree.
	Worktree string `json:"worktree,omitempty"`
	// Project is the Project's slug (see Slug), required with Worktree and
	// refused without it. It names the worktree folder and branch on the
	// machine: ~/.reet/worktrees/<project>/<repo>/<worktree>, reet/<project>-<worktree>.
	Project string `json:"project,omitempty"`
	// ProjectID is the controller's folder project the session belongs to.
	// It is the controller's own record: never encoded, so never sent to a
	// machine nor stored with the create's parameters.
	ProjectID string `json:"-"`
	// TitleOrigin is who owns Title (see TitleOrigin). Like ProjectID it is
	// the controller's own record: never encoded.
	TitleOrigin TitleOrigin `json:"-"`
	// LaunchSession opens the tool's terminal on that native session id (for
	// OpenCode, `--session`). Never encoded: the connector builds the Create
	// locally. Incompatible with Resume and Structured.
	LaunchSession string `json:"-"`
	Base          string `json:"base,omitempty"`
	Effort        string `json:"effort,omitempty"`
	// Provider is the billing source a structured Cline chat starts on (a
	// provider id Cline reports). Empty keeps Cline's own choice. It is only
	// sent to a connector that negotiated acp-provider-v1.
	Provider string `json:"provider,omitempty"`
	Yolo     bool   `json:"yolo,omitempty"`
	// Resume continues the agent's last conversation in Dir. It is offered
	// only for a closed session, and only on a machine that negotiated
	// resume-v1 and whose preset has a reviewed resume command.
	Resume bool `json:"resume,omitempty"`
	// Structured opens an OpenCode chat over ACP instead of a terminal. It is
	// sent only on a connection that negotiated acp-v1; the machine never
	// substitutes a terminal for it.
	Structured bool `json:"structured,omitempty"`
	// Trust confirms that the person accepted the repository plugin warning.
	// It is meaningful only with Structured.
	Trust bool `json:"trust,omitempty"`
	// ResumeACP reopens a structured OpenCode session from its own session id.
	// It is meaningful only with Structured; the controller creates a new row.
	ResumeACP string `json:"resumeAcp,omitempty"`
}

func (Create) kind() string  { return "create" }
func (c Create) valid() bool { return c.Validate() == nil }
func (c Create) Validate() error {
	validIdentity := ValidTitle(c.Title) && ValidDimensions(c.Cols, c.Rows) &&
		c.validLaunch() && c.validWorktree()
	validBase := c.Base == "" || (c.Worktree != "" && validRefName(c.Base))
	if !validIdentity || !validBase {
		return ErrInvalid
	}
	structuredExtras := c.Resume || c.ResumeACP != "" && (c.Worktree != "" || c.Project != "" || c.Base != "")
	validChat := ReviewedChatTool(c.Tool) && !structuredExtras
	if c.Structured && !validChat {
		return ErrInvalid
	}
	if c.Trust && !c.Structured {
		return ErrInvalid
	}
	validSession := ValidACPSessionIDFor(c.Tool, c.LaunchSession)
	validLaunchSession := validSession && !c.Resume && !c.Structured
	if c.LaunchSession != "" && !validLaunchSession {
		return ErrInvalid
	}
	validResume := c.Structured && ValidACPSessionIDFor(c.Tool, c.ResumeACP)
	if c.ResumeACP != "" && !validResume {
		return ErrInvalid
	}
	return nil
}

type Attach struct {
	NativeView   bool   `json:"nativeView,omitempty"`
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
	// LaunchFailed means the requested agent could not be started.
	LaunchFailed            ResultCode = "launch_failed"
	LaunchOptionUnsupported ResultCode = "launch_option_unsupported"
	YoloRefused             ResultCode = "yolo_refused"
	// UploadsOff refuses an upload before any stream is opened because the
	// machine's local upload switch or terminal consent is off.
	UploadsOff ResultCode = "uploads_off"
	// UploadBusy refuses an upload before any stream is opened because two
	// transfers are already in flight on this connection.
	UploadBusy ResultCode = "upload_busy"
	// ACPUnavailable means the machine did not negotiate acp-v1 or has no
	// supported OpenCode to start.
	ACPUnavailable ResultCode = "acp_unavailable"
	// ACPVersion means OpenCode is missing or older than the verified floor.
	ACPVersion ResultCode = "acp_version"
	// ACPTrustRequired means the folder has .opencode plugins and the start
	// did not carry the person's trust confirmation.
	ACPTrustRequired ResultCode = "acp_trust_required"
	// QuestionInvalid means an answer failed the machine's validation. The
	// question is still waiting, so the person can correct and resubmit. Detail
	// says which field and why. It is sent only to a controller that negotiated
	// acp-question-v1.
	QuestionInvalid ResultCode = "question_invalid"
)

// ConflictStarting is the one detail a conflict result may carry: the session
// is still starting (a create, a restore or another reattach), so the request
// may succeed once it settles. A conflict without it means another session
// owns what was asked for.
const ConflictStarting = "starting"

func (c ResultCode) Valid() bool {
	return c == OK || c == Unavailable || c == Denied || c == Capacity || c == NotFound || c == Failed || c == Conflict || c == LaunchFailed || c == LaunchOptionUnsupported || c == YoloRefused || c == WorktreeChanged || c == WorktreeRefused || c == UploadsOff || c == UploadBusy || c == ACPUnavailable || c == ACPVersion || c == ACPTrustRequired || c == QuestionInvalid
}

type Result struct {
	Code ResultCode `json:"code"`
	// Reason and Detail explain a worktree_refused answer; they are never set
	// on any other code, so every other result encodes exactly as before.
	Reason WorktreeReason `json:"reason,omitempty"`
	Detail string         `json:"detail,omitempty"`
}

func (Result) kind() string { return "result" }
func (r Result) valid() bool {
	if !r.Code.Valid() {
		return false
	}
	invalidReason := r.Code != WorktreeRefused || !r.Reason.Valid()
	if r.Reason != "" && invalidReason {
		return false
	}
	if r.Detail != "" {
		switch r.Code {
		case WorktreeRefused:
			return r.Reason != "" && validWorktreeDetail(r.Detail)
		case ACPUnavailable, ACPVersion, ACPTrustRequired:
			return validChatText(r.Detail, 500)
		case QuestionInvalid:
			return validChatText(r.Detail, 200)
		case Conflict:
			return r.Detail == ConflictStarting
		default:
			return false
		}
	}
	return true
}

type SessionChanged struct {
	State      SessionState `json:"state"`
	ExitCode   *int         `json:"exitCode"`
	ActivityAt int64        `json:"activityAt,omitempty"`
	BellAt     int64        `json:"bellAt,omitempty"`
	ExitStatus *int         `json:"exitStatus,omitempty"`
	// WorktreePath and WorktreeRepo say where a worktree create made the
	// worktree: its folder and the repository name status and remove need.
	// They are set together, only in answer to a worktree create (which only
	// a controller that negotiated worktree-v1 sends), so every other session
	// change encodes exactly as before.
	WorktreePath string `json:"worktreePath,omitempty"`
	WorktreeRepo string `json:"worktreeRepo,omitempty"`
	// ACPSessionID is set only for a structured chat.
	ACPSessionID string `json:"acpSessionId,omitempty"`
}

func (SessionChanged) kind() string { return "session_changed" }
func (s SessionChanged) valid() bool {
	paired := (s.WorktreePath == "") == (s.WorktreeRepo == "")
	validPair := s.WorktreePath == "" || (validDir(s.WorktreePath) && validRepoName(s.WorktreeRepo))
	if !paired || !validPair {
		return false
	}
	validTimes := validAttentionTime(s.ActivityAt) && validAttentionTime(s.BellAt)
	if !validTimes || !validExit(s.State, s.ExitStatus) {
		return false
	}
	bothSet := s.ExitCode != nil && s.ExitStatus != nil
	if bothSet && *s.ExitCode != *s.ExitStatus {
		return false
	}
	return s.State.Valid() && validExit(s.State, s.ExitCode) && (s.ACPSessionID == "" || ValidACPHandle(s.ACPSessionID))
}

func (m Message) Validate() error {
	if m.Body == nil {
		return ErrInvalid
	}
	pointerNil := reflect.ValueOf(m.Body).Kind() == reflect.Pointer && reflect.ValueOf(m.Body).IsNil()
	if pointerNil {
		return ErrInvalid
	}
	validEnvelope := m.Version == Version && ValidID(m.RequestID) && ValidCounter(m.Generation)
	if !validEnvelope || !m.Body.valid() {
		return ErrInvalid
	}
	kind := m.Body.kind()
	session := kind == "native_view" || kind == "native_view_result" || kind == "create" || kind == "attach" || kind == "resize" || kind == "renew_attachment" || kind == "detach" || kind == "end" || kind == "history" || kind == "command_history" || kind == "session_changed" || kind == "upload" || kind == "acp_prompt" || kind == "acp_cancel" || kind == "acp_permission" || kind == "acp_question_answer" || kind == "acp_mode" || kind == "acp_agent_mode" || kind == "session_mode" || kind == "chat_reattach" || kind == "acp_model" || kind == "acp_event" || kind == "acp_control" || kind == "acp_control_result" || kind == "acp_preview" || kind == "acp_preview_result"
	attachment := kind == "attach" || kind == "resize" || kind == "renew_attachment" || kind == "detach" || kind == "history" || kind == "command_history" || kind == "upload" || kind == "result" && m.AttachmentID != ""
	requiresSession := session && !ValidID(m.SessionID)
	straySession := !session && m.SessionID != "" && (kind != "result" || !ValidID(m.SessionID))
	if requiresSession || straySession {
		return ErrInvalid
	}
	if attachment {
		valid := ValidID(m.SessionID) && ValidID(m.AttachmentID) && ValidCounter(m.AttachmentGeneration)
		if !valid {
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
	validData := len(data) > 0 && len(data) <= MaxControlBytes && utf8.Valid(data)
	if !validData || !uniqueKeys(data) {
		return Message{}, ErrInvalid
	}
	var e envelope
	validJSON := strictJSON(data, &e) == nil
	nonEmpty := len(e.Body) > 0 && !bytes.Equal(bytes.TrimSpace(e.Body), []byte("null"))
	if !validJSON || !nonEmpty {
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
	case "worktree_status":
		b = &WorktreeStatus{}
	case "worktree_remove":
		b = &WorktreeRemove{}
	case "worktree_state":
		b = &WorktreeState{}
	case "worktree_result":
		b = &WorktreeResult{}
	case "inspect":
		b = &Inspect{}
	case "inspect_result":
		b = &InspectResult{}
	case "browse":
		b = &Browse{}
	case "browse_result":
		b = &BrowseResult{}
	case "branches":
		b = &Branches{}
	case "branches_result":
		b = &BranchesResult{}
	case "branch_prune_plan":
		b = &BranchPrunePlan{}
	case "branch_prune_plan_result":
		b = &BranchPrunePlanResult{}
	case "branch_prune":
		b = &BranchPrune{}
	case "branch_prune_result":
		b = &BranchPruneResult{}
	case "branch_sync":
		b = &BranchSync{}
	case "branch_sync_result":
		b = &BranchSyncResult{}
	case "upload":
		b = &Upload{}
	case "models":
		b = &Models{}
	case "models_result":
		b = &ModelsResult{}
	case "software_apply":
		b = &SoftwareApply{}
	case "software_apply_result":
		b = &SoftwareApplyResult{}
	case "result":
		b = &Result{}
	case "session_changed":
		b = &SessionChanged{}
	case "launch_progress":
		b = &LaunchProgress{}
	case "acp_prompt":
		b = &ACPPrompt{}
	case "acp_preview":
		b = &ACPPreview{}
	case "acp_preview_result":
		b = &ACPPreviewResult{}
	case "acp_control":
		b = &ACPControl{}
	case "acp_control_result":
		b = &ACPControlResult{}
	case "acp_model":
		b = &ACPModel{}
	case "acp_cancel":
		b = &ACPCancel{}
	case "acp_permission":
		b = &ACPPermission{}
	case "acp_mode":
		b = &ACPMode{}
	case "acp_agent_mode":
		b = &ACPAgentMode{}
	case "native_view":
		b = &NativeView{}
	case "native_view_result":
		b = &NativeViewResult{}
	case "session_mode":
		b = &SessionMode{}
	case "chat_reattach":
		b = &ChatReattach{}
	case "acp_question_answer":
		b = &ACPQuestionAnswer{}
	case "acp_event":
		b = &ACPEvent{}
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
