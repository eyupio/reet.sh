package agentaccounts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// Every credential a test writes carries this marker, so a test can say that
// none of it ever appears in an answer.
const secret = "SECRET-DO-NOT-LEAK"

var testNow = time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)

// provider is a stand-in for the agents' providers: it records what it was
// sent and answers what the test set.
type provider struct {
	t      *testing.T
	server *httptest.Server
	mu     sync.Mutex
	calls  []*http.Request
	status int
	body   string
	header http.Header
}

func newProvider(t *testing.T) *provider {
	p := &provider{t: t, status: 200, header: http.Header{}}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.calls = append(p.calls, r.Clone(context.Background()))
		for key, values := range p.header {
			w.Header()[key] = values
		}
		w.WriteHeader(p.status)
		fmt.Fprint(w, p.body)
	}))
	t.Cleanup(p.server.Close)
	return p
}

func (p *provider) answer(status int, body string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status, p.body = status, body
}

func (p *provider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func (p *provider) last() *http.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		p.t.Fatal("the provider was not asked")
	}
	return p.calls[len(p.calls)-1]
}

type fixture struct {
	t      *testing.T
	home   string
	claude *provider
	codex  *provider
	now    time.Time
	reader *Reader
}

func newFixture(t *testing.T, env ...string) *fixture {
	f := &fixture{t: t, home: t.TempDir(), claude: newProvider(t), codex: newProvider(t), now: testNow}
	f.reader = New(Options{
		Home: f.home, Env: env, GOOS: "linux",
		ClaudeUsageURL: f.claude.server.URL + "/api/oauth/usage", CodexUsageURL: f.codex.server.URL + "/backend-api/wham/usage",
		Now: func() time.Time { return f.now },
	})
	return f
}

