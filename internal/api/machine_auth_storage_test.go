package api

import (
	"database/sql"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eyupio/reet.sh/internal/store"
)

// Every route that checks a machine's secret answers a refusal with 403 and a
// storage failure with 500, so a database fault never reads as a revoked machine.
func TestRefuseMachineSeparatesARefusalFromAStorageFailure(t *testing.T) {
	w := httptest.NewRecorder()
	refuseMachine(w, store.ErrUnknownMachine)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "missing or revoked") {
		t.Fatalf("a refusal: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	refuseMachine(w, errors.New(`no such column: keep_synced`))
	if w.Code != 500 || strings.Contains(w.Body.String(), "missing or revoked") || strings.Contains(w.Body.String(), "keep_synced") {
		t.Fatalf("a storage failure: %d %s", w.Code, w.Body.String())
	}
}

func TestThePrivateHelloAnswers500ForAStorageFailureNot403(t *testing.T) {
	srv, st, agent, secret, ownerToken := privateFixture(t, AuthAccounts, "")
	srv.s.private = &fakePrivateTransport{address: "tc-private-capability-fixture"}
	ts := httptest.NewServer(srv)
	defer ts.Close()
	hello := func(key string) (int, string) {
		return doClient(t, ts.Client(), ts, "POST", "/api/v2/sync/private", `{}`,
			"X-Reet-Token", ownerToken, "X-Reet-Agent", agent.ID, "X-Reet-Agent-Key", key)
	}
	if code, body := hello(secret); code != 200 {
		t.Fatalf("a good secret: %d %s", code, body)
	}
	if code, body := hello("wrong"); code != 403 || !strings.Contains(body, "missing or revoked") {
		t.Fatalf("wrong secret: %d %s", code, body)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(st.Path(), "reet.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`ALTER TABLE sync_agents RENAME COLUMN keep_synced TO keep_synced_lost`); err != nil {
		t.Fatal(err)
	}
	if code, body := hello(secret); code != 500 || strings.Contains(body, "missing or revoked") || strings.Contains(body, "keep_synced") {
		t.Fatalf("storage failure: %d %s", code, body)
	}
}
