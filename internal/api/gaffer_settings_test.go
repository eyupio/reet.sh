package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestGafferSettingsKeepCredentialsPrivateAndRequireTheOwningBrowserAccount(t *testing.T) {
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
	code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/providers", `{"name":"OpenAI","kind":"openai","model":"model-a","enabled":true,"apiKey":"synthetic-sensitive-key","makeDefault":true}`)
	if code != 200 || strings.Contains(body, "synthetic-sensitive-key") || strings.Contains(body, "keySealed") {
		t.Fatalf("save: %d %s", code, body)
	}
	var out struct {
		Providers []store.GafferProvider `json:"providers"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Providers) != 1 {
		t.Fatalf("providers: %v %s", err, body)
	}
	p := out.Providers[0]
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/providers", `{"name":"OpenAI","kind":"openai","model":"model-a","enabled":true}`); code != 422 {
		t.Fatalf("duplicate name: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/providers", `{"name":"Paused","kind":"openai","model":"model-a","makeDefault":true}`); code != 422 {
		t.Fatalf("disabled default: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "POST", "/api/v2/gaffer/providers/"+p.ID+"/test", `{"revision":1}`, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign-origin probe: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "GET", "/api/v2/gaffer/providers", ""); code != 401 {
		t.Fatalf("signed-out settings: %d", code)
	}
	if _, err := st.CreateAPIToken(1, "Gaffer refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	for _, client := range []*http.Client{owner, {}} {
		if code, _ := gafferClient(t, client, srv, "GET", "/api/v2/gaffer/providers", "", "Authorization", "Bearer synthetic-api-token"); code != 403 {
			t.Fatalf("API token settings: %d", code)
		}
	}
	if !p.HasKey || !p.Default {
		t.Fatalf("metadata: %+v", p)
	}
	stored, err := st.GafferProviderFor(1, p.ID)
	if err != nil || stored.KeySealed == "" || strings.Contains(stored.KeySealed, "synthetic-sensitive-key") {
		t.Fatalf("sealed credential: %v", err)
	}
	password, _ := auth.HashPassword("long-enough-password")
	for _, role := range []string{"member", "viewer"} {
		u, err := st.CreateUser(role+"@example.test", role, role, password)
		if err != nil {
			t.Fatal(err)
		}
		client := &http.Client{Jar: mustJar(t)}
		if code, _ := gafferClient(t, client, srv, "POST", "/api/v2/auth/sign-in", `{"email":"`+u.Email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatal(code)
		}
		for _, action := range []string{"test", "default", "enabled"} {
			code, _ := gafferClient(t, client, srv, "POST", "/api/v2/gaffer/providers/"+p.ID+"/"+action, `{"revision":1,"enabled":true}`)
			want := 404
			if role == "viewer" {
				want = 403
			}
			if code != want {
				t.Fatalf("%s %s: %d", role, action, code)
			}
		}
	}
	code, _ = gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/providers/"+p.ID, `{"revision":1,"name":"Updated","kind":"openai","model":"model-b","enabled":true}`)
	if code != 200 {
		t.Fatalf("update: %d", code)
	}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/providers/"+p.ID+"/test", `{"revision":1}`); code != 409 {
		t.Fatalf("stale test: %d", code)
	}
	events, _ := st.ListEventsFor(1, 100, "")
	for _, e := range events {
		if strings.Contains(e.Detail, "synthetic-sensitive-key") || strings.Contains(e.Detail, stored.KeySealed) {
			t.Fatal("credential reached the audit log")
		}
	}
}

func TestGafferModelsReadUnsavedConfigurationWithoutPersistingOrSpendingALiveRequest(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); h.Close(); st.Close() })
	client := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, client, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatal(code)
	}
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"}]}`)
	}))
	defer fake.Close()
	input := fmt.Sprintf(`{"name":"Local","kind":"openai_compatible","baseUrl":%q,"localOnly":true}`, fake.URL)
	code, body := gafferClient(t, client, srv, "POST", "/api/v2/gaffer/providers/models", input)
	if code != 200 || !strings.Contains(body, `["a","z"]`) {
		t.Fatalf("models: %d %s", code, body)
	}
	rows, _ := st.GafferProvidersFor(1)
	if len(rows) != 0 {
		t.Fatal("model discovery saved a provider")
	}
	if code, _ := gafferClient(t, client, srv, "POST", "/api/v2/gaffer/providers", `{"name":"x","kind":"openai","model":"m","unknown":"field"}`); code != 400 {
		t.Fatalf("unknown field: %d", code)
	}
}

func gafferClient(t *testing.T, client *http.Client, server *httptest.Server, method, path, body string, headers ...string) (int, string) {
	headers = append(headers, "Origin", "http://localhost:8080")
	return doClient(t, client, server, method, path, body, headers...)
}

// The crew is the owning browser's to read and save; a tier it cannot run is
// refused, and what is saved is what the next read returns.
func TestGafferCrewRoutesSaveAndReadTheAccountsTiers(t *testing.T) {
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
	code, body := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/crew", "")
	if code != 200 || !strings.Contains(body, `"saved":false`) || !strings.Contains(body, `"review":"heavy"`) || !strings.Contains(body, `"merge":"light"`) {
		t.Fatalf("before any save: %d %s", code, body)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "GET", "/api/v2/gaffer/crew", ""); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "PUT", "/api/v2/gaffer/crew", `{"light":{},"standard":{},"heavy":{}}`, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	// A threshold outside what a person may choose is refused whole.
	if code, _ := gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/crew", `{"light":{},"standard":{},"heavy":{},"balanceBelow":95}`); code != 422 {
		t.Fatalf("threshold out of range: %d", code)
	}
	crew := `{"light":{"tool":"codex","model":"small","effort":"low"},"standard":{"tool":"opencode"},"heavy":{"tool":"claude","model":"opus","effort":"high","backup":{"tool":"codex","model":"big"}},"balanceBelow":35}`
	code, body = gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/crew", crew)
	if code != 200 || !strings.Contains(body, `"saved":true`) || !strings.Contains(body, `"model":"opus"`) || !strings.Contains(body, `"balanceBelow":35`) || !strings.Contains(body, `"backup":{"tool":"codex","model":"big"}`) || !strings.Contains(body, `"balance":{"default":20,"max":90,"min":5}`) {
		t.Fatalf("save: %d %s", code, body)
	}
	code, body = gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/crew", "")
	if code != 200 || !strings.Contains(body, `"tool":"codex"`) || !strings.Contains(body, `"effort":"high"`) {
		t.Fatalf("read back: %d %s", code, body)
	}
	for name, bad := range map[string]string{
		"an agent that cannot chat": `{"light":{"tool":"bash"},"standard":{},"heavy":{}}`,
		"an unknown field":          `{"light":{},"standard":{},"heavy":{},"extra":{}}`,
	} {
		if code, _ := gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/crew", bad); code < 400 || code >= 500 {
			t.Errorf("%s: %d", name, code)
		}
	}
}
