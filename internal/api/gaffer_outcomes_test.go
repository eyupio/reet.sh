package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// Approving an outcome starts agents on a machine, so it needs the owning
// writer's browser, the canonical origin and remote sessions switched on.
func TestGafferOutcomeRoutesNeedTheOwningBrowserAndRemoteSessions(t *testing.T) {
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
	for _, path := range []string{"/api/v2/gaffer/outcomes", "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32)} {
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
		"yolo":               strings.Replace(plan, `"confirmed":true`, `"confirmed":true,"yolo":true`, 1),
		"session yolo":       strings.Replace(plan, `"title":"Export"`, `"title":"Export","yolo":true`, 1),
	} {
		if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes", bad); code != 400 {
			t.Fatalf("%s: %d", name, code)
		}
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
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/outcomes/"+strings.Repeat("a", 32)+"/stop", `{}`); code != 400 {
		t.Fatalf("stop without a revision: %d", code)
	}
}
