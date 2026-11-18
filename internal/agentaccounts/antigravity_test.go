package agentaccounts

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/terminalproto"
)

const antigravityFile = ".gemini/antigravity-cli/antigravity-oauth-token"

func antigravityToken(expires time.Time) string {
	id := jwt(map[string]any{"email": "person@example.com", "sub": "1234567890"})
	return fmt.Sprintf(`{"token":{"access_token":"ya29.%s","token_type":"Bearer","refresh_token":"1//%s","expiry":%q},"id_token":%q}`, secret, secret, expires.Format(time.RFC3339Nano), id)
}

const antigravityUsage = `{"groups":[{"name":"Gemini","buckets":[` +
	`{"bucketId":"gemini-5h","remainingFraction":0.75,"resetTime":"2026-11-14T12:00:00Z"},` +
	`{"bucketId":"gemini-weekly","remainingFraction":0.402,"resetTime":"2026-11-18T00:00:00Z"}]},` +
	`{"name":"Others","buckets":[{"bucketId":"3p-weekly","resetTime":"2026-11-18T00:00:00Z"},{"bucketId":"3p-5h","remainingFraction":1},{"bucketId":"something-new","remainingFraction":0.5}]}]}`

func TestAntigravitySignInAndLimits(t *testing.T) {
	for name, file := range map[string]func(string) string{
		"plain": func(s string) string { return s },
		"keyring": func(s string) string {
			return antigravityKeyringPrefix + base64.StdEncoding.EncodeToString([]byte(s)) + "\n"
		},
	} {
		f := newFixture(t)
		f.write(antigravityFile, file(antigravityToken(f.now.Add(time.Hour))))
		a := f.one("antigravity", false, false)
		want := terminalproto.AgentAccount{Tool: "antigravity", State: terminalproto.AccountSignedIn, Method: terminalproto.AccountSubscription, Account: "person@example.com", LimitsState: terminalproto.AccountLimitsNotAsked}
		if fmt.Sprint(a) != fmt.Sprint(want) {
			t.Fatalf("%s: antigravity = %+v, want %+v", name, a, want)
		}
		if f.asked() != 0 {
			t.Fatalf("%s: a provider was asked though the allowance was not", name)
		}
		f.antigravity.answer(200, antigravityUsage)
		a = f.one("antigravity", true, false)
		wantLimits := []terminalproto.AccountLimit{
			{Kind: terminalproto.AccountLimitSession, UsedPercent: 25, ResetsAt: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC).UnixMilli()},
			{Kind: terminalproto.AccountLimitWeek, UsedPercent: 60, ResetsAt: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC).UnixMilli()},
			// A bucket that does not say what is left has none left.
			{Kind: terminalproto.AccountLimitSession, Label: "Other models", UsedPercent: 0},
			{Kind: terminalproto.AccountLimitWeek, Label: "Other models", UsedPercent: 100, ResetsAt: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC).UnixMilli()},
		}
		if a.LimitsState != terminalproto.AccountLimitsRead || fmt.Sprint(a.Limits) != fmt.Sprint(wantLimits) {
			t.Fatalf("%s: limits = %+v, want %+v", name, a, wantLimits)
		}
		request := f.antigravity.last()
		if request.Method != "POST" || request.URL.Path != "/v1internal:retrieveUserQuotaSummary" || f.antigravity.bodies[0] != "{}" ||
			request.Header.Get("Authorization") != "Bearer ya29."+secret || request.Header.Get("Content-Type") != "application/json" || request.Header.Get("User-Agent") != "antigravity" {
			t.Fatalf("%s: request: %s %s %q %v", name, request.Method, request.URL, f.antigravity.bodies[0], request.Header)
		}
		if f.antigravityStable.count() != 0 {
			t.Fatalf("%s: the second endpoint was asked though the first answered", name)
		}
	}
}

