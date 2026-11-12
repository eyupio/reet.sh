package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eyupio/reet.sh/internal/auth"
	"github.com/eyupio/reet.sh/internal/gaffer"
	"github.com/eyupio/reet.sh/internal/meta"
	"github.com/eyupio/reet.sh/internal/sealed"
	"github.com/eyupio/reet.sh/internal/store"
)

func TestGafferConversationHTTPStreamsDurableTurnsWithoutBrowserOwnership(t *testing.T) {
	t.Parallel()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal, _ := sealed.NewSealer(strings.Repeat("gaffer-test-", 8), "test")
	h := New(st, meta.Fetcher{}, "bootstrap", WithAuthMode(AuthAccounts), WithPublicURL("http://localhost:8080"), WithGafferProviders(seal))
	server := httptest.NewServer(h)
	t.Cleanup(func() { server.Close(); h.Close(); st.Close() })
	client := &http.Client{Jar: mustJar(t)}
	code, _ := gafferClient(t, client, server, "POST", "/api/v2/auth/setup", `{"name":"Owner","email":"owner@example.test","password":"long-enough-password"}`, "X-Reet-Token", "bootstrap")
	if code != 200 {
		t.Fatal(code)
	}
	release := make(chan struct{})
	cancelled := make(chan struct{}, 2)
	var calls atomic.Int32
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", strings.Repeat("progress ", 600))
		w.(http.Flusher).Flush()
		select {
		case <-release:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Done.\"}}]}\n\ndata: [DONE]\n\n")
		case <-r.Context().Done():
			cancelled <- struct{}{}
		}
	}))
	t.Cleanup(func() { h.s.gafferProviders.Close(); fake.Close() })
	code, body := gafferClient(t, client, server, "POST", "/api/v2/gaffer/providers", fmt.Sprintf(`{"name":"Fake","kind":"openai_compatible","baseUrl":%q,"model":"fake-model","enabled":true,"localOnly":true,"makeDefault":true}`, fake.URL))
	if code != 200 {
		t.Fatalf("provider %d %s", code, body)
	}
	code, body = gafferClient(t, client, server, "POST", "/api/v2/gaffer/conversations", `{}`)
	if code != 200 {
		t.Fatalf("create %d %s", code, body)
	}
	var view gaffer.ConversationView
	if err = json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	path := "/api/v2/gaffer/conversations/" + view.Conversation.ID
	request := `{"id":"` + strings.Repeat("a", 32) + `","revision":1,"text":"sentinel-private-gaffer-prompt"}`
	if code, body := gafferClient(t, client, server, "GET", "/api/v2/gaffer/conversations?all=true", ""); code != 200 || !strings.Contains(body, view.Conversation.ID) {
		t.Fatalf("owned index %d %s", code, body)
	}
	password, _ := auth.HashPassword("long-enough-password")
	for _, role := range []string{"member", "viewer"} {
		user, err := st.CreateUser(role+"@example.test", role, role, password)
		if err != nil {
			t.Fatal(err)
		}
		other := &http.Client{Jar: mustJar(t)}
		if code, _ := gafferClient(t, other, server, "POST", "/api/v2/auth/sign-in", `{"email":"`+user.Email+`","password":"long-enough-password"}`); code != 200 {
			t.Fatal(code)
		}
		for _, route := range []struct{ method, path, body string }{
			{"GET", path, ""},
			{"GET", path + "/turns/" + strings.Repeat("a", 32) + "/events", ""},
			{"POST", path + "/turns", request},
			{"DELETE", path, `{"revision":1}`},
		} {
			want := 404
			if role == "viewer" {
				want = 403
			}
			if code, _ := gafferClient(t, other, server, route.method, route.path, route.body); code != want {
				t.Fatalf("%s %s %s: %d", role, route.method, route.path, code)
			}
		}
	}
	if code, _ := gafferClient(t, client, server, "POST", path+"/turns", request, "Authorization", "Bearer unwanted-api-token"); code != 403 {
		t.Fatalf("token beside browser: %d", code)
	}
	if code, _ := doClient(t, client, server, "POST", path+"/turns", request, "Origin", "https://different.example"); code != 403 {
		t.Fatalf("foreign origin: %d", code)
	}
	code, body = gafferClient(t, client, server, "POST", path+"/turns", request)
	if code != 202 {
		t.Fatalf("admit %d %s", code, body)
	}
	var turn store.GafferTurn
	if err = json.Unmarshal([]byte(body), &turn); err != nil {
		t.Fatal(err)
	}
	events := path + "/turns/" + turn.ID + "/events"
	response, err := client.Get(server.URL + events)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "text/event-stream" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("SSE headers %+v", response.Header)
	}
	buffer := make([]byte, 128)
	if _, err = response.Body.Read(buffer); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	select {
	case <-cancelled:
		t.Fatal("closing observer cancelled the model")
	default:
	}
	close(release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, body = gafferClient(t, client, server, "GET", path, "")
		if code != 200 {
			t.Fatal(code, body)
		}
		if err = json.Unmarshal([]byte(body), &view); err != nil {
			t.Fatal(err)
		}
		if len(view.Turns) == 1 && view.Turns[0].State == "completed" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not complete %s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	code, body = gafferClient(t, client, server, "POST", path+"/turns", request)
	if code != 202 || calls.Load() != 1 {
		t.Fatalf("duplicate request %d calls %d %s", code, calls.Load(), body)
	}
	response, err = client.Get(server.URL + events)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !strings.Contains(string(raw), `"state":"completed"`) {
		t.Fatalf("reconnect %v %s", err, raw)
	}
	if code, _ := gafferClient(t, client, server, "PATCH", path, fmt.Sprintf(`{"revision":1,"providerId":%q}`, view.Conversation.ProviderID)); code != 409 {
		t.Fatalf("stale override %d", code)
	}
	eventsLog, _ := st.ListEventsFor(1, 100, "")
	for _, event := range eventsLog {
		if strings.Contains(event.Detail, "sentinel-private-gaffer-prompt") || strings.Contains(event.Detail, "progress ") {
			t.Fatal("conversation text reached Activity")
		}
	}
}
