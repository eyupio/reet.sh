//go:build linux || darwin

package opencodeacp

import (
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
	"sort"
	"strings"
	"syscall"
	"time"
)

// Version is the only OpenCode version the probe covered. A different build is
// refused until it has been probed again (permission precedence, the local HTTP
// API and project plugin loading can all change between releases).
const Version = "1.19.5"

var (
	ErrVersion           = errors.New("OpenCode 1.19.5 is required")
	ErrPluginsUnreadable = errors.New("the folder's .opencode plugins could not be read")
)

// TrustError means the folder has repository plugins. OpenCode runs them when a
// session opens; only a person who trusts the repository may continue.
type TrustError struct{ Plugins []string }

func (e *TrustError) Error() string {
	return "this folder has OpenCode plugins: " + strings.Join(e.Plugins, ", ")
}

// Options is one chat session's connector-owned launch data. No controller
// argument beyond these fields reaches the process.
type Options struct {
	Binary string
	Dir    string
	Home   string
	Model  string
	Trust  bool
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
	Title        string
	ToolKind     string
	Status       string
	State        string
	Text         string
	Command      string
	Path         string
	Diff         string
	Locations    []string
	Allow        *bool
	Bad          bool
	Truncated    bool
	Reason       string
	Detail       string
}

func boolPtr(v bool) *bool { return &v }

// ProbeVersion runs the connector-owned binary with a minimal environment and
// requires exactly Version. Nothing from the controller or the project config
// is used.
func ProbeVersion(ctx context.Context, binary, dir, home string, base []string) (string, error) {
	if binary == "" {
		return "", ErrVersion
	}
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.Command(binary, "--version")
	cmd.Dir = dir
	cmd.Env = minimalEnvironment(home, base)
	cmd.Stdin = nil
	cmd.SysProcAttr = procAttr()
	out := &cappedBuffer{limit: 256}
	cmd.Stdout = out
	cmd.Stderr = &cappedBuffer{limit: 256}
	if err := cmd.Start(); err != nil {
		return "", ErrVersion
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr = <-done
	}
	if runCtx.Err() != nil || waitErr != nil || out.overflow {
		return "", ErrVersion
	}
	got := strings.TrimSpace(out.String())
	if got != Version {
		return "", fmt.Errorf("%w (found %s)", ErrVersion, safeVersion(got))
	}
	return got, nil
}

func safeVersion(s string) string {
	if s == "" {
		return "no version"
	}
	if len(s) > 32 || !strings.Contains(s, ".") {
		return "an unknown version"
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r == '.' || r == '-' || r == '+') {
			return "an unknown version"
		}
	}
	return s
}

// Plugins reports the repository plugin names that OpenCode would run when a
// session opens. A missing .opencode is no plugins; an unreadable one is an
// error because a plugin could be hidden behind it.
func Plugins(dir string) ([]string, error) {
	root := filepath.Join(dir, ".opencode")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrPluginsUnreadable
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if name == "package.json" || strings.HasPrefix(name, "plugin") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	if len(names) > 20 {
		names = names[:20]
	}
	return names, nil
}

const configTemplate = `{"$schema":"https://opencode.ai/config.json","autoupdate":false,"share":"disabled","default_agent":"build","permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"},"agent":{"build":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"plan":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"general":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"explore":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}}}}`

// ConfigContent is the reet-owned config. It asks for every built-in primary
// agent at top level and per agent, denies task and webfetch, and pins the mode.
func ConfigContent(model string) string {
	if model == "" {
		return configTemplate
	}
	var object map[string]any
	if json.Unmarshal([]byte(configTemplate), &object) != nil {
		return configTemplate
	}
	object["model"] = model
	data, err := json.Marshal(object)
	if err != nil {
		return configTemplate
	}
	return string(data)
}

func serverPassword(random io.Reader) (string, error) {
	if random == nil {
		random = rand.Reader
	}
	var b [32]byte
	if _, err := io.ReadFull(random, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var allowedKeys = map[string]bool{
	"PATH": true, "XDG_CONFIG_HOME": true, "XDG_DATA_HOME": true,
	"XDG_STATE_HOME": true, "XDG_CACHE_HOME": true, "TMPDIR": true,
	"TMP": true, "TEMP": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true,
	"TZ": true, "USER": true, "LOGNAME": true,
}

// minimalEnvironment selects only the allowlist above and replaces HOME with
// the connector's home. Every inherited OPENCODE_* is dropped before reet's own
// values are added.
func minimalEnvironment(home string, base []string) []string {
	values := map[string]string{}
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.HasPrefix(key, "OPENCODE_") || !allowedKeys[key] {
			continue
		}
		if strings.ContainsAny(value, "\x00\n\r") {
			continue
		}
		values[key] = value
	}
	values["HOME"] = home
	return envList(values)
}

func chatEnvironment(home string, base []string, password, model string) []string {
	values := map[string]string{}
	for _, entry := range minimalEnvironment(home, base) {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	values["OPENCODE_CONFIG_CONTENT"] = ConfigContent(model)
	values["OPENCODE_DISABLE_PROJECT_CONFIG"] = "1"
	values["OPENCODE_SERVER_PASSWORD"] = password
	values["OPENCODE_DISABLE_MODELS_FETCH"] = "1"
	values["OPENCODE_DISABLE_AUTOUPDATE"] = "1"
	values["OPENCODE_DISABLE_DEFAULT_PLUGINS"] = "1"
	values["OPENCODE_DISABLE_LSP_DOWNLOAD"] = "1"
	values["OPENCODE_DISABLE_CLAUDE_CODE"] = "1"
	values["OPENCODE_DISABLE_EXTERNAL_SKILLS"] = "1"
	return envList(values)
}

func envList(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

type cappedBuffer struct {
	limit    int
	overflow bool
	buf      []byte
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if len(b.buf)+len(p) > b.limit {
		b.overflow = true
		remaining := b.limit - len(b.buf)
		if remaining > 0 {
			b.buf = append(b.buf, p[:remaining]...)
		}
		return len(p), nil
	}
	b.buf = append(b.buf, p...)
	return len(p), nil
}
func (b *cappedBuffer) String() string { return string(b.buf) }
