package agentaccounts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// codex reads Codex's sign-in from auth.json in its folder (~/.codex, or the
// folder CODEX_HOME names). A ChatGPT sign-in holds tokens; its address and
// plan are claims of the identity token. An API key is a sign-in with no plan.
// Codex can also be set to keep its sign-in in the system keyring, which this
// reader does not open: such a machine reads as having no sign-in file.
func (r *Reader) codex() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "codex", State: terminalproto.AccountNone}, ask: askCodex}
	dir, _, ok := r.folder("CODEX_HOME", ".codex")
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, err := readFile(filepath.Join(dir, "auth.json"), maxCredentialBytes)
	if errors.Is(err, os.ErrNotExist) {
		return out
	}
	var file struct {
		APIKey string `json:"OPENAI_API_KEY"`
		Tokens *struct {
			IDToken     string `json:"id_token"`
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if err != nil || json.Unmarshal(data, &file) != nil {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	if file.Tokens == nil || file.Tokens.AccessToken == "" {
		if file.APIKey != "" {
			out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountAPIKey
		}
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.token = file.Tokens.AccessToken
	var identity struct {
		Email string `json:"email"`
		Auth  struct {
			Plan string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if jwtClaims(file.Tokens.IDToken, &identity) {
		out.account.Account = label(identity.Email, 128)
		out.account.Plan = label(strings.ToLower(identity.Auth.Plan), 32)
	}
	var access struct {
		Expires float64 `json:"exp"`
	}
	if jwtClaims(file.Tokens.AccessToken, &access) && access.Expires > 0 && access.Expires < 1e12 {
		out.expires = time.Unix(int64(access.Expires), 0)
	}
	out.accountID = file.Tokens.AccountID
	out.identity = "codex\x00" + file.Tokens.AccountID + "\x00" + out.account.Account + "\x00" + out.account.Plan
	return out
}

// askCodex reads the allowance of a ChatGPT plan: one or two windows, each a
// used percentage, its length and when it starts again.
func askCodex(ctx context.Context, r *Reader, s signIn) ([]terminalproto.AccountLimit, terminalproto.AccountLimitsState, time.Duration) {
	headers := map[string]string{}
	if validHeaderValue(s.accountID) {
		headers["chatgpt-account-id"] = s.accountID
	}
	body, state, wait := r.get(ctx, r.opts.CodexUsageURL, s.token, headers)
	if state != terminalproto.AccountLimitsRead {
		return nil, state, wait
	}
	type window struct {
		UsedPercent *float64 `json:"used_percent"`
		Seconds     float64  `json:"limit_window_seconds"`
		ResetAt     float64  `json:"reset_at"`
		ResetAfter  float64  `json:"reset_after_seconds"`
	}
	var doc struct {
		RateLimit *struct {
			Primary   *window `json:"primary_window"`
			Secondary *window `json:"secondary_window"`
		} `json:"rate_limit"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.RateLimit == nil {
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	now := r.opts.Now()
	var limits []terminalproto.AccountLimit
	for _, w := range []*window{doc.RateLimit.Primary, doc.RateLimit.Secondary} {
		if w == nil || w.UsedPercent == nil {
			continue
		}
		limit := terminalproto.AccountLimit{Kind: windowKind(w.Seconds), UsedPercent: percent(*w.UsedPercent)}
		switch {
		case w.ResetAt > 0 && w.ResetAt < 1e12:
			limit.ResetsAt = int64(w.ResetAt) * 1000
		case w.ResetAfter > 0 && w.ResetAfter < 1e9:
			limit.ResetsAt = now.Add(time.Duration(w.ResetAfter) * time.Second).UnixMilli()
		}
		limits = append(limits, limit)
	}
	if len(limits) == 0 {
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	return limits, terminalproto.AccountLimitsRead, 0
}

// windowKind names a window by its length: up to six hours is a session, up
// to a day and a half a day, up to eight days a week, anything longer a month.
// A window with no length is taken as the session window, the shortest.
func windowKind(seconds float64) terminalproto.AccountLimitKind {
	switch hours := seconds / 3600; {
	case hours <= 6:
		return terminalproto.AccountLimitSession
	case hours <= 36:
		return terminalproto.AccountLimitDay
	case hours <= 8*24:
		return terminalproto.AccountLimitWeek
	}
	return terminalproto.AccountLimitMonth
}

// validHeaderValue is true for a short value of letters, digits and the
// punctuation an account id uses, so nothing read from a file can add a
// header line.
func validHeaderValue(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, c := range s {
		ok := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.'
		if !ok {
			return false
		}
	}
	return true
}