func (f *fixture) write(rel, body string) {
	f.t.Helper()
	path := filepath.Join(f.home, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// read answers the two agents and checks the rule that holds for every
// answer: it is a valid protocol answer and nothing of a credential is in it.
func (f *fixture) read(limits, refresh bool) (claude, codex terminalproto.AgentAccount) {
	f.t.Helper()
	accounts := f.reader.Read(context.Background(), limits, refresh)
	if len(accounts) != 2 || accounts[0].Tool != "claude" || accounts[1].Tool != "codex" {
		f.t.Fatalf("answer is not claude then codex: %+v", accounts)
	}
	message := terminalproto.Message{Version: terminalproto.Version, RequestID: strings.Repeat("a", 32), Generation: 1, Body: terminalproto.AccountsResult{Accounts: accounts}}
	wire, err := terminalproto.Encode(message)
	if err != nil {
		f.t.Fatalf("answer is not a valid protocol answer: %v %+v", err, accounts)
	}
	if strings.Contains(string(wire), secret) {
		f.t.Fatalf("a credential is in the answer: %s", wire)
	}
	return accounts[0], accounts[1]
}

func claudeCredentials(expires time.Time) string {
	return fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-%s","refreshToken":"sk-ant-ort01-%s","expiresAt":%d,"scopes":["user:inference"],"subscriptionType":"max"}}`, secret, secret, expires.UnixMilli())
}

const claudeProfile = `{"numStartups":4,"oauthAccount":{"accountUuid":"11111111-2222-3333-4444-555555555555","emailAddress":"person@example.com","organizationName":"Example"},"projects":{}}`

func jwt(claims map[string]any) string {
	part := func(v any) string { raw, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(raw) }
	return part(map[string]string{"alg": "RS256"}) + "." + part(claims) + "." + base64.RawURLEncoding.EncodeToString([]byte(secret))
}

func codexAuth(expires time.Time) string {
	id := jwt(map[string]any{"email": "person@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_plan_type": "plus", "chatgpt_account_id": "acct-1"}})
	access := jwt(map[string]any{"exp": expires.Unix(), "marker": secret})
	return fmt.Sprintf(`{"OPENAI_API_KEY":null,"tokens":{"id_token":%q,"access_token":%q,"refresh_token":"rt-%s","account_id":"acct-1"},"last_refresh":"2026-11-14T08:00:00Z"}`, id, access, secret)
}

const claudeUsage = `{"five_hour":{"utilization":19.4,"resets_at":"2026-11-14T12:00:00.000000+00:00"},"seven_day":{"utilization":60.5,"resets_at":"2026-11-18T00:00:00Z"},"seven_day_opus":null,"seven_day_sonnet":{"utilization":3,"resets_at":null},"extra_usage":{"is_enabled":false}}`

const codexUsage = `{"plan_type":"plus","rate_limit":{"allowed":true,"limit_reached":false,"primary_window":{"used_percent":7,"limit_window_seconds":18000,"reset_after_seconds":600,"reset_at":1794650400},"secondary_window":{"used_percent":41.6,"limit_window_seconds":604800,"reset_after_seconds":86400}},"credits":{"has_credits":false}}`

func TestNoSignInIsNoneAndAsksNobody(t *testing.T) {
	f := newFixture(t)
	claude, codex := f.read(true, true)
	for _, a := range []terminalproto.AgentAccount{claude, codex} {
		if a.State != terminalproto.AccountNone || a.LimitsState != terminalproto.AccountLimitsNotAsked || a.Account != "" || a.Method != "" {
			t.Fatalf("an agent with no sign-in: %+v", a)
		}
	}
	if f.claude.count()+f.codex.count() != 0 {
		t.Fatal("a provider was asked with no sign-in")
	}
}

func TestSignInIsReadFromFilesWithoutTheNetwork(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
	f.write(".claude.json", claudeProfile)
	f.write(".codex/auth.json", codexAuth(f.now.Add(time.Hour)))
	claude, codex := f.read(false, false)
	want := terminalproto.AgentAccount{Tool: "claude", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", Plan: "max", LimitsState: terminalproto.AccountLimitsNotAsked}
	if fmt.Sprint(claude) != fmt.Sprint(want) {
		t.Fatalf("claude = %+v, want %+v", claude, want)
	}
	want = terminalproto.AgentAccount{Tool: "codex", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", Plan: "plus", LimitsState: terminalproto.AccountLimitsNotAsked}
	if fmt.Sprint(codex) != fmt.Sprint(want) {
		t.Fatalf("codex = %+v, want %+v", codex, want)
	}
	if f.claude.count()+f.codex.count() != 0 {
		t.Fatal("a provider was asked though the allowance was not")
	}
}

func TestLimitsAreReadWithTheAgentsOwnSignIn(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
	f.write(".claude.json", claudeProfile)
	f.write(".codex/auth.json", codexAuth(f.now.Add(time.Hour)))
	f.claude.answer(200, claudeUsage)
	f.codex.answer(200, codexUsage)
	claude, codex := f.read(true, false)

	if claude.LimitsState != terminalproto.AccountLimitsRead || claude.LimitsAt != f.now.UnixMilli() {
		t.Fatalf("claude limits: %+v", claude)
	}
	wantClaude := []terminalproto.AccountLimit{
		{Kind: terminalproto.AccountLimitSession, UsedPercent: 19, ResetsAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC).UnixMilli()},
		{Kind: terminalproto.AccountLimitWeek, UsedPercent: 61, ResetsAt: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC).UnixMilli()},
		{Kind: terminalproto.AccountLimitWeek, Label: "Sonnet", UsedPercent: 3},
	}
	if fmt.Sprint(claude.Limits) != fmt.Sprint(wantClaude) {
		t.Fatalf("claude limits = %+v, want %+v", claude.Limits, wantClaude)
	}
	request := f.claude.last()
	if request.Method != "GET" || request.URL.Path != "/api/oauth/usage" || request.Header.Get("Authorization") != "Bearer sk-ant-oat01-"+secret || request.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
		t.Fatalf("claude request: %s %s %v", request.Method, request.URL, request.Header)
	}

	wantCodex := []terminalproto.AccountLimit{
		{Kind: terminalproto.AccountLimitSession, UsedPercent: 7, ResetsAt: 1794650400000},
		{Kind: terminalproto.AccountLimitWeek, UsedPercent: 42, ResetsAt: f.now.Add(24 * time.Hour).UnixMilli()},
	}
	if codex.LimitsState != terminalproto.AccountLimitsRead || fmt.Sprint(codex.Limits) != fmt.Sprint(wantCodex) {
		t.Fatalf("codex limits = %+v (%s), want %+v", codex.Limits, codex.LimitsState, wantCodex)
	}
	request = f.codex.last()
	if request.URL.Path != "/backend-api/wham/usage" || !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") || request.Header.Get("chatgpt-account-id") != "acct-1" {
		t.Fatalf("codex request: %s %v", request.URL, request.Header)
	}
	for _, request := range []*http.Request{f.claude.last(), f.codex.last()} {
		if request.Header.Get("User-Agent") != "reet" {
			t.Fatalf("the request does not say who is asking: %v", request.Header)
		}
	}
}

