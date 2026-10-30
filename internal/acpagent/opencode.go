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
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// OpenCode's ACP adapter is the native `opencode acp` server. The floor is the
// version AOE verifies for reliable edit/think classification and permission
// context; a newer release is accepted because the handshake still names and
// versions the agent.
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
// its ACP prompt response only reports a completed turn. It mirrors AOE's
// recovery from OpenCode's SQLite store.
func openCodePromptError(home, sessionID string, afterMS int64) string {
	if home == "" || sessionID == "" {
		return ""
	}
	path := openCodeDBPath(home)
	if path == "" {
		return ""
	}
	return openCodePromptErrorAt(path, sessionID, afterMS)
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
