//go:build linux || darwin

package terminalagent

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/agentaccounts"
	"github.com/eyupio/reet.sh/internal/terminalhost"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// accountsConnector is modelsConnector with the sign-in reader pointed at a
// stand-in provider, and a Claude sign-in written into the connector's home
// when signedIn is set. The real providers are never reachable from a test.
func accountsConnector(t *testing.T, features string, provider http.Handler, signedIn bool, edit func(*terminalhost.ManagerOptions)) (func() error, func(terminalproto.Accounts, string) error, replies) {
	t.Helper()
	server := httptest.NewServer(provider)
	t.Cleanup(server.Close)
	original := newAccountReader
	newAccountReader = func(home string) *agentaccounts.Reader {
		return agentaccounts.New(agentaccounts.Options{Home: home, GOOS: "linux", ClaudeUsageURL: server.URL + "/claude", CodexUsageURL: server.URL + "/codex"})
	}
	t.Cleanup(func() { newAccountReader = original })
	var home string
	handle, got := modelsConnector(t, features, func(opts *terminalhost.ManagerOptions) {
		home = opts.Home
		if edit != nil {
			edit(opts)
		}
	})
	if signedIn {
		if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-SECRET","refreshToken":"sk-ant-ort01-SECRET","expiresAt":%d,"subscriptionType":"pro"}}`, time.Now().Add(time.Hour).UnixMilli())
		if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	waitIdle := func() error {
		deadline := time.Now().Add(10 * time.Second)
		for accountsRunning.Load() != 0 {
			if time.Now().After(deadline) {
				return errors.New("sign-in read still running")
			}
			time.Sleep(5 * time.Millisecond)
		}
		return nil
	}
	ask := func(in terminalproto.Accounts, id string) error { return handle(t.Context(), message(id, &in)) }
	return waitIdle, ask, got
}

func TestAccountsNeedsNegotiation(t *testing.T) {
	asked := false
	for _, features := range []string{"", terminalproto.ModelsFeature} {
		_, ask, got := accountsConnector(t, features, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }), true, nil)
		if err := ask(terminalproto.Accounts{Limits: true}, "accounts-guard-0001"); !errors.Is(err, terminalproto.ErrInvalid) {
			t.Fatalf("features %q: %v", features, err)
		}
		nothingSent(t, got)
	}
	if asked {
		t.Fatal("a provider was asked without agent-accounts-v1")
	}
}

// Terminal access gates reading a sign-in as it gates a shell.
func TestAccountsAreOffWithTerminalAccess(t *testing.T) {
	asked := false
	for name, consent := range map[string]func() error{"off": func() error { return errors.New("terminal access is off") }, "nil": nil} {
		t.Run(name, func(t *testing.T) {
			_, ask, got := accountsConnector(t, terminalproto.AgentAccountsFeature, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked = true }), true,
				func(opts *terminalhost.ManagerOptions) { opts.Consent = consent })
			if err := ask(terminalproto.Accounts{Limits: true}, "accounts-off-00001"); err != nil {
				t.Fatal(err)
			}
			body, ok := next(t, got, "accounts-off-00001").Body.(terminalproto.AccountsResult)
			if !ok || body.Reason != terminalproto.AccountsOff || len(body.Accounts) != 0 {
				t.Fatalf("body: %+v", body)
			}
		})
	}
	if asked {
		t.Fatal("a provider was asked with terminal access off")
	}
}

func TestAccountsAnswerTheSignInAndAskForLimitsOnlyWhenAsked(t *testing.T) {
	requests := 0
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"five_hour":{"utilization":25,"resets_at":"2026-11-14T12:00:00Z"}}`)
	})
	waitIdle, ask, got := accountsConnector(t, terminalproto.AgentAccountsFeature, provider, true, nil)
	if err := ask(terminalproto.Accounts{}, "accounts-plain-0001"); err != nil {
		t.Fatal(err)
	}
	reply := next(t, got, "accounts-plain-0001")
	body := reply.Body.(terminalproto.AccountsResult)
	if len(body.Accounts) != 2 || body.Accounts[0].Tool != "claude" || body.Accounts[0].State != terminalproto.AccountSignedIn || body.Accounts[0].Plan != "pro" || body.Accounts[0].LimitsState != terminalproto.AccountLimitsNotAsked {
		t.Fatalf("sign-in: %+v", body)
	}
	if body.Accounts[1].Tool != "codex" || body.Accounts[1].State != terminalproto.AccountNone {
		t.Fatalf("codex: %+v", body.Accounts[1])
	}
	if requests != 0 {
		t.Fatal("a provider was asked though the allowance was not")
	}
	if err := waitIdle(); err != nil {
		t.Fatal(err)
	}
	if err := ask(terminalproto.Accounts{Limits: true}, "accounts-limits-001"); err != nil {
		t.Fatal(err)
	}
	reply = next(t, got, "accounts-limits-001")
	claude := reply.Body.(terminalproto.AccountsResult).Accounts[0]
	if requests != 1 || claude.LimitsState != terminalproto.AccountLimitsRead || len(claude.Limits) != 1 || claude.Limits[0].UsedPercent != 25 {
		t.Fatalf("limits after %d requests: %+v", requests, claude)
	}
	wire, err := terminalproto.Encode(reply)
	if err != nil || strings.Contains(string(wire), "SECRET") {
		t.Fatalf("the answer carries a credential or does not encode: %v %s", err, wire)
	}
}

// One read runs at a time: a second request is told so at once and does not
// reach the provider.
func TestAccountsOneAtATime(t *testing.T) {
	entered, release := make(chan struct{}, 4), make(chan struct{})
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-release
		fmt.Fprint(w, `{"five_hour":{"utilization":1,"resets_at":""}}`)
	})
	waitIdle, ask, got := accountsConnector(t, terminalproto.AgentAccountsFeature, provider, true, nil)
	if err := ask(terminalproto.Accounts{Limits: true}, "accounts-first-0001"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the provider was never asked")
	}
	if err := ask(terminalproto.Accounts{Limits: true}, "accounts-second-001"); err != nil {
		t.Fatal(err)
	}
	if body := next(t, got, "accounts-second-001").Body.(terminalproto.AccountsResult); body.Reason != terminalproto.AccountsConflict {
		t.Fatalf("second request: %+v", body)
	}
	close(release)
	if body := next(t, got, "accounts-first-0001").Body.(terminalproto.AccountsResult); body.Reason != "" || len(body.Accounts) != 2 {
		t.Fatalf("first request: %+v", body)
	}
	if err := waitIdle(); err != nil {
		t.Fatal(err)
	}
	if len(entered) != 0 {
		t.Fatal("the refused request reached the provider")
	}
}