func TestFreshFiguresAnswerWithoutAskingAgain(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(24*time.Hour)))
	f.claude.answer(200, claudeUsage)
	first, _ := f.read(true, false)
	f.now = f.now.Add(4 * time.Minute)
	again, _ := f.read(true, false)
	if f.claude.count() != 1 || again.LimitsAt != first.LimitsAt || again.LimitsState != terminalproto.AccountLimitsRead {
		t.Fatalf("figures four minutes old were read again (%d requests): %+v", f.claude.count(), again)
	}
	// A person asking now gets a new read.
	refreshed, _ := f.read(true, true)
	if f.claude.count() != 2 || refreshed.LimitsAt != f.now.UnixMilli() {
		t.Fatalf("a refresh did not ask (%d requests): %+v", f.claude.count(), refreshed)
	}
	// But not twice within the floor, however often it is pressed.
	f.now = f.now.Add(10 * time.Second)
	f.read(true, true)
	if f.claude.count() != 2 {
		t.Fatalf("a refresh inside the floor asked the provider again (%d requests)", f.claude.count())
	}
	// And figures past their time are read again unprompted.
	f.now = f.now.Add(6 * time.Minute)
	f.read(true, false)
	if f.claude.count() != 3 {
		t.Fatalf("stale figures were not read again (%d requests)", f.claude.count())
	}
}

func TestAFailureKeepsTheLastFiguresAndSaysWhy(t *testing.T) {
	for status, want := range map[int]terminalproto.AccountLimitsState{
		401: terminalproto.AccountLimitsRefused, 403: terminalproto.AccountLimitsRefused,
		429: terminalproto.AccountLimitsRateLimited, 500: terminalproto.AccountLimitsFailed, 302: terminalproto.AccountLimitsFailed,
	} {
		f := newFixture(t)
		f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(24*time.Hour)))
		f.claude.answer(200, claudeUsage)
		first, _ := f.read(true, false)
		f.now = f.now.Add(10 * time.Minute)
		f.claude.answer(status, `{"error":"no"}`)
		failed, _ := f.read(true, false)
		if failed.LimitsState != want || failed.LimitsAt != first.LimitsAt || fmt.Sprint(failed.Limits) != fmt.Sprint(first.Limits) {
			t.Fatalf("status %d: %+v, want state %s with the figures of %d", status, failed, want, first.LimitsAt)
		}
		// A failure stands for a minute before an unprompted retry.
		f.now = f.now.Add(40 * time.Second)
		f.read(true, false)
		if f.claude.count() != 2 {
			t.Fatalf("status %d: retried %d seconds after failing", status, 40)
		}
		f.now = f.now.Add(30 * time.Second)
		f.claude.answer(200, claudeUsage)
		recovered, _ := f.read(true, false)
		if recovered.LimitsState != terminalproto.AccountLimitsRead || recovered.LimitsAt != f.now.UnixMilli() {
			t.Fatalf("status %d: did not recover: %+v", status, recovered)
		}
	}
}

func TestARateLimitWaitsAsLongAsTheProviderAsks(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(24*time.Hour)))
	f.claude.header.Set("Retry-After", "600")
	f.claude.answer(429, `{}`)
	f.read(true, false)
	f.now = f.now.Add(5 * time.Minute)
	limited, _ := f.read(true, true)
	if f.claude.count() != 1 || limited.LimitsState != terminalproto.AccountLimitsRateLimited {
		t.Fatalf("asked again inside the provider's wait (%d requests): %+v", f.claude.count(), limited)
	}
	f.now = f.now.Add(6 * time.Minute)
	f.read(true, false)
	if f.claude.count() != 2 {
		t.Fatalf("did not ask once the wait was over (%d requests)", f.claude.count())
	}
}

