package agentaccounts

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"

	// OpenCode 2 keeps its sign-in in SQLite; the driver is the pure-Go one
	// the rest of reet already uses.
	_ "modernc.org/sqlite"
)

const (
	// openCodeStoreBusy is how long, in milliseconds, reading OpenCode's own
	// store waits for OpenCode to finish a write.
	openCodeStoreBusy = "250"
	// openCodeStoreWait bounds the whole read of the store.
	openCodeStoreWait = 2 * time.Second
)

// openCodeIntegrations are the sign-ins that can carry an OpenCode Go plan,
// the Go one first.
var openCodeIntegrations = []string{"opencode-go", "opencode"}

// openCode reads OpenCode's key for its own models (OpenCode Zen, and the Go
// plan on top of it). Its folder is opencode in $XDG_DATA_HOME, or in
// ~/.local/share. The key comes from the first of:
//
//   - OpenCode 2's store, opencode.db in that folder or the file OPENCODE_DB
//     names, when it has both the session_v2 and the credential tables. Its
//     auth.json is then out of date and is not read.
//   - auth.json in that folder.
//   - OPENCODE_API_KEY in the connector's own environment.
//
// A key is a sign-in to a plan only if the provider says so: until it is
// asked the account has no name or plan, and when it answers that the key has
// no Go plan there is nothing to show.
func (r *Reader) openCode() signIn {
	out := signIn{account: terminalproto.AgentAccount{Tool: "opencode", State: terminalproto.AccountNone}, ask: askOpenCode}
	dir, _, ok := r.folder("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if !ok {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	dir = filepath.Join(dir, "opencode")
	db := filepath.Join(dir, "opencode.db")
	if named := r.env("OPENCODE_DB"); named != "" {
		if !filepath.IsAbs(named) {
			out.account.State = terminalproto.AccountUnreadable
			return out
		}
		db = filepath.Clean(named)
	}
	key, store, err := openCodeStoreKey(db)
	if err != nil {
		out.account.State = terminalproto.AccountUnreadable
		return out
	}
	if !store {
		key, err = openCodeAuthKey(filepath.Join(dir, "auth.json"))
		if err != nil {
			out.account.State = terminalproto.AccountUnreadable
			return out
		}
	}
	if key == "" {
		key = strings.TrimSpace(r.env("OPENCODE_API_KEY"))
	}
	if key == "" || len(key) > maxCredentialBytes {
		return out
	}
	out.account.State, out.account.Method = terminalproto.AccountSignedIn, terminalproto.AccountSubscription
	out.token = key
	// There is no account name to tell keys apart by, so a mark of the key
	// does, in memory only: figures read with one key are not shown for the next.
	out.identity = "opencode\x00" + keyMark(key)
	return out
}

// keyMark tells one key from another without keeping the key: a keyed digest
// under a secret made afresh for this process, so the mark means nothing
// outside it and cannot be tried against guesses of the key.
func keyMark(key string) string {
	markOnce.Do(func() {
		if _, err := rand.Read(markSecret[:]); err != nil {
			panic("agentaccounts: no randomness for the key mark: " + err.Error())
		}
	})
	mac := hmac.New(sha256.New, markSecret[:])
	mac.Write([]byte(key))
	return hex.EncodeToString(mac.Sum(nil))
}

var (
	markOnce   sync.Once
	markSecret [32]byte
)

// openCodeStoreKey reads the key from OpenCode 2's store at path. store is
// false when there is no such store there: no file, or a database without
// OpenCode 2's tables (an older OpenCode's). An error is a store that is
// there but cannot be read.
func openCodeStoreKey(path string) (key string, store bool, err error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return "", false, errUnreadable
	}
	ctx, cancel := context.WithTimeout(context.Background(), openCodeStoreWait)
	defer cancel()
	// Opened read-only and never written. While OpenCode has the store open
	// in WAL mode its -wal and -shm files are there and are shared with it;
	// otherwise the store is opened immutable, so not even those are made.
	query := url.Values{"_pragma": {"busy_timeout(" + openCodeStoreBusy + ")"}}
	query.Set("mode", "ro")
	if !exists(path+"-wal") || !exists(path+"-shm") {
		query.Set("immutable", "1")
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: query.Encode()}
	db, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return "", false, errUnreadable
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var tables int
	if err := db.QueryRowContext(ctx, `select count(*) from sqlite_master where type = 'table' and name in ('session_v2', 'credential')`).Scan(&tables); err != nil {
		return "", false, errUnreadable
	}
	if tables != 2 {
		return "", false, nil
	}
	for _, integration := range openCodeIntegrations {
		var value sql.NullString
		err := db.QueryRowContext(ctx, `select value from credential where integration_id = ? order by active desc, time_created desc, id desc limit 1`, integration).Scan(&value)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil || len(value.String) > maxCredentialBytes {
			return "", true, errUnreadable
		}
		var credential struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		}
		if json.Unmarshal([]byte(value.String), &credential) != nil {
			return "", true, errUnreadable
		}
		if credential.Type == "key" && credential.Key != "" {
			return credential.Key, true, nil
		}
	}
	return "", true, nil
}

