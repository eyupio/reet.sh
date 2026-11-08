//go:build linux || darwin

package acpagent

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// ProtocolVersion is the ACP protocol this client speaks.
const ProtocolVersion = 1

var (
	// ErrVersion means the agent binary or the ACP handshake is outside the
	// compatibility floor for its adapter.
	ErrVersion = errors.New("the agent version is not supported")
	// ErrPluginsUnreadable means the agent's repository trust surface could
	// not be read, so the chat was refused rather than hidden.
	ErrPluginsUnreadable = errors.New("the folder's agent plugins could not be read")
	// ErrModel means the agent did not accept the session's chosen model.
	ErrModel = errors.New("the agent did not use the chosen model")
	// ErrModelUnconfirmed means a switch may have been applied, but the reply
	// did not establish the current model. It is never reported as the old one.
	ErrModelUnconfirmed = fmt.Errorf("%w: the current model is not confirmed", ErrModel)
	// ErrProvider means the agent did not use the session's chosen provider.
	ErrProvider = errors.New("the agent did not use the chosen provider")
	// ErrProviderUnconfirmed means a provider switch may have been applied, but
	// the reply did not read it back.
	ErrProviderUnconfirmed = fmt.Errorf("%w: the current provider is not confirmed", ErrProvider)
	// ErrUnsupported means the catalogue tool has no ACP adapter here.
	ErrUnsupported = errors.New("that agent does not support chats")
	// ErrCodexLogin means session creation needs a login made outside reet.
	ErrCodexLogin = errors.New("Sign in to Codex on that machine first.")
	// ErrClaudeLogin means the first prompt needs a login made outside reet:
	// Claude creates a session without one and answers at the prompt instead.
	ErrClaudeLogin = errors.New("Sign in to Claude on that machine first.")
	// ErrClineLogin means session creation needs a login made outside reet.
	ErrClineLogin = errors.New("Sign in to Cline on that machine first.")
	// ErrNoStoredConversation means session/load was asked for a conversation
	// the adapter does not have. Cline stores a chat at its first prompt, so a
	// chat that never ran one cannot be reopened.
	ErrNoStoredConversation = errors.New("the agent has no stored conversation for this chat")
	// AGY reuses the machine's selected login; reet never starts an OAuth flow.
	ErrAntigravityLogin = errors.New("Sign in to Google Antigravity on that machine first (run agy).")
	// ErrMode means the requested mode is not one this profile lets a person
	// choose, or the agent did not confirm the switch.
	ErrMode = errors.New("the agent did not switch to that mode")
	// ErrModeUnconfirmed means the agent did not answer a mode switch, or did not
	// name the mode in its answer, so which mode the chat is in is not known. It is
	// never reported as the old mode.
	ErrModeUnconfirmed = errors.New("the agent did not confirm which mode the chat is in")
)

// TrustError means the folder carries files the agent would execute. Only a
// person who trusts the repository may start a chat there.
type TrustError struct{ Files []string }

func (e *TrustError) Error() string {
	return "this folder has agent extensions: " + strings.Join(e.Files, ", ")
}

// CheckTrust is the start's trust gate on its own: a *TrustError when the
// folder carries files the agent would execute and the person did not trust
// it, ErrPluginsUnreadable when they cannot be read, and nil otherwise. A
// caller about to stop something for a chat asks it first.
func (s Spec) CheckTrust(dir string, trusted bool) error {
	if s.Trust == nil {
		return nil
	}
	files, err := s.Trust(dir)
	if err != nil {
		return err
	}
	if len(files) > 0 && !trusted {
		return &TrustError{Files: files}
	}
	return nil
}

