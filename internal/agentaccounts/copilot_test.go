package agentaccounts

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

func copilotToken(name string) string { return "gho_" + name + secret }

func copilotHosts(entries ...[3]string) string {
	var parts []string
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf(`%q:{"user":%q,"oauth_token":%q}`, e[0], e[1], e[2]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

const copilotPaid = `{"login":"Person","access_type_sku":"plus_monthly_subscriber","copilot_plan":"individual_pro","quota_reset_date":"2026-12-01",` +
	`"quota_snapshots":{"premium_interactions":{"entitlement":300,"remaining":120,"percent_remaining":40.2,"unlimited":false},` +
	`"chat":{"entitlement":0,"percent_remaining":100,"unlimited":true},"completions":{"entitlement":0,"percent_remaining":100,"unlimited":true}}}`

const copilotFree = `{"login":"person","access_type_sku":"free_limited_copilot","copilot_plan":"individual","limited_user_reset_date":"2026-12-03T00:00:00Z",` +
	`"monthly_quotas":{"chat":50,"completions":2000},"limited_user_quotas":{"chat":10,"completions":1500}}`

func TestCopilotSignInAndLimits(t *testing.T) {
	f := newFixture(t)
	f.write(".config/github-copilot/hosts.json", copilotHosts([3]string{"github.com", "Person", copilotToken("hosts")}))
	f.write(".config/github-copilot/apps.json", copilotHosts(
		[3]string{"github.com:Iv1.b507a08c87ecfe98", "person", copilotToken("apps")},
		[3]string{"ghe.example.com:Iv1.1", "person", copilotToken("enterprise")},
	))
	a := f.one("copilot", false, false)
	want := terminalproto.AgentAccount{Tool: "copilot", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person", LimitsState: terminalproto.AccountLimitsNotAsked}
	if fmt.Sprint(a) != fmt.Sprint(want) {
		t.Fatalf("copilot = %+v, want %+v", a, want)
	}
	if f.asked() != 0 {
		t.Fatal("a provider was asked though the allowance was not")
	}
	f.copilot.answer(200, copilotPaid)
	a = f.one("copilot", true, false)
	wantLimits := []terminalproto.AccountLimit{{Kind: terminalproto.AccountLimitMonth, Label: "Premium", UsedPercent: 60, ResetsAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC).UnixMilli()}}
	if a.LimitsState != terminalproto.AccountLimitsRead || a.Plan != "individual_pro" || a.Account != "Person" || fmt.Sprint(a.Limits) != fmt.Sprint(wantLimits) {
		t.Fatalf("copilot limits = %+v, want %+v", a, wantLimits)
	}
	request := f.copilot.last()
	for header, value := range map[string]string{
		"Authorization": "token " + copilotToken("apps"), "User-Agent": copilotUserAgent, "Editor-Version": copilotEditorVersion,
		"Editor-Plugin-Version": copilotPluginVersion, "X-GitHub-Api-Version": copilotAPIVersion,
	} {
		if request.Header.Get(header) != value {
			t.Fatalf("copilot request %s = %q, want %q", header, request.Header.Get(header), value)
		}
	}
	if request.Method != "GET" || request.URL.Path != "/copilot_internal/user" || f.copilot.count() != 1 {
		t.Fatalf("copilot request: %s %s, %d requests", request.Method, request.URL, f.copilot.count())
	}
}

func TestCopilotFreePlanIsReadFromItsMonthlyQuotas(t *testing.T) {
	f := newFixture(t)
	f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:app", "person", copilotToken("apps")}))
	f.copilot.answer(200, copilotFree)
	a := f.one("copilot", true, false)
	resets := time.Date(2026, 12, 3, 0, 0, 0, 0, time.UTC).UnixMilli()
	wantLimits := []terminalproto.AccountLimit{
		{Kind: terminalproto.AccountLimitMonth, Label: "Chat", UsedPercent: 80, ResetsAt: resets},
		{Kind: terminalproto.AccountLimitMonth, Label: "Completions", UsedPercent: 25, ResetsAt: resets},
	}
	if a.LimitsState != terminalproto.AccountLimitsRead || a.Plan != "free" || fmt.Sprint(a.Limits) != fmt.Sprint(wantLimits) {
		t.Fatalf("free plan = %+v, want %+v", a, wantLimits)
	}
}

func TestCopilotTriesTheLoginsTokensInTurn(t *testing.T) {
	f := newFixture(t)
	f.write(".config/github-copilot/apps.json", copilotHosts(
		[3]string{"github.com:a", "person", copilotToken("first")},
		[3]string{"github.com:b", "someone-else", copilotToken("other")},
		[3]string{"github.com:c", "PERSON", copilotToken("second")},
	))
	f.copilot.route = func(r *http.Request) (int, string) {
		if r.Header.Get("Authorization") == "token "+copilotToken("second") {
			return 200, copilotPaid
		}
		return 404, `{"message":"Not Found"}`
	}
	a := f.one("copilot", true, false)
	if a.LimitsState != terminalproto.AccountLimitsRead || f.copilot.count() != 2 {
		t.Fatalf("the login's second token: %+v after %d requests", a, f.copilot.count())
	}
	for _, request := range f.copilot.calls {
		if request.Header.Get("Authorization") == "token "+copilotToken("other") {
			t.Fatal("another login's token was sent")
		}
	}
	// No token with Copilot: signed in, with no plan to read.
	g := newFixture(t)
	g.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}, [3]string{"github.com:b", "person", copilotToken("b")}))
	g.copilot.answer(404, `{}`)
	a = g.one("copilot", true, false)
	if a.State != terminalproto.AccountSignedIn || a.LimitsState != terminalproto.AccountLimitsUnsupported || len(a.Limits) != 0 || g.copilot.count() != 2 {
		t.Fatalf("no Copilot: %+v after %d requests", a, g.copilot.count())
	}
	// A refused token is refused.
	h := newFixture(t)
	h.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
	h.copilot.answer(401, `{}`)
	if a := h.one("copilot", true, false); a.LimitsState != terminalproto.AccountLimitsRefused {
		t.Fatalf("a refused token: %+v", a)
	}
}