func TestAntigravityAsksTheSecondEndpointOnlyWhenTheFirstFails(t *testing.T) {
	for status, want := range map[int]terminalproto.AccountLimitsState{500: terminalproto.AccountLimitsRead, 503: terminalproto.AccountLimitsRead, 404: terminalproto.AccountLimitsRead, 429: terminalproto.AccountLimitsRateLimited, 401: terminalproto.AccountLimitsRefused, 403: terminalproto.AccountLimitsRefused} {
		f := newFixture(t)
		f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
		f.antigravity.answer(status, `{}`)
		f.antigravityStable.answer(200, antigravityUsage)
		a := f.one("antigravity", true, false)
		asked := 0
		if want == terminalproto.AccountLimitsRead {
			asked = 1
		}
		if a.LimitsState != want || f.antigravityStable.count() != asked {
			t.Fatalf("first endpoint %d: %+v, second asked %d times", status, a, f.antigravityStable.count())
		}
	}
	// A first endpoint that cannot be reached is a failure too.
	g := newFixture(t)
	g.write(antigravityFile, antigravityToken(g.now.Add(time.Hour)))
	g.antigravity.server.Close()
	g.antigravityStable.answer(200, antigravityUsage)
	if a := g.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsRead || g.antigravityStable.count() != 1 {
		t.Fatalf("first endpoint unreachable: %+v, second asked %d times", a, g.antigravityStable.count())
	}
	// Both failing is a failure, and the second is asked once.
	f := newFixture(t)
	f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
	f.antigravity.answer(503, `{}`)
	f.antigravityStable.answer(502, `{}`)
	if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsFailed || f.antigravity.count() != 1 || f.antigravityStable.count() != 1 {
		t.Fatalf("both failing: %+v", a)
	}
}

func TestAntigravityExpiredTokenIsNeverSent(t *testing.T) {
	f := newFixture(t)
	f.write(antigravityFile, antigravityToken(f.now.Add(10*time.Second)))
	before, _ := os.ReadFile(filepath.Join(f.home, antigravityFile))
	a := f.one("antigravity", true, true)
	if a.State != terminalproto.AccountSignedIn || a.LimitsState != terminalproto.AccountLimitsRenew || len(a.Limits) != 0 {
		t.Fatalf("an expired sign-in: %+v", a)
	}
	if f.asked() != 0 {
		t.Fatal("an expired token was sent")
	}
	if after, _ := os.ReadFile(filepath.Join(f.home, antigravityFile)); string(before) != string(after) {
		t.Fatal("the sign-in file was changed")
	}
}

func TestAntigravityWithoutAPlanHasNothingToShow(t *testing.T) {
	f := newFixture(t)
	f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
	f.antigravity.answer(200, `{"groups":[]}`)
	if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsUnsupported || len(a.Limits) != 0 {
		t.Fatalf("no buckets: %+v", a)
	}
}

