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

// claude reads Claude Code's sign-in. On Linux it is a file,
// .credentials.json in the configuration folder (~/.claude, or the folder
// CLAUDE_CONFIG_DIR names). On macOS Claude Code keeps it in the keychain,
// which this reader does not open, so a missing file there is "cannot tell",
// not "not signed in". The account's address comes from .claude.json.
func (r *Reader) claude() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "claude", State: terminalproto.AccountNone}, ask: askClaude}
	dir, moved, ok := r.folder("CLAUDE_CONFIG_DIR", ".claude")
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, err := readFile(filepath.Join(dir, ".credentials.json"), maxCredentialBytes)
	if errors.Is(err, os.ErrNotExist) {
		if r.goos() == "darwin" {
			out.account.State = terminalproto.AccountUnreadable
		}
		return out
	}
	var file struct {
		OAuth *struct {
			AccessToken      string  `json:"accessToken"`
			RefreshToken     string  `json:"refreshToken"`
			ExpiresAt        float64 `json:"expiresAt"`
			SubscriptionType string  `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
	}
	if err != nil || json.Unmarshal(data, &file) != nil {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	if file.OAuth == nil || (file.OAuth.AccessToken == "" && file.OAuth.RefreshToken == "") {
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.account.Plan = label(strings.ToLower(file.OAuth.SubscriptionType), 32)
	out.token = file.OAuth.AccessToken
	if file.OAuth.ExpiresAt > 0 && file.OAuth.ExpiresAt < 1e15 {
		out.expires = time.UnixMilli(int64(file.OAuth.ExpiresAt))
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

func (r *Reader) goos() string {
	if r.opts.GOOS != "" {
		return r.opts.GOOS
	}
	return runtime.GOOS
}

// askClaude reads the allowance of a Claude plan: the five-hour window, the
// week, and the week's share for a model the plan limits on its own. Each is a
// used percentage and the time it starts again.
func askClaude(ctx context.Context, r *Reader, s signIn) ([]terminalproto.AccountLimit, terminalproto.AccountLimitsState, time.Duration) {
	body, state, wait := r.get(ctx, r.opts.ClaudeUsageURL, s.token, map[string]string{"anthropic-beta": "oauth-2025-04-20"})
	if state != terminalproto.AccountLimitsRead {
		return nil, state, wait
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
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	var limits []terminalproto.AccountLimit
	add := func(w *window, kind terminalproto.AccountLimitKind, name string) {
		if w == nil || w.Utilization == nil {
			return
		}
		limit := terminalproto.AccountLimit{Kind: kind, Label: name, UsedPercent: percent(*w.Utilization)}
		if at, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil && at.UnixMilli() > 0 {
			limit.ResetsAt = at.UnixMilli()
		}
		limits = append(limits, limit)
	}
	add(doc.FiveHour, terminalproto.AccountLimitSession, "")
	add(doc.Week, terminalproto.AccountLimitWeek, "")
	add(doc.Opus, terminalproto.AccountLimitWeek, "Opus")
	add(doc.Sonnet, terminalproto.AccountLimitWeek, "Sonnet")
	if len(limits) == 0 {
		// An answer with no window this reader knows is not an allowance.
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	return limits, terminalproto.AccountLimitsRead, 0
}
