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
// ANTIGRAVITY_APP_DATA_DIR names), or from the desktop keyring the agent's own
// keyring library writes to (the Linux Secret Service through secret-tool, or
// the macOS login keychain). The later of the two is used. The access token
// and its expiry come from the sign-in; the address is a claim of its identity
// token.
func (r *Reader) antigravity() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "antigravity", State: terminalproto.AccountNone}, ask: askAntigravity}
	dir, _, ok := r.folder("ANTIGRAVITY_APP_DATA_DIR", filepath.Join(".gemini", "antigravity-cli"))
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	data, readErr := readFile(filepath.Join(dir, "antigravity-oauth-token"), maxCredentialBytes)
	file, fileOK, fileReadable := antigravitySignIn(data)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		fileOK, fileReadable = false, false
	}
	// A CLI keeps its sign-in in the keyring when the desktop session has one,
	// and refreshes the file alone when it does not: the later token wins.
	var item string
	var keychainOK bool
	if item, keychainOK = r.linuxKeyringItem("gemini", "antigravity"); !keychainOK {
		item, keychainOK = r.macKeychainItem("gemini", "antigravity")
	}
	if keychainOK {
		if keychain, parsed, readable := antigravitySignIn([]byte(item)); readable && parsed && (!fileOK || keychain.ExpiresAt > file.ExpiresAt) {
			file, fileOK = keychain, true
		}
	}
	if !fileOK {
		if !fileReadable {
			out.account.State = terminalproto.AccountUnreadable
		}
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.token = file.AccessToken
	if file.ExpiresAt > 0 {
		out.expires = time.UnixMilli(file.ExpiresAt)
	}
	var identity struct {
		Email   string `json:"email"`
		Subject string `json:"sub"`
	}
	if jwtClaims(file.IDToken, &identity) {
		out.account.Account = label(identity.Email, 128)
	}
	if identity.Subject != "" || out.account.Account != "" {
		out.identity = "antigravity\x00" + identity.Subject + "\x00" + out.account.Account
	} else {
		// No identity token: a mark of the sign-in tells it from the next,
		// so a new sign-in is not shown the last one's figures. The refresh
		// token is marked when there is one, as it outlives each access token.
		key := file.RefreshToken
		if key == "" {
			key = file.AccessToken
		}
		out.identity = "antigravity\x00\x00\x00" + keyMark(key)
	}
	return out
}

// antigravityAuth is the part of Antigravity's sign-in this package uses.
type antigravityAuth struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresAt    int64 // milliseconds since 1970, 0 when not said
}

// antigravitySignIn reads a sign-in document, plain or in the keyring
// library's base64 form.
func antigravitySignIn(data []byte) (antigravityAuth, bool, bool) {
	if len(data) == 0 {
		return antigravityAuth{}, false, true
	}
	data = bytes.TrimSpace(data)
	if rest, found := bytes.CutPrefix(data, []byte(antigravityKeyringPrefix)); found {
		decoded, err := base64.StdEncoding.DecodeString(string(rest))
		if err != nil {
			return antigravityAuth{}, false, false
		}
		data = decoded
	}
	type token struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		Expiry       time.Time `json:"expiry"`
		IDToken      string    `json:"id_token"`
	}
	var file struct {
		Token   *token `json:"token"`
		IDToken string `json:"id_token"`
	}
	if json.Unmarshal(data, &file) != nil {
		return antigravityAuth{}, false, false
	}
	if file.Token == nil || file.Token.AccessToken == "" {
		return antigravityAuth{}, false, true
	}
	out := antigravityAuth{AccessToken: file.Token.AccessToken, RefreshToken: file.Token.RefreshToken, IDToken: file.Token.IDToken}
	if out.IDToken == "" {
		out.IDToken = file.IDToken
	}
	if !file.Token.Expiry.IsZero() && file.Token.Expiry.UnixMilli() > 0 {
		out.ExpiresAt = file.Token.Expiry.UnixMilli()
	}
	return out, true, true
}

// askAntigravity reads the allowance of an Antigravity plan: a five-hour and a
// weekly window for the plan's own models, and the same two for other models.
// Each is a share left and the time it starts again. The daily endpoint is
// asked first; only when it fails (no answer, an error, or not found) is the
// stable one asked, once. A refused sign-in or a rate limit is the provider's
// answer and stands, with its Retry-After.
//
// The request names itself as the agent's own client (User-Agent:
// antigravity), as the agent does.
func askAntigravity(ctx context.Context, r *Reader, s signIn) allowance {
	headers := map[string]string{"User-Agent": "antigravity"}
	got := r.send(ctx, http.MethodPost, r.opts.AntigravityUsageURL, "Bearer "+s.token, []byte("{}"), headers)
	if got.state == terminalproto.AccountLimitsFailed {
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
		limits = append(limits, terminalproto.AccountLimit{Kind: w.kind, Label: w.label, UsedPercent: percent(100 - left*100), ResetsAt: rfc3339Millis(b.ResetTime)})
	}
	if len(limits) == 0 {
		// Buckets this reader does not know are an answer in another shape.
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	return allowance{limits: limits, state: terminalproto.AccountLimitsRead}
}
