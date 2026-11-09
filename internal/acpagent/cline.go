//go:build linux || darwin

package acpagent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Cline is the launch profile for Cline's built-in ACP mode (Cline CLI 3.0.74,
// measured 5 November 2026 with `cline --acp`; see
// docs/research/cline-acp-probe.md).
//
// Unlike Claude Code and Codex, the ACP server is the cline binary itself, so
// there is no adapter to install beside it. --auto-approve false is part of the
// launch, not a suggestion: without it the CLI auto-approves every tool and no
// approval card is ever raised. RequireOptions re-checks that the session
// actually reports auto_approve=false, so a changed default cannot quietly skip
// the approval contract.
//
// The measured session starts in "act" and offers "plan"; session/new reports
// the model catalogue and mode through ACP configOptions, set_config_option
// confirms both a mode and a model switch, and session/load replays a stored
// conversation. Cline ignores a model at session/new, so a chosen model is
// applied after the session opens and read back.
var Cline = Spec{
	Key:          "cline",
	Command:      "cline",
	Args:         []string{"--acp", "--auto-approve", "false"},
	DisplayName:  "Cline",
	ExpectedName: "cline",
	MinVersion:   "3.0.74",
	RequireMode:  "act",
	StartMode:    "act",
	// The person may move a chat between making changes and planning only; the
	// mode that runs changes is the one a new chat starts in.
	ChoosableModes: []string{"act", "plan"},
	// Every tool call is refused at launch unless the session confirms the CLI's
	// auto-approve switch is off.
	RequireOptions:       map[string]string{"auto_approve": "false"},
	ConfirmModelBySwitch: true,
	// Cline's provider option is its billing source (Usage-Billing, ClinePass).
	Provider: true,
	// session/load replays a stored conversation, so a lost chat reconnects in
	// place under the same reet session id.
	Reattach: true,
	// Cline keeps its sign-in, settings, hooks and sessions under ~/.cline.
	// CLINE_DIR routes that folder without inheriting a provider redirect, and
	// XDG directories are not inherited for a profile that uses this route.
	LoginHomeEnv: "CLINE_DIR",
	LoginHomeDir: ".cline",
	// Only the credential the CLI was verified to read. Provider redirects and
	// other agents' keys are never inherited.
	EnvAllowlist: []string{"CLINE_API_KEY"},
	// A provider failure that never reaches ACP is written to Cline's own log
	// under CLINE_DIR, so the reason can be shown instead of a silent turn.
	RecoverError: clineLogError,
	RecoverLive:  clineLogError,
}

// clineLogError recovers the provider error Cline logged for a turn it ended
// without an ACP message. Cline writes newline-delimited JSON under
// <CLINE_DIR>/data/logs/cline.log: level 50 is an error, and a failed turn
// carries the session id and err.message. Only lines after the turn started are
// considered, and only that session's, so another chat's failure is never shown.
func clineLogError(root, sessionID string, afterMS int64) string {
	if root == "" || sessionID == "" {
		return ""
	}
	return clineLogErrorAt(filepath.Join(root, "data", "logs", "cline.log"), sessionID, afterMS)
}

// clineLogTail bounds the log read: a turn's failure is written at its end.
const clineLogTail = 1 << 20

func clineLogErrorAt(path, sessionID string, afterMS int64) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return ""
	}
	start := info.Size() - clineLogTail
	if start < 0 {
		start = 0
	}
	data := make([]byte, info.Size()-start)
	n, _ := file.ReadAt(data, start)
	data = data[:n]
	if start > 0 {
		lineEnd := bytes.IndexByte(data, '\n')
		if lineEnd < 0 {
			return ""
		}
		data = data[lineEnd+1:]
	}
	newest := int64(0)
	message := ""
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var entry struct {
			Level     int    `json:"level"`
			Time      string `json:"time"`
			SessionID string `json:"sessionId"`
			Message   string `json:"message"`
			Err       struct {
				Message string `json:"message"`
			} `json:"err"`
		}
		if json.Unmarshal(line, &entry) != nil || entry.Level < 50 || entry.SessionID != sessionID {
			continue
		}
		text := strings.TrimSpace(entry.Err.Message)
		if text == "" {
			text = strings.TrimSpace(entry.Message)
		}
		if text == "" {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, entry.Time)
		if err != nil || at.UnixMilli() < afterMS {
			continue
		}
		if ms := at.UnixMilli(); message == "" || ms >= newest {
			newest, message = ms, text
		}
	}
	if len(message) > 500 {
		message = cutUTF8(message, 500)
	}
	return message
}
