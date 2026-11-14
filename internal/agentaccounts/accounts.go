// Package agentaccounts reads, on the machine, what an AI agent's own sign-in
// says: whether one is there, whose it is and, when asked, how much of the
// account's plan allowance is left.
//
// The rules it keeps:
//
//   - A credential is read only to be used, from this machine, against the
//     agent's own provider. It is never returned, logged or put in an error.
//   - A sign-in is never renewed or rewritten. Agents rotate their renewal
//     tokens, so renewing one here could sign the agent out. A token that has
//     run out is reported as needing renewal, which the agent does itself the
//     next time it runs.
//   - No agent is started and no command is run.
//   - Without Limits nothing is sent anywhere: the answer comes from files.
package agentaccounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const (
	// The providers' own endpoints for an account's allowance.
	claudeUsageURL = "https://api.anthropic.com/api/oauth/usage"
	codexUsageURL  = "https://chatgpt.com/backend-api/wham/usage"

	// maxCredentialBytes bounds a sign-in file; real ones are a few kilobytes.
	maxCredentialBytes = 256 << 10
	// maxProfileBytes bounds Claude's settings file, which also holds
	// per-project history and grows to megabytes.
	maxProfileBytes = 16 << 20
	// maxUsageBytes bounds a provider's answer.
	maxUsageBytes = 256 << 10

	// freshFor is how long read figures answer without asking again.
	freshFor = 5 * time.Minute
	// askFloor is the least time between two requests for one agent, a
	// person-pressed refresh included.
	askFloor = 30 * time.Second
	// retryAfter is how long a failure stands before an unprompted retry.
	retryAfter = time.Minute
	// tokenMargin treats a token about to run out as run out, so a request
	// is never sent with one that expires on the way.
	tokenMargin = 30 * time.Second
	// requestTimeout bounds one request to a provider.
	requestTimeout = 10 * time.Second
)

// Options configure a Reader. The zero value of every field but Home works.
type Options struct {
	// Home is the enrolled user's home folder.
	Home string
	// Env is the environment the agents would run with; it may move their
	// folders (CLAUDE_CONFIG_DIR, CODEX_HOME). Nil reads none.
	Env []string
	// GOOS is the operating system; empty means this one. Claude Code keeps
	// its sign-in in the keychain on macOS, which is not opened.
	GOOS string
	// Client makes the allowance requests. Nil uses a client that follows no
	// redirect, so a sign-in is never sent on to another host.
	Client *http.Client
	// ClaudeUsageURL and CodexUsageURL replace the providers' endpoints in tests.
	ClaudeUsageURL, CodexUsageURL string
	// Now replaces the clock in tests.
	Now func() time.Time
}

// Reader reads the agents' sign-ins and remembers the last allowance figures
// it read for each, for as long as the process lives.
type Reader struct {
	opts Options
	mu   sync.Mutex
	last map[string]remembered
}

// remembered is the last allowance read for one agent and account.
type remembered struct {
	identity  string
	limits    []terminalproto.AccountLimit
	limitsAt  time.Time // when limits were read; zero when none ever were
	state     terminalproto.AccountLimitsState
	askedAt   time.Time
	retryFrom time.Time // a provider's own "not before", after a rate limit
}

