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

// Planning an outcome is a signed-in writer's browser request. It returns a
// plan to review and starts, saves and grants nothing.
func TestGafferOutcomePlanNeedsTheOwningBrowserAndStartsNothing(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	srv := httptest.NewServer(h)
	asked := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked++
		call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "plan", "type": "function", "function": map[string]any{"name": "propose_sessions", "arguments": `{"summary":"One job.","sessions":[{"title":"Export","prompt":"Add CSV export. Raise a pull request. Do not merge."}]}`}}}}, "finish_reason": "tool_calls"}}})
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", call)
	}))
	t.Cleanup(func() { provider.Close(); srv.Close(); h.Close(); st.Close() })
	owner := &http.Client{Jar: mustJar(t)}
	if code, _ := gafferClient(t, owner, srv, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap"); code != 200 {
		t.Fatalf("setup %d", code)
	}
	code, body := gafferClient(t, owner, srv, "POST", "/api/v2/gaffer/providers", fmt.Sprintf(`{"name":"Planner","kind":"openai_compatible","baseUrl":%q,"model":"plan-model","enabled":true,"localOnly":true,"apiKey":"synthetic-key","makeDefault":true}`, provider.URL))
	var out struct {
		Providers []store.GafferProvider `json:"providers"`
	}
	if err := json.Unmarshal([]byte(body), &out); code != 200 || err != nil || len(out.Providers) != 1 {
		t.Fatalf("save: %d %v %s", code, err, body)
	}
	path := "/api/v2/gaffer/providers/" + out.Providers[0].ID + "/plan"
	ask := `{"revision":1,"objective":"Ship CSV export.","project":"billing","sessions":2}`
	if code, _ := doClient(t, owner, srv, "POST", path, ask, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, ask); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	if _, err := st.CreateAPIToken(1, "Plan refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "POST", path, ask, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token: %d", code)
	}
	for name, bad := range map[string]string{"no revision": `{"objective":"x","sessions":1}`, "unknown field": `{"revision":1,"objective":"x","sessions":1,"start":true}`} {
		if code, _ := gafferClient(t, owner, srv, "POST", path, bad); code != 400 {
			t.Fatalf("%s: %d", name, code)
		}
	}
	if code, _ := gafferClient(t, owner, srv, "POST", path, `{"revision":1,"objective":"x","sessions":9}`); code != 422 {
		t.Fatalf("too many sessions: %d", code)
	}
	if asked != 0 {
		t.Fatalf("refused requests reached the provider %d times", asked)
	}
	code, body = gafferClient(t, owner, srv, "POST", path, ask)
	var plan struct {
		Summary  string `json:"summary"`
		Model    string `json:"model"`
		Sessions []struct{ Title, Prompt string }
	}
	if err := json.Unmarshal([]byte(body), &plan); code != 200 || err != nil || plan.Summary != "One job." || plan.Model != "plan-model" || len(plan.Sessions) != 1 || plan.Sessions[0].Title != "Export" || strings.Contains(body, "synthetic-key") {
		t.Fatalf("plan: %d %v %s", code, err, body)
	}
	if asked != 1 {
		t.Fatalf("provider requests: %d", asked)
	}
	if list, err := st.GafferConversationsFor(1); err != nil || len(list) != 0 {
		t.Fatalf("planning saved conversations: %v %v", list, err)
	}
}
