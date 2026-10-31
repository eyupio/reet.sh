//go:build linux || darwin

package acpagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCode's ACP adapter is the native `opencode acp` server. The floor is the
// version covered by reet's local probe for edit/think classification and
// permission context; a newer release is accepted because the handshake still
// names and versions the agent.
var OpenCode = Spec{
	Key:          "opencode",
	Command:      "opencode",
	Args:         []string{"acp"},
	DisplayName:  "OpenCode",
	ExpectedName: "OpenCode",
	MinVersion:   "1.19.5",
	RequireMode:  "build",
	EnvAllowlist: []string{
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"GOOGLE_GENERATIVE_AI_API_KEY",
		"GOOGLE_API_KEY",
		"GEMINI_API_KEY",
		"OPENROUTER_API_KEY",
		"OPENCODE_API_KEY",
	},
	Trust:        openCodePlugins,
	ConfigEnv:    openCodeConfigEnv,
	RecoverError: openCodePromptError,
	RecoverLive:  openCodeLiveError,
}

// agents is the catalogue of adapters this connector can start. Adding an
// agent is a Spec entry plus a compatibility verification, not a new client.
var agents = map[string]Spec{OpenCode.Key: OpenCode}

const openCodeConfigTemplate = `{"$schema":"https://opencode.ai/config.json","autoupdate":false,"share":"disabled","default_agent":"build","permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"},"agent":{"build":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"plan":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"general":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}},"explore":{"permission":{"edit":"ask","bash":"ask","external_directory":"ask","task":"deny","webfetch":"deny"}}}}`

func openCodeConfigContent(model string) string {
	if model == "" {
		return openCodeConfigTemplate
	}
	var object map[string]any
	if json.Unmarshal([]byte(openCodeConfigTemplate), &object) != nil {
		return openCodeConfigTemplate
	}
	object["model"] = model
	data, err := json.Marshal(object)
	if err != nil {
		return openCodeConfigTemplate
	}
	return string(data)
}

// openCodeConfigEnv is the reet-owned OpenCode environment. It asks for every
// built-in primary agent at top level and per agent, denies task and webfetch,
// pins the mode, and stops background fetches and installs.
func openCodeConfigEnv(launch LaunchEnv) map[string]string {
	return map[string]string{
		"OPENCODE_CONFIG_CONTENT":          openCodeConfigContent(launch.Model),
		"OPENCODE_DISABLE_PROJECT_CONFIG":  "1",
		"OPENCODE_SERVER_PASSWORD":         launch.Password,
		"OPENCODE_DISABLE_MODELS_FETCH":    "1",
		"OPENCODE_DISABLE_AUTOUPDATE":      "1",
		"OPENCODE_DISABLE_DEFAULT_PLUGINS": "1",
		"OPENCODE_DISABLE_LSP_DOWNLOAD":    "1",
		"OPENCODE_DISABLE_CLAUDE_CODE":     "1",
		"OPENCODE_DISABLE_EXTERNAL_SKILLS": "1",
	}
}

// openCodePlugins reports the repository plugin names OpenCode would run when
// a session opens. A missing .opencode is no plugins; an unreadable one is an
// error because a plugin could be hidden behind it.
func openCodePlugins(dir string) ([]string, error) {
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

// openCodePromptError recovers the provider error OpenCode stores locally when
// its ACP prompt response only reports a completed turn. It reads the local
// OpenCode store for the matching session and turn.
func openCodePromptError(home, sessionID string, afterMS int64) string {
	if home == "" || sessionID == "" {
		return ""
	}
	if path := openCodeDBPath(home); path != "" {
		if message := openCodePromptErrorAt(path, sessionID, afterMS); message != "" {
			return message
		}
	}
	return openCodeLogErrorAt(openCodeLogPath(home), sessionID, afterMS)
}

func openCodeDataDir(home string) string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "opencode")
	}
	return filepath.Join(home, ".local", "share", "opencode")
}

// openCodeDBPath is the most recently written opencode.db or
// opencode-<name>.db in the agent's data directory.
func openCodeDBPath(home string) string {
	dir := openCodeDataDir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, entry := range entries {
		name := entry.Name()
		if name != "opencode.db" && !(strings.HasPrefix(name, "opencode-") && strings.HasSuffix(name, ".db")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) {
			newest = filepath.Join(dir, name)
			newestTime = info.ModTime()
		}
	}
	return newest
}

