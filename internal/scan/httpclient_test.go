package scan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAgentRequestDoesNotFollowRedirectToAnotherHost(t *testing.T) {
	var leaked atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked.Store(r.Header.Get("X-Reet-Token") + "|" + r.Header.Get("X-Reet-Agent") + "|" + r.Header.Get("X-Reet-Agent-Key"))
		w.Write([]byte("{}"))
	}))
	defer other.Close()
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer controller.Close()
	c := agentConfig{Server: controller.URL, Token: "token", ID: "agent", Secret: "secret"}
	err := agentRequestContext(context.Background(), c, "/api/v2/sync/poll", map[string]string{}, nil, "test")
	if err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Fatalf("expected the redirect to be refused, got %v", err)
	}
	if got := leaked.Load(); got != nil {
		t.Fatalf("credentials reached the other host: %v", got)
	}
}

func TestAgentRequestFollowsRedirectOnTheSameHost(t *testing.T) {
	controller := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/moved" {
			http.Redirect(w, r, "/moved", http.StatusTemporaryRedirect)
			return
		}
		if r.Header.Get("X-Reet-Agent-Key") != "secret" {
			t.Errorf("agent key missing after a same-host redirect")
		}
		w.Write([]byte("{}"))
	}))
	defer controller.Close()
	c := agentConfig{Server: controller.URL, Token: "token", ID: "agent", Secret: "secret"}
	if err := agentRequestContext(context.Background(), c, "/api/v2/sync/poll", map[string]string{}, nil, "test"); err != nil {
		t.Fatal(err)
	}
}