func TestAntigravityStrangeAnswerIsAFailure(t *testing.T) {
	for name, body := range map[string]string{
		"not json": "<html>", "no groups": `{}`, "only buckets it does not know": `{"groups":[{"buckets":[{"bucketId":"new","remainingFraction":1}]}]}`,
		"another shape": `{"groups":{"a":1}}`,
	} {
		f := newFixture(t)
		f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
		f.antigravity.answer(200, body)
		if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsFailed || len(a.Limits) != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
}

func TestAntigravityWhatCannotBeReadIsSaid(t *testing.T) {
	for name, body := range map[string]string{
		"broken": `{"token":`, "bad keyring form": antigravityKeyringPrefix + "%%%", "oversized": strings.Repeat(" ", maxCredentialBytes+1),
		"bad expiry": `{"token":{"access_token":"x","expiry":"tomorrow"}}`,
	} {
		f := newFixture(t)
		f.write(antigravityFile, body)
		if a := f.one("antigravity", true, false); a.State != terminalproto.AccountUnreadable || f.asked() != 0 {
			t.Fatalf("%s: %+v", name, a)
		}
	}
	// A file with no access token is no sign-in.
	f := newFixture(t)
	f.write(antigravityFile, `{"token":{"access_token":""}}`)
	if a := f.one("antigravity", true, false); a.State != terminalproto.AccountNone {
		t.Fatalf("an empty sign-in: %+v", a)
	}
	// A folder the agent would resolve against its own working directory cannot be followed.
	if a := newFixture(t, "ANTIGRAVITY_APP_DATA_DIR=agy").one("antigravity", false, false); a.State != terminalproto.AccountUnreadable {
		t.Fatalf("a relative folder: %+v", a)
	}
}

func TestAntigravityMovedFolderIsFollowed(t *testing.T) {
	moved := t.TempDir()
	f := newFixture(t, "ANTIGRAVITY_APP_DATA_DIR="+moved)
	f.write(antigravityFile, antigravityToken(f.now.Add(time.Hour)))
	if a := f.one("antigravity", false, false); a.State != terminalproto.AccountNone {
		t.Fatalf("the default folder was read though the agent was moved: %+v", a)
	}
	if err := os.WriteFile(filepath.Join(moved, "antigravity-oauth-token"), []byte(antigravityToken(f.now.Add(time.Hour))), 0o600); err != nil {
		t.Fatal(err)
	}
	if a := f.one("antigravity", false, false); a.Account != "person@example.com" {
		t.Fatalf("the moved folder was not read: %+v", a)
	}
}

// A rate limit is the provider's answer, not a failure: it is said, the
// second endpoint is not asked, and the provider's Retry-After is honoured.
func TestAntigravityRateLimitWaitsAsLongAsTheProviderAsks(t *testing.T) {
	f := newFixture(t)
	f.write(antigravityFile, antigravityToken(f.now.Add(2*time.Hour)))
	f.antigravity.header.Set("Retry-After", "600")
	f.antigravity.answer(429, `{}`)
	f.antigravityStable.answer(200, antigravityUsage)
	if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsRateLimited {
		t.Fatalf("rate limited: %+v", a)
	}
	f.now = f.now.Add(5 * time.Minute)
	if a := f.one("antigravity", true, true); a.LimitsState != terminalproto.AccountLimitsRateLimited || f.antigravity.count() != 1 {
		t.Fatalf("asked again inside the provider's wait (%d requests): %+v", f.antigravity.count(), a)
	}
	f.now = f.now.Add(6 * time.Minute)
	f.antigravity.answer(200, antigravityUsage)
	if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsRead || f.antigravity.count() != 2 {
		t.Fatalf("did not ask once the wait was over (%d requests): %+v", f.antigravity.count(), a)
	}
	if f.antigravityStable.count() != 0 {
		t.Fatalf("the second endpoint was asked %d times on a rate limit", f.antigravityStable.count())
	}
}

// Without an identity token there is no account to tell sign-ins apart by: a
// mark of the token does, so a new sign-in is not shown the last one's figures.
func TestAntigravityWithoutAnIdentityTokenDoesNotInheritFigures(t *testing.T) {
	signIn := func(name string) string {
		return fmt.Sprintf(`{"token":{"access_token":"ya29.%s%s","refresh_token":"1//%s%s","expiry":%q}}`, name, secret, name, secret, testNow.Add(2*time.Hour).Format(time.RFC3339))
	}
	f := newFixture(t)
	f.write(antigravityFile, signIn("first"))
	f.antigravity.answer(200, antigravityUsage)
	if a := f.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsRead || a.Account != "" {
		t.Fatalf("first sign-in: %+v", a)
	}
	f.write(antigravityFile, signIn("second"))
	f.now = f.now.Add(time.Minute)
	f.antigravity.answer(500, `{}`)
	f.antigravityStable.answer(500, `{}`)
	if a := f.one("antigravity", true, false); len(a.Limits) != 0 || a.LimitsAt != 0 || a.LimitsState != terminalproto.AccountLimitsFailed {
		t.Fatalf("a second sign-in was shown the first one's figures: %+v", a)
	}
	// The same sign-in, its access token renewed, keeps its figures.
	g := newFixture(t)
	g.write(antigravityFile, signIn("first"))
	g.antigravity.answer(200, antigravityUsage)
	g.one("antigravity", true, false)
	g.write(antigravityFile, strings.Replace(signIn("first"), "ya29.first", "ya29.renewed", 1))
	g.now = g.now.Add(time.Minute)
	if a := g.one("antigravity", true, false); a.LimitsState != terminalproto.AccountLimitsRead || len(a.Limits) == 0 || g.antigravity.count() != 1 {
		t.Fatalf("a renewed token lost its figures: %+v after %d requests", a, g.antigravity.count())
	}
}