// Spec is one ACP adapter. It is data, not a code path per agent: the generic
// client owns the handshake, turn and permission protocol, and a profile only
// describes how to launch the agent and what its compatibility floor is.
type Spec struct {
	// Key is the stable catalogue id (for example "opencode").
	Key string
	// Command is the binary name resolved on PATH; Args are its ACP arguments.
	Command string
	Args    []string
	// DisplayName is the name a person sees in messages.
	DisplayName string
	// ExpectedName is what initialize's agentInfo.name must report. Empty
	// accepts any name.
	ExpectedName string
	// MinVersion is the lowest semver accepted by both the binary's
	// --version probe and the ACP initialize response. Empty skips the
	// external probe and accepts any reported version.
	MinVersion string
	// RequireMode is the ACP session mode that must be active before a turn
	// may start. Empty accepts whatever the adapter reports.
	RequireMode string
	// StartMode is the mode a session is switched to when the adapter starts in
	// another one (Codex starts in "agent", which never asks). The switch uses
	// session/set_config_option and is accepted only when the adapter's reply
	// reports the new mode; otherwise the chat is refused. Empty never switches.
	StartMode string
	// AllowOmittedModelAck preserves OpenCode's existing acknowledgement with
	// no configOptions field. An explicit list is always authoritative; other
	// profiles must read the applied model back, as measured for Claude.
	AllowOmittedModelAck bool
	// Reattach reports that this adapter can reopen a stored conversation under
	// the same reet session id (session/load), so a lost chat is reconnected in
	// place rather than continued as a new row. It is published in the
	// descriptor only on a connection that negotiated acp-agent-reattach-v1.
	Reattach bool
	// Provider reports that this agent's provider option is a billing source reet
	// surfaces and switches (Cline). Another adapter's own option list is left alone.
	Provider bool
	// ConfirmModelBySwitch applies a chosen model with session/set_config_option
	// once the session is open, and requires the reply to report it. An adapter
	// that starts on its configured model and ignores a model at session/new
	// (Cline) would otherwise refuse every start that names one. The switch is
	// never assumed: an unconfirmed reply refuses the chat.
	ConfirmModelBySwitch bool
	// RequireOptions names config option values that must be active before a
	// turn may start, for example {"auto_approve": "false"}. A missing option or
	// another value refuses the chat rather than trusting a launch flag: Cline's
	// --auto-approve false is a request, and the session's own report is the
	// acknowledgement that no tool call can run without a card.
	RequireOptions map[string]string
	// EnvAllowlist names additional inherited variables the adapter may
	// read (provider credentials), on top of the infrastructure allowlist.
	EnvAllowlist []string
	// LoginHomeEnv routes only this agent's own configuration directory through
	// the replaced HOME. LoginHomeDir is its default subdirectory of the real
	// HOME; an explicit environment value takes precedence. Paths must be absolute.
	// Profiles using this route do not inherit XDG home directories.
	LoginHomeEnv string
	LoginHomeDir string
	// SessionMeta is sent as _meta on session/new and session/load. A profile
	// uses it for adapter options that must be fixed by reet, never by the
	// repository. Nil sends no _meta.
	SessionMeta map[string]any
	// ChoosableModes lists the modes a person may switch a live chat to. Empty
	// offers no switch. A mode outside the list is refused even if the agent has
	// it (Codex's "agent-full-access" is never offered).
	ChoosableModes []string
	// Options describes other selectable options the pinned adapter was
	// measured to offer, such as effort or model-config values. It is
	// descriptive only: it does not authorize a switch, and Modes remains the
	// permission-sensitive list. Values can depend on the current model.
	Options []terminalproto.ACPAgentOption
	// EndChatOnStopWhileToolRuns ends the whole chat when a person stops a turn
	// that has a tool still running. For an adapter whose session/cancel ends the
	// turn but leaves the command alive (Codex), the only way to be sure nothing
	// keeps running is to retire the process group, so the person is told and the
	// chat ends. A stop with no tool running only cancels the turn.
	EndChatOnStopWhileToolRuns bool
	// Trust reports executable files or configuration surfaces in dir that need
	// acknowledgement. Nil means this adapter has no repository trust surface.
	Trust func(dir string) ([]string, error)
	// ConfigEnv supplies the adapter's private environment for one session
	// (owned config, password, opt-outs). It is applied after the inherited
	// allowlist, so an inherited value can never override it.
	ConfigEnv func(LaunchEnv) map[string]string
	// RecoverError recovers a turn error the adapter dropped from ACP. It is
	// consulted only when a completed turn streamed no progress. Nil means
	// the adapter reports its own errors.
	RecoverError func(home, sessionID string, afterMS int64) string
	// RecoverLive recovers an error the adapter logged while a turn is still
	// silent and has not reported anything over ACP. It is consulted by the
	// silence watchdog, never for a turn that streamed progress.
	RecoverLive func(home, sessionID string, afterMS int64) string
}