func TestCopilotWithEveryWindowUnlimitedHasNothingToShow(t *testing.T) {
	f := newFixture(t)
	f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
	f.copilot.answer(200, `{"login":"person","copilot_plan":"business","quota_snapshots":{"chat":{"unlimited":true},"completions":{"unlimited":true},"premium_interactions":{"unlimited":true}}}`)
	a := f.one("copilot", true, false)
	if a.LimitsState != terminalproto.AccountLimitsUnsupported || a.Plan != "business" || len(a.Limits) != 0 {
		t.Fatalf("unlimited: %+v", a)
	}
}

func TestCopilotStrangeAnswerIsAFailure(t *testing.T) {
	for name, body := range map[string]string{"not json": "<html>", "no quotas": `{"login":"person"}`, "another shape": `{"quota_snapshots":[1]}`} {
		f := newFixture(t)
		f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
		f.copilot.answer(200, body)
		if a := f.one("copilot", true, false); a.LimitsState != terminalproto.AccountLimitsFailed || len(a.Limits) != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
}

func TestCopilotWhatCannotBeReadIsSaid(t *testing.T) {
	for name, body := range map[string]string{"broken": `{"github.com":`, "oversized": strings.Repeat(" ", maxCredentialBytes+1)} {
		f := newFixture(t)
		f.write(".config/github-copilot/hosts.json", body)
		if a := f.one("copilot", true, false); a.State != terminalproto.AccountUnreadable || f.asked() != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
	// A broken file beside a good one does not hide the good one.
	f := newFixture(t)
	f.write(".config/github-copilot/hosts.json", `{"github.com":`)
	f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
	if a := f.one("copilot", false, false); a.State != terminalproto.AccountSignedIn {
		t.Fatalf("a good file beside a broken one: %+v", a)
	}
	// Only another host's sign-in, or an entry with no token, is none.
	g := newFixture(t)
	g.write(".config/github-copilot/apps.json", copilotHosts([3]string{"ghe.example.com:a", "person", copilotToken("a")}, [3]string{"github.com:b", "person", ""}))
	if a := g.one("copilot", true, false); a.State != terminalproto.AccountNone {
		t.Fatalf("no github.com token: %+v", a)
	}
}

func TestCopilotMovedFolderIsFollowed(t *testing.T) {
	config := t.TempDir()
	f := newFixture(t, "XDG_CONFIG_HOME="+config)
	f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "wrong", copilotToken("a")}))
	if a := f.one("copilot", false, false); a.State != terminalproto.AccountNone {
		t.Fatalf("the default folder was read though the agent was moved: %+v", a)
	}
	if err := os.MkdirAll(filepath.Join(config, "github-copilot"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "github-copilot/hosts.json"), []byte(copilotHosts([3]string{"github.com", "person", copilotToken("a")})), 0o600); err != nil {
		t.Fatal(err)
	}
	if a := f.one("copilot", false, false); a.Account != "person" {
		t.Fatalf("the moved folder was not read: %+v", a)
	}
	if a := newFixture(t, "XDG_CONFIG_HOME=conf").one("copilot", false, false); a.State != terminalproto.AccountUnreadable {
		t.Fatalf("a relative folder: %+v", a)
	}
}