func TestAnExpiredTokenIsNeverSentAndNeverRenewed(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(10*time.Second)))
	f.write(".codex/auth.json", codexAuth(f.now.Add(-time.Hour)))
	before, _ := os.ReadFile(filepath.Join(f.home, ".claude/.credentials.json"))
	claude, codex := f.read(true, true)
	for _, a := range []terminalproto.AgentAccount{claude, codex} {
		if a.State != terminalproto.AccountSignedIn || a.LimitsState != terminalproto.AccountLimitsRenew || len(a.Limits) != 0 {
			t.Fatalf("an expired sign-in: %+v", a)
		}
	}
	if f.claude.count()+f.codex.count() != 0 {
		t.Fatal("an expired token was sent")
	}
	after, _ := os.ReadFile(filepath.Join(f.home, ".claude/.credentials.json"))
	if string(before) != string(after) {
		t.Fatal("the sign-in file was changed")
	}
}

func TestAnAPIKeyIsASignInWithNoPlan(t *testing.T) {
	f := newFixture(t)
	f.write(".codex/auth.json", fmt.Sprintf(`{"OPENAI_API_KEY":"sk-%s","tokens":null}`, secret))
	_, codex := f.read(true, true)
	if codex.State != terminalproto.AccountSignedIn || codex.Method != terminalproto.AccountAPIKey || codex.LimitsState != terminalproto.AccountLimitsUnsupported || codex.Account != "" {
		t.Fatalf("an API key: %+v", codex)
	}
	if f.codex.count() != 0 {
		t.Fatal("the provider was asked for a plan an API key does not have")
	}
}

func TestAnotherAccountDoesNotInheritFigures(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(24*time.Hour)))
	f.write(".claude.json", claudeProfile)
	f.claude.answer(200, claudeUsage)
	f.read(true, false)
	f.write(".claude.json", strings.Replace(strings.Replace(claudeProfile, "person@", "other@", 1), "11111111", "99999999", 1))
	f.now = f.now.Add(time.Minute)
	f.claude.answer(500, `{}`)
	other, _ := f.read(true, false)
	if other.Account != "other@example.com" || len(other.Limits) != 0 || other.LimitsAt != 0 || other.LimitsState != terminalproto.AccountLimitsFailed {
		t.Fatalf("a second account was shown the first one's figures: %+v", other)
	}
}