// ClaudeSessionMeta is the measured option set for the Claude Code ACP adapter
// (SC2 probe, 31 October 2026): only the user's own settings are read, so a
// repository's .claude/settings.json and settings.local.json cannot add hooks,
// widen permissions or change the mode. Without it the adapter reads user,
// project and local settings and runs a repository's hooks with no approval card.
// It is not attached to any registered profile: Claude stays disabled.
var ClaudeSessionMeta = map[string]any{
	"claudeCode": map[string]any{"options": map[string]any{"settingSources": []string{"user"}}},
}

// sessionRequest builds the session/new or session/load call for a launch.
func sessionRequest(spec Spec, dir, resume string) (string, map[string]any) {
	method, params := "session/new", map[string]any{"cwd": dir, "mcpServers": []any{}}
	if resume != "" {
		method = "session/load"
		params["sessionId"] = resume
	}
	if spec.SessionMeta != nil {
		params["_meta"] = spec.SessionMeta
	}
	return method, params
}

// configOption is one entry of an adapter's configOptions reply. CurrentValue
// keeps the text form of a string and of Cline's boolean auto_approve option.
type configOption struct {
	ID           string      `json:"id"`
	Category     string      `json:"category"`
	CurrentValue optionValue `json:"currentValue"`
}

// checkRequiredOptions confirms a profile's pinned option values before a turn.
// A reply that omits an option, or reports another value, refuses the chat.
func (s Spec) checkRequiredOptions(options []configOption) error {
	if len(s.RequireOptions) == 0 {
		return nil
	}
	values := make(map[string]string, len(options))
	for _, option := range options {
		values[option.ID] = string(option.CurrentValue)
	}
	for id, want := range s.RequireOptions {
		if values[id] != want {
			return fmt.Errorf("%w: %s did not confirm %s=%s", ErrSafety, s.displayName(), id, want)
		}
	}
	return nil
}

func (s Spec) displayName() string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return s.Key
}

func (s Spec) minVersion() (semver, bool) {
	if s.MinVersion == "" {
		return semver{}, false
	}
	parsed, ok := parseSemver(s.MinVersion)
	return parsed, ok
}

// Lookup returns the registered adapter for a catalogue tool.
func Lookup(tool string) (Spec, bool) {
	if tool == ClaudeCandidate.Key && ClaudeEnabled() {
		return ClaudeCandidate, true
	}
	if tool == CodexCandidate.Key && CodexEnabled() {
		return CodexCandidate, true
	}
	spec, ok := agents[tool]
	return spec, ok
}

// ClaudeSwitch is the environment variable that opts one machine in to Claude
// chats. It is off by default: until the real-credential and missing-credential
// checks have been run, Claude is registered but dark.
const ClaudeSwitch = "REET_EXPERIMENTAL_CLAUDE_CHAT"

// ClaudeEnabled reports whether this machine has opted in to Claude chats.
func ClaudeEnabled() bool { return os.Getenv(ClaudeSwitch) == "1" }

// CodexSwitch is the environment variable that opts one machine in to Codex
// chats. Off by default: Codex is registered but dark until a real-login run and
// the approval card changes are done.
const CodexSwitch = "REET_EXPERIMENTAL_CODEX_CHAT"

// CodexEnabled reports whether this machine has opted in to Codex chats.
func CodexEnabled() bool { return os.Getenv(CodexSwitch) == "1" }

// VersionError explains why an adapter was refused.
type VersionError struct {
	Agent    string
	Found    string
	Required string
	Reason   string
}

