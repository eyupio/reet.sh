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
//   - No agent is started and no command is run: a sign-in kept only in a
//     keychain or keyring, or handed out only by another program's command,
//     is not read.
//   - Without Limits nothing is sent anywhere: the answer comes from files.
package agentaccounts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	claudeUsageURL   = "https://api.anthropic.com/api/oauth/usage"
	codexUsageURL    = "https://chatgpt.com/backend-api/wham/usage"
	openCodeUsageURL = "https://opencode.ai/zen/go/v1/usage"
	// Antigravity's allowance is asked of the daily endpoint first and, when
	// that fails for a reason other than the sign-in, once of the stable one.
	antigravityUsageURL         = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	antigravityFallbackUsageURL = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	copilotUserURL              = "https://api.github.com/copilot_internal/user"

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
	// requestTimeout bounds one request to a provider, and also everything
	// one agent's allowance takes, a second endpoint or a second token
	// included, so the answer stays inside the connector's wait. The
	// providers are asked side by side, so it is also all their time.
	requestTimeout = 9 * time.Second
	// filesMargin is the time allowed for the other sign-in files, each a
	// plain file of at most maxCredentialBytes (maxProfileBytes for Claude's
	// settings), read one after another, and for putting the answer together.
	filesMargin = time.Second
)

// ReadTakesAtMost is the longest a Read takes: the sign-in files are read one
// after another, OpenCode's store taking up to openCodeStoreWait (1 s) and the
// rest filesMargin (1 s), then the providers are asked side by side, for
// requestTimeout (9 s) at most: 1 + 1 + 9 = 11 s. The connector answers a read
// that takes longer timed out after 12 s, so this leaves a second for the
// reply.
const ReadTakesAtMost = openCodeStoreWait + filesMargin + requestTimeout

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
	// The URL fields replace the providers' endpoints in tests.
	ClaudeUsageURL, CodexUsageURL, OpenCodeUsageURL  string
	AntigravityUsageURL, AntigravityFallbackUsageURL string
	CopilotUserURL                                   string
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
	identity string
	// plan and account are what the provider's answer said, for agents whose
	// files do not say (an OpenCode key, a Copilot plan); empty otherwise.
	plan, account string
	limits        []terminalproto.AccountLimit
	limitsAt      time.Time // when limits were read; zero when none ever were
	state         terminalproto.AccountLimitsState
	askedAt       time.Time
	retryFrom     time.Time // a provider's own "not before", after a rate limit
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
	if opts.OpenCodeUsageURL == "" {
		opts.OpenCodeUsageURL = openCodeUsageURL
	}
	if opts.AntigravityUsageURL == "" {
		opts.AntigravityUsageURL = antigravityUsageURL
	}
	if opts.AntigravityFallbackUsageURL == "" {
		opts.AntigravityFallbackUsageURL = antigravityFallbackUsageURL
	}
	if opts.CopilotUserURL == "" {
		opts.CopilotUserURL = copilotUserURL
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
	// alternates are further tokens for the same account, tried in order
	// when the provider says the one before has no plan (Copilot).
	alternates []string
	expires    time.Time // zero when the sign-in does not say
	// identity tells one account from another, so figures read for one are
	// never shown for the next.
	identity string
	// accountID is the provider's own id for the account, sent with the
	// request where the provider wants it.
	accountID string
	// ask reads the allowance with token.
	ask func(ctx context.Context, r *Reader, s signIn) allowance
}

// allowance is what asking a provider came to. limits are set only when state
// is read. plan and account are set only by a provider whose answer names
// them; they replace what the files said.
type allowance struct {
	limits        []terminalproto.AccountLimit
	state         terminalproto.AccountLimitsState
	wait          time.Duration // a provider's own "not before", after a rate limit
	plan, account string
}

