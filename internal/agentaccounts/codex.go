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
// folder CODEX_HOME names), or from the macOS login keychain's `Codex Auth`
// item when the file holds none. A ChatGPT sign-in holds tokens; its address
// and plan are claims of the identity token. An API key is a sign-in with no
// plan. The later of the file and the keychain is used: a CLI that cannot
// write the keychain refreshes the file alone.
func (r *Reader) codex() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "codex", State: terminalproto.AccountNone}, ask: askCodex}
	dir, _, ok := r.folder("CODEX_HOME", ".codex")
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, readErr := readFile(filepath.Join(dir, "auth.json"), maxCredentialBytes)
	file, fileOK, fileReadable := codexSignIn(data)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		fileOK, fileReadable = false, false
	}
	if item, keychainOK := r.macKeychainItem("Codex Auth", ""); keychainOK {
		if keychain, parsed, readable := codexSignIn([]byte(item)); readable && parsed && (!fileOK || codexLater(keychain, file)) {
			file, fileOK = keychain, true
		}
	}
	if !fileOK {
		if !fileReadable {
			out.account.State = terminalproto.AccountUnreadable
		}
		return out
	}
	if file.AccessToken == "" {
		if file.APIKey != "" {
			out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountAPIKey
		}
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.token = file.AccessToken
	var identity struct {
		Email string `json:"email"`
		Auth  struct {
			Plan string `json:"chatgpt_plan_type"`
		} `json:"https://api.openai.com/auth"`
	}
	if jwtClaims(file.IDToken, &identity) {
		out.account.Account = label(identity.Email, 128)
		out.account.Plan = label(strings.ToLower(identity.Auth.Plan), 32)
	}
	if file.ExpiresAt > 0 {
		out.expires = time.Unix(file.ExpiresAt, 0)
	}
	out.accountID = file.AccountID
	out.identity = "codex\x00" + file.AccountID + "\x00" + out.account.Account + "\x00" + out.account.Plan
	return out
}

// codexCredential is the part of Codex's credential document this package uses.
type codexCredential struct {
	APIKey      string
	IDToken     string
	AccessToken string
	AccountID   string
	ExpiresAt   int64 // seconds since 1970, 0 when not said
}

// codexSignIn reads Codex's credential document.
func codexSignIn(data []byte) (codexCredential, bool, bool) {
	if len(data) == 0 {
		return codexCredential{}, false, true
	}
	var file struct {
		APIKey string `json:"OPENAI_API_KEY"`
		Tokens *struct {
			IDToken     string `json:"id_token"`
			AccessToken string `json:"access_token"`
			AccountID   string `json:"account_id"`
		} `json:"tokens"`
	}
	if json.Unmarshal(data, &file) != nil {
		return codexCredential{}, false, false
	}
	out := codexCredential{APIKey: file.APIKey}
	if file.Tokens == nil {
		return out, out.APIKey != "", true
	}
	out.IDToken, out.AccessToken, out.AccountID = file.Tokens.IDToken, file.Tokens.AccessToken, file.Tokens.AccountID
	var access struct {
		Expires float64 `json:"exp"`
	}
	if jwtClaims(out.AccessToken, &access) && access.Expires > 0 && access.Expires < 1e12 {
		out.ExpiresAt = int64(access.Expires)
	}
	return out, out.AccessToken != "" || out.APIKey != "", true
}

// codexLater reports whether a is the later of two sign-ins.
func codexLater(a, b codexCredential) bool {
	if a.ExpiresAt == 0 {
		return false
	}
	if b.ExpiresAt == 0 {
		return true
	}
	return a.ExpiresAt > b.ExpiresAt
}

// askCodex reads the allowance of a ChatGPT plan: one or two windows, each a
// used percentage, its length and when it starts again.
func askCodex(ctx context.Context, r *Reader, s signIn) allowance {
	headers := map[string]string{}
	if validHeaderValue(s.accountID) {
		headers["chatgpt-account-id"] = s.accountID
	}
	body, state, wait := r.get(ctx, r.opts.CodexUsageURL, s.token, headers)
	if state != terminalproto.AccountLimitsRead {
		return allowance{state: state, wait: wait}
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
		return allowance{state: terminalproto.AccountLimitsFailed}
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
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	return allowance{limits: limits, state: terminalproto.AccountLimitsRead}
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