// New returns a Reader for one machine user.
func New(opts Options) *Reader {
	if opts.Client == nil {
		opts.Client = &http.Client{
			Timeout:       requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}
	}
	if opts.ClaudeUsageURL == "" {
		opts.ClaudeUsageURL = claudeUsageURL
	}
	if opts.CodexUsageURL == "" {
		opts.CodexUsageURL = codexUsageURL
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Reader{opts: opts, last: map[string]remembered{}}
}

// signIn is one agent's sign-in as its files give it. token and expires are
// used only to ask for the allowance and never leave this package.
type signIn struct {
	account terminalproto.AgentAccount
	token   string
	expires time.Time // zero when the sign-in does not say
	// identity tells one account from another, so figures read for one are
	// never shown for the next.
	identity string
	// accountID is the provider's own id for the account, sent with the
	// request where the provider wants it.
	accountID string
	// ask reads the allowance with token.
	ask func(ctx context.Context, r *Reader, s signIn) ([]terminalproto.AccountLimit, terminalproto.AccountLimitsState, time.Duration)
}

// Read answers for every agent with a reviewed reader, in a fixed order. With
// limits it also reads each signed-in plan's allowance from its provider, the
// providers side by side; refresh asks again instead of answering figures
// read in the last few minutes. It never returns an error: what could not be
// read is a state.
func (r *Reader) Read(ctx context.Context, limits, refresh bool) []terminalproto.AgentAccount {
	readers := []func() signIn{r.claude, r.codex}
	out := make([]terminalproto.AgentAccount, len(readers))
	var wg sync.WaitGroup
	for i, read := range readers {
		s := read()
		a := s.account
		a.LimitsState = terminalproto.AccountLimitsNotAsked
		switch {
		case a.State != terminalproto.AccountSignedIn:
		case a.Method != terminalproto.AccountSubscription:
			a.LimitsState = terminalproto.AccountLimitsUnsupported
		case limits:
			wg.Add(1)
			go func() { defer wg.Done(); out[i] = r.withLimits(ctx, s, refresh) }()
			continue
		}
		out[i] = a
	}
	wg.Wait()
	return out
}

// withLimits fills in the allowance for a signed-in plan: from memory while
// the figures are fresh, otherwise from the provider. A failure keeps the last
// figures read for the same account and says what went wrong.
func (r *Reader) withLimits(ctx context.Context, s signIn, refresh bool) terminalproto.AgentAccount {
	a, now := s.account, r.opts.Now()
	r.mu.Lock()
	kept, known := r.last[a.Tool]
	if known && kept.identity != s.identity {
		// Another account signed in: the old figures are not this one's.
		kept, known = remembered{}, false
		delete(r.last, a.Tool)
	}
	r.mu.Unlock()

	answer := func(k remembered) terminalproto.AgentAccount {
		a.LimitsState = k.state
		if len(k.limits) > 0 {
			a.Limits, a.LimitsAt = append([]terminalproto.AccountLimit(nil), k.limits...), k.limitsAt.UnixMilli()
		}
		return a
	}
	if known {
		sinceAsk := now.Sub(kept.askedAt)
		fresh := kept.state == terminalproto.AccountLimitsRead && sinceAsk < freshFor && !refresh
		tooSoon := sinceAsk < askFloor || now.Before(kept.retryFrom)
		waiting := kept.state != terminalproto.AccountLimitsRead && sinceAsk < retryAfter && !refresh
		if fresh || tooSoon || waiting {
			return answer(kept)
		}
	}
	next := remembered{identity: s.identity, limits: kept.limits, limitsAt: kept.limitsAt, askedAt: now}
	if !s.expires.IsZero() && !s.expires.After(now.Add(tokenMargin)) {
		// Nothing is sent with a token that has run out, and it is not renewed.
		next.state = terminalproto.AccountLimitsRenew
	} else {
		limits, state, wait := s.ask(ctx, r, s)
		next.state = state
		if state == terminalproto.AccountLimitsRead {
			next.limits, next.limitsAt = limits, now
		}
		if wait > 0 {
			next.retryFrom = now.Add(wait)
		}
	}
	r.mu.Lock()
	r.last[a.Tool] = next
	r.mu.Unlock()
	return answer(next)
}

// get asks a provider for an allowance document. The token goes in the
// Authorization header of this one request and nowhere else. It returns the
// body on 200; otherwise the state the status means and, for a rate limit,
// how long the provider asked to wait.
func (r *Reader) get(ctx context.Context, url, token string, headers map[string]string) ([]byte, terminalproto.AccountLimitsState, time.Duration) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "reet")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := r.opts.Client.Do(req)
	if err != nil {
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, terminalproto.AccountLimitsRefused, 0
	case http.StatusTooManyRequests:
		return nil, terminalproto.AccountLimitsRateLimited, retryAfterHeader(res.Header.Get("Retry-After"))
	default:
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, maxUsageBytes+1))
	if err != nil || len(body) > maxUsageBytes {
		return nil, terminalproto.AccountLimitsFailed, 0
	}
	return body, terminalproto.AccountLimitsRead, 0
}

// retryAfterHeader reads a Retry-After in seconds, bounded to an hour. A date
// or anything else is ignored: the ordinary retry pause applies.
func retryAfterHeader(value string) time.Duration {
	seconds := 0
	for _, c := range strings.TrimSpace(value) {
		if c < '0' || c > '9' || seconds > 3600 {
			return 0
		}
		seconds = seconds*10 + int(c-'0')
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

// env returns the value of key in the reader's environment, or "".
func (r *Reader) env(key string) string {
	value := ""
	for _, pair := range r.opts.Env {
		if name, v, ok := strings.Cut(pair, "="); ok && name == key {
			value = v
		}
	}
	return value
}

// folder is where an agent keeps its files: the absolute folder its
// environment variable names, or the default under the home. ok is false for
// a variable that is set but not absolute, which the agent itself would
// resolve against a working directory this reader does not have.
func (r *Reader) folder(variable, fallback string) (dir string, moved, ok bool) {
	if value := r.env(variable); value != "" {
		if !filepath.IsAbs(value) {
			return "", true, false
		}
		return filepath.Clean(value), true, true
	}
	if !filepath.IsAbs(r.opts.Home) {
		return "", false, false
	}
	return filepath.Join(r.opts.Home, fallback), false, true
}

var errUnreadable = errors.New("unreadable")

// readFile reads a plain file of at most limit bytes. A missing file is
// os.ErrNotExist; anything that is not a plain file, is larger, or cannot be
// read is errUnreadable.
func readFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, errUnreadable
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errUnreadable
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errUnreadable
	}
	return data, nil
}

// label makes a provider's word safe to show: no control or invisible
// characters, trimmed, at most max characters. Anything else is dropped whole
// rather than repaired.
func label(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return ""
	}
	for _, c := range s {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == ' ' || c == ' ' {
			return ""
		}
	}
	return s
}

// jwtClaims decodes the claims of a JSON web token without checking its
// signature: they are read for a name and an expiry to show, never trusted
// for access.
func jwtClaims(token string, into any) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(parts[1]) > maxCredentialBytes {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, into) == nil
}

// percent rounds a provider's used share to a whole percentage within 0–100.
func percent(used float64) int {
	switch {
	case used != used || used <= 0: // NaN reads as nothing used
		return 0
	case used >= 100:
		return 100
	}
	return int(used + 0.5)
}