// openCodeAuthKey reads the key from OpenCode's auth.json. A missing file, or
// one without an OpenCode key, is no key.
func openCodeAuthKey(path string) (string, error) {
	data, err := readFile(path, maxCredentialBytes)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	var file map[string]json.RawMessage
	if err != nil || json.Unmarshal(data, &file) != nil {
		return "", errUnreadable
	}
	for _, integration := range openCodeIntegrations {
		var entry struct {
			Type string `json:"type"`
			Key  string `json:"key"`
		}
		if raw, ok := file[integration]; ok && json.Unmarshal(raw, &entry) == nil && entry.Type == "api" && entry.Key != "" {
			return entry.Key, nil
		}
	}
	return "", nil
}

// askOpenCode reads the allowance of an OpenCode Go plan: its rolling
// (session), weekly and monthly windows, each a used percentage and the time it
// starts again. A key without a Go plan is refused with 403, which says there
// is no plan to read rather than that the key is bad.
func askOpenCode(ctx context.Context, r *Reader, s signIn) allowance {
	got := r.send(ctx, http.MethodGet, r.opts.OpenCodeUsageURL, "Bearer "+s.token, nil, nil)
	if got.status == http.StatusForbidden {
		return allowance{state: terminalproto.AccountLimitsUnsupported}
	}
	if got.state != terminalproto.AccountLimitsRead {
		return allowance{state: got.state, wait: got.wait}
	}
	type window struct {
		Percent  *float64        `json:"percent"`
		ResetsAt json.RawMessage `json:"resetsAt"`
	}
	var doc struct {
		Plan  *string `json:"plan"`
		Usage *struct {
			Rolling *window `json:"rolling"`
			Weekly  *window `json:"weekly"`
			Monthly *window `json:"monthly"`
		} `json:"usage"`
	}
	if json.Unmarshal(got.body, &doc) != nil || doc.Usage == nil {
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	var limits []terminalproto.AccountLimit
	add := func(w *window, kind terminalproto.AccountLimitKind) {
		if w == nil || w.Percent == nil {
			return
		}
		limits = append(limits, terminalproto.AccountLimit{Kind: kind, UsedPercent: percent(*w.Percent), ResetsAt: resetTime(w.ResetsAt)})
	}
	add(doc.Usage.Rolling, terminalproto.AccountLimitSession)
	add(doc.Usage.Weekly, terminalproto.AccountLimitWeek)
	add(doc.Usage.Monthly, terminalproto.AccountLimitMonth)
	if len(limits) == 0 {
		return allowance{state: terminalproto.AccountLimitsFailed}
	}
	plan := "go"
	if doc.Plan != nil {
		plan = label(strings.ToLower(*doc.Plan), 32)
	}
	return allowance{limits: limits, state: terminalproto.AccountLimitsRead, plan: plan}
}

// resetTime reads when a window starts again, in milliseconds: a time in
// RFC 3339, or a number of seconds or milliseconds since 1970. Anything else
// is zero, "not said".
func resetTime(raw json.RawMessage) int64 {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if at, err := time.Parse(time.RFC3339, text); err == nil && at.UnixMilli() > 0 {
			return at.UnixMilli()
		}
		return 0
	}
	var number float64
	if json.Unmarshal(raw, &number) != nil || number <= 0 {
		return 0
	}
	switch {
	case number < 1e11: // seconds
		return int64(number) * 1000
	case number < 1e14: // milliseconds
		return int64(number)
	}
	return 0
}

// exists is true for a path that is there, of whatever kind.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
