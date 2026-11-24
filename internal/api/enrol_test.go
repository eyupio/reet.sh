package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/store"
)

const (
	codeRoute      = "/api/v2/sync/enrol-codes"
	badCodeMessage = "this enrolment code is not valid; make a new one under Machines, Add a machine"
)

// enrolAccounts is an accounts instance whose owner has a browser session, a
// personal API token and an MCP token.
type enrolAccounts struct {
	ts       *httptest.Server
	st       *store.Store
	browser  *http.Client
	uid      int64
	token    string
	mcpToken string
}

func newEnrolAccounts(t *testing.T) enrolAccounts {
	t.Helper()
	ts, st, browser, uid := mcpAccounts(t)
	token := mcpJSON(t, browser, ts, "POST", "/api/v2/tokens", map[string]any{"name": "old machine"}, 200)["token"].(string)
	mcpToken, _ := mcpManual(t, browser, ts, "write")
	return enrolAccounts{ts, st, browser, uid, token, mcpToken}
}

func (e enrolAccounts) newCode(t *testing.T) string {
	t.Helper()
	r := mcpJSON(t, e.browser, e.ts, "POST", codeRoute, map[string]any{}, 201)
	code, _ := r["code"].(string)
	expires, _ := r["expiresAt"].(float64)
	if !strings.HasPrefix(code, "reet_enrol_") || len(r) != 2 {
		t.Fatalf("code reply: %v", r)
	}
	if left := time.Until(time.Unix(int64(expires), 0)); left < 14*time.Minute || left > 15*time.Minute+time.Second {
		t.Fatalf("code lives %s", left)
	}
	return code
}

// enrol sends a machine's enrolment with the given credential header.
func (e enrolAccounts) enrol(t *testing.T, host string, hdr ...string) (int, string) {
	t.Helper()
	return do(t, e.ts, "POST", "/api/v2/sync/agents", `{"host":"`+host+`","version":"test"}`, hdr...)
}

