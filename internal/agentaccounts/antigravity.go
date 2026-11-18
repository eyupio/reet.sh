package agentaccounts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// antigravityKeyringPrefix marks a sign-in file written in the form the
// agent's keyring library uses: the prefix, then base64 of the JSON.
const antigravityKeyringPrefix = "go-keyring-base64:"

// antigravityOtherModels labels the windows Antigravity keeps for models that
// are not its own.
const antigravityOtherModels = "Other models"

// antigravity reads Google Antigravity's sign-in from antigravity-oauth-token
// in its folder (~/.gemini/antigravity-cli, or the folder
// ANTIGRAVITY_APP_DATA_DIR names). The access token and its expiry are the
// file's token; the address is a claim of its identity token. Antigravity can
// keep its sign-in in the desktop keyring or the macOS keychain instead, which
// this reader does not open: such a machine reads as having no sign-in file.
func (r *Reader) antigravity() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "antigravity", State: terminalproto.AccountNone}, ask: askAntigravity}
	dir, _, ok := r.folder("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(".gemini", "antigravity-cli"))
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, err := readFile(filepath.Join(dir, "antigravity-oauth-token"), maxCredentialBytes)
	if errors.Is(err, os.ErrNotExist) {
		return out
	}
	if err == nil {
		data = bytes.TrimSpace(data)
		if rest, found := bytes.CutPrefix(data, []byte(antigravityKeyringPrefix)); found {
			data, err = base64.StdEncoding.DecodeString(string(rest))
		}
	}
	type token struct {
		AccessToken string    `json:"access_token"`
		Expiry      time.Time `json:"expiry"`
		IDToken     string    `json:"id_token"`
	}
	var file struct {
		Token   *token `json:"token"`
		IDToken string `json:"id_token"`
	}
	if err != nil || json.Unmarshal(data, &file) != nil {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	if file.Token == nil || file.Token.AccessToken == "" {
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.token = file.Token.AccessToken
	if !file.Token.Expiry.IsZero() && file.Token.Expiry.UnixMilli() > 0 {
		out.expires = file.Token.Expiry
	}
	idToken := file.IDToken
	if idToken == "" {
		idToken = file.Token.IDToken
	}
	var identity struct {
		Email   string `json:"email"`
		Subject string `json:"sub"`
	}
	if jwtClaims(idToken, &identity) {
		out.account.Account = label(identity.Email, 128)
	}
	out.identity = "antigravity\x00" + identity.Subject + "\x00" + out.account.Account
	return out
}

// askAntigravity reads the allowance of an Antigravity plan: a five-hour and a
// weekly window for the plan's own models, and the same two for other models.
// Each is a share left and the time it starts again. The daily endpoint is
// asked first; when it fails for a reason other than the sign-in, the stable
// one is asked once.
func askAntigravity(ctx context.Context, r *Reader, s signIn) allowance {
	headers := map[string]string{"User-Agent": "antigravity"}
	got := r.send(ctx, http.MethodPost, r.opts.AntigravityUsageURL, "Bearer "+s.token, []byte("{}"), headers)
	if got.state != terminalproto.AccountLimitsRead && got.state != terminalproto.AccountLimitsRefused {
		got = r.send(ctx, http.MethodPost, r.opts.AntigravityFallbackUsageURL, "Bearer "+s.token, []byte("{}"), headers)
	}
	if got.state != terminalproto.AccountLimitsRead {
		return allowance{state: got.state, wait: got.wait}
	}
	type bucket struct {
		ID        string   `json:"bucketId"`
		Remaining *float64 `json:"remainingFraction"`
		ResetTime string   `json:"resetTime"`
	}
	var doc struct {
		Groups *[]struct {
			Buckets []bucket `json:"buckets"`
		} `json:"groups"`
	}
	if json.Unmarshal(got.body, &doc) != nil || doc.Groups == nil {
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	found, some := map[string]bucket{}, false
	for _, group := range *doc.Groups {
		for _, b := range group.Buckets {
			some = true
			if _, seen := found[b.ID]; !seen {
				found[b.ID] = b
			}
		}
	}
	if !some {
		// Signed in, but the account has no plan allowance to read.
		return allowance{state: terminalproto.AccountLimitsUnsupported}
	}
	var limits []terminalproto.AccountLimit
	for _, w := range []struct {
		id, label string
		kind      terminalproto.AccountLimitKind
	}{
		{"gemini-5h", "", terminalproto.AccountLimitSession},
		{"gemini-weekly", "", terminalproto.AccountLimitWeek},
		{"3p-5h", antigravityOtherModels, terminalproto.AccountLimitSession},
		{"3p-weekly", antigravityOtherModels, terminalproto.AccountLimitWeek},
	} {
		b, ok := found[w.id]
		if !ok {
			continue
		}
		left := 0.0 // a bucket that does not say what is left has none left
		if b.Remaining != nil {
			left = *b.Remaining
		}
		limit := terminalproto.AccountLimit{Kind: w.kind, Label: w.label, UsedPercent: percent(100 - left*100)}
		if at, err := time.Parse(time.RFC3339, b.ResetTime); err == nil && at.UnixMilli() > 0 {
			limit.ResetsAt = at.UnixMilli()
		}
		limits = append(limits, limit)
	}
	if len(limits) == 0 {
		// Buckets this reader does not know are an answer in another shape.
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	return allowance{limits: limits, state: terminalproto.AccountLimitsRead}
}
