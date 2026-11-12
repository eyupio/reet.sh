package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

// Gaffer's tools start off, are chosen per tool by the browser account, and
// run only while switched on, as that account, through the same tool code an
// MCP client uses. The session tools that need a per-connection grant are
// never offered.
func TestGafferToolsAreOffUntilChosenAndRunAsTheAccount(t *testing.T) {
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
	user, _, _ := st.UserByEmail("owner@example.test")
	host := gafferToolHost{h.s}
	ctx := context.Background()

	// Refusals: signed out, an API token, another origin.
	if code, _ := gafferClient(t, &http.Client{}, srv, "GET", "/api/v2/gaffer/tools", ""); code != 401 {
		t.Fatalf("signed out: %d", code)
	}
	if _, err := st.CreateAPIToken(user.ID, "Tools refusal test", auth.HashToken("synthetic-api-token")); err != nil {
		t.Fatal(err)
	}
	if code, _ := gafferClient(t, &http.Client{}, srv, "PUT", "/api/v2/gaffer/tools", `{"modes":{}}`, "Authorization", "Bearer synthetic-api-token"); code != 403 {
		t.Fatalf("API token: %d", code)
	}
	if code, _ := doClient(t, owner, srv, "PUT", "/api/v2/gaffer/tools", `{"modes":{"reet_list_factory":"allow"}}`, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}

	code, body := gafferClient(t, owner, srv, "GET", "/api/v2/gaffer/tools", "")
	var listed struct {
		Tools []gafferToolView `json:"tools"`
	}
	if code != 200 || json.Unmarshal([]byte(body), &listed) != nil || len(listed.Tools) < 30 {
		t.Fatalf("list: %d %s", code, body)
	}
	groups := map[string]string{}
	for _, tool := range listed.Tools {
		if tool.Mode != "off" {
			t.Fatalf("%s starts %s", tool.Name, tool.Mode)
		}
		if !gafferToolOffered(tool.Name) || tool.Title == "" || tool.Group == "" {
			t.Fatalf("listed tool %+v", tool)
		}
		groups[tool.Name] = tool.Group
	}
	if groups["reet_approve_pipeline_merge"] != "Factory: merge" || groups["reet_start_outcome"] != "Factory: start agents" || groups["reet_list_factory"] != "Factory: read" || groups["reet_send_worker_message"] != "" || groups["reet_get_context"] != "" {
		t.Fatalf("groups: %v", groups)
	}
	if len(host.GafferTools(user.ID)) != 0 {
		t.Fatal("tools were on before the person chose any")
	}
	if _, err := host.CallGafferTool(ctx, user.ID, "reet_list_factory", nil); err == nil || !strings.Contains(err.Error(), "switched off") {
		t.Fatalf("a tool that is off ran: %v", err)
	}

	// Unknown tools, session tools and unknown modes are refused whole.
	for _, bad := range []string{`{"modes":{"reet_nope":"allow"}}`, `{"modes":{"reet_send_worker_message":"allow"}}`, `{"modes":{"reet_list_factory":"always"}}`, `{"modes":{"reet_list_factory":"allow"},"extra":1}`} {
		if code, _ := gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/tools", bad); code != 400 {
			t.Fatalf("%s: %d", bad, code)
		}
	}
	code, body = gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/tools", `{"modes":{"reet_list_factory":"allow","reet_stop_outcome":"ask","reet_list_tags":"off"}}`)
	if code != 200 || !strings.Contains(body, `"name":"reet_list_factory"`) {
		t.Fatalf("save: %d %s", code, body)
	}
	on := map[string]string{}
	for _, tool := range host.GafferTools(user.ID) {
		on[tool.Name] = tool.Mode
		if len(tool.Parameters) == 0 {
			t.Fatalf("%s has no parameters", tool.Name)
		}
	}
	if len(on) != 2 || on["reet_list_factory"] != "allow" || on["reet_stop_outcome"] != "ask" {
		t.Fatalf("switched on: %v", on)
	}
	out, err := host.CallGafferTool(ctx, user.ID, "reet_list_factory", json.RawMessage(`{}`))
	if err != nil || !strings.Contains(out, `"outcomes":[]`) || !strings.Contains(out, `"controller"`) {
		t.Fatalf("allowed tool: %s %v", out, err)
	}
	// A shelf tool goes through the API as the account, and its change is attributed to Gaffer.
	if code, _ = gafferClient(t, owner, srv, "PUT", "/api/v2/gaffer/tools", `{"modes":{"reet_create_tag":"allow","reet_list_tags":"allow"}}`); code != 200 {
		t.Fatalf("save again: %d", code)
	}
	if _, err = host.CallGafferTool(ctx, user.ID, "reet_list_factory", nil); err == nil {
		t.Fatal("a tool left out of the new choice still ran")
	}
	if out, err = host.CallGafferTool(ctx, user.ID, "reet_create_tag", json.RawMessage(`{"name":"from-gaffer"}`)); err != nil {
		t.Fatalf("create tag: %s %v", out, err)
	}
	if out, err = host.CallGafferTool(ctx, user.ID, "reet_list_tags", nil); err != nil || !strings.Contains(out, "from-gaffer") {
		t.Fatalf("list tags: %s %v", out, err)
	}
	if _, err = host.CallGafferTool(ctx, user.ID, "reet_create_tag", json.RawMessage(`{"nope":1}`)); err == nil {
		t.Fatal("arguments outside the schema were accepted")
	}
	// A read-only account has no tools whatever is saved.
	if len((gafferToolHost{h.s}).GafferTools(user.ID+99)) != 0 {
		t.Fatal("an unknown account was given tools")
	}
	events, err := st.ListEventsFor(user.ID, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, e := range events {
		if e.Action == "gaffer-tools-changed" {
			changed++
		}
	}
	if changed != 2 {
		t.Fatalf("%d gaffer-tools-changed events", changed)
	}
}