func TestEnrolCodesAreMadeOnlyFromASignedInBrowser(t *testing.T) {
	e := newEnrolAccounts(t)
	e.newCode(t)
	for name, c := range map[string]struct {
		hdr  []string
		want int
		msg  string
	}{
		"personal token":    {[]string{"X-Reet-Token", e.token}, 403, "create enrolment codes from a signed-in browser"},
		"bearer token":      {[]string{"Authorization", "Bearer " + e.token}, 403, "create enrolment codes from a signed-in browser"},
		"MCP token":         {[]string{"Authorization", "Bearer " + e.mcpToken}, 403, "MCP credentials are valid only at the /mcp endpoint"},
		"no session":        {nil, 401, ""},
		"another site's":    {[]string{"Origin", "https://evil.invalid"}, 403, "cross-site"},
		"cross-site header": {[]string{"Sec-Fetch-Site", "cross-site"}, 403, "cross-site"},
	} {
		client := http.DefaultClient
		if strings.Contains(name, "site") {
			client = e.browser
		}
		code, body := doClient(t, client, e.ts, "POST", codeRoute, `{}`, c.hdr...)
		if code != c.want || !strings.Contains(body, c.msg) {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
	events, _ := e.st.ListEventsFor(e.uid, 50, "")
	made := 0
	for _, ev := range events {
		if ev.Action == "enrol-code-create" {
			made++
			if strings.Contains(ev.Detail, "reet_enrol_") {
				t.Fatalf("event holds the code: %s", ev.Detail)
			}
		}
	}
	if made != 1 {
		t.Fatalf("%d code events: %+v", made, events)
	}
}

func TestEnrolCodeEnrolsOneMachineOnce(t *testing.T) {
	e := newEnrolAccounts(t)
	code := e.newCode(t)
	status, body := e.enrol(t, "studio", "X-Reet-Token", code)
	var got struct {
		Agent  store.SyncAgent `json:"agent"`
		Secret string          `json:"secret"`
	}
	if status != 201 || json.Unmarshal([]byte(body), &got) != nil || got.Agent.ID == "" || got.Secret == "" || !got.Agent.KeepSynced {
		t.Fatalf("enrolment: %d %s", status, body)
	}
	if id, _ := e.st.EnrolCodeAgent(auth.HashToken(code)); id != got.Agent.ID {
		t.Fatalf("code recorded %q", id)
	}
	// Used, unknown and expired codes all get the same answer.
	expired := "reet_enrol_expired"
	if err := e.st.CreateEnrolCode(e.uid, auth.HashToken(expired), time.Now().Add(-time.Second).UnixMilli(), 20); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{code, "reet_enrol_unknown", expired} {
		status, body := e.enrol(t, "laptop", "Authorization", "Bearer "+bad)
		if status != 401 || !strings.Contains(body, badCodeMessage) {
			t.Fatalf("%s: %d %s", bad, status, body)
		}
	}
	// A code opens nothing else, not even before it is used.
	fresh := e.newCode(t)
	for _, route := range [][2]string{{"GET", "/api/v2/sync"}, {"POST", "/api/import"}, {"GET", "/api/v2/tokens"}, {"PATCH", "/api/v2/sync/agents"}, {"POST", "/api/v2/sync/poll"}, {"GET", "/quick?t=" + fresh}} {
		body := ""
		if route[0] != "GET" {
			body = `{}`
		}
		status, text := do(t, e.ts, route[0], route[1], body, "X-Reet-Token", fresh)
		if status != 403 || !strings.Contains(text, "only for enrolling a machine") {
			t.Fatalf("%s %s with a code: %d %s", route[0], route[1], status, text)
		}
	}
	if status, _ := e.enrol(t, "laptop", "X-Reet-Token", fresh); status != 201 {
		t.Fatalf("code refused after being shown elsewhere: %d", status)
	}
	events, _ := e.st.ListEventsFor(e.uid, 50, "")
	enrolled := 0
	for _, ev := range events {
		if ev.Action == "machine-enrol" {
			enrolled++
			if !strings.Contains(ev.Detail, `"via":"code"`) || strings.Contains(ev.Detail, "reet_enrol_") || strings.Contains(ev.Detail, got.Secret) {
				t.Fatalf("enrolment event: %s", ev.Detail)
			}
		}
	}
	if enrolled != 2 {
		t.Fatalf("%d enrolment events", enrolled)
	}
}

func TestOneEnrolCodeRacedByTwoMachinesEnrolsOne(t *testing.T) {
	e := newEnrolAccounts(t)
	code := e.newCode(t)
	var wg sync.WaitGroup
	statuses := make([]int, 2)
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], _ = e.enrol(t, "machine-"+strconv.Itoa(i), "X-Reet-Token", code)
		}(i)
	}
	wg.Wait()
	if statuses[0]+statuses[1] != 201+401 {
		t.Fatalf("two enrolments with one code: %v", statuses)
	}
	if agents, _ := e.st.SyncAgentsFor(e.uid); len(agents) != 1 {
		t.Fatalf("machines: %+v", agents)
	}
}

func TestUnusedEnrolCodesPerAccountAreCapped(t *testing.T) {
	e := newEnrolAccounts(t)
	for range maxEnrolCodes {
		e.newCode(t)
	}
	status, body := doClient(t, e.browser, e.ts, "POST", codeRoute, `{}`)
	if status != 429 || !strings.Contains(body, "unused enrolment codes") {
		t.Fatalf("code past the cap: %d %s", status, body)
	}
}

// An older machine enrols with a personal token, as before, and is marked as
// holding one; the enrolment is audited without the token.
func TestPersonalTokenStillEnrolsAMachine(t *testing.T) {
	e := newEnrolAccounts(t)
	status, body := e.enrol(t, "old-laptop", "X-Reet-Token", e.token)
	if status != 201 {
		t.Fatalf("token enrolment: %d %s", status, body)
	}
	events, _ := e.st.ListEventsFor(e.uid, 50, "")
	for _, ev := range events {
		if ev.Action == "machine-enrol" {
			if !strings.Contains(ev.Detail, `"via":"token"`) || strings.Contains(ev.Detail, e.token) {
				t.Fatalf("enrolment event: %s", ev.Detail)
			}
			return
		}
	}
	t.Fatal("no enrolment event")
}