// Every new reader keeps its last figures through a refusal, a rate limit and
// a failure, and says which it was, as Claude Code and Codex do.
func TestNewReadersKeepTheLastFiguresThroughAFailure(t *testing.T) {
	type setup struct {
		tool  string
		write func(f *fixture)
		p     func(f *fixture) *provider
		good  string
	}
	for _, s := range []setup{
		{"opencode", func(f *fixture) {
			f.write(".local/share/opencode/auth.json", fmt.Sprintf(`{"opencode":{"type":"api","key":%q}}`, openCodeKey("zen")))
		}, func(f *fixture) *provider { return f.openCode }, openCodeUsage},
		{"antigravity", func(f *fixture) { f.write(antigravityFile, antigravityToken(f.now.Add(24*time.Hour))) }, func(f *fixture) *provider { return f.antigravity }, antigravityUsage},
		{"copilot", func(f *fixture) {
			f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
		}, func(f *fixture) *provider { return f.copilot }, copilotPaid},
	} {
		for status, want := range map[int]terminalproto.AccountLimitsState{401: terminalproto.AccountLimitsRefused, 429: terminalproto.AccountLimitsRateLimited, 500: terminalproto.AccountLimitsFailed} {
			f := newFixture(t)
			s.write(f)
			s.p(f).answer(200, s.good)
			first := f.one(s.tool, true, false)
			if first.LimitsState != terminalproto.AccountLimitsRead {
				t.Fatalf("%s: first read: %+v", s.tool, first)
			}
			f.now = f.now.Add(10 * time.Minute)
			s.p(f).answer(status, `{}`)
			// Antigravity's second endpoint fails the same way.
			f.antigravityStable.answer(status, `{}`)
			failed := f.one(s.tool, true, false)
			if failed.LimitsState != want || failed.LimitsAt != first.LimitsAt || fmt.Sprint(failed.Limits) != fmt.Sprint(first.Limits) || failed.Plan != first.Plan || failed.Account != first.Account {
				t.Fatalf("%s, status %d: %+v, want state %s with %+v", s.tool, status, failed, want, first)
			}
		}
	}
}

