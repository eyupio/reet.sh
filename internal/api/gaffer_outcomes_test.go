package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
	"github.com/eyupio/reet.sh/internal/terminalbroker"
	"github.com/eyupio/reet.sh/internal/terminalproto"
)

// The effort chosen for an outcome stage is carried through the controller's
// launch path into the structured create the machine receives, even when the
// agent's descriptor names no thought levels.
func TestOutcomeStartCarriesTheSelectedEffortToTheMachine(t *testing.T) {
	caps := chatCaps()
	caps.AgentsV2, caps.ACPLiveControls, caps.ACPStartOptions = true, true, true
	caps.Tools = []string{"opencode"}
	entries := []terminalproto.ACPAgent{{Tool: "opencode", Adapter: "opencode", State: "available", MinimumVersion: "1.19.5"}}
	caps.ACPAgents = &entries
	f := launchAPIFixture(t, caps)
	_, _, err := outcomeStarter{f.srv.s}.StartOutcomeSession(t.Context(), f.uid, gaffer.OutcomeLaunch{
		AgentID: f.agent.ID, RequestID: terminalbroker.ID(), Title: "Export API", Tool: "opencode",
		Effort: "medium", Dir: "/tmp", Trust: true, Prompt: "Add the CSV export endpoint with tests.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent := f.sent(); len(sent) != 1 || !sent[0].Structured || sent[0].Effort != "medium" || !sent[0].Trust {
		t.Fatalf("sent: %+v", sent)
	}
}

// Approving an outcome starts agents on a machine, so it needs the owning
// writer's browser, the canonical origin and remote sessions switched on.
func TestGafferOutcomeRoutesNeedTheOwningBrowserAndRemoteSessions(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	plan := `{"objective":"Ship CSV export.","projectId":"missing","tool":"opencode","maxReviews":8,"hours":24,"confirmed":true,"sessions":[{"title":"Export","prompt":"Add CSV export. Do not merge."}]}`
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", plan, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", "/api/v2/gaffer/outcomes", plan); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	if _, err := st.CreateAPIToken(1, "Outcome refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v2/gaffer/outcomes", "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32), "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32) + "/history"} {
		if code, _ := gafferClient(t, &http.Client{}, srv, "GET", path, "", "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token read of %s: %d", path, code)
		}
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", "/api/v2/gaffer/outcomes", plan, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token create: %d", code)
	}
	// A fractional limit or an unknown field never reaches the plan: the body is strict.
	for name, bad := range map[string]string{
		"fractional reviews": strings.Replace(plan, `"maxReviews":8`, `"maxReviews":1.5`, 1),
		"session yolo":       strings.Replace(plan, `"title":"Export"`, `"title":"Export","yolo":true`, 1),
	} {
		if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", bad); code != 400 {
			t.Fatalf("%s: %d", name, code)
		}
	}
	// YOLO is the person's to choose for an outcome, never a session's or a plan's, and only where the account allows it.
	if code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", strings.Replace(plan, `"confirmed":true`, `"confirmed":true,"yolo":true`, 1)); code != 403 || !strings.Contains(body, "Remote sessions are disabled") && !strings.Contains(body, "allow YOLO") {
		t.Fatalf("yolo without the account setting: %d %s", code, body)
	}
	// Remote sessions are off on this controller, so nothing can be approved.
	if code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", plan); code != 403 || !strings.Contains(body, "Remote sessions are disabled") {
		t.Fatalf("remote sessions off: %d %s", code, body)
	}
	code, body := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes", "")
	if code != 200 || strings.TrimSpace(body) != `{"outcomes":[]}` {
		t.Fatalf("list: %d %s", code, body)
	}
	if code, _ := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32), ""); code != 404 {
		t.Fatalf("unknown outcome: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32)+"/stop", `{"revision":1}`); code != 404 {
		t.Fatalf("stop unknown outcome: %d", code)
	}
	// A merge is approved by the owning browser, explicitly, for a pipeline that exists.
	merge := "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32) + "/sessions/a/merge"
	if code, _ := doClient(t, owner, srv, "POST", merge, `{"revision":1,"confirmed":true}`, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign-origin merge: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", merge, `{"revision":1,"confirmed":true}`, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token merge: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", merge, `{"revision":1}`); code != 400 {
		t.Fatalf("unconfirmed merge: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", merge, `{"revision":1,"confirmed":true}`); code != 404 {
		t.Fatalf("merge of an unknown pipeline: %d", code)
	}
	// Trying again, adding pipelines and asking for a report are the owning browser's requests too.
	base := "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32)
	for path, body := range map[string]string{base + "/sessions/a/retry": `{"revision":1}`, base + "/sessions": `{"revision":1,"confirmed":true,"sessions":[{"title":"More","prompt":"Do it. Do not merge."}]}`, base + "/review": `{"revision":1}`} {
		if code, _ := doClient(t, owner, srv, "POST", path, body, "Origin", "https://different.example"); code != 403 {
			t.Fatalf("foreign origin %s: %d", path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token %s: %d", path, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{}`); code != 400 {
			t.Fatalf("no revision %s: %d", path, code)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/review", `{"revision":1}`); code != 404 {
		t.Fatalf("report on an unknown outcome: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", base+"/sessions", `{"revision":1,"confirmed":true,"yolo":true,"sessions":[]}`); code != 400 {
		t.Fatalf("unknown field on added pipelines: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32)+"/stop", `{}`); code != 400 {
		t.Fatalf("stop without a revision: %d", code)
	}
	// Archiving, restoring and deleting are the owning browser's requests, and name the revision they mean.
	for _, action := range []string{"archive", "restore"} {
		path := base + "/" + action
		if code, _ := doClient(t, owner, srv, "POST", path, `{"revision":1}`, "Origin", "https://different.example"); code != 403 {
			t.Fatalf("foreign origin %s: %d", action, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{}`); code != 400 {
			t.Fatalf("%s without a revision: %d", action, code)
		}
		if code, _ := gafferClient(t, owner, srv, "POST", path, `{"revision":1}`); code != 404 {
			t.Fatalf("%s of an unknown outcome: %d", action, code)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", base, ""); code != 400 {
		t.Fatalf("delete without a revision: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "DELETE", base+"?revision=1", ""); code != 404 {
		t.Fatalf("delete of an unknown outcome: %d", code)
	}
}
