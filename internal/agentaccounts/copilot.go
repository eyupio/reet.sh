package agentaccounts

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// How the Copilot request presents itself. The endpoint answers only a
// request that presents itself as an editor's Copilot Chat, so these say that.
// They are kept here, together, so they are easy to change or remove.
const (
	copilotUserAgent     = "GitHubCopilotChat/0.26.7"
	copilotEditorVersion = "vscode/1.96.2"
	copilotPluginVersion = "copilot-chat/0.26.7"
	copilotAPIVersion    = "2025-04-01"
)

// copilotHeaders are the headers sent with the Copilot request beside the
// token.
func copilotHeaders() map[string]string {
	return map[string]string{
		"User-Agent":            copilotUserAgent,
		"Editor-Version":        copilotEditorVersion,
		"Editor-Plugin-Version": copilotPluginVersion,
		"X-GitHub-Api-Version":  copilotAPIVersion,
	}
}

// copilot reads GitHub Copilot's own sign-in from apps.json and hosts.json in
// its folder (github-copilot in $XDG_CONFIG_HOME, or in ~/.config). Each holds
// GitHub tokens by host, with the login each belongs to. The first login found
// is the account; its tokens are tried in turn when asking for the allowance.
// Only github.com's keys are read ("github.com", or "github.com:" and an app
// id): a key for any other host, even one whose name starts the same way, is
// never sent to api.github.com. The GitHub CLI's own sign-in is read too, so
// a person signed in to Copilot only through `gh` is found.
func (r *Reader) copilot() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "copilot", State: terminalproto.AccountNone}, ask: askCopilot}
	dir, _, ok := r.folder("XDG_CONFIG_HOME", ".config")
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	dir = filepath.Join(dir, "github-copilot")
	login, display, unreadable := "", "", false
	var tokens []string
	for _, name := range []string{"apps.json", "hosts.json"} {
		data, err := readFile(filepath.Join(dir, name), maxCredentialBytes)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		var file map[string]struct {
			Token string `json:"oauth_token"`
			User  string `json:"user"`
		}
		if err != nil || json.Unmarshal(data, &file) != nil {
			unreadable = true
			continue
		}
		hosts := make([]string, 0, len(file))
		for host := range file {
			if host == "github.com" || strings.HasPrefix(host, "github.com:") {
				hosts = append(hosts, host)
			}
		}
		sort.Strings(hosts)
		for _, host := range hosts {
			entry := file[host]
			user := strings.ToLower(strings.TrimSpace(entry.User))
			if entry.Token == "" || user == "" {
				continue
			}
			if login == "" {
				login, display = user, entry.User
			}
			if user == login && !slices.Contains(tokens, entry.Token) {
				tokens = append(tokens, entry.Token)
			}
		}
	}
	// The GitHub CLI holds a token of its own for every account it is signed
	// in to. An editor's token can outlive its value by years, so the CLI's
	// tokens are tried after it when the first is refused.
	ghLogin, ghTokens := r.githubCLI()
	if login == "" && ghLogin != "" {
		login, display = strings.ToLower(ghLogin), ghLogin
	}
	for _, token := range ghTokens {
		if !slices.Contains(tokens, token) {
			tokens = append(tokens, token)
		}
	}
	if len(tokens) == 0 {
		if unreadable {
			out.account.State = terminalproto.AccountUnreadable
		}
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.account.Account = label(display, 128)
	out.token, out.alternates = tokens[0], tokens[1:]
	out.identity = "copilot\x00" + login
	return out
}

// githubCLI reads the GitHub CLI's sign-ins for github.com: every account it
// reports as signed in, and its token for each. An older CLI that cannot list
// accounts still gives the active account's token.
func (r *Reader) githubCLI() (login string, tokens []string) {
	if out, ok := r.readTool("gh", "auth", "status", "--hostname", "github.com", "--json", "hosts"); ok {
		var doc struct {
			Hosts map[string][]struct {
				Login       string `json:"login"`
				State       string `json:"state"`
				TokenSource string `json:"tokenSource"`
			} `json:"hosts"`
		}
		if json.Unmarshal([]byte(out), &doc) == nil {
			for _, account := range doc.Hosts["github.com"] {
				// A token from the environment is not a stored sign-in: the
				// per-user lookup finds nothing for it.
				if account.State != "success" || account.Login == "" || account.TokenSource == "GH_TOKEN" || account.TokenSource == "GITHUB_TOKEN" {
					continue
				}
				token, ok := r.readTool("gh", "auth", "token", "--hostname", "github.com", "--user", account.Login)
				if !ok || slices.Contains(tokens, token) {
					continue
				}
				if login == "" {
					login = account.Login
				}
				tokens = append(tokens, token)
			}
			if len(tokens) > 0 {
				return login, tokens
			}
		}
	}
	if token, ok := r.readTool("gh", "auth", "token", "--hostname", "github.com"); ok {
		return "", []string{token}
	}
	return "", nil
}