func (e *VersionError) Error() string {
	found := e.Found
	if found == "" {
		found = "no version"
	}
	reason := e.Reason
	if reason == "" {
		reason = "the version is not supported"
	}
	if e.Required != "" {
		return fmt.Sprintf("%s reported %s; %s %s is required", e.Agent, found, e.Agent, e.Required)
	}
	return fmt.Sprintf("%s reported %s; %s", e.Agent, found, reason)
}

func (e *VersionError) Unwrap() error { return ErrVersion }

// semver is the small numeric subset ACP agent versions need. It accepts the
// same MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD] shape as semver without adding a
// dependency for one comparison.
type semver struct {
	parts []int
	pre   []string
	raw   string
}

func parseSemver(raw string) (semver, bool) {
	text := strings.TrimSpace(raw)
	text = strings.TrimPrefix(text, "v")
	if text == "" {
		return semver{}, false
	}
	text = strings.TrimPrefix(text, "V")

	var pre []string
	if i := strings.IndexByte(text, '+'); i >= 0 {
		text = text[:i]
	}
	if i := strings.IndexByte(text, '-'); i >= 0 {
		pre = strings.Split(text[i+1:], ".")
		text = text[:i]
	}
	fields := strings.Split(text, ".")
	if len(fields) < 2 || len(fields) > 3 {
		return semver{}, false
	}
	parts := make([]int, len(fields))
	for i, field := range fields {
		if field == "" || len(field) > 9 {
			return semver{}, false
		}
		value, err := strconv.Atoi(field)
		if err != nil || value < 0 {
			return semver{}, false
		}
		parts[i] = value
	}
	for len(parts) < 3 {
		parts = append(parts, 0)
	}
	return semver{parts: parts, pre: pre, raw: raw}, true
}

// extractSemver picks the first version-shaped token out of --version output.
func extractSemver(raw string) (semver, bool) {
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+')
	}) {
		if parsed, ok := parseSemver(token); ok {
			return parsed, true
		}
	}
	return semver{}, false
}

func (v semver) compare(other semver) int {
	for i := range v.parts {
		if v.parts[i] < other.parts[i] {
			return -1
		}
		if v.parts[i] > other.parts[i] {
			return 1
		}
	}
	if len(v.pre) == 0 && len(other.pre) == 0 {
		return 0
	}
	if len(v.pre) == 0 {
		return 1
	}
	if len(other.pre) == 0 {
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(other.pre); i++ {
		left, right := v.pre[i], other.pre[i]
		leftNumber, leftErr := strconv.Atoi(left)
		rightNumber, rightErr := strconv.Atoi(right)
		switch {
		case leftErr == nil && rightErr == nil:
			if leftNumber != rightNumber {
				if leftNumber < rightNumber {
					return -1
				}
				return 1
			}
		case leftErr == nil:
			return -1
		case rightErr == nil:
			return 1
		default:
			if left != right {
				if left < right {
					return -1
				}
				return 1
			}
		}
	}
	switch {
	case len(v.pre) < len(other.pre):
		return -1
	case len(v.pre) > len(other.pre):
		return 1
	default:
		return 0
	}
}

func (v semver) atLeast(min semver) bool { return v.compare(min) >= 0 }

func (v semver) String() string { return v.raw }

func safeName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "an empty name"
	}
	if len(s) > 80 {
		s = s[:80]
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "an unknown name"
		}
	}
	return s
}

// checkVersion verifies a version string against a spec's floor.
func checkVersion(spec Spec, raw string) error {
	min, required := spec.minVersion()
	if !required {
		return nil
	}
	found, ok := parseSemver(raw)
	if !ok {
		return &VersionError{Agent: spec.displayName(), Found: raw, Required: spec.MinVersion, Reason: "the reported version is not valid"}
	}
	if !found.atLeast(min) {
		return &VersionError{Agent: spec.displayName(), Found: found.String(), Required: spec.MinVersion}
	}
	return nil
}