// Every agent signed in, with every window read, still fits one answer.
func TestEveryAgentReadFitsOneAnswer(t *testing.T) {
	f := newFixture(t, "OPENCODE_API_KEY="+openCodeKey("zen"))
	f.write(".claude/.credentials.json", claudeCredentials(f.now.Add(time.Hour)))
	f.write(".claude.json", claudeProfile)
	f.write(".codex/auth.json", codexAuth(f.now.Add(time.Hour)))
	f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
	f.write(".config/github-copilot/apps.json", copilotHosts([3]string{"github.com:a", "person", copilotToken("a")}))
	f.claude.answer(200, claudeUsage)
	f.codex.answer(200, codexUsage)
	f.openCode.answer(200, openCodeUsage)
	f.antigravity.answer(200, antigravityUsage)
	f.copilot.answer(200, `{"login":"person","copilot_plan":"business","quota_reset_date":"2026-12-01","quota_snapshots":{`+
		`"premium_interactions":{"entitlement":300,"percent_remaining":40},"chat":{"entitlement":500,"percent_remaining":90},"completions":{"entitlement":2000,"percent_remaining":75}}}`)
	accounts := f.all(true, false)
	for tool, a := range accounts {
		if a.LimitsState != terminalproto.AccountLimitsRead {
			t.Fatalf("%s: %+v", tool, a)
		}
	}
	if got := len(accounts["copilot"].Limits); got != 3 {
		t.Fatalf("copilot windows: %+v", accounts["copilot"])
	}
}

func TestCopilotLookAlikeHostsAreNotGitHub(t *testing.T) {
	f := newFixture(t)
	f.write(".config/github-copilot/hosts.json", copilotHosts(
		[3]string{"github.comcast.com", "aaa", copilotToken("lookalike")},
		[3]string{"github.com", "person", copilotToken("real")},
	))
	f.write(".config/github-copilot/apps.json", copilotHosts(
		[3]string{"github.company.com:Iv1.1", "aaa", copilotToken("enterprise")},
		[3]string{"github.com.evil.example:Iv1.2", "aaa", copilotToken("evil")},
	))
	f.copilot.answer(200, copilotPaid)
	a := f.one("copilot", true, false)
	if a.State != terminalproto.AccountSignedIn || a.LimitsState != terminalproto.AccountLimitsRead || f.copilot.count() != 1 {
		t.Fatalf("beside look-alike hosts: %+v after %d requests", a, f.copilot.count())
	}
	if got := f.copilot.last().Header.Get("Authorization"); got != "token "+copilotToken("real") {
		t.Fatalf("sent %q, want the github.com token", got)
	}
	// Look-alike hosts alone: nothing is signed in and nothing is sent.
	g := newFixture(t)
	g.write(".config/github-copilot/apps.json", copilotHosts(
		[3]string{"github.company.com:Iv1.1", "person", copilotToken("enterprise")},
		[3]string{"github.comcast.com", "person", copilotToken("lookalike")},
	))
	g.copilot.answer(200, copilotPaid)
	if a := g.one("copilot", true, false); a.State != terminalproto.AccountNone || g.copilot.count() != 0 {
		t.Fatalf("look-alike hosts alone: %+v after %d requests", a, g.copilot.count())
	}
}

// A GitHub token in the connector's environment is not a sign-in: it is not
// read through the CLI that lists it, nor through an older one that cannot.
func TestCopilotLeavesAnEnvironmentTokenAlone(t *testing.T) {
	token := "ghp_env" + secret
	for name, table := range map[string]map[string]string{
		"a CLI that lists accounts": {
			"gh auth status --hostname github.com --json hosts": `{"hosts":{"github.com":[{"login":"ci-bot","state":"success","tokenSource":"GH_TOKEN"}]}}`,
			"gh auth token --hostname github.com":               token,
		},
		"an older CLI": {"gh auth token --hostname github.com": token},
	} {
		f := newFixture(t, "GH_TOKEN="+token)
		runFor(f, table)
		if a := f.one("copilot", true, false); a.State != terminalproto.AccountNone || f.copilot.count() != 0 {
			t.Fatalf("%s: %+v after %d requests", name, a, f.copilot.count())
		}
	}
}