// askCopilot reads the allowance of a Copilot plan: the month's premium
// requests, chats and completions, each as a share left. A paid plan gives
// them as quota snapshots; the free plan as monthly quotas and what is left of
// them. A window without a limit has nothing to show and is left out. A token
// whose account has no Copilot is answered 404, and the login's next token is
// tried.
func askCopilot(ctx context.Context, r *Reader, s signIn) allowance {
	var got sent
	refused, none := false, false
	for _, token := range append([]string{s.token}, s.alternates...) {
		got = r.send(ctx, http.MethodGet, r.opts.CopilotUserURL, "token "+token, nil, copilotHeaders())
		switch {
		case got.status == http.StatusNotFound:
			none = true
			continue
		case got.state == terminalproto.AccountLimitsRefused:
			refused = true
			continue
		case got.state != terminalproto.AccountLimitsRead:
			return allowance{state: got.state, wait: got.wait}
		}
		return copilotAllowance(got.body)
	}
	if refused || !none {
		return allowance{state: terminalproto.AccountLimitsRefused}
	}
	return allowance{state: terminalproto.AccountLimitsUnsupported}
}

// copilotAllowance reads Copilot's answer about one account.
func copilotAllowance(body []byte) allowance {
	type snapshot struct {
		Entitlement *float64 `json:"entitlement"`
		Remaining   *float64 `json:"percent_remaining"`
		Unlimited   bool     `json:"unlimited"`
	}
	var doc struct {
		Login     string               `json:"login"`
		Plan      string               `json:"copilot_plan"`
		SKU       string               `json:"access_type_sku"`
		Snapshots map[string]*snapshot `json:"quota_snapshots"`
		ResetDate string               `json:"quota_reset_date"`
		Monthly   map[string]float64   `json:"monthly_quotas"`
		Left      map[string]float64   `json:"limited_user_quotas"`
		FreeReset string               `json:"limited_user_reset_date"`
	}
	if json.Unmarshal(body, &doc) != nil || (doc.Snapshots == nil && doc.Monthly == nil) {
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	out := allowance{account: label(doc.Login, 128), plan: label(strings.ToLower(doc.Plan), 32)}
	if strings.Contains(strings.ToLower(doc.SKU), "free") {
		out.plan = "free"
	}
	windows := []struct{ key, label string }{{"premium_interactions", "Premium"}, {"chat", "Chat"}, {"completions", "Completions"}}
	resets := copilotDate(doc.ResetDate)
	for _, w := range windows {
		snap := doc.Snapshots[w.key]
		if snap == nil || snap.Unlimited || snap.Remaining == nil || (snap.Entitlement != nil && *snap.Entitlement <= 0) {
			continue
		}
		out.limits = append(out.limits, terminalproto.AccountLimit{Kind: terminalproto.AccountLimitMonth, Label: w.label, UsedPercent: percent(100 - *snap.Remaining), ResetsAt: resets})
	}
	if len(out.limits) == 0 && doc.Monthly != nil {
		resets = copilotDate(doc.FreeReset)
		for _, w := range windows {
			total, ok := doc.Monthly[w.key]
			left, known := doc.Left[w.key]
			if !ok || !known || total <= 0 {
				continue
			}
			out.limits = append(out.limits, terminalproto.AccountLimit{Kind: terminalproto.AccountLimitMonth, Label: w.label, UsedPercent: percent(100 - left/total*100), ResetsAt: resets})
		}
	}
	if len(out.limits) == 0 {
		// Every window is unlimited: signed in, with no allowance to read.
		out.state = terminalproto.AccountLimitsUnsupported
		return out
	}
	out.state = terminalproto.AccountLimitsRead
	return out
}

// copilotDate reads a reset date, a day ("2026-12-01", taken as the start of
// that day in UTC) or a time in RFC 3339, in milliseconds. Anything else is
// zero, "not said".
func copilotDate(value string) int64 {
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if at, err := time.Parse(layout, value); err == nil && at.UnixMilli() > 0 {
			return at.UnixMilli()
		}
	}
	return 0
}