// Read answers for every agent with a reviewed reader, in a fixed order. With
// limits it also reads each signed-in plan's allowance from its provider, the
// providers side by side; refresh asks again instead of answering figures
// read in the last few minutes. It never returns an error: what could not be
// read is a state.
func (r *Reader) Read(ctx context.Context, limits, refresh bool) []terminalproto.AgentAccount {
	readers := []func() signIn{r.claude, r.codex, r.openCode, r.antigravity, r.copilot}
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
		if k.plan != "" {
			a.Plan = k.plan
		}
		if k.account != "" {
			a.Account = k.account
		}
		if k.state == terminalproto.AccountLimitsUnsupported {
			// The provider says the account has no plan of the kind read
			// here: there are no figures, old or new, to show for it.
			return a
		}
		if len(k.limits) > 0 {
			a.Limits, a.LimitsAt = append([]terminalproto.AccountLimit(nil), k.limits...), k.limitsAt.UnixMilli()
		}
		return a
	}
	if known {
		sinceAsk := now.Sub(kept.askedAt)
		// An answer that the account has no plan stands as long as figures do.
		settled := kept.state == terminalproto.AccountLimitsRead || kept.state == terminalproto.AccountLimitsUnsupported
		fresh := settled && sinceAsk < freshFor && !refresh
		tooSoon := sinceAsk < askFloor || now.Before(kept.retryFrom)
		waiting := !settled && sinceAsk < retryAfter && !refresh
		if fresh || tooSoon || waiting {
			return answer(kept)
		}
	}
	// Until a new answer says otherwise, what the last one said of the plan
	// stands with its figures.
	next := remembered{identity: s.identity, plan: kept.plan, account: kept.account, limits: kept.limits, limitsAt: kept.limitsAt, askedAt: now}
	if !s.expires.IsZero() && !s.expires.After(now.Add(tokenMargin)) {
		// Nothing is sent with a token that has run out, and it is not renewed.
		next.state = terminalproto.AccountLimitsRenew
	} else {
		askCtx, cancel := context.WithTimeout(ctx, requestTimeout)
		got := s.ask(askCtx, r, s)
		cancel()
		next.state = got.state
		switch got.state {
		case terminalproto.AccountLimitsRead:
			next.limits, next.limitsAt = got.limits, now
			next.plan, next.account = got.plan, got.account
		case terminalproto.AccountLimitsUnsupported:
			next.limits, next.limitsAt = nil, time.Time{}
			next.plan, next.account = got.plan, got.account
		}
		if got.wait > 0 {
			next.retryFrom = now.Add(got.wait)
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
	got := r.send(ctx, http.MethodGet, url, "Bearer "+token, nil, headers)
	return got.body, got.state, got.wait
}

// sent is one provider request's outcome: the body on 200, the HTTP status
// (zero when no answer came) and the state that status means.
type sent struct {
	body   []byte
	status int
	state  terminalproto.AccountLimitsState
	wait   time.Duration
}

// send makes one request to a provider, with authorization as its
// Authorization header and body, when there is one, as JSON. The sign-in goes
// in that header of this one request and nowhere else.
func (r *Reader) send(ctx context.Context, method, url, authorization string, body []byte, headers map[string]string) sent {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var payload io.Reader
	if body != nil {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return sent{state: terminalproto.AccountLimitsFailed}
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "reet")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	res, err := r.opts.Client.Do(req)
	if err != nil {
		return sent{state: terminalproto.AccountLimitsFailed}
	}
	defer res.Body.Close()
	out := sent{status: res.StatusCode}
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		out.state = terminalproto.AccountLimitsRefused
		return out
	case http.StatusTooManyRequests:
		out.state, out.wait = terminalproto.AccountLimitsRateLimited, retryAfterHeader(res.Header.Get("Retry-After"))
		return out
	default:
		out.state = terminalproto.AccountLimitsFailed
		return out
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxUsageBytes+1))
	if err != nil || len(data) > maxUsageBytes {
		out.state = terminalproto.AccountLimitsFailed
		return out
	}
	out.body, out.state = data, terminalproto.AccountLimitsRead
	return out
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
// read is errUnreadable. A link is followed only to a plain file.
//
// The path is looked at before it is opened: opening a named pipe or a device
// can wait for ever, and a sign-in path is no place for either. It is opened
// without waiting too (where the system has that), in case one is put there
// in between, and looked at again once open.
func readFile(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errUnreadable
	}
	f, err := os.OpenFile(path, os.O_RDONLY|openNoWait, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, errUnreadable
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, errUnreadable
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, errUnreadable
	}
	return data, nil
}

// keyMark tells one key from another without keeping the key: a keyed digest
// under a secret made afresh for this process, so the mark means nothing
// outside it and cannot be tried against guesses of the key. It is kept in
// memory only, as part of an identity, where a sign-in has no account name
// to tell it from the next (an OpenCode key, an Antigravity sign-in without
// an identity token).
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

// label makes a provider's word safe to show: no control or invisible
// characters, trimmed, at most max characters. Anything else is dropped whole
// rather than repaired.
func label(s string, max int) string {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || utf8.RuneCountInString(s) > max {
		return ""
	}
	for _, c := range s {
		if unicode.IsControl(c) || unicode.Is(unicode.Cf, c) || c == '\u2028' || c == '\u2029' {
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
