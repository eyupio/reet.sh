package agentaccounts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// claude reads Claude Code's sign-in. It is .credentials.json in the
// configuration folder (~/.claude, or the folder CLAUDE_CONFIG_DIR names),
// or, on macOS, the login keychain's `Claude Code-credentials` item. A CLI
// that cannot write the keychain refreshes the file alone, and the keychain
// can then hold an older token, so the later of the two is used. The
// account's address comes from .claude.json.
func (r *Reader) claude() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "claude", State: terminalproto.AccountNone}, ask: askClaude}
	dir, moved, ok := r.folder("CLAUDE_CONFIG_DIR", ".claude")
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, readErr := readFile(filepath.Join(dir, ".credentials.json"), maxCredentialBytes)
	file, fileOK, fileReadable := claudeSignIn(data)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		fileOK, fileReadable = false, false
	}
	// macOS may hold the same sign-in in the keychain; the later token wins.
	var keychain claudeToken
	keychainOK := false
	if item, found := r.macKeychainItem("Claude Code-credentials", r.env("USER")); found {
		if token, ok, readable := claudeSignIn([]byte(item)); readable && ok {
			keychain, keychainOK = token, true
		}
	}
	if keychainOK && (!fileOK || keychain.ExpiresAt > file.ExpiresAt) {
		file, fileOK = keychain, true
	}
	if !fileOK {
		if !fileReadable {
			out.account.State = terminalproto.AccountUnreadable
		} else if r.goos() == "darwin" {
			// The keychain may hold a sign-in this session cannot read.
			out.account.State = terminalproto.AccountUnreadable
		}
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.account.Plan = label(file.Plan, 32)
	out.token = file.AccessToken
	if file.ExpiresAt > 0 {
		out.expires = time.UnixMilli(file.ExpiresAt)
	}
	if out.token == "" {
		// Only the renewal token is there: the agent must run to get a new one.
		out.expires = time.Unix(1, 0)
	}
	// .claude.json sits beside the folder by default and inside it when the
	// folder was moved.
	profile := filepath.Join(r.opts.Home, ".claude.json")
	if moved {
		profile = filepath.Join(dir, ".claude.json")
	}
	uuid := ""
	if data, err := readFile(profile, maxProfileBytes); err == nil {
		var doc struct {
			Account *struct {
				Email string `json:"emailAddress"`
				UUID  string `json:"accountUuid"`
			} `json:"oauthAccount"`
		}
		if json.Unmarshal(data, &doc) == nil && doc.Account != nil {
			out.account.Account, uuid = label(doc.Account.Email, 128), doc.Account.UUID
		}
	}
	out.identity = "claude\x00" + uuid + "\x00" + out.account.Account + "\x00" + out.account.Plan
	return out
}

// claudeToken is the part of Claude Code's sign-in this package uses.
type claudeToken struct {
	AccessToken string
	ExpiresAt   int64 // milliseconds since 1970, 0 when not said
	Plan        string
}

// claudeSignIn reads Claude Code's credential document. It reports whether
// the document could be read at all, which tells a file that will not parse
// (unreadable) from one that simply holds no sign-in (MCP sign-ins only).
func claudeSignIn(data []byte) (claudeToken, bool, bool) {
	if len(data) == 0 {
		return claudeToken{}, false, true
	}
	var file struct {
		OAuth *struct {
			AccessToken      string  `json:"accessToken"`
			RefreshToken     string  `json:"refreshToken"`
			ExpiresAt        float64 `json:"expiresAt"`
			SubscriptionType string  `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(data, &file) != nil {
		return claudeToken{}, false, false
	}
	if file.OAuth == nil || (file.OAuth.AccessToken == "" && file.OAuth.RefreshToken == "") {
		return claudeToken{}, false, true
	}
	token := claudeToken{AccessToken: file.OAuth.AccessToken, Plan: strings.ToLower(file.OAuth.SubscriptionType)}
	if file.OAuth.ExpiresAt > 0 && file.OAuth.ExpiresAt < 1e15 {
		token.ExpiresAt = int64(file.OAuth.ExpiresAt)
	}
	return token, true, true
}

func (r *Reader) goos() string {
	if r.opts.GOOS != "" {
		return r.opts.GOOS
	}
	return runtime.GOOS
}

// askClaude reads the allowance of a Claude plan: the five-hour window, the
// week, and the week's share for a model the plan limits on its own. Each is a
// used percentage and the time it starts again.
func askClaude(ctx context.Context, r *Reader, s signIn) allowance {
	body, state, wait := r.get(ctx, r.opts.ClaudeUsageURL, s.token, map[string]string{"anthropic-beta": "oauth-2025-04-20"})
	if state != terminalproto.AccountLimitsRead {
		return allowance{state: state, wait: wait}
	}
	type window struct {
		Utilization *float64 `json:"utilization"`
		ResetsAt    string   `json:"resets_at"`
	}
	var doc struct {
		FiveHour *window `json:"five_hour"`
		Week     *window `json:"seven_day"`
		Opus     *window `json:"seven_day_opus"`
		Sonnet   *window `json:"seven_day_sonnet"`
	}
	if json.Unmarshal(body, &doc) != nil {
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	var limits []terminalproto.AccountLimit
	add := func(w *window, kind terminalproto.AccountLimitKind, name string) {
		if w == nil || w.Utilization == nil {
			return
		}
		limits = append(limits, terminalproto.AccountLimit{Kind: kind, Label: name, UsedPercent: percent(*w.Utilization), ResetsAt: rfc3339Millis(w.ResetsAt)})
	}
	add(doc.FiveHour, terminalproto.AccountLimitSession, "")
	add(doc.Week, terminalproto.AccountLimitWeek, "")
	add(doc.Opus, terminalproto.AccountLimitWeek, "Opus")
	add(doc.Sonnet, terminalproto.AccountLimitWeek, "Sonnet")
	if len(limits) == 0 {
		// An answer with no window this reader knows is not an allowance.
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	return allowance{limits: limits, state: terminalproto.AccountLimitsRead}
}