const openCodeErrorV2 = `SELECT time_created, json_extract(data, '$.error.message')
	FROM session_message
	WHERE session_id = ?1 AND type = 'assistant' AND time_created >= ?2
	  AND json_extract(data, '$.error.message') IS NOT NULL
	ORDER BY time_created DESC LIMIT 1`

const openCodeErrorV1 = `SELECT CAST(json_extract(data, '$.time.created') AS INTEGER),
	json_extract(data, '$.error.data.message')
	FROM message
	WHERE session_id = ?1 AND json_extract(data, '$.role') = 'assistant'
	  AND CAST(json_extract(data, '$.time.created') AS INTEGER) >= ?2
	  AND json_extract(data, '$.error.data.message') IS NOT NULL
	ORDER BY CAST(json_extract(data, '$.time.created') AS INTEGER) DESC LIMIT 1`

func openCodePromptErrorAt(path, sessionID string, afterMS int64) string {
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return ""
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, _ = db.ExecContext(ctx, "PRAGMA busy_timeout=100")

	var newest int64
	message := ""
	for _, query := range []string{openCodeErrorV2, openCodeErrorV1} {
		var created int64
		var found sql.NullString
		if err := db.QueryRowContext(ctx, query, sessionID, afterMS).Scan(&created, &found); err != nil {
			continue
		}
		text := strings.TrimSpace(found.String)
		if text == "" {
			continue
		}
		if message == "" || created > newest {
			newest, message = created, text
		}
	}
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

// openCodeLogPath is where 1.19.5 writes its provider and session log. The ACP
// adapter does not forward every provider failure, so this is the only live
// record of an error while a turn is still silent.
func openCodeLogPath(home string) string {
	return filepath.Join(openCodeDataDir(home), "log", "opencode.log")
}

// openCodeLiveError recovers the newest provider error OpenCode logged for a
// turn that has not reported it over ACP. Only errors after the turn started
// are considered, and the small title-model retries are ignored because they
// do not stop the main turn.
func openCodeLiveError(home, sessionID string, afterMS int64) string {
	if home == "" || sessionID == "" {
		return ""
	}
	return openCodeLogErrorAt(openCodeLogPath(home), sessionID, afterMS)
}

const openCodeLogTail = 1 << 20

func openCodeLogErrorAt(path, sessionID string, afterMS int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return ""
	}
	start := info.Size() - openCodeLogTail
	if start < 0 {
		start = 0
	}
	data := make([]byte, info.Size()-start)
	n, _ := file.ReadAt(data, start)
	data = data[:n]
	if start > 0 {
		lineEnd := strings.IndexByte(string(data), '\n')
		if lineEnd < 0 {
			return ""
		}
		data = data[lineEnd+1:]
	}
	newest := int64(0)
	message := ""
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, "session.id="+sessionID) ||
			!strings.Contains(line, "level=ERROR") ||
			!strings.Contains(line, `message="stream error"`) ||
			strings.Contains(line, "small=true") {
			continue
		}
		created, ok := openCodeLogTimestamp(line)
		if !ok || created < afterMS {
			continue
		}
		text := strings.TrimPrefix(openCodeLogQuoted(line, "error.error"), "AI_APICallError: ")
		if text == "" {
			continue
		}
		if message == "" || created > newest {
			newest, message = created, text
		}
	}
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func openCodeLogTimestamp(line string) (int64, bool) {
	raw := openCodeLogPlain(line, "timestamp")
	if raw == "" {
		return 0, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return 0, false
	}
	return parsed.UnixMilli(), true
}

func openCodeLogPlain(line, key string) string {
	marker := key + "="
	at := strings.Index(line, marker)
	if at < 0 {
		return ""
	}
	rest := line[at+len(marker):]
	if end := strings.IndexAny(rest, " \t"); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func openCodeLogQuoted(line, key string) string {
	marker := key + `="`
	at := strings.Index(line, marker)
	if at < 0 {
		return ""
	}
	rest := line[at+len(marker):]
	for i := 0; i < len(rest); i++ {
		if rest[i] == '\\' {
			i++
			continue
		}
		if rest[i] == '"' {
			if value, err := strconv.Unquote(`"` + rest[:i] + `"`); err == nil {
				return value
			}
			return rest[:i]
		}
	}
	return ""
}