func TestMovedFoldersAreFollowed(t *testing.T) {
	moved := t.TempDir()
	f := newFixture(t, "CLAUDE_CONFIG_DIR="+filepath.Join(moved, "claude"), "CODEX_HOME="+filepath.Join(moved, "codex"))
	// The default places hold someone else's sign-in, which must not be read.
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
	f.write(".claude.json", strings.Replace(claudeProfile, "person@", "wrong@", 1))
	f.write(".codex/auth.json", codexAuth(f.now.Add(time.Hour)))
	claude, codex := f.read(false, false)
	if claude.State != terminalproto.AccountNone || codex.State != terminalproto.AccountNone {
		t.Fatalf("the default folders were read though the agents were moved: %+v %+v", claude, codex)
	}
	for rel, body := range map[string]string{"claude/.credentials.json": claudeCredentials(f.now.Add(time.Hour)), "claude/.claude.json": claudeProfile, "codex/auth.json": codexAuth(f.now.Add(time.Hour))} {
		path := filepath.Join(moved, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	claude, codex = f.read(false, false)
	if claude.Account != "person@example.com" || codex.Account != "person@example.com" {
		t.Fatalf("the moved folders were not read: %+v %+v", claude, codex)
	}
	// A folder the agent would resolve against its own working directory cannot be followed.
	relative := newFixture(t, "CLAUDE_CONFIG_DIR=claude", "CODEX_HOME=./codex")
	claude, codex = relative.read(true, false)
	if claude.State != terminalproto.AccountUnreadable || codex.State != terminalproto.AccountUnreadable {
		t.Fatalf("a relative folder: %+v %+v", claude, codex)
	}
}

func TestWhatCannotBeReadIsSaidNotGuessed(t *testing.T) {
	f := newFixture(t)
	f.write(".claude/.credentials.json", `{"claudeAiOauth":`)
	f.write(".codex/auth.json", strings.Repeat("x", maxCredentialBytes+1))
	claude, codex := f.read(true, false)
	if claude.State != terminalproto.AccountUnreadable || codex.State != terminalproto.AccountUnreadable {
		t.Fatalf("a broken and an oversized file: %+v %+v", claude, codex)
	}
	// A folder where the file should be is not a sign-in file.
	g := newFixture(t)
	if err := os.MkdirAll(filepath.Join(g.home, ".claude/.credentials.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	claude, _ = g.read(true, false)
	if claude.State != terminalproto.AccountUnreadable {
		t.Fatalf("a folder in the file's place: %+v", claude)
	}
	// An empty sign-in is none, and so is a file that holds something else.
	h := newFixture(t)
	h.write(".claude/.credentials.json", `{"claudeAiOauth":{"accessToken":"","refreshToken":""}}`)
	h.write(".codex/auth.json", `{"OPENAI_API_KEY":null,"tokens":{"access_token":""}}`)
	claude, codex = h.read(true, false)
	if claude.State != terminalproto.AccountNone || codex.State != terminalproto.AccountNone {
		t.Fatalf("empty sign-ins: %+v %+v", claude, codex)
	}
}

func TestMacKeepsClaudeInTheKeychain(t *testing.T) {
	f := newFixture(t)
	f.reader.opts.GOOS = "darwin"
	claude, codex := f.read(true, false)
	if claude.State != terminalproto.AccountUnreadable {
		t.Fatalf("a Mac with no file was called signed out: %+v", claude)
	}
	if codex.State != terminalproto.AccountNone {
		t.Fatalf("codex on a Mac: %+v", codex)
	}
}

func TestAStrangeAnswerIsAFailureNotFigures(t *testing.T) {
	for name, body := range map[string]string{
		"not json": "<html>", "no windows": `{"five_hour":null,"seven_day":null}`, "another shape": `{"limits":[1,2]}`,
		"oversized": `{"five_hour":{"utilization":1,"resets_at":"` + strings.Repeat("x", maxUsageBytes) + `"}}`,
	} {
		f := newFixture(t)
		f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
		f.write(".codex/auth.json", codexAuth(f.now.Add(time.Hour)))
		f.claude.answer(200, body)
		f.codex.answer(200, body)
		claude, codex := f.read(true, false)
		if claude.LimitsState != terminalproto.AccountLimitsFailed || codex.LimitsState != terminalproto.AccountLimitsFailed || len(claude.Limits)+len(codex.Limits) != 0 {
			t.Fatalf("%s: %+v %+v", name, claude, codex)
		}
	}
	// Figures outside 0–100 are held to it, and a name that is not plain text is dropped.
	f := newFixture(t)
	f.write(".claude/.credentials.json", strings.Replace(claudeCredentials(f.now.Add(time.Hour)), `"max"`, `"m\u0007x"`, 1))
	f.write(".claude.json", strings.Replace(claudeProfile, "person@example.com", `per\nson@example.com`, 1))
	f.claude.answer(200, `{"five_hour":{"utilization":250,"resets_at":"soon"},"seven_day":{"utilization":-4,"resets_at":""}}`)
	claude, _ := f.read(true, false)
	if claude.Plan != "" || claude.Account != "" {
		t.Fatalf("a name with control characters was passed on: %+v", claude)
	}
	want := []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitSession, UsedPercent: 100}, {Kind: terminalproto.AccountLimitWeek, UsedPercent: 0}}
	if fmt.Sprint(claude.Limits) != fmt.Sprint(want) {
		t.Fatalf("limits = %+v, want %+v", claude.Limits, want)
	}
}

func TestARedirectIsNotFollowedWithTheSignIn(t *testing.T) {
	elsewhere := newProvider(t)
	f := newFixture(t)
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
	f.claude.header.Set("Location", elsewhere.server.URL+"/collect")
	f.claude.answer(307, "")
	claude, _ := f.read(true, false)
	if elsewhere.count() != 0 {
		t.Fatal("the sign-in followed a redirect to another host")
	}
	if claude.LimitsState != terminalproto.AccountLimitsFailed {
		t.Fatalf("a redirect: %+v", claude)
	}
}

func TestWindowKindFollowsItsLength(t *testing.T) {
	for seconds, want := range map[float64]terminalproto.AccountLimitKind{
		0: terminalproto.AccountLimitSession, 18000: terminalproto.AccountLimitSession, 86400: terminalproto.AccountLimitDay,
		604800: terminalproto.AccountLimitWeek, 2592000: terminalproto.AccountLimitMonth,
	} {
		if got := windowKind(seconds); got != want {
			t.Errorf("windowKind(%v) = %s, want %s", seconds, got, want)
		}
	}
}
