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

// A delivery review is read and decided by the owning browser account only:
// not signed out, not from another origin, not with an API token, and never
// for an outcome the account does not have. Bodies are strict, and a decision
// is refused where remote sessions are off, as a merge approval is.
func TestGafferDeliveryRoutesNeedTheOwningBrowser(t *testing.T) {
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
	if _, err := st.CreateAPIToken(1, "Delivery refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	base := "/api/v2/gaffer/outcomes/" + strings.Repeat("a", 32) + "/sessions/a/delivery"
	request := strings.Repeat("1", 32)
	decision := `{"requestId":"` + request + `","decision":"accept","criteriaRevision":1,"reviewRevision":1,"commit":"` + strings.Repeat("a", 40) + `","statuses":{},"confirmed":true}`
	routes := []struct{ method, path, body string }{
		{"GET", base, ""},
		{"PUT", base + "/criteria", `{"criteriaRevision":1,"criteria":[{"text":"It exports."}]}`},
		{"POST", base + "/evidence", `{"requestId":"` + request + `","reviewRevision":1,"criterionId":"c","kind":"note","verdict":"passed","text":"Tried it."}`},
		{"POST", base + "/decision", decision},
	}
	for _, route := range routes {
		if code, _ := gafferClient(t, &http.Client{}, srv, route.method, route.path, route.body); code != 401 {
			t.Fatalf("signed out %s %s: %d", route.method, route.path, code)
		}
		if code, _ := gafferClient(t, &http.Client{}, srv, route.method, route.path, route.body, "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token %s %s: %d", route.method, route.path, code)
		}
		if route.method != "GET" {
			if code, _ := doClient(t, owner, srv, route.method, route.path, route.body, "Origin", "https://different.example"); code != 403 {
				t.Fatalf("foreign origin %s %s: %d", route.method, route.path, code)
			}
			// An unknown field never reaches the review: the body is strict.
			if code, _ := gafferClient(t, owner, srv, route.method, route.path, strings.Replace(route.body, "{", `{"mergeApproved":true,`, 1)); code != 400 {
				t.Fatalf("unknown field %s %s: %d", route.method, route.path, code)
			}
		}
	}
	// The owner's own requests reach the review, which this account does not have.
	for _, route := range routes[:3] {
		if code, body := gafferClient(t, owner, srv, route.method, route.path, route.body); code != 404 || strings.Contains(body, "criteria") {
			t.Fatalf("unknown outcome %s %s: %d %s", route.method, route.path, code, body)
		}
	}
	// Remote sessions are off on this controller: a decision can start a session, so it is refused.
	if code, body := gafferClient(t, owner, srv, "POST", base+"/decision", decision); code != 409 || !strings.Contains(body, "Remote sessions are disabled") {
		t.Fatalf("decision with remote sessions off: %d %s", code, body)
	}
}
